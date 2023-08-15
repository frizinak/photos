package importer

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc64"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/frizinak/phodo/exif"
	"github.com/frizinak/phodo/img48"
	"github.com/frizinak/phodo/jpeg"
	"github.com/frizinak/phodo/pipeline"
	"github.com/frizinak/phodo/pipeline/element"
	"github.com/frizinak/phodo/pipeline/element/core"
	"github.com/frizinak/photos/meta"
)

type sidecar interface {
	Path() string
	Hash(w io.Writer)
	Edited() bool
}

func (i *Importer) convertPP3(input, output string, pp PP3, size int, created time.Time, lat, lng *float64) error {
	pp.ResizeLongest(size)

	pp3TempPath := fmt.Sprintf("%s.tmp.pp3", output)
	err := pp.SaveTo(pp3TempPath)
	defer os.Remove(pp3TempPath)
	if err != nil {
		return err
	}

	tmp := output
	if filepath.Ext(output) != ".jpg" {
		// bruuh
		tmp = output + ".rawtherapeehack.jpg"
	}

	args := []string{
		"-Y",
		"-o", tmp,
		"-q",
		"-p", pp3TempPath,
		"-c", input,
	}
	i.verbose.Printf("rawtherapee-cli %s", strings.Join(args, " "))
	cmd := exec.Command("rawtherapee-cli", args...)

	buf := bytes.NewBuffer(nil)
	cmd.Stderr = buf
	if err := cmd.Run(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("%s: %s", err, buf)
	}

	err = i.jpegRewrite(tmp, func(e *exif.Exif) (bool, error) {
		return i.Exif(e, created, lat, lng)
	})

	if err != nil {
		os.Remove(tmp)
		return err
	}

	if tmp != output {
		return os.Rename(tmp, output)
	}

	return nil
}

func (i *Importer) convertPho(input, output string, pho Pho, size int, created time.Time, lat, lng *float64) error {
	p, ok := pho.Convert()
	if !ok {
		return fmt.Errorf("no .convert pipeline in '%s'", pho.Path())
	}

	conf, err := i.phodoConf()
	if err != nil {
		return err
	}

	// TODO perhaps pass size variable
	line := pipeline.New().
		Add(element.LoadFile(input)).
		Add(p.Element).
		Add(pipeline.ElementFunc(func(ctx pipeline.Context, img *img48.Img) (*img48.Img, error) {
			_, err := i.Exif(img.Exif, created, lat, lng)
			return img, err
		})).
		Add(element.Resize(size, size, "", core.ResizeMax|core.ResizeNoUpscale)).
		Add(element.SaveFile(output, ".jpg", 92))

	rctx := pipeline.NewContext(conf.Verbose, i.log.Writer(), pipeline.ModeConvert, context.Background())
	_, err = line.Do(rctx, nil)
	return err
}

func (i *Importer) Exif(e *exif.Exif, created time.Time, lat, lng *float64) (bool, error) {
	write := false

	w, err := i.ExifTZ(e, created)
	write = write || w
	if err != nil {
		return write, err
	}

	if lat != nil && lng != nil {
		w, err = i.ExifGPS(e, created, *lat, *lng)
		write = write || w
	}

	return write, err
}

func (i *Importer) ExifTZ(e *exif.Exif, created time.Time) (bool, error) {
	write := false
	tzParse := func(dt string) (string, error) {
		tm, err := time.Parse("2006:01:02 15:04:05", dt)
		if err != nil {
			return "", err
		}

		s := tm.Sub(created)
		if s > time.Hour*24 || s < -time.Hour*24 {
			return "", errors.New("impossible timezone correction")
		}

		min := s.Minutes()
		sgn := "+"
		if min < 0 {
			sgn = "-"
			min = -min
		}
		h := int(min / 60)
		m := int(min - float64(h*60))
		return fmt.Sprintf("%s%02d:%02d", sgn, h, m), nil
	}

	tzFix := func(dt []uint16, set func(value string)) error {
		timeEntry := e.Find(dt...)
		if timeEntry == nil {
			return nil
		}
		if v, ok := timeEntry.Value().Value.(string); ok {
			tzValue, err := tzParse(v)
			if err != nil {
				return err
			}

			set(tzValue)
		}

		return nil
	}

	err := tzFix([]uint16{0x8769, 0x9003}, func(v string) {
		write = true
		e.Ensure(0, 0x8769, exif.TypeUint32).IFDSet.
			Ensure(0, 0x9011, exif.TypeASCII).
			SetString(v)
	})
	if err != nil {
		return write, err
	}

	err = tzFix([]uint16{0x0132}, func(v string) {
		write = true
		e.Ensure(0, 0x8769, exif.TypeUint32).IFDSet.
			Ensure(0, 0x9010, exif.TypeASCII).
			SetString(v)
	})
	if err != nil {
		return write, err
	}

	return write, nil
}

func (i *Importer) ExifGPS(e *exif.Exif, created time.Time, lat, lng float64) (bool, error) {
	hms := func(v float64) (h, m, s float64) {
		h = float64(int(v))
		m = float64(int((v - h) * 60))
		s = (v - h - m/60) * 3600
		return
	}

	hms24 := func(values []float64, denoms []int) [][2]int {
		if len(values) != len(denoms) {
			panic("every value should match one denom")
		}

		val := make([][2]int, len(values))
		for i := range values {
			val[i][0] = int(values[i] * float64(denoms[i]))
			val[i][1] = denoms[i]
		}
		return val
	}

	coords := func(v float64) [][2]int {
		valh, valm, vals := hms(v)
		return hms24([]float64{valh, valm, vals}, []int{1, 1, 10000})
	}

	gps := e.Ensure(0, 0x8825, exif.TypeUint32)
	latref, lngref := "N", "E"
	if lat < 0 {
		latref = "S"
	}
	if lng < 0 {
		lngref = "W"
	}

	latvalue := coords(math.Abs(lat))
	lngvalue := coords(math.Abs(lng))

	utc := created.UTC()
	timevalue := hms24(
		[]float64{float64(utc.Hour()), float64(utc.Minute()), float64(utc.Second())},
		[]int{1, 1, 1},
	)

	gps.IFDSet.Ensure(0, 0x0001, exif.TypeASCII).SetString(latref)
	gps.IFDSet.Ensure(0, 0x0002, exif.TypeUrational).SetRationals(latvalue)
	gps.IFDSet.Ensure(0, 0x0003, exif.TypeASCII).SetString(lngref)
	gps.IFDSet.Ensure(0, 0x0004, exif.TypeUrational).SetRationals(lngvalue)

	gps.IFDSet.Ensure(0, 0x0007, exif.TypeUrational).SetRationals(timevalue)
	gps.IFDSet.Ensure(0, 0x001d, exif.TypeASCII).SetString(utc.Format("2006:01:02"))

	return true, nil
}

func (i *Importer) jpegRewrite(file string, rewrite func(*exif.Exif) (bool, error)) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	exif, err := exif.Read(f)
	if err != nil {
		return err
	}

	written, err := rewrite(exif)
	if !written || err != nil {
		return err
	}

	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	tmp := file + ".tmp"
	w, err := os.Create(tmp)
	if err != nil {
		return err
	}

	err = jpeg.OverwriteExif(f, w, exif)
	w.Close()
	if err != nil {
		os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, file)
}

func (i *Importer) JPEGGPS(file string, created time.Time, lat, lng float64) error {
	return i.jpegRewrite(file, func(e *exif.Exif) (bool, error) {
		return i.ExifGPS(e, created, lat, lng)
	})
}

func (i *Importer) JPEGTZ(file string, t time.Time) error {
	return i.jpegRewrite(file, func(e *exif.Exif) (bool, error) {
		return i.ExifTZ(e, t)
	})
}

func (i *Importer) convertIfUpdated(
	loc *meta.Location,
	link,
	dir,
	output string,
	sidecar sidecar,
	converted map[string]meta.Converted,
	size int,
	created time.Time,
	checkOnly bool,
) (bool, string, error) {
	h := crc64.New(crc64.MakeTable(crc64.ISO))
	fmt.Fprintf(h, "%d\n", size)
	sidecar.Hash(h)
	hash := hex.EncodeToString(h.Sum(nil))

	output = fmt.Sprintf("%s.jpg", output)
	rel, err := filepath.Rel(dir, output)
	if err != nil {
		return false, rel, err
	}

	_, err = os.Stat(output)
	exists := false
	if err == nil {
		exists = true
	} else if !os.IsNotExist(err) {
		return false, rel, err
	}

	if h, ok := converted[rel]; exists && ok && h.Hash == hash {
		return false, rel, nil
	}
	converted[rel] = meta.Converted{Hash: hash, Size: size}

	if checkOnly {
		return true, rel, nil
	}
	os.MkdirAll(filepath.Dir(output), 0755)
	var lat, lng *float64
	if loc != nil {
		lat, lng = &loc.Lat, &loc.Lng
	}

	switch sc := sidecar.(type) {
	case PP3:
		return true, rel, i.convertPP3(link, output, sc, size, created, lat, lng)
	case Pho:
		return true, rel, i.convertPho(link, output, sc, size, created, lat, lng)
	}
	return false, rel, fmt.Errorf("unsupported sidecar file of type %T", sidecar)
}

func (i *Importer) Unedited(f *File) (bool, error) {
	pp3edited := true
	phoedited := true
	err := i.walkLinks(f, func(link string) (bool, error) {
		pho, err := i.GetPho(link)
		if err != nil && errors.Is(err, os.ErrNotExist) {
			err = nil
			phoedited = false
		}
		if err != nil {
			return false, err
		}
		phoedited = phoedited && pho.Edited()

		pp3, err := i.GetPP3(link)
		if err != nil && errors.Is(err, os.ErrNotExist) {
			err = nil
			pp3edited = false
		}
		if err != nil {
			return false, err
		}

		pp3edited = pp3edited && pp3.Edited()

		if !pp3edited && !phoedited {
			return false, nil
		}

		return true, nil
	})

	return !pp3edited && !phoedited, err
}

func (i *Importer) CheckConvert(f *File, sizes []int) (bool, error) {
	return i.fileConvert(f, sizes, true)
}

func (i *Importer) Convert(f *File, sizes []int) error {
	_, err := i.fileConvert(f, sizes, false)
	return err
}

func (i *Importer) fileConvert(f *File, sizes []int, checkOnly bool) (bool, error) {
	links := []string{}
	sidecars := []sidecar{}
	err := i.walkLinks(f, func(link string) (bool, error) {
		pho, err := i.GetPho(link)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		if err == nil && pho.Edited() {
			sidecars = append(sidecars, pho)
			links = append(links, link)
			return true, nil
		}

		pp3, err := i.GetPP3(link)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		if err == nil {
			sidecars = append(sidecars, pp3)
			links = append(links, link)
		}

		return true, nil
	})
	if err != nil {
		return false, err
	}

	if len(links) == 0 {
		return false, nil
	}

	m, err := GetMeta(f)
	if err != nil {
		return false, err
	}

	conv := m.Conv
	if conv == nil {
		conv = make(map[string]meta.Converted)
	}
	rels := make(map[string]struct{}, len(conv))
	changed := false
	for n, link := range links {
		for _, s := range sizes {
			custom, err := filepath.Rel(i.colDir, link)
			if err != nil {
				return false, err
			}
			base := filepath.Join(i.convDir, custom)
			dir := filepath.Dir(base)
			fn := filepath.Base(base)
			ext := filepath.Ext(fn)
			fn = fn[0 : len(fn)-len(ext)]
			output := filepath.Join(dir, strconv.Itoa(s), fn)
			conv, rel, err := i.convertIfUpdated(
				m.Location,
				links[n],
				i.convDir,
				output,
				sidecars[n],
				conv,
				s,
				m.CreatedTime(),
				checkOnly,
			)
			changed = changed || conv
			if err != nil {
				return false, err
			}
			rels[rel] = struct{}{}
		}
	}

	for k := range conv {
		if _, ok := rels[k]; !ok {
			changed = true
			delete(conv, k)
		}
	}

	if checkOnly {
		return changed, nil
	}

	m.Conv = conv

	return changed, SaveMeta(f, m)
}
