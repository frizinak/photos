package importer

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/frizinak/photos/imagemagick"
	"github.com/frizinak/photos/pp3"
)

var (
	ErrPreviewNotPossible = errors.New("preview not possible for filetype")
)

var (
	rawtherapeeAvailable    int
	rawtherapeeAvailableSem sync.Mutex
)

func PreviewFile(f *File) string {
	return f.Path() + ".preview"
}

func (i *Importer) rawtherapeeAvailable() bool {
	if rawtherapeeAvailable != 0 {
		return rawtherapeeAvailable == 1
	}
	rawtherapeeAvailableSem.Lock()
	defer rawtherapeeAvailableSem.Unlock()
	if rawtherapeeAvailable != 0 {
		return rawtherapeeAvailable == 1
	}

	_, err := exec.LookPath("rawtherapee-cli")
	if err == nil {
		i.verbose.Println("creating previews with rawtherapee")
		rawtherapeeAvailable = 1
		return true
	}

	i.verbose.Println("creating previews with convert")
	rawtherapeeAvailable = -1
	return false
}

func (i *Importer) MakePreview(f *File) error {
	typ, err := imagemagick.TypeForExt(filepath.Ext(f.Path()))
	if err != nil {
		return ErrPreviewNotPossible
	}

	if !i.rawtherapeeAvailable() {
		return makePreviewImageMagick(f, typ)
	}

	out := PreviewFile(f)
	tmp := out + ".tmp"
	pp, err := pp3.New(tmp + ".pp3")
	if err != nil {
		return err
	}
	pp.ResizeLongest(1920)
	if err := i.convert(f.Path(), tmp, pp); err != nil {
		return err
	}

	return os.Rename(tmp, out)
}

func makePreviewImageMagick(f *File, typ imagemagick.Type) error {
	src, err := os.Open(f.Path())
	if err != nil {
		return err
	}
	defer src.Close()

	real := PreviewFile(f)
	tmp := real + ".tmp"
	dst, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer dst.Close()

	c := imagemagick.JPEGConfig{
		Quality: 85,
		Type:    typ,
		Height:  1080,
	}
	if err := imagemagick.ToJPEG(src, dst, c); err != nil {
		return err
	}
	return os.Rename(tmp, real)
}

func GetPreview(f *File) (io.ReadCloser, error) {
	return os.Open(PreviewFile(f))
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
