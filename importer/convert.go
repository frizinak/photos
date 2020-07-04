package importer

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"gopkg.in/ini.v1"
)

func (i *Importer) convert(link, dir, output string, pp3 *ini.File, converted map[string]string, size int) (string, error) {
	pp3TempPath, hash, err := i.pp3Convert(link, pp3, size)
	defer os.Remove(pp3TempPath)
	if err != nil {
		return "", err
	}

	output = fmt.Sprintf("%s.jpg", output)
	rel, err := filepath.Rel(dir, output)
	if err != nil {
		return rel, err
	}

	_, err = os.Stat(output)
	exists := false
	if err == nil {
		exists = true
	} else if !os.IsNotExist(err) {
		return rel, err
	}

	if h, ok := converted[rel]; exists && ok && h == hash {
		return rel, nil
	}
	converted[rel] = hash

	os.MkdirAll(filepath.Dir(output), 0755)

	cmd := exec.Command(
		"rawtherapee-cli",
		"-Y",
		"-o", output,
		"-q",
		"-p", pp3TempPath,
		"-c", link,
	)
	buf := bytes.NewBuffer(nil)
	cmd.Stderr = buf
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		return rel, fmt.Errorf("%s: %s", err, buf)
	}
	return rel, nil
}

func (i *Importer) pp3Convert(link string, pp3 *ini.File, size int) (string, string, error) {
	width, _ := pp3.Section("Crop").Key("W").Int()
	height, _ := pp3.Section("Crop").Key("H").Int()

	which := "1"
	if height > width {
		which = "2"
	}

	s := strconv.Itoa(size)
	resize := pp3.Section("Resize")
	resize.Key("Enabled").SetValue("true")
	resize.Key("Scale").SetValue("1")
	resize.Key("AppliesTo").SetValue("Cropped area")
	resize.Key("Method").SetValue("Lanczos")
	resize.Key("DataSpecified").SetValue(which)
	resize.Key("Width").SetValue(s)
	resize.Key("Height").SetValue(s)
	resize.Key("AllowUpscaling").SetValue("false")

	buf := bytes.NewBuffer(nil)
	if _, err := pp3.WriteTo(buf); err != nil {
		return "", "", err
	}

	tmppath := fmt.Sprintf("%s.pp3.%d", link, size)
	f, err := os.Create(tmppath)
	if err != nil {
		return "", "", err
	}

	del := func() {
		f.Close()
		os.Remove(tmppath)
	}

	tee := io.TeeReader(buf, f)
	hash := sha512.New()
	if _, err := io.Copy(hash, tee); err != nil {
		del()
		return tmppath, "", err
	}
	h := hash.Sum(nil)
	hex := hex.EncodeToString(h)
	return tmppath, hex, err
}

func (i *Importer) Convert(f *File, sizes []int) error {
	links := []string{}
	pp3s := []*ini.File{}
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
		return err
	}

	if len(links) == 0 {
		return nil
	}

	meta, err := GetMeta(f)
	if err != nil {
		return err
	}
	for n := range links {
		conv := meta.Converted
		if conv == nil {
			conv = make(map[string]string)
		}
		rels := make(map[string]struct{}, len(conv))
		for _, s := range sizes {
			base := i.NicePath(i.convDir, f, meta)
			dir := filepath.Dir(base)
			fn := filepath.Base(base)
			ext := filepath.Ext(fn)
			fn = fn[0 : len(fn)-len(ext)]
			output := filepath.Join(dir, strconv.Itoa(s), fn)
			rel, err := i.convert(links[n], i.convDir, output, pp3s[n], conv, s)
			if err != nil {
				return err
			}
			rels[rel] = struct{}{}
		}

		for k := range conv {
			if _, ok := rels[k]; !ok {
				delete(conv, k)
			}
		}

		meta.Converted = conv
	}

	return SaveMeta(f, meta)
}
