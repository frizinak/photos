package importer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/frizinak/photos/meta"
)

func rmEmpty(dir string) (bool, error) {
	d, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	list, err := d.Readdir(-1)
	d.Close()
	if err != nil {
		return false, err
	}

	files := false
	for _, item := range list {
		fp := filepath.Join(dir, item.Name())
		if item.IsDir() {
			f, err := rmEmpty(fp)
			if err != nil {
				return files, err
			}
			if f {
				files = true
			}
			continue
		}

		files = true
	}

	if !files {
		if err := os.Remove(dir); err != nil {
			return files, err
		}
	}

	return files, nil
}

func (i *Importer) DoCleanup(paths []string) error {
	for _, f := range paths {
		if err := os.Remove(f); err != nil {
			return err
		}
	}

	if _, err := rmEmpty(i.convDir); err != nil {
		return err
	}
	_, err := rmEmpty(i.colDir)
	return err
}

func (i *Importer) Cleanup(minRating int) ([]string, error) {
	all := make([]*File, 0, 1000)
	err := i.All(func(f *File) (bool, error) {
		all = append(all, f)
		return true, nil
	})

	if err != nil {
		return nil, err
	}

	converted := make(map[string]struct{}, len(all))
	pp3 := make(map[string]struct{}, len(all))

	for _, f := range all {
		m, err := GetMeta(f)
		if err != nil {
			return nil, err
		}

		if (m.Deleted || m.Rating <= minRating) && len(m.Conv) != 0 {
			m.Conv = make(map[string]meta.Converted)
			if err = SaveMeta(f, m); err != nil {
				return nil, err
			}
		}

		if m.Deleted {
			continue
		}

		for n := range m.Conv {
			converted[n] = struct{}{}
		}

		for _, n := range m.PP3 {
			pp3[n] = struct{}{}
		}
	}

	delete := make([]string, 0)
	_, err = i.scanDir(i.convDir, func(path string) (bool, error) {
		rel, err := filepath.Rel(i.convDir, path)
		if err != nil {
			return false, err
		}

		if _, ok := converted[rel]; !ok {
			delete = append(delete, path)
		}

		return true, nil
	})

	if err != nil {
		return delete, err
	}

	_, err = i.scanDir(i.colDir, func(path string) (bool, error) {
		if strings.ToLower(filepath.Ext(path)) != ".pp3" {
			return true, nil
		}

		rel, err := filepath.Rel(i.colDir, path)
		if err != nil {
			return false, err
		}

		if _, ok := pp3[rel]; !ok {
			delete = append(delete, path)
		}

		return true, nil
	})

	return delete, err
}
