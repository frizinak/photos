package importer

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	rawexts = map[string]struct{}{
		".raf":  {},
		".nef":  {},
		".dng":  {},
		".tiff": {},
		".tif":  {},
		".3fr":  {},
		".ari":  {},
		".arw":  {},
		".srf":  {},
		".sr2":  {},
		".bay":  {},
		".braw": {},
		".cri":  {},
		".crw":  {},
		".cr2":  {},
		".cr3":  {},
		".cap":  {},
		".iiq":  {},
		".eip":  {},
		".dcs":  {},
		".dcr":  {},
		".drf":  {},
		".k25":  {},
		".kdc":  {},
		".erf":  {},
		".fff":  {},
		".gpr":  {},
		".jxs":  {},
		".mef":  {},
		".mdc":  {},
		".mos":  {},
		".mrw":  {},
		".nrw":  {},
		".orf":  {},
		".pef":  {},
		".ptx":  {},
		".pxn":  {},
		".r3d":  {},
		".raw":  {},
		".rw2":  {},
		".rwl":  {},
		".rwz":  {},
		".srw":  {},
		".tco":  {},
		".x3f":  {},
	}
	imageexts = map[string]struct{}{
		".jpg":  {},
		".jpeg": {},
		".png":  {},
	}
	videoexts = map[string]struct{}{
		".mov":  {},
		".mp4":  {},
		".mkv":  {},
		".webm": {},
		".avi":  {},
	}
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

func (f *File) BasePath() string {
	if f._pathwb == "" {
		f._pathwb = filepath.Join(f.dir, f.BaseFilename())
	}
	return f._pathwb
}

func (f *File) Path() string {
	if f._path == "" {
		f._path = filepath.Join(f.dir, f.Filename())
	}
	return f._path
}

func (f *File) BaseFilename() string { return f.fn }

func (f *File) Filename() string {
	if f._fn == "" {
		f._fn = fmt.Sprintf("%013d-%s", f.bytes, f.fn)
	}
	return f._fn
}

func (f *File) Bytes() int64 { return f.bytes }

func (f *File) TypeRAW() bool   { return FileTypeRAW(f.fn) }
func (f *File) TypeImage() bool { return FileTypeImage(f.fn) }
func (f *File) TypeVideo() bool { return FileTypeVideo(f.fn) }

type Files []*File

func (f Files) Len() int           { return len(f) }
func (f Files) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }
func (f Files) Less(i, j int) bool { return f[i].BaseFilename() < f[j].BaseFilename() }

func ext(s string) string {
	return strings.ToLower(filepath.Ext(s))
}

func FileTypeRAW(file string) bool {
	_, ok := rawexts[ext(file)]
	return ok
}
func FileTypeImage(file string) bool {
	_, ok := imageexts[ext(file)]
	return ok
}
func FileTypeVideo(file string) bool {
	_, ok := videoexts[ext(file)]
	return ok
}

func ImageExtList(n []string) []string { return mlist(n, imageexts) }
func VideoExtList(n []string) []string { return mlist(n, videoexts) }
func RawExtList(n []string) []string   { return mlist(n, rawexts) }

func mlist(l []string, m map[string]struct{}) []string {
	if l == nil {
		l = make([]string, 0, len(m))
	}
	for v := range m {
		l = append(l, v)
	}
	return l
}
