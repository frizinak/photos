package tags

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/frizinak/binary"
	"github.com/frizinak/phodo/exif"
)

type Aperture struct{ fraction }
type ShutterSpeed struct{ fraction }
type FocalLength struct{ fraction }
type ISO int

func (i ISO) BinaryEncode(w *binary.Writer)     { w.WriteUint32(uint32(i)) }
func (i ISO) BinaryDecode(r *binary.Reader) ISO { return ISO(r.ReadUint32()) }

type fraction [2]uint

func (f fraction) BinaryEncode(w *binary.Writer) {
	w.WriteUint64(uint64(f[0]))
	w.WriteUint64(uint64(f[1]))
}

func (f fraction) BinaryDecode(r *binary.Reader) fraction {
	f[0] = uint(r.ReadUint64())
	f[1] = uint(r.ReadUint64())
	return f
}

func (f fraction) Nil() bool { return f[0] == 0 || f[1] == 0 }

func (f fraction) String() string {
	if f[0] == 0 {
		return "0"
	}
	if f[1] == 1 {
		return strconv.FormatUint(uint64(f[0]), 10)
	}
	return fmt.Sprintf("%d/%d", f[0], f[1])
}

func (f fraction) Float() float64 {
	if f.Nil() {
		return 0
	}
	return float64(f[0]) / float64(f[1])
}

func (a Aperture) String() string     { return fmt.Sprintf("f/%.1f", a.Float()) }
func (s ShutterSpeed) String() string { return fmt.Sprintf("%ss", s.fraction.String()) }
func (f FocalLength) String() string  { return fmt.Sprintf("%.2fmm", f.Float()) }
func (i ISO) String() string          { return fmt.Sprintf("iso%d", i) }

func (i ISO) Nil() bool { return i == 0 }

func (f fraction) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]uint(f))
}

func (f *fraction) UnmarshalJSON(d []byte) error {
	return json.Unmarshal(d, (*[2]uint)(f))
}

type Device struct {
	Make, Model string
}

func (d Device) BinaryEncode(w *binary.Writer) {
	w.WriteString(d.Make, 16)
	w.WriteString(d.Model, 16)
}

func (d Device) BinaryDecode(r *binary.Reader) Device {
	d.Make = r.ReadString(16)
	d.Model = r.ReadString(16)
	return d
}

type CameraInfo struct {
	Device
	Lens         Device
	Aperture     Aperture
	ShutterSpeed ShutterSpeed
	FocalLength  FocalLength
	ISO          ISO
}

func (c CameraInfo) BinaryEncode(w *binary.Writer) {
	c.Device.BinaryEncode(w)
	c.Lens.BinaryEncode(w)
	c.Aperture.BinaryEncode(w)
	c.ShutterSpeed.BinaryEncode(w)
	c.FocalLength.BinaryEncode(w)
	c.ISO.BinaryEncode(w)
}

func (c CameraInfo) BinaryDecode(r *binary.Reader) CameraInfo {
	c.Device = c.Device.BinaryDecode(r)
	c.Lens = c.Lens.BinaryDecode(r)
	c.Aperture = Aperture{c.Aperture.BinaryDecode(r)}
	c.ShutterSpeed = ShutterSpeed{c.ShutterSpeed.BinaryDecode(r)}
	c.FocalLength = FocalLength{c.FocalLength.BinaryDecode(r)}
	c.ISO = c.ISO.BinaryDecode(r)
	return c
}

func (c CameraInfo) DeviceString() string {
	if c.Lens.Make == "" && c.Lens.Model == "" {
		return fmt.Sprintf(
			"%s %s [?]",
			c.Make,
			c.Model,
		)
	}
	return fmt.Sprintf(
		"%s %s [%s %s]",
		c.Make,
		c.Model,
		c.Lens.Make,
		c.Lens.Model,
	)
}

func (c CameraInfo) ExposureString() string {
	items := make([]string, 0, 4)
	l := []interface {
		Nil() bool
		String() string
	}{
		c.Aperture,
		c.ShutterSpeed,
		c.ISO,
		c.FocalLength,
	}
	for _, i := range l {
		if !i.Nil() {
			items = append(items, i.String())
		}
	}
	return strings.Join(items, " ")

}

func (c CameraInfo) String() string {
	return fmt.Sprintf("%s - %s", c.DeviceString(), c.ExposureString())
}

type Tags struct {
	ex *exif.Exif
	ff *FFProbeInfo
}

var fre = regexp.MustCompile(`^\[?([0-9]+)/([0-9]+)\]?$`)

func commonDenominator(a, b uint) uint {
	if a == b {
		return a
	}
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a > b {
		return commonDenominator(a-b, b)
	}
	return commonDenominator(a, b-a)
}

func (t *Tags) CameraInfo() (CameraInfo, bool) {
	c := CameraInfo{}
	if t.ex == nil {
		return c, false
	}

	gets := func(f ...uint16) string {
		v := t.ex.Find(f...).Value().Value
		s, _ := v.(string)
		return strings.Trim(s, "\x00")
	}

	getf := func(f ...uint16) fraction {
		v := t.ex.Find(f...)
		val := v.Value()
		ints := val.Ints()
		if len(ints) != 0 {
			return fraction{uint(ints[0]), 1}
		}

		var num, denom uint
		switch rv := val.Value.(type) {
		case [][2]uint32:
			if len(rv) != 0 {
				num = uint(rv[0][0])
				denom = uint(rv[0][1])
			}
		}

		if c := commonDenominator(num, denom); c != 0 {
			return fraction{num / c, denom / c}
		}
		return fraction{num, denom}
	}

	c.Make, c.Model = gets(0x010f), gets(0x0110)
	c.Lens.Make, c.Lens.Model = gets(0x8769, 0xa433), gets(0x8769, 0xa434)

	c.Aperture = Aperture{getf(0x8769, 0x829d)}
	c.ShutterSpeed = ShutterSpeed{getf(0x8769, 0x829a)}
	isos := t.ex.Find(0x8769, 0x8827).Value().Ints()
	if len(isos) != 0 {
		c.ISO = ISO(isos[0])
	}
	c.FocalLength = FocalLength{getf(0x8769, 0x920a)}

	return c, true
}

func (t *Tags) Bounds() image.Rectangle {
	if t.ff != nil {
		return t.ff.Bounds()
	}

	return image.Rectangle{}
}

func (t *Tags) Duration() time.Duration {
	if t.ff != nil {
		return t.ff.Duration()
	}

	// perhaps return shutterspeed ;)
	return 0
}

func (t *Tags) Date() time.Time {
	if t.ex != nil {
		d, err := t.exifDate()
		if err != nil {
			panic(err)
		}
		return d
	}
	if t.ff != nil {
		return t.ff.Date()
	}
	return time.Time{}
}

func (t *Tags) exifDate() (time.Time, error) {
	var dt time.Time
	tag, _ := t.ex.Find(0x8769, 0x9003).Value().Value.(string)
	offset := []uint16{0x8769, 0x9011}
	if tag == "" {
		tag, _ = t.ex.Find(0x0132).Value().Value.(string)
		offset = []uint16{0x8769, 0x9010}
		if tag == "" {
			return dt, errors.New("no datetime exif tag found")
		}
	}

	exOffset, _ := t.ex.Find(offset...).Value().Value.(string)
	if exOffset != "" && exOffset[len(exOffset)-1] != ':' {
		return time.Parse("2006:01:02 15:04:05 -07:00", tag+" "+exOffset)
	}

	return time.ParseInLocation("2006:01:02 15:04:05", tag, time.Local)
}

func ParseExif(path string) (*Tags, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	exif, err := exif.Read(f)
	f.Close()

	return &Tags{ex: exif}, err
}

type FFProbeInfo struct {
	Streams []struct {
		Width        int `json:"width"`
		Height       int `json:"height"`
		SideDataList []struct {
			Rotation int `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		Tags     struct {
			CreationTime time.Time `json:"creation_time"`
		} `json:"tags"`
	} `json:"format"`
}

func (ff *FFProbeInfo) Date() time.Time {
	return ff.Format.Tags.CreationTime
}

func (ff *FFProbeInfo) Duration() time.Duration {
	f, err := strconv.ParseFloat(ff.Format.Duration, 64)
	if err != nil {
		return 0
	}
	return time.Duration(f*1e6) * 1e3
}

func (ff *FFProbeInfo) Bounds() image.Rectangle {
	var w, h int
	for _, s := range ff.Streams {
		for _, sd := range s.SideDataList {
			if sd.Rotation == 90 || sd.Rotation == 270 {
				s.Width, s.Height = s.Height, s.Width
				break
			}
		}
		if s.Width > w {
			w = s.Width
		}
		if s.Height > h {
			h = s.Height
		}
	}

	return image.Rect(0, 0, w, h)
}

func ParseFFProbe(path string) (*Tags, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v",
		"quiet",
		path,
		"-print_format",
		"json",
		"-show_entries",
		"format=duration:format_tags=creation_time:stream", //width,height",
	)

	stdout, stderr := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	ff := &FFProbeInfo{}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe err: %s, %w", stderr.String(), err)
	}

	dec := json.NewDecoder(stdout)
	return &Tags{ff: ff}, dec.Decode(ff)
}
