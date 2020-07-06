package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"gopkg.in/ini.v1"
)

func init() {
	ini.PrettyEqual = false
	ini.PrettyFormat = false
	ini.PrettySection = true
}

func (i *Importer) GetPP3(link string) (*ini.File, string, error) {
	pp3Path := fmt.Sprintf("%s.pp3", link)
	pp3, err := ini.LoadSources(
		ini.LoadOptions{IgnoreInlineComment: true},
		pp3Path,
	)
	return pp3, pp3Path, err
}

func (i *Importer) PP3ToMeta(link string) error {
	file, err := i.link(link)
	if err != nil {
		return err
	}

	meta, err := EnsureMeta(file)
	if err != nil {
		return err
	}

	pp3, pp3Path, err := i.GetPP3(link)
	if err != nil {
		return err
	}

	general := pp3.Section("General")
	if k, err := general.GetKey("InTrash"); err == nil {
		deleted, err := k.Bool()
		if err != nil {
			return err
		}
		meta.Deleted = deleted
	}

	if k, err := general.GetKey("Rank"); err == nil {
		rank, err := k.Int()
		if err != nil {
			return err
		}
		meta.Rating = rank
	}

	currentTime := time.Now().Local()
	if err := SaveMeta(file, meta); err != nil {
		return err
	}

	os.Chtimes(metaFile(file), currentTime, currentTime)
	os.Chtimes(pp3Path, currentTime, currentTime)
	return nil
}

func (i *Importer) MetaToPP3(link string) error {
	file, err := i.link(link)
	if err != nil {
		return err
	}

	meta, err := EnsureMeta(file)
	if err != nil {
		return err
	}

	pp3, pp3Path, err := i.GetPP3(link)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		f, err := os.Create(pp3Path)
		if err != nil {
			return err
		}
		f.Close()
		pp3, err = ini.LoadSources(ini.LoadOptions{IgnoreInlineComment: true}, pp3Path)
		if err != nil {
			return err
		}
		version := pp3.Section("Version")
		version.Key("AppVersion").SetValue("5.8")
		version.Key("Version").SetValue("346")
		general := pp3.Section("General")
		general.Key("Rank").SetValue("0")
		general.Key("ColorLabel").SetValue("0")
		general.Key("InTrash").SetValue("false")

		crop := pp3.Section("Crop")
		crop.Key("Enabled").SetValue("false")
		crop.Key("X").SetValue("-1")
		crop.Key("Y").SetValue("-1")
		crop.Key("W").SetValue("-1")
		crop.Key("H").SetValue("-1")
		crop.Key("FixedRatio").SetValue("true")
		crop.Key("Ratio").SetValue("As Image")
		crop.Key("Guide").SetValue("Frame")
	}

	general := pp3.Section("General")
	general.Key("Rank").SetValue(strconv.Itoa(meta.Rating))
	general.Key("InTrash").SetValue(strconv.FormatBool(meta.Deleted))

	currentTime := time.Now().Local()
	if err := pp3.SaveTo(pp3Path); err != nil {
		return err
	}
	os.Chtimes(metaFile(file), currentTime, currentTime)
	os.Chtimes(pp3Path, currentTime, currentTime)
	return nil
}

type mtime struct {
	file string
	time time.Time
}

type mtimes []mtime

func (m mtimes) Less(i, j int) bool { return m[i].time.Before(m[j].time) }
func (m mtimes) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m mtimes) Len() int           { return len(m) }

func (i *Importer) syncMetaAndPP3(f *File) ([]string, error) {
	metaPath := metaFile(f)
	links, err := i.FindLinks(f)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, nil
	}

	var metaUpdate time.Time
	metaStat, err := os.Stat(metaPath)
	if err != nil {
		metaStat = nil
		if !os.IsNotExist(err) {
			return nil, err
		}
		_, err := MakeMeta(f)
		if err != nil {
			return nil, err
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
				return nil, err
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
	switch {
	case last.time.After(metaUpdate):
		if err := i.PP3ToMeta(last.file); err != nil {
			return list, err
		}
		for n := 0; n < len(mt)-1; n++ {
			meta2pp3 = append(meta2pp3, mt[n])
		}
	case metaUpdate.After(last.time):
		fallthrough
	case last.time == time.Time{}:
		for n := 0; n < len(mt); n++ {
			meta2pp3 = append(meta2pp3, mt[n])
		}
	default:
		return list, err
	}

	for n := range meta2pp3 {
		if err := i.MetaToPP3(meta2pp3[n].file); err != nil {
			return list, err
		}
	}

	return list, nil
}

func (i *Importer) SyncMetaAndPP3(f *File) error {
	if !i.supportedPP3(f.Path()) {
		return nil
	}

	pp3s, err := i.syncMetaAndPP3(f)
	if err != nil {
		return err
	}

	meta, err := GetMeta(f)
	if err != nil {
		return err
	}

	meta.PP3 = []string{}
	for _, pp3 := range pp3s {
		rel, err := filepath.Rel(i.colDir, pp3+".pp3")
		if err != nil {
			return err
		}
		meta.PP3 = append(meta.PP3, rel)
	}

	return SaveMeta(f, meta)
}
