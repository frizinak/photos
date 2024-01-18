package importer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/frizinak/phodo/phodo"
	"github.com/frizinak/phodo/pipeline"
	"github.com/frizinak/phodo/pipeline/element"
	"github.com/frizinak/photos/imagemagick"
	"github.com/frizinak/photos/pp3"
	"github.com/frizinak/photos/tags"
	"golang.org/x/image/bmp"
)

var (
	ErrPreviewNotPossible = errors.New("preview not possible for filetype")
)

var (
	pgens = make([]PreviewGen, 0)

	execs      = map[string]bool{}
	execsMutex sync.RWMutex
)

func execAvailable(bin string) bool {
	execsMutex.RLock()
	a, ok := execs[bin]
	execsMutex.RUnlock()
	if ok {
		return a
	}
	execsMutex.Lock()
	_, err := exec.LookPath(bin)
	execs[bin] = err == nil
	execsMutex.Unlock()
	return err == nil
}

type PreviewGen interface {
	Name() string
	Supports(f *File) bool
	Make(i *Importer, f *File, output string) error
}

func RegisterPreviewGen(p PreviewGen) {
	pgens = append(pgens, p)
}

type PhoPreviewGen struct {
	PreviewFile string
}

func (pho *PhoPreviewGen) Name() string { return "Phodo" }
func (pho *PhoPreviewGen) Supports(f *File) bool {
	return f.TypeRAW() || f.TypeImage()
}

func (pho *PhoPreviewGen) Make(i *Importer, f *File, output string) error {
	conf, err := i.phodoConf()
	if err != nil {
		return err
	}

	root, err := phodo.LoadScript(conf, pho.PreviewFile)
	if err != nil {
		return err
	}

	p, ok := root.Get(".main")
	if !ok {
		return errors.New("no .main pipeline found in preview phodo definition")
	}

	tmp := output + ".tmp"
	line := pipeline.New()
	line.Add(element.LoadFile(f.Path()))
	line.Add(p.Element)
	line.Add(element.SaveFile(tmp, ".jpg", 75))
	rctx := pipeline.NewContext(pipeline.VerboseNone, io.Discard, pipeline.ModeConvert, context.Background())
	_, err = line.Do(rctx, nil)
	if err != nil {
		os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, output)
}

type RTPreviewGen struct {
}

func (rt *RTPreviewGen) Name() string { return "RawTherapee" }
func (rt *RTPreviewGen) Supports(f *File) bool {
	return rt.Available() && (f.TypeRAW() || f.TypeImage())
}
func (rt *RTPreviewGen) Available() bool { return execAvailable("rawtherapee-cli") }

func (rt *RTPreviewGen) Make(i *Importer, f *File, output string) error {
	tmp := output + ".tmp"
	var pp PP3
	var err error
	pp.PP3, err = pp3.New(tmp + ".pp3")
	if err != nil {
		return err
	}
	if err := i.convertPP3(f.Path(), tmp, pp, 1920, info{created: time.Time{}}); err != nil {
		return err
	}

	return os.Rename(tmp, output)
}

type IMPreviewGen struct{}

func (im *IMPreviewGen) Name() string    { return "ImageMagick" }
func (im *IMPreviewGen) Available() bool { return execAvailable("magick") }
func (im *IMPreviewGen) Supports(f *File) bool {
	return im.Available() && (f.TypeRAW() || f.TypeImage())
}

func (im *IMPreviewGen) Make(i *Importer, f *File, output string) error {
	tmp := output + ".tmp"
	dst, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer dst.Close()

	c := imagemagick.JPEGConfig{
		Quality: 85,
		Height:  1080,
	}
	if err := imagemagick.ToJPEG(f.Path(), dst, c); err != nil {
		return err
	}
	return os.Rename(tmp, output)
}

type VidPreviewGen struct{}

func (vid *VidPreviewGen) Name() string    { return "FFMPEG" }
func (vid *VidPreviewGen) Available() bool { return execAvailable("ffmpeg") }
func (vid *VidPreviewGen) Supports(f *File) bool {
	return vid.Available() && f.TypeVideo()
}

func (vid *VidPreviewGen) Make(i *Importer, f *File, output string) error {
	p, err := tags.ParseFFProbe(f.Path())
	if err != nil {
		return err
	}
	horiz := 3
	vert := 3
	const xres = 2560
	const pad = 64

	b := p.Bounds()
	aspect := float64(b.Dy()) / float64(b.Dx())
	if aspect > 1.77 {
		horiz = int(aspect * float64(vert) * 1.77)
	}
	if aspect < 1.77 {
		vert = int(float64(horiz) / aspect / 1.77)
	}

	if horiz > 8 {
		horiz = 8
	}
	if vert > 8 {
		vert = 8
	}

	dur := p.Duration()
	timePad := dur / 20
	n := timePad
	iv := (dur - 2*timePad) / time.Duration(horiz*vert)
	w := xres / horiz
	h := int(float64(w) * aspect)
	canvas := image.NewNRGBA(image.Rect(0, 0, xres, h*vert))
	xpad := w / pad
	ypad := h / int(aspect*pad)
	w -= xpad
	h -= ypad

	iteration := 0
	rect := image.Rect(xpad/2, ypad/2, w+xpad/2, h+ypad/2)
	errs := make(chan error)
	var m sync.Mutex
	rl := make(chan struct{}, 4)
	for {
		if n >= dur-timePad {
			break
		}

		go func(n time.Duration, rect image.Rectangle) {
			rl <- struct{}{}
			defer func() { <-rl }()
			reader, writer := io.Pipe()
			cmd := exec.Command("ffmpeg",
				"-loglevel", "error",
				"-hide_banner",
				"-ss", fmt.Sprintf("%.3f", n.Seconds()),
				"-i", f.Path(),
				"-frames:v", "1",
				"-s", fmt.Sprintf("%dx%d", w, h),
				"-c:v", "bmp",
				"-f", "rawvideo",
				"-",
			)
			//n += iv

			type res struct {
				image.Image
				err error
			}
			d := make(chan res, 1)
			go func() {
				img, err := bmp.Decode(reader)
				d <- res{img, err}
			}()

			buf := bytes.NewBuffer(nil)
			cmd.Stdout = writer
			cmd.Stderr = buf
			err = cmd.Run()
			if err != nil {
				writer.Close()
				errs <- fmt.Errorf("%w: %s", err, buf.String())
				return
			}

			o := <-d
			if o.err != nil {
				errs <- o.err
				return
			}

			m.Lock()
			draw.Draw(canvas, rect, o.Image, image.Point{}, draw.Over)
			m.Unlock()
			errs <- nil
		}(n, rect)

		rect.Min.X += xpad + w
		rect.Max.X += xpad + w
		iteration++
		if iteration%horiz == 0 {
			rect.Min.X, rect.Max.X = xpad/2, w+xpad/2
			rect.Min.Y += ypad + h
			rect.Max.Y += ypad + h
		}
		n += iv
	}

	var gerr error
	for i := 0; i < iteration; i++ {
		err := <-errs
		if gerr == nil && err != nil {
			gerr = err
		}
	}
	if gerr != nil {
		return gerr
	}

	tmp := output + ".tmp"
	fh, err := os.Create(tmp)
	if err != nil {
		return err
	}

	err = jpeg.Encode(fh, canvas, &jpeg.Options{Quality: 70})
	fh.Close()
	if err != nil {
		os.Remove(tmp)
	}

	return os.Rename(tmp, output)
}

func PreviewFile(f *File) string {
	return f.Path() + ".preview"
}

func (i *Importer) MakePreview(f *File) error {
	for _, g := range pgens {
		if g.Supports(f) {
			return g.Make(i, f, PreviewFile(f))
		}
	}

	return ErrPreviewNotPossible
}

func GetPreview(f *File) (io.ReadCloser, error) {
	return os.Open(PreviewFile(f))
}

func (i *Importer) HasPreview(f *File) (exists, possible bool) {
	sup := false
	for _, g := range pgens {
		if g.Supports(f) {
			sup = true
			break
		}
	}
	if !sup {
		return false, false
	}

	fh, err := GetPreview(f)
	if fh != nil {
		fh.Close()
	}
	return err == nil, true
}

func (i *Importer) EnsurePreview(f *File) error {
	p, err := GetPreview(f)
	if err != nil {
		if os.IsNotExist(err) {
			return i.MakePreview(f)
		}
		return err
	}
	return p.Close()
}
