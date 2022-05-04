package importer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/frizinak/photos/meta"
	"github.com/frizinak/photos/pp3"
)

func (i *Importer) convert(input, output string, pp *pp3.PP3) error {
	pp3TempPath := fmt.Sprintf("%s.tmp.pp3", output)
	err := pp.SaveTo(pp3TempPath)
	defer os.Remove(pp3TempPath)
	if err != nil {
		return err
	}

	tmp := output
	if filepath.Ext(output) != ".jpg" {
		// bruuh
		tmp = output + ".rawtherapeehack.jpg"
	}

	args := []string{
		"-Y",
		"-o", tmp,
		"-q",
		"-p", pp3TempPath,
		"-c", input,
	}
	i.verbose.Printf("rawtherapee-cli %s", strings.Join(args, " "))
	cmd := exec.Command("rawtherapee-cli", args...)

	buf := bytes.NewBuffer(nil)
	cmd.Stderr = buf
	if err := cmd.Run(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("%s: %s", err, buf)
	}

	if tmp != output {
		return os.Rename(tmp, output)
	}

	return nil
}

func (i *Importer) convertIfUpdated(
	link,
	dir,
	output string,
	pp *pp3.PP3,
	converted map[string]meta.Converted,
	size int,
	checkOnly bool,
) (bool, string, error) {
	pp.ResizeLongest(size)
	hash := pp.Hash()
	output = fmt.Sprintf("%s.jpg", output)
	rel, err := filepath.Rel(dir, output)
	if err != nil {
		return false, rel, err
	}

	_, err = os.Stat(output)
	exists := false
	if err == nil {
		exists = true
	} else if !os.IsNotExist(err) {
		return false, rel, err
	}

	if h, ok := converted[rel]; exists && ok && h.Hash == hash {
		return false, rel, nil
	}
	converted[rel] = meta.Converted{Hash: hash, Size: size}

	if checkOnly {
		return true, rel, nil
	}
	os.MkdirAll(filepath.Dir(output), 0755)
	return true, rel, i.convert(link, output, pp)
}

func (i *Importer) Unedited(f *File) (bool, error) {
	edited := true
	err := i.walkLinks(f, func(link string) (bool, error) {
		pp3, _, err := i.GetPP3(link)
		if err != nil {
			if os.IsNotExist(err) {
				edited = false
				return false, nil
			}
			return false, err
		}
		if !pp3Edited(pp3) {
			edited = false
			return false, nil
		}

		return true, nil
	})

	return !edited, err
}

func pp3Edited(pp *pp3.PP3) bool {
	return pp.Has("Exposure", "Compensation")
}

func (i *Importer) CheckConvert(f *File, sizes []int) (bool, error) {
	return i.fileConvert(f, sizes, true)
}

func (i *Importer) Convert(f *File, sizes []int) error {
	_, err := i.fileConvert(f, sizes, false)
	return err
}

func (i *Importer) fileConvert(f *File, sizes []int, checkOnly bool) (bool, error) {
	links := []string{}
	pp3s := []*pp3.PP3{}
	err := i.walkLinks(f, func(link string) (bool, error) {
		pp3, _, err := i.GetPP3(link)
		if err != nil {
			if os.IsNotExist(err) {
				return true, nil
			}
			return false, err
		}

		pp3s = append(pp3s, pp3)
		links = append(links, link)
		return true, err
	})
	if err != nil {
		return false, err
	}

	if len(links) == 0 {
		return false, nil
	}

	m, err := GetMeta(f)
	if err != nil {
		return false, err
	}

	conv := m.Conv
	if conv == nil {
		conv = make(map[string]meta.Converted)
	}
	rels := make(map[string]struct{}, len(conv))
	changed := false
	for n, link := range links {
		for _, s := range sizes {
			custom, err := filepath.Rel(i.colDir, link)
			if err != nil {
				return false, err
			}
			base := filepath.Join(i.convDir, custom)
			dir := filepath.Dir(base)
			fn := filepath.Base(base)
			ext := filepath.Ext(fn)
			fn = fn[0 : len(fn)-len(ext)]
			output := filepath.Join(dir, strconv.Itoa(s), fn)
			conv, rel, err := i.convertIfUpdated(
				links[n],
				i.convDir,
				output,
				pp3s[n],
				conv,
				s,
				checkOnly,
			)
			changed = changed || conv
			if err != nil {
				return false, err
			}
			rels[rel] = struct{}{}
		}
	}

	for k := range conv {
		if _, ok := rels[k]; !ok {
			changed = true
			delete(conv, k)
		}
	}

	if checkOnly {
		return changed, nil
	}

	m.Conv = conv

	return changed, SaveMeta(f, m)
}
