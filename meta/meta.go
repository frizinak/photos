package meta

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Tags []string

func (t Tags) Unique() Tags {
	tm := make(map[string]struct{}, len(t))
	for _, tag := range t {
		tm[tag] = struct{}{}
	}
	nt := make(Tags, 0, len(t))
	for i := range tm {
		nt = append(nt, i)
	}
	return nt
}

type Meta struct {
	Checksum     string
	Size         int64
	RealFilename string
	BaseFilename string
	Created      int64

	Deleted bool
	Rating  int

	PP3       []string
	Converted map[string]string

	Tags Tags
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

func Load(path string) (Meta, error) {
	m := Meta{}
	f, err := os.Open(path)
	if err != nil {
		return m, err
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&m)
	if err != nil {
		err = fmt.Errorf("could not load meta %s: %w", path, err)
	}
	return m, err
}

func (m Meta) Save(path string) error {
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
