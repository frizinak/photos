package importer

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

type Exists func(*File) bool
type Add func(src string, clean *File) error
type Progress func(n, total int)

type Backend interface {
	Available() (bool, error)
	Import(log *log.Logger, destination string, exists Exists, add Add, prog Progress) error
}

var (
	lock             sync.Mutex
	backends         = map[string]Backend{}
	alphaRE          = regexp.MustCompile(`[^A-Za-z0-9.\-_]+`)
	supportedExtList = map[string]struct{}{
		".nef":  struct{}{},
		".jpg":  struct{}{},
		".dng":  struct{}{},
		".tiff": struct{}{},
		".mov":  struct{}{},
	}
	pp3ExtList = map[string]struct{}{
		".nef":  struct{}{},
		".jpg":  struct{}{},
		".dng":  struct{}{},
		".tiff": struct{}{},
	}
)

func clean(str string) string {
	return alphaRE.ReplaceAllString(str, "-")
}

func Register(name string, backend Backend) {
	lock.Lock()
	backends[name] = backend
	lock.Unlock()
}

type Importer struct {
	log     *log.Logger
	verbose *log.Logger
	rawDir  string
	colDir  string
	convDir string

	symlinkSem   sync.RWMutex
	symlinkCache map[string][]LinkInfo
}

func New(log, verbose *log.Logger, rawDir, colDir, convDir string) *Importer {
	i := &Importer{
		log:     log,
		verbose: verbose,
		rawDir:  rawDir, colDir: colDir, convDir: convDir,
	}
	i.ClearCache()
	return i
}

func (i *Importer) ClearCache() {
	i.symlinkSem.Lock()
	i.symlinkCache = make(map[string][]LinkInfo)
	i.symlinkSem.Unlock()
}

func (i *Importer) IsImage(file string) bool {
	return i.supportedPP3(file)
}

func (i *Importer) supported(file string) bool {
	ext := strings.ToLower(filepath.Ext(file))
	_, ok := supportedExtList[ext]
	return ok
}

func (i *Importer) supportedPP3(file string) bool {
	ext := strings.ToLower(filepath.Ext(file))
	_, ok := pp3ExtList[ext]
	return ok
}

func (i *Importer) SupportedExtList() []string {
	l := make([]string, 0, len(supportedExtList))
	for i := range supportedExtList {
		l = append(l, i)
	}
	return l
}

func (i *Importer) Import(checksum bool, progress Progress) error {
	os.MkdirAll(i.rawDir, 0755)

	exists := func(f *File) bool {
		p := (NewFile(i.rawDir, f.bytes, f.fn)).Path()
		s, err := os.Stat(p)
		if os.IsNotExist(err) {
			if checksum {
				i.log.Printf("Would import %s from %s", p, f.Path())
			}
			return false
		}

		if checksum {
			return false
		}

		if s.IsDir() || err != nil {
			if err == nil {
				err = fmt.Errorf("file '%s' exists as a directory", p)
			}
			panic(err)
		}

		return true
	}

	add := func(src string, f *File) error {
		if !i.supported(f.fn) {
			return fmt.Errorf("unsupported extension %s", f.Path())
		}

		p := NewFile(i.rawDir, f.bytes, f.fn)
		dest := p.Path()
		i.verbose.Printf("importing %s to %s", f.Path(), p.Path())

		if checksum {
			defer os.Remove(src)
			existing, err := sum(dest)
			if os.IsNotExist(err) {
				return nil
			}
			newfile, err := sum(src)
			if err != nil {
				return err
			}
			if newfile != existing {
				i.log.Printf("Duplicate filename '%s' -> '%s' different checksum", f.Path(), dest)
			}
			return nil
		}

		err := os.Rename(src, dest)
		if err != nil {
			return err
		}

		_, err = MakeMeta(p)
		return err
	}

	lock.Lock()
	defer lock.Unlock()
	for n, b := range backends {
		tmpdest := fmt.Sprintf("%s/tmp-%s", i.rawDir, clean(n))
		os.RemoveAll(tmpdest)
		os.MkdirAll(tmpdest, 0700)
		defer os.RemoveAll(tmpdest)
		ok, err := b.Available()
		if err != nil {
			return err
		}

		if !ok {
			continue
		}

		i.log.Printf("Importing with %s", n)
		if err := b.Import(i.verbose, tmpdest, exists, add, progress); err != nil {
			return err
		}
	}

	return nil
}

func (i *Importer) AllCounted(it func(f *File, n, total int) (bool, error)) error {
	files := Files{}
	err := i.All(func(f *File) (bool, error) {
		files = append(files, f)
		return true, nil
	})
	if err != nil {
		return err
	}
	for n, rf := range files {
		if cont, err := it(rf, n+1, len(files)); !cont || err != nil {
			return err
		}
	}

	return nil
}

func (i *Importer) All(it func(f *File) (bool, error)) error {
	d, err := os.Open(i.rawDir)
	if err != nil {
		return err
	}
	defer d.Close()
	var items []os.FileInfo
	for err == nil {
		items, err = d.Readdir(20)
		for _, f := range items {
			if !i.supported(f.Name()) {
				continue
			}
			rf := NewFileFromPath(filepath.Join(i.rawDir, f.Name()))
			if cont, err := it(rf); !cont || err != nil {
				return err
			}
		}
	}

	if err == io.EOF {
		err = nil
	}

	return err
}

func sum(path string) (string, error) {
	cs := sha512.New()
	rf, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer rf.Close()
	if _, err = io.Copy(cs, rf); err != nil {
		return "", err
	}
	return hex.EncodeToString(cs.Sum(nil)), nil
}
