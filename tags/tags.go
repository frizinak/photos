package tags

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

type Tags struct {
	ex  *exif.Exif
	ff  *FFProbeInfo
	err error
}

func (t *Tags) Err() error { return t.err }

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
	case ".nef", ".dng", ".tiff", ".jpg":
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
