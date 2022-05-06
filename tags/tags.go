package tags

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
)

type parser struct {
}

const (
	exifOffsetTime    = "OffsetTime"
	exifOffsetTimeFB1 = "OffsetTimeFallback1"
	exifOffsetTimeFB2 = "OffsetTimeFallback2"
)

var exifFields = map[uint16]exif.FieldName{
	0x9010: exifOffsetTime,
	0x9011: exifOffsetTimeFB1,
	0x9012: exifOffsetTimeFB2,
}

func (p *parser) Parse(x *exif.Exif) error {
	ptr, err := x.Get("ExifIFDPointer")
	if err != nil {
		return nil
	}
	offset, err := ptr.Int(0)
	if err != nil {
		return nil
	}
	r := bytes.NewReader(x.Raw)
	_, err = r.Seek(int64(offset), io.SeekStart)
	if err != nil {
		return err
	}
	subDir, _, err := tiff.DecodeDir(r, x.Tiff.Order)
	if err != nil {
		return nil
	}

	x.LoadTags(subDir, exifFields, false)
	return nil
}

func init() {
	exif.RegisterParsers(&parser{})
}

type Aperture struct{ fraction }
type ShutterSpeed struct{ fraction }
type FocalLength struct{ fraction }
type ISO int

type fraction [2]uint

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

type CameraInfo struct {
	Device
	Lens         Device
	Aperture     Aperture
	ShutterSpeed ShutterSpeed
	FocalLength  FocalLength
	ISO          ISO
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
	ex  *exif.Exif
	ff  *FFProbeInfo
	err error
}

func (t *Tags) Err() error { return t.err }

var fre = regexp.MustCompile(`^([0-9]+)/([0-9]+)$`)

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

	getstr := func(f exif.FieldName) string {
		v, _ := t.ex.Get(f)
		if v == nil {
			return ""
		}
		return strings.Trim(v.String(), "\" ")
	}

	getf := func(f exif.FieldName) fraction {
		v := getstr(f)
		sm := fre.FindStringSubmatch(v)
		if len(sm) == 3 {
			_nom, err := strconv.ParseUint(sm[1], 10, 64)
			if err != nil {
				return fraction{0, 1}
			}
			_denom, err := strconv.ParseUint(sm[2], 10, 64)
			if err != nil {
				return fraction{0, 1}
			}
			nom, denom := uint(_nom), uint(_denom)
			if c := commonDenominator(nom, denom); c != 0 {
				return fraction{nom / c, denom / c}
			}
			return fraction{nom, denom}
		}

		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			n = 0
		}
		return fraction{uint(n), 1}
	}

	c.Make, c.Model = getstr(exif.Make), getstr(exif.Model)
	c.Lens.Make, c.Lens.Model = getstr(exif.LensMake), getstr(exif.LensModel)

	c.Aperture = Aperture{getf(exif.FNumber)}
	c.ShutterSpeed = ShutterSpeed{getf(exif.ExposureTime)}
	iso, err := strconv.Atoi(getstr(exif.ISOSpeedRatings))
	if err == nil {
		c.ISO = ISO(iso)
	}
	c.FocalLength = FocalLength{getf(exif.FocalLength)}

	return c, true
}

func (t *Tags) Date(tzOffsetMinutes int) time.Time {
	if t.ex != nil {
		d, err := t.exifDate(tzOffsetMinutes)
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

func (t *Tags) exifDate(tzOffset int) (time.Time, error) {
	var dt time.Time
	tag, err := t.ex.Get("DateTimeOriginal")
	if err != nil {
		tag, err = t.ex.Get("DateTime")
		if err != nil {
			return dt, err
		}
	}

	var exOffset string
	names := []exif.FieldName{exifOffsetTime, exifOffsetTimeFB1, exifOffsetTimeFB2}
	for _, n := range names {
		o, err := t.ex.Get(n)
		if err != nil {
			continue
		}
		v, err := o.StringVal()
		if err != nil {
			continue
		}
		v = strings.TrimRight(v, "\x00")
		if v == "" {
			continue
		}
		exOffset = v
		break
	}

	s, err := tag.StringVal()
	if err != nil {
		return dt, err
	}

	if exOffset != "" {
		return time.Parse("2006:01:02 15:04:05 -07:00", s+" "+exOffset)
	}

	dt, err = time.ParseInLocation(
		"2006:01:02 15:04:05",
		strings.TrimRight(s, "\x00"),
		time.FixedZone("Fixed", int((time.Minute*time.Duration(tzOffset)).Seconds())),
	)
	if err != nil {
		return dt, err
	}
	return dt.Local(), nil
}

func Parse(path string) *Tags {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".nef", ".raf", ".dng", ".tiff", ".tif", ".jpg":
		ex, err := ParseExif(path)
		return &Tags{ex: ex, err: err}
	default:
		ff, err := ParseFFProbe(path)
		return &Tags{ff: ff, err: err}
	}
}

func ParseExif(path string) (*exif.Exif, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return exif.Decode(f)
}

type FFProbeInfo struct {
	Format struct {
		Tags struct {
			CreationTime time.Time `json:"creation_time"`
		} `json:"tags"`
	} `json:"format"`
}

func (ff *FFProbeInfo) Date() time.Time {
	return ff.Format.Tags.CreationTime
}

func ParseFFProbe(path string) (*FFProbeInfo, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v",
		"quiet",
		path,
		"-print_format",
		"json",
		"-show_entries",
		"format_tags=creation_time",
	)

	stdout, stderr := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	ff := &FFProbeInfo{}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe err: %s, %w", stderr.String(), err)
	}

	dec := json.NewDecoder(stdout)
	return ff, dec.Decode(ff)
}
