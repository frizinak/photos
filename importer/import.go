package importer

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

type Exists func(*File, io.ReadSeeker, int64) (bool, error)
type Add func(src string, dest *File) error
type Progress func(n, total int)

type Import struct {
	exists   Exists
	add      Add
	progress Progress
}

func (i *Import) Exists(f *File, r io.ReadSeeker, maxProbes int64) (bool, error) {
	return i.exists(f, r, maxProbes)
}
func (i *Import) Add(src string, dest *File) error { return i.add(src, dest) }
func (i *Import) Progress(n, total int)            { i.progress(n, total) }

type Backend interface {
	Available() (bool, error)
	Import(log *log.Logger, destination string, i *Import) error
}

var (
	lock     sync.Mutex
	backends = map[string]Backend{}
	alphaRE  = regexp.MustCompile(`[^A-Za-z0-9.\-_]+`)
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

	symlinkSem        sync.RWMutex
	symlinkCache      map[string][]LinkInfo
	symlinkCachePaths map[string]map[string]struct{}
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
	i.symlinkCachePaths = make(map[string]map[string]struct{})
	i.symlinkSem.Unlock()
}

func (i *Importer) supported(file string) bool {
	return FileTypeRAW(file) || FileTypeImage(file) || FileTypeVideo(file)
}

func (i *Importer) supportedPP3(file string) bool {
	return FileTypeRAW(file) || FileTypeImage(file)
}

func (i *Importer) ImageExtList(n []string) []string { return ImageExtList(n) }
func (i *Importer) VideoExtList(n []string) []string { return VideoExtList(n) }
func (i *Importer) RawExtList(n []string) []string   { return RawExtList(n) }

func (i *Importer) Import(checksum bool, progress Progress) error {
	os.MkdirAll(i.rawDir, 0755)

	im := &Import{}
	im.progress = progress
	im.exists = func(f *File, r io.ReadSeeker, maxProbes int64) (bool, error) {
		p := (NewFile(i.rawDir, f.bytes, f.fn)).Path()
		s, err := os.Stat(p)
		if os.IsNotExist(err) {
			if checksum {
				i.log.Printf("Would import %s from %s", p, f.Path())
			}
			return false, nil
		}

		if checksum {
			return false, nil
		}

		if s.IsDir() || err != nil {
			if err == nil {
				err = fmt.Errorf("file '%s' exists as a directory", p)
			}
			return false, err
		}

		// exists
		if r == nil {
			i.log.Printf("[WARN] skipping %s, exists as %s but file contents were not compared", f.fn, p)
			return true, nil
		}

		ex, err := os.Open(p)
		if err != nil {
			return true, err
		}
		defer ex.Close()

		const probeSize = 1024
		bufEx := make([]byte, probeSize)
		bufNw := make([]byte, probeSize)
		probes := f.bytes / probeSize
		if probes > maxProbes {
			probes = maxProbes
		}
		jump := f.bytes / probes

		var i int64
		for ; i < f.bytes-probeSize; i += jump {
			r.Seek(i, io.SeekStart)
			ex.Seek(i, io.SeekStart)
			if _, err := ex.Read(bufEx); err != nil {
				return false, err
			}

			if _, err := r.Read(bufNw); err != nil {
				return false, err
			}

			if !bytes.Equal(bufNw, bufEx) {
				return true, fmt.Errorf(
					"file %s exists but is not identical to %s\nthe file will not be imported!\nmake a manual backup of this file and wait till a solution is implemented",
					p,
					f.BasePath(),
				)
			}
		}

		return true, nil
	}

	im.add = func(src string, f *File) error {
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
		if err := b.Import(i.verbose, tmpdest, im); err != nil {
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
	var items []os.DirEntry
	for err == nil {
		items, err = d.ReadDir(50)
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
