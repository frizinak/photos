package pp3

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
)

func init() {
	ini.PrettyEqual = false
	ini.PrettyFormat = false
	ini.PrettySection = true
}

type PP3 struct {
	path string
	ini  *ini.File
}

func Load(path string) (*PP3, error) {
	return load(path, ini.LoadOptions{IgnoreInlineComment: true})
}

func New(path string) (*PP3, error) {
	// dafuq is this api
	p, err := load(path, ini.LoadOptions{IgnoreInlineComment: true, Loose: true})
	if err != nil {
		return p, err
	}

	p.Set("Version", "AppVersion", "5.8")
	p.Set("Version", "Version", "346")

	p.Set("General", "Rank", "0")
	p.Set("General", "ColorLabel", "0")
	p.Set("General", "InTrash", "false")

	p.Set("Crop", "Enabled", "false")
	p.Set("Crop", "X", "-1")
	p.Set("Crop", "Y", "-1")
	p.Set("Crop", "W", "-1")
	p.Set("Crop", "H", "-1")
	p.Set("Crop", "FixedRatio", "false")
	p.Set("Crop", "Ratio", "As Image")
	p.Set("Crop", "Guide", "Frame")

	return p, nil
}

func load(path string, opts ini.LoadOptions) (*PP3, error) {
	ini, err := ini.LoadSources(opts, path)
	if err != nil {
		return nil, err
	}
	return &PP3{
		path,
		ini,
	}, nil
}

func (pp *PP3) Save() error {
	return pp.SaveTo(pp.path)
}

func (pp *PP3) SaveTo(path string) error {
	tmp := path + ".tmp"
	if err := pp.ini.SaveTo(tmp); err != nil {
		os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, path)
}

func (pp *PP3) WriteTo(w io.Writer) error {
	_, err := pp.ini.WriteTo(w)
	return err
}

func (pp *PP3) str(s *ini.Section, k *ini.Key) string {
	return fmt.Sprintf("%s.%s=%s", s.Name(), k.Name(), k.Value())
}

func (pp *PP3) strings(s *ini.Section, str *[]string) {
	for _, k := range s.Keys() {
		*str = append(*str, pp.str(s, k))
	}
}

func (pp *PP3) Key(section, key string) *ini.Key {
	return pp.ini.Section(section).Key(key)
}

func (pp *PP3) Has(section, key string) bool {
	return pp.ini.Section(section).HasKey(key)
}

func (pp *PP3) Get(section, key string) string {
	return pp.Key(section, key).Value()
}

func (pp *PP3) Set(section, key, value string) {
	pp.Key(section, key).SetValue(value)
}

func pf(s, pref string) bool {
	return strings.HasPrefix(s, pref)
}

func (pp *PP3) filter(list []string) []string {
	l := make([]string, 0, len(list))
	del := []string{}
	for _, s := range list {
		if strings.HasSuffix(s, ".Enabled=false") {
			del = append(del, s[0:len(s)-13])
		}
	}

outer:
	for _, s := range list {
		switch {
		case pf(s, "Exif."):
			fallthrough
		case pf(s, "IPTC."):
			k := pp.Key("MetaData", "Mode")
			if k.Value() == "" {
				pp.ini.Section("MetaData").DeleteKey("Mode")
				continue outer
			}
			v, err := k.Int()
			if err == nil && v != 1 {
				continue outer
			}

		case pf(s, "General.ColorLabel="):
			fallthrough
		case pf(s, "General.InTrash="):
			fallthrough
		case pf(s, "General.Rank="):
			fallthrough
		case pf(s, "Version."):
			continue outer
		}

		for _, d := range del {
			if pf(s, d) {
				continue outer
			}
		}

		l = append(l, s)
	}

	return l
}

func (pp *PP3) Hash() string {
	d := make([]string, 0)
	for _, s := range pp.ini.Sections() {
		pp.strings(s, &d)
	}

	d = pp.filter(d)
	sort.Strings(d)
	str := strings.Join(d, "\n")
	sum := sha512.Sum512([]byte(str))
	return hex.EncodeToString(sum[:])
}

func (pp *PP3) Trashed() bool {
	return pp.Key("General", "InTrash").MustBool()
}

func (pp *PP3) Trash(v bool) {
	s := "true"
	if !v {
		s = "false"
	}
	pp.Set("General", "InTrash", s)
}

func (pp *PP3) Rank() int     { return pp.Key("General", "Rank").MustInt() }
func (pp *PP3) SetRank(v int) { pp.Set("General", "Rank", strconv.Itoa(v)) }

func (pp *PP3) Keywords() []string {
	v := strings.Split(pp.Get("IPTC", "Keywords"), ";")
	l := make([]string, 0, len(v))
	for _, kw := range v {
		kw = strings.TrimSpace(kw)
		if kw != "" {
			l = append(l, kw)
		}
	}

	return l
}

func (pp *PP3) SetKeywords(v []string) {
	pp.Set("IPTC", "Keywords", strings.Join(v, ";")+";")
}

func (pp *PP3) ResizeLongest(size int) {
	width := pp.Key("Crop", "W").MustInt()
	height := pp.Key("Crop", "H").MustInt()
	which := "1"
	if height > width {
		which = "2"
	}

	s := strconv.Itoa(size)

	pp.Set("Resize", "Enabled", "true")
	pp.Set("Resize", "Scale", "1")
	pp.Set("Resize", "AppliesTo", "Cropped area")
	pp.Set("Resize", "Method", "Lanczos")
	pp.Set("Resize", "DataSpecified", which)
	pp.Set("Resize", "Width", s)
	pp.Set("Resize", "Height", s)
	pp.Set("Resize", "AllowUpscaling", "false")
}
