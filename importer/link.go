package importer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/frizinak/photos/meta"
)

type LinkInfo struct {
	dir  string
	fn   string
	link *File
}

func (i *Importer) FindLinks(f *File) (links []string, err error) {
	links = []string{}
	err = i.walkLinks(f, func(l string) (bool, error) {
		links = append(links, l)
		return true, nil
	})

	return
}

func (i *Importer) walkLinks(f *File, cb func(string) (bool, error)) error {
	fabs, err := Abs(f.Path())
	if err != nil {
		return err
	}

	return i.scanLinks(i.colDir, func(l LinkInfo) (bool, error) {
		if fabs == l.link.Path() {
			return cb(filepath.Join(l.dir, l.fn))
		}
		return true, nil
	})
}

func (i *Importer) fileFromLink(link string) (*File, error) {
	var f *File
	target, err := Abs(link)
	if err != nil {
		if os.IsNotExist(err) {
			os.Remove(link)
		}
		return f, err
	}

	return NewFileFromPath(target), nil
}

func (i *Importer) scanDir(dir string, cb func(path string) (bool, error)) (bool, error) {
	d, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return true, err
	}
	list, err := d.Readdir(-1)
	d.Close()
	if err != nil {
		return true, err
	}

	for _, item := range list {
		fp := filepath.Join(dir, item.Name())
		if item.IsDir() {
			cont, err := i.scanDir(fp, cb)
			if !cont || err != nil {
				return cont, err
			}
			continue
		}

		cont, err := cb(fp)
		if err != nil || !cont {
			return cont, err
		}
	}

	return true, nil
}

func (i *Importer) scanLinks(dir string, cb func(LinkInfo) (bool, error)) error {
	i.symlinkSem.Lock()
	if _, ok := i.symlinkCache[dir]; !ok {
		i.symlinkCache[dir] = make([]LinkInfo, 0)
		_, err := i.scanDir(dir, func(path string) (bool, error) {
			fn := filepath.Base(path)
			if !i.supported(fn) {
				return true, nil
			}

			file, err := i.fileFromLink(path)
			if err != nil {
				if os.IsNotExist(err) {
					return true, nil
				}
				return false, err
			}

			meta, err := GetMeta(file)
			if err != nil {
				return false, err
			}

			if meta.Deleted {
				if err := os.Remove(path); err != nil {
					return false, err
				}
			}

			i.symlinkCache[dir] = append(
				i.symlinkCache[dir],
				LinkInfo{dir: filepath.Dir(path), fn: fn, link: file},
			)
			return true, nil
		})
		if err != nil {
			i.symlinkSem.Unlock()
			return err
		}
	}
	i.symlinkSem.Unlock()

	i.symlinkSem.RLock()
	defer i.symlinkSem.RUnlock()
	for _, l := range i.symlinkCache[dir] {
		if cont, err := cb(l); !cont || err != nil {
			return err
		}
	}

	return nil
}

func Abs(path string) (string, error) {
	rp, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(rp)
}

func NicePath(dir string, f *File, meta meta.Meta) string {
	d := meta.CreatedTime()
	return filepath.Join(
		dir,
		d.Format("2006"),
		d.Format("01-02 Mon"),
		"misc",
		fmt.Sprintf(
			"%s--%s",
			d.Format("2006-01-02-15-04"),
			f.FilenameWithoutBytes(),
		),
	)
}

func (i *Importer) Link(f *File) error {
	real, err := Abs(f.Path())
	if err != nil {
		return err
	}

	os.MkdirAll(i.colDir, 0755)
	exists := false
	err = i.scanLinks(i.colDir, func(l LinkInfo) (bool, error) {
		if l.link.Path() == real {
			exists = true
			return false, nil
		}
		return true, nil
	})

	if err != nil || exists {
		return err
	}

	meta, err := GetMeta(f)
	if err != nil {
		return err
	}

	if meta.Deleted {
		return nil
	}

	linkDest := NicePath(i.colDir, f, meta)
	linkDir := filepath.Dir(linkDest)
	os.MkdirAll(linkDir, 0755)
	linkDir, err = Abs(linkDir)
	if err != nil {
		return err
	}

	link, err := filepath.Rel(linkDir, real)
	if err != nil {
		return fmt.Errorf("Refuse to make non-relative symlinks, make sure both your raw directory and collection directory are on the same filesystem: %w", err)
	}

	linkDest = filepath.Join(linkDir, filepath.Base(linkDest))
	i.verbose.Printf("linking '%s' to '%s'", link, linkDest)
	if err := os.Symlink(link, linkDest); err != nil {
		return err
	}

	return nil
}
