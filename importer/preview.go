package importer

import (
	"bytes"
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

type rtPreviewGen struct {
}

func (rt *rtPreviewGen) Name() string { return "RawTherapee" }
func (rt *rtPreviewGen) Supports(f *File) bool {
	return rt.Available() && (f.TypeRAW() || f.TypeImage())
}
func (rt *rtPreviewGen) Available() bool { return execAvailable("rawtherapee-cli") }

func (rt *rtPreviewGen) Make(i *Importer, f *File, output string) error {
	tmp := output + ".tmp"
	pp, err := pp3.New(tmp + ".pp3")
	if err != nil {
		return err
	}
	pp.ResizeLongest(1920)
	if err := i.convert(f.Path(), tmp, pp, time.Time{}, nil, nil); err != nil {
		return err
	}

	return os.Rename(tmp, output)
}

type imPreviewGen struct{}

func (im *imPreviewGen) Name() string    { return "ImageMagick" }
func (im *imPreviewGen) Available() bool { return execAvailable("magick") }
func (im *imPreviewGen) Supports(f *File) bool {
	return im.Available() && (f.TypeRAW() || f.TypeImage())
}

func (im *imPreviewGen) Make(i *Importer, f *File, output string) error {
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

type vidPreviewGen struct{}

func (vid *vidPreviewGen) Name() string    { return "FFMPEG" }
func (vid *vidPreviewGen) Available() bool { return execAvailable("ffmpeg") }
func (vid *vidPreviewGen) Supports(f *File) bool {
	return vid.Available() && f.TypeVideo()
}

func (vid *vidPreviewGen) Make(i *Importer, f *File, output string) error {
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

func init() {
	RegisterPreviewGen(&rtPreviewGen{})
	RegisterPreviewGen(&imPreviewGen{})
	RegisterPreviewGen(&vidPreviewGen{})
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
