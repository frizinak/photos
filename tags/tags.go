package tags

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

type Tags struct {
	ex  *exif.Exif
	ff  *FFProbeInfo
	err error
}

func (t *Tags) Err() error { return t.err }

func (t *Tags) Date() time.Time {
	if t.ex != nil {
		d, err := t.ex.DateTime()
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
