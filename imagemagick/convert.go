package imagemagick

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
)

type JPEGConfig struct {
	Quality int
	Width   int
	Height  int
}

func ToJPEG(file string, w io.Writer, c JPEGConfig) error {
	args := []string{"convert", file}

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

	args = append(args, "-strip", "-quality", strconv.Itoa(c.Quality), "jpeg:-")
	cmd := exec.Command("magick", args...)

	cmd.Stdout = w
	buf := bytes.NewBuffer(nil)
	cmd.Stderr = buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, buf.String())
	}

	return nil
}
