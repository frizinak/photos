package importer

import (
	"os"

	"github.com/frizinak/photos/meta"
	"github.com/frizinak/photos/tags"
)

func metaFile(f *File) string {
	return f.Path() + ".meta"
}

func MakeMeta(f *File) (meta.Meta, error) {
	m, err := GetMeta(f)
	if err != nil && !os.IsNotExist(err) {
		return m, err
	}

	if err != nil {
		m = meta.New(f.Bytes(), f.FilenameWithoutBytes(), f.Filename())
	}

	tags := tags.Parse(f.Path())
	if err = tags.Err(); err != nil {
		return m, err
	}
	date := tags.Date()
	m.Created = date.Unix()
	m.Checksum, err = sum(f.Path())
	if err != nil {
		return m, err
	}

	return m, m.Save(metaFile(f))
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
			return MakeMeta(f)
		}
	}
	return m, err
}
