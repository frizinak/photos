package importer

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type File struct {
	dir   string
	bytes int64
	fn    string

	_fn     string
	_path   string
	_pathwb string
}

func NewFile(dir string, bytes int64, fn string) *File {
	return &File{dir: dir, bytes: bytes, fn: fn}
}

func NewFileFromPath(path string) *File {
	ffn := strings.SplitN(filepath.Base(path), "-", 2)
	dir := filepath.Dir(path)
	fn := ffn[1]
	bytes, err := strconv.ParseInt(ffn[0], 10, 64)
	if err != nil {
		panic(fmt.Errorf("invalid path %s", path))
	}

	return &File{dir: dir, bytes: bytes, fn: fn}
}

func (f *File) PathWithoutBytes() string {
	if f._pathwb == "" {
		f._pathwb = filepath.Join(f.dir, f.FilenameWithoutBytes())
	}
	return f._pathwb
}

func (f *File) Path() string {
	if f._path == "" {
		f._path = filepath.Join(f.dir, f.Filename())
	}
	return f._path
}

func (f *File) FilenameWithoutBytes() string { return f.fn }

func (f *File) Filename() string {
	if f._fn == "" {
		f._fn = fmt.Sprintf("%013d-%s", f.bytes, f.fn)
	}
	return f._fn
}

func (f *File) Bytes() int64 { return f.bytes }

type Files []*File

func (f Files) Len() int           { return len(f) }
func (f Files) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }
func (f Files) Less(i, j int) bool { return f[i].FilenameWithoutBytes() < f[j].FilenameWithoutBytes() }
