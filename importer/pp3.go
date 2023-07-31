package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/frizinak/photos/pp3"
)

type PP3 struct {
	path string
	*pp3.PP3
}

func (pp PP3) Edited() bool {
	if pp.PP3 == nil {
		return false
	}
	return pp.Has("Exposure", "Compensation")
}

func (i *Importer) GetPP3(link string) (PP3, error) {
	pp3Path := fmt.Sprintf("%s.pp3", link)
	pp3, err := pp3.Load(pp3Path)
	return PP3{pp3Path, pp3}, err
}

func (pp PP3) Path() string { return pp.path }

func (pp PP3) Save() error { return pp.SaveTo(pp.path) }

func (i *Importer) PP3ToMeta(link string) error {
	file, err := i.fileFromLink(link)
	if err != nil {
		return err
	}

	m, err := EnsureMeta(file)
	if err != nil {
		return err
	}

	pp, err := i.GetPP3(link)
	if err != nil {
		return err
	}

	m.Deleted = pp.Trashed()
	m.Rating = pp.Rank()
	m.Tags = pp.Keywords()

	return SaveMeta(file, m)
}

func (i *Importer) MetaToPP3(link string) error {
	file, err := i.fileFromLink(link)
	if err != nil {
		return err
	}

	meta, err := EnsureMeta(file)
	if err != nil {
		return err
	}

	pp, err := i.GetPP3(link)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		pp.PP3, err = pp3.New(pp.Path())
		if err != nil {
			return err
		}
	}

	pp.SetRank(meta.Rating)
	pp.Trash(meta.Deleted)
	pp.SetKeywords(meta.Tags.Unique())

	return pp.Save()
}

type mtime struct {
	file string
	time time.Time
}

type mtimes []mtime

func (m mtimes) Less(i, j int) bool { return m[i].time.Before(m[j].time) }
func (m mtimes) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m mtimes) Len() int           { return len(m) }

func (i *Importer) syncMetaAndPP3(f *File) (bool, []string, error) {
	metaPath := metaFile(f)
	links, err := i.FindLinks(f)
	if err != nil {
		return false, nil, err
	}
	if len(links) == 0 {
		return false, nil, nil
	}

	var metaUpdate time.Time
	metaStat, err := os.Stat(metaPath)
	if err != nil {
		metaStat = nil
		if !os.IsNotExist(err) {
			return false, nil, err
		}
		_, err := MakeMeta(f)
		if err != nil {
			return false, nil, err
		}
	}

	if metaStat != nil {
		metaUpdate = metaStat.ModTime()
	}

	mt := make(mtimes, len(links))
	for n, link := range links {
		pp3Path := fmt.Sprintf("%s.pp3", link)
		var pp3Update time.Time

		pp3Stat, err := os.Stat(pp3Path)
		if err != nil {
			pp3Stat = nil
			if !os.IsNotExist(err) {
				return false, nil, err
			}
		}

		if pp3Stat != nil {
			pp3Update = pp3Stat.ModTime()
		}
		mt[n] = mtime{link, pp3Update}
	}

	sort.Sort(mt)
	list := make([]string, 0, len(mt))
	for n := range mt {
		list = append(list, mt[n].file)
	}

	last := mt[len(mt)-1]
	meta2pp3 := make(mtimes, 0, len(mt))

	for _, v := range mt {
		if v.time == (time.Time{}) {
			meta2pp3 = append(meta2pp3, v)
		}
	}

	changed := false
	switch {
	case last.time.After(metaUpdate):
		i.verbose.Printf("sync pp3 to meta from %s to %s", last.file, f.Path())
		changed = true
		if err := i.PP3ToMeta(last.file); err != nil {
			return changed, list, err
		}
		for n := 0; n < len(mt)-1; n++ {
			meta2pp3 = append(meta2pp3, mt[n])
		}
	case metaUpdate.After(last.time):
		meta2pp3 = append(meta2pp3, mt...)
	}

	if len(meta2pp3) == 0 {
		return changed, list, err
	}

	uniq := make(map[string]struct{}, len(meta2pp3))
	m2p := make(mtimes, 0, len(meta2pp3))
	for _, v := range meta2pp3 {
		if _, ok := uniq[v.file]; ok {
			continue
		}

		uniq[v.file] = struct{}{}
		m2p = append(m2p, v)
	}

	changed = true
	for _, v := range m2p {
		i.verbose.Printf("sync meta to pp3 from %s to %s", f.Path(), v.file)
		if err := i.MetaToPP3(v.file); err != nil {
			return changed, list, err
		}
	}

	return changed, list, nil
}

func (i *Importer) SyncMetaAndPP3(f *File) error {
	if !i.supportedPP3(f.Path()) {
		return nil
	}

	changed, paths, err := i.syncMetaAndPP3(f)
	if err != nil {
		return err
	}

	meta, err := GetMeta(f)
	if err != nil {
		return err
	}

	files := []string{metaFile(f)}
	newpp := []string{}
	for _, path := range paths {
		pp3 := path + ".pp3"
		files = append(files, pp3)
		rel, err := filepath.Rel(i.colDir, pp3)
		if err != nil {
			return err
		}
		newpp = append(newpp, rel)
	}
	sort.Strings(meta.PP3)
	sort.Strings(newpp)
	if !strEqual(meta.PP3, newpp) {
		changed = true
		meta.PP3 = newpp
		if err := SaveMeta(f, meta); err != nil {
			return err
		}
	}

	if changed {
		now := time.Now().Local()
		for _, f := range files {
			if err := os.Chtimes(f, now, now); err != nil {
				return err
			}
		}
	}

	return nil
}

func strEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
