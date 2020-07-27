package fs

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/frizinak/photos/importer"
)

type FS struct {
	dir       string
	recursive bool
	exts      map[string]struct{}
}

func New(dir string, recursive bool, exts []string) *FS {
	m := map[string]struct{}{}
	for _, e := range exts {
		e = strings.ToLower(e)
		m[e] = struct{}{}
	}
	return &FS{dir, recursive, m}
}

func (f *FS) Available() (bool, error) {
	stat, err := os.Stat(f.dir)
	if err == nil && !stat.IsDir() {
		return true, fmt.Errorf("'%s' is not a directory", f.dir)
	}
	return true, err
}

func (f *FS) scan(dir string, data *[]*importer.File) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	list, err := d.Readdir(-1)
	d.Close()
	if err != nil {
		return err
	}

	for _, item := range list {
		fn := item.Name()
		if item.IsDir() {
			if f.recursive {
				err := f.scan(filepath.Join(dir, item.Name()), data)
				if err != nil {
					return err
				}
			}
			continue
		}

		ext := filepath.Ext(fn)
		if _, ok := f.exts[strings.ToLower(ext)]; !ok {
			continue
		}

		*data = append(*data, importer.NewFile(dir, item.Size(), fn))
	}

	return nil
}

func (f *FS) Import(log *log.Logger, destination string, imp *importer.Import) error {
	workers := 8
	work := make(chan *importer.File, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			for f := range work {
				r, err := os.Open(f.BasePath())
				if err != nil {
					errs <- fmt.Errorf("%s could not be opened: %w", f.BasePath(), err)
					break
				}

				exists, err := imp.Exists(f, r, 100)
				r.Close()
				if err != nil {
					errs <- err
					break
				}
				if exists {
					continue
				}

				fn := filepath.Base(f.BasePath())
				d := importer.NewFile(destination, f.Bytes(), fn)
				p := d.Path()
				if err := copy(f.BasePath(), p); err != nil {
					errs <- fmt.Errorf("%s could not be copied to %s: %w", f.BasePath(), p, err)
					break
				}

				if err := imp.Add(p, d); err != nil {
					errs <- err
					break
				}
			}
			wg.Done()
		}()
	}

	d := []*importer.File{}
	err := f.scan(f.dir, &d)
	if err != nil {
		return err
	}

	n := 0
	var wErr error

outer:
	for _, f := range d {
		n++
		imp.Progress(n, len(d))
		select {
		case work <- f:
			continue
		case err := <-errs:
			wErr = err
			break outer
		}
	}

	close(work)
	wg.Wait()
	if wErr != nil {
		return wErr
	}
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

func copy(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()

	_, err = io.Copy(d, s)
	return err
}
