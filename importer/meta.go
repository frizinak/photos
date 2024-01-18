package importer

import (
	"os"
	"time"

	"github.com/frizinak/photos/meta"
	"github.com/frizinak/photos/tags"
)

func metaFile(f *File) string {
	return f.Path() + ".meta"
}

func MakeMeta(f *File, date time.Time) (meta.Meta, error) {
	m, err := GetMeta(f)
	if err != nil && !os.IsNotExist(err) {
		return m, err
	}

	if err != nil {
		m = meta.New(f.Bytes(), f.BaseFilename(), f.Filename())
		m.Checksum, err = sum(f.Path())
		if err != nil {
			return m, err
		}

	}

	p := tags.ParseExif
	if f.TypeVideo() {
		p = tags.ParseFFProbe
	}
	tags, err := p(f.Path())
	if err != nil {
		return m, err
	}

	m = metaTime(m, tags, date)

	if ci, ok := tags.CameraInfo(); ok {
		m.CameraInfo = &ci
	}

	return m, m.Save(metaFile(f))
}

func metaTime(m meta.Meta, tags *tags.Tags, date time.Time) meta.Meta {
	if date != (time.Time{}) {
		m.CreatedOverride = true
		m.Created = date.Unix()
	}

	if m.CreatedOverride {
		return m
	}

	m.Created = tags.Date().Unix()
	return m
}

func GetMeta(f *File) (meta.Meta, error) {
	return meta.Load(metaFile(f))
}

func SaveMeta(f *File, m meta.Meta) error {
	return m.Save(metaFile(f))
}

func EnsureMeta(f *File) (meta.Meta, error) {
	m, err := GetMeta(f)
	if err != nil {
		if os.IsNotExist(err) {
			return MakeMeta(f, time.Time{})
		}
	}
	return m, err
}
