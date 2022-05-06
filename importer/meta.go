package importer

import (
	"os"

	"github.com/frizinak/photos/meta"
	"github.com/frizinak/photos/tags"
)

func metaFile(f *File) string {
	return f.Path() + ".meta"
}

func MakeMeta(f *File, tzOffsetMinutes int) (meta.Meta, error) {
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

	tags := tags.Parse(f.Path())
	if err = tags.Err(); err != nil {
		return m, err
	}
	date := tags.Date(tzOffsetMinutes)
	m.Created = date.Unix()

	if ci, ok := tags.CameraInfo(); ok {
		m.CameraInfo = &ci
	}

	return m, m.Save(metaFile(f))
}

func GetMeta(f *File) (meta.Meta, error) {
	return meta.Load(metaFile(f))
}

func SaveMeta(f *File, m meta.Meta) error {
	return m.Save(metaFile(f))
}

func EnsureMeta(f *File, tzOffsetMinutes int) (meta.Meta, error) {
	m, err := GetMeta(f)
	if err != nil {
		if os.IsNotExist(err) {
			return MakeMeta(f, tzOffsetMinutes)
		}
	}
	return m, err
}
