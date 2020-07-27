package libgphoto2

import (
	"io"
	"log"
	"os"

	gp2 "github.com/frizinak/gphoto2go"
	"github.com/frizinak/photos/importer"
)

func init() {
	importer.Register("libgphoto2", New())
}

type LibGPhoto2 struct {
	cam *gp2.Camera
}

func New() *LibGPhoto2 {
	return &LibGPhoto2{}
}

func (l *LibGPhoto2) init() error {
	if l.cam != nil {
		return nil
	}

	cam := &gp2.Camera{}
	if err := cam.Init(); err != nil {
		return err
	}

	l.cam = cam
	return nil
}

func (l *LibGPhoto2) close() error {
	cam := l.cam
	l.cam = nil
	return cam.Exit()
}

func (l *LibGPhoto2) Available() (bool, error) {
	if err := l.init(); err != nil {
		if e, ok := err.(*gp2.Error); ok && (e.Is(gp2.ErrModelNotFound) || e.Is(gp2.ErrNotSupported)) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (l *LibGPhoto2) Import(
	log *log.Logger,
	destination string,
	imp *importer.Import,
) error {
	defer l.close()
	if err := l.init(); err != nil {
		return err
	}

	dirs, err := l.cam.RListFolders("/")
	if err != nil {
		return err
	}

	type f struct {
		f    *importer.File
		size int64
		dir  string
		name string
	}

	files := make([]*f, 0)
	total := 0
	skips := 0
	done := 0
	prog := func() {
		imp.Progress(skips+done, total)
	}
	for _, dir := range dirs {
		names, err := l.cam.ListFiles(dir)
		if err != nil {
			return err
		}

		for _, name := range names {
			total++
			i, err := l.cam.Info(dir, name)
			if err != nil {
				return err
			}
			files = append(files, &f{nil, i.Size, dir, name})
			prog()
		}
	}

	ifiles := make([]*f, 0, len(files))
	for _, f := range files {
		r := l.cam.ReadSeeker(f.dir, f.name)
		file := importer.NewFile(f.dir, f.size, f.name)
		exists, err := imp.Exists(file, r, 10)
		r.Close()
		if err != nil {
			return err
		}

		if exists {
			skips++
			prog()
			continue
		}
		file = importer.NewFile(destination, f.size, f.name)
		f.f = file
		ifiles = append(ifiles, f)
		prog()
	}

	buf := make([]byte, 1024*1024*100)
	for _, f := range ifiles {
		src := f.f.Path()
		w, err := os.Create(src)
		if err != nil {
			return err
		}
		r := l.cam.ReadSeeker(f.dir, f.name)
		_, err = io.CopyBuffer(w, r, buf)
		r.Close()
		w.Close()
		if err != nil {
			return err
		}

		if err := imp.Add(src, f.f); err != nil {
			return err
		}
		done++
		prog()
	}

	return nil
}
