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

func (f *FS) Import(log *log.Logger, destination string, exists importer.Exists, add importer.Add) error {
	workers := 8
	work := make(chan *importer.File, workers)
	var wg sync.WaitGroup
	var wErr error
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			for f := range work {
				if exists(f) {
					continue
				}

				fn := filepath.Base(f.PathWithoutBytes())
				d := importer.NewFile(destination, f.Bytes(), fn)
				p := d.Path()
				if err := copy(f.PathWithoutBytes(), p); err != nil {
					wErr = err
					break
				}

				if err := add(p, d); err != nil {
					wErr = err
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

	for _, f := range d {
		if wErr != nil {
			break
		}
		work <- f
	}
	close(work)
	wg.Wait()
	return wErr
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
