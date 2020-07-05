package importer

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/frizinak/photos/tiff"
)

var (
	ErrPreviewNotPossible = errors.New("preview not possible for filetype")
)

func previewFile(f *File) string {
	return f.Path() + ".preview"
}

func MakePreview(f *File) error {
	fp := f.Path()
	typ, err := tiff.TypeForExt(filepath.Ext(fp))
	if err != nil {
		return ErrPreviewNotPossible
	}
	src, err := os.Open(fp)
	if err != nil {
		return err
	}
	defer src.Close()

	real := previewFile(f)
	tmp := real + ".tmp"
	dst, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer dst.Close()

	c := tiff.JPEGConfig{
		Quality: 85,
		Type:    typ,
		Height:  1080,
	}
	if err := tiff.ToJPEG(src, dst, c); err != nil {
		return err
	}
	return os.Rename(tmp, real)
}

func GetPreview(f *File) (io.ReadCloser, error) {
	return os.Open(previewFile(f))
}

func EnsurePreview(f *File) error {
	p, err := GetPreview(f)
	if err != nil {
		if os.IsNotExist(err) {
			return MakePreview(f)
		}
		return err
	}
	return p.Close()
}
