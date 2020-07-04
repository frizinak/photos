package tiff

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type Type string

const (
	NEF  Type = "nef"
	DNG  Type = "dng"
	TIFF Type = "tiff"
)

func TypeForExt(ext string) (Type, error) {
	var t Type
	switch strings.ToLower(ext) {
	case ".nef":
		t = NEF
	case ".dng":
		t = DNG
	case ".tiff":
		t = TIFF
	}

	if t == "" {
		return t, fmt.Errorf("could not find type for ext %s", ext)
	}

	return t, nil
}

type JPEGConfig struct {
	Quality int
	Type    Type
	Width   int
	Height  int
}

func JPEG(r io.Reader, w io.Writer, c JPEGConfig) error {
	args := []string{
		fmt.Sprintf("%s:-", c.Type),
	}

	var x, y string
	if c.Width != 0 {
		x = strconv.Itoa(c.Width)
	}
	if c.Height != 0 {
		y = strconv.Itoa(c.Height)
	}

	if x != "" || y != "" {
		args = append(args, "-resize", fmt.Sprintf("%sx%s", x, y))
	}

	args = append(args, "-quality", strconv.Itoa(c.Quality), "jpeg:-")
	cmd := exec.Command("convert", args...)

	cmd.Stdin = r
	cmd.Stdout = w
	buf := bytes.NewBuffer(nil)
	cmd.Stderr = buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, buf.String())
	}

	return nil
}
