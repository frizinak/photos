package meta

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	jsoniter "github.com/json-iterator/go"
)

type Tags []string

func (t Tags) Map() map[string]struct{} {
	tm := make(map[string]struct{}, len(t))
	for _, tag := range t {
		tm[tag] = struct{}{}
	}
	return tm
}

func (t Tags) Unique() Tags {
	nt := make(Tags, 0, len(t))
	for i := range t.Map() {
		nt = append(nt, i)
	}
	sort.Strings(nt)
	return nt
}

type Converted struct {
	Hash string
	Size int
}

type Meta struct {
	Checksum     string
	Size         int64
	RealFilename string
	BaseFilename string
	Created      int64

	Deleted bool
	Rating  int

	PP3  []string
	Conv map[string]Converted

	Tags     Tags
	Location *Location
}

type Location struct {
	Lat     float64
	Lng     float64
	Name    string
	Address string
}

func New(size int64, real string, base string) Meta {
	m := Meta{}
	m.Size = size
	m.RealFilename = real
	m.BaseFilename = base
	return m
}

func (m Meta) CreatedTime() time.Time {
	return time.Unix(m.Created, 0)
}

var j = jsoniter.Config{
	EscapeHTML:                    false,
	ObjectFieldMustBeSimpleString: true,
}.Froze()

func Load(path string) (Meta, error) {
	m := Meta{}
	f, err := os.Open(path)
	if err != nil {
		return m, err
	}
	defer f.Close()
	err = j.NewDecoder(f).Decode(&m)
	if err != nil {
		err = fmt.Errorf("could not load meta %s: %w", path, err)
	}
	return m, err
}

func (m Meta) Save(path string) error {
	m.Tags = m.Tags.Unique()
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if err := enc.Encode(m); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}
