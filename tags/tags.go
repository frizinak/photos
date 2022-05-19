package tags

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	jpegstructure "github.com/dsoprea/go-jpeg-image-structure/v2"
)

type Aperture struct{ fraction }
type ShutterSpeed struct{ fraction }
type FocalLength struct{ fraction }
type ISO int

type fraction [2]uint

func (f fraction) Nil() bool { return f[0] == 0 || f[1] == 0 }

func (f fraction) String() string {
	if f[0] == 0 {
		return "0"
	}
	if f[1] == 1 {
		return strconv.FormatUint(uint64(f[0]), 10)
	}
	return fmt.Sprintf("%d/%d", f[0], f[1])
}

func (f fraction) Float() float64 {
	if f.Nil() {
		return 0
	}
	return float64(f[0]) / float64(f[1])
}

func (a Aperture) String() string     { return fmt.Sprintf("f/%.1f", a.Float()) }
func (s ShutterSpeed) String() string { return fmt.Sprintf("%ss", s.fraction.String()) }
func (f FocalLength) String() string  { return fmt.Sprintf("%.2fmm", f.Float()) }
func (i ISO) String() string          { return fmt.Sprintf("iso%d", i) }

func (i ISO) Nil() bool { return i == 0 }

func (f fraction) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]uint(f))
}

func (f *fraction) UnmarshalJSON(d []byte) error {
	return json.Unmarshal(d, (*[2]uint)(f))
}

type Device struct {
	Make, Model string
}

type CameraInfo struct {
	Device
	Lens         Device
	Aperture     Aperture
	ShutterSpeed ShutterSpeed
	FocalLength  FocalLength
	ISO          ISO
}

func (c CameraInfo) DeviceString() string {
	if c.Lens.Make == "" && c.Lens.Model == "" {
		return fmt.Sprintf(
			"%s %s [?]",
			c.Make,
			c.Model,
		)
	}
	return fmt.Sprintf(
		"%s %s [%s %s]",
		c.Make,
		c.Model,
		c.Lens.Make,
		c.Lens.Model,
	)
}

func (c CameraInfo) ExposureString() string {
	items := make([]string, 0, 4)
	l := []interface {
		Nil() bool
		String() string
	}{
		c.Aperture,
		c.ShutterSpeed,
		c.ISO,
		c.FocalLength,
	}
	for _, i := range l {
		if !i.Nil() {
			items = append(items, i.String())
		}
	}
	return strings.Join(items, " ")

}

func (c CameraInfo) String() string {
	return fmt.Sprintf("%s - %s", c.DeviceString(), c.ExposureString())
}

type Tags struct {
	ex  map[uint16]exif.ExifTag
	ff  *FFProbeInfo
	err error
}

func (t *Tags) Err() error { return t.err }

var fre = regexp.MustCompile(`^\[?([0-9]+)/([0-9]+)\]?$`)

func (t *Tags) exif(f uint16) string {
	tag, ok := t.ex[f]
	if !ok {
		return ""
	}

	return strings.Trim(tag.Formatted, "\" ")
}

func commonDenominator(a, b uint) uint {
	if a == b {
		return a
	}
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a > b {
		return commonDenominator(a-b, b)
	}
	return commonDenominator(a, b-a)
}

func (t *Tags) CameraInfo() (CameraInfo, bool) {
	c := CameraInfo{}
	if t.ex == nil {
		return c, false
	}

	getf := func(f uint16) fraction {
		v := t.exif(f)
		sm := fre.FindStringSubmatch(v)
		if len(sm) == 3 {
			_nom, err := strconv.ParseUint(sm[1], 10, 64)
			if err != nil {
				return fraction{0, 1}
			}
			_denom, err := strconv.ParseUint(sm[2], 10, 64)
			if err != nil {
				return fraction{0, 1}
			}
			nom, denom := uint(_nom), uint(_denom)
			if c := commonDenominator(nom, denom); c != 0 {
				return fraction{nom / c, denom / c}
			}
			return fraction{nom, denom}
		}

		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			n = 0
		}
		return fraction{uint(n), 1}
	}

	c.Make, c.Model = t.exif(0x010f), t.exif(0x0110)
	c.Lens.Make, c.Lens.Model = t.exif(0xa433), t.exif(0xa434)

	c.Aperture = Aperture{getf(0x829d)}
	c.ShutterSpeed = ShutterSpeed{getf(0x829a)}
	iso, err := strconv.Atoi(strings.Trim(t.exif(0x8827), "[]"))
	if err == nil {
		c.ISO = ISO(iso)
	}
	c.FocalLength = FocalLength{getf(0x920a)}

	return c, true
}

func (t *Tags) Date() time.Time {
	if t.ex != nil {
		d, err := t.exifDate()
		if err != nil {
			panic(err)
		}
		return d
	}
	if t.ff != nil {
		return t.ff.Date()
	}
	return time.Time{}
}

func (t *Tags) exifDate() (time.Time, error) {
	var dt time.Time
	tag := t.exif(0x9003)
	var offset uint16 = 0x9011
	if tag == "" {
		tag = t.exif(0x0132)
		offset = 0x9010
		if tag == "" {
			return dt, errors.New("no datetime exif tag found")
		}
	}

	exOffset := t.exif(offset)
	if exOffset != "" && exOffset[len(exOffset)-1] != ':' {
		return time.Parse("2006:01:02 15:04:05 -07:00", tag+" "+exOffset)
	}

	return time.ParseInLocation("2006:01:02 15:04:05", tag, time.Local)
}

func Parse(path string) *Tags {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".nef", ".raf", ".dng", ".tiff", ".tif", ".jpg":
		ex, err := ParseExif(path)
		return &Tags{ex: ex, err: err}
	default:
		ff, err := ParseFFProbe(path)
		return &Tags{ff: ff, err: err}
	}
}

func ParseExif(path string) (map[uint16]exif.ExifTag, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	raw, err := exif.SearchAndExtractExifWithReader(f)
	if err != nil {
		return nil, err
	}
	tags, _, err := exif.GetFlatExifData(raw, nil)
	if err != nil {
		return nil, err
	}
	m := make(map[uint16]exif.ExifTag, len(tags))
	for _, tag := range tags {
		m[tag.TagId] = tag
	}

	return m, nil
}

func EditJPEGExif(file string, out io.Writer, cbs ...func(*exif.IfdBuilder) (bool, error)) error {
	parser := jpegstructure.NewJpegMediaParser()
	d, err := parser.ParseFile(file)
	if err != nil {
		return err
	}
	rd := d.(*jpegstructure.SegmentList)
	builder, err := rd.ConstructExifBuilder()
	if err != nil {
		return err
	}

	set := false
	for _, cb := range cbs {
		ok, err := cb(builder)
		set = set || ok
		if err != nil {
			return err
		}
	}

	if !set {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(out, f)
		return err
	}

	if err := rd.SetExif(builder); err != nil {
		return err
	}

	return rd.Write(out)
}

func JPEGExifTZ(ts time.Time, force bool) func(*exif.IfdBuilder) (bool, error) {
	set := false
	v := func(b *exif.IfdBuilder, dt, offset uint16) error {
		exifBuilder, err := b.ChildWithTagId(0x8769)
		if err != nil {
			return err
		}

		if !force {
			if t, _ := exifBuilder.FindTag(offset); t != nil {
				return nil
			}
		}

		t, _ := b.FindTag(dt)
		if t == nil {
			t, _ = exifBuilder.FindTag(dt)
		}
		if t == nil {
			return nil
		}
		tm, err := time.Parse("2006:01:02 15:04:05", string(bytes.Trim(t.Value().Bytes(), "\x00")))
		if err != nil {
			return err
		}

		s := tm.Sub(ts)
		if s > time.Hour*24 || s < -time.Hour*24 {
			return errors.New("impossible timezone correction")
		}

		min := s.Minutes()
		sgn := "+"
		if min < 0 {
			sgn = "-"
			min = -min
		}
		h := int(min / 60)
		m := int(min - float64(h*60))
		str := fmt.Sprintf("%s%02d:%02d\x00", sgn, h, m)
		value := exif.NewIfdBuilderTagValueFromBytes([]byte(str))

		set = true
		return exifBuilder.Set(
			exif.NewBuilderTag("IFD/Exif", offset, exifcommon.TypeAscii, value, binary.BigEndian),
		)
	}

	return func(b *exif.IfdBuilder) (bool, error) {
		if err := v(b, 0x0132, 0x9010); err != nil {
			return set, err
		}
		if err := v(b, 0x9003, 0x9011); err != nil {
			return set, err
		}

		return set, nil
	}
}

type FFProbeInfo struct {
	Format struct {
		Tags struct {
			CreationTime time.Time `json:"creation_time"`
		} `json:"tags"`
	} `json:"format"`
}

func (ff *FFProbeInfo) Date() time.Time {
	return ff.Format.Tags.CreationTime
}

func ParseFFProbe(path string) (*FFProbeInfo, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v",
		"quiet",
		path,
		"-print_format",
		"json",
		"-show_entries",
		"format_tags=creation_time",
	)

	stdout, stderr := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	ff := &FFProbeInfo{}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe err: %s, %w", stderr.String(), err)
	}

	dec := json.NewDecoder(stdout)
	return ff, dec.Decode(ff)
}
