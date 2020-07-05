package importer

import (
	"path/filepath"
	"strings"
)

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
		meta, err := GetMeta(f)
		if err != nil {
			return nil, err
		}

		if meta.Deleted {
			continue
		}

		if meta.Rating > minRating {
			for n := range meta.Converted {
				converted[n] = struct{}{}
			}
		}

		for _, n := range meta.PP3 {
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
