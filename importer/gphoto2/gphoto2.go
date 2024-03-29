package gphoto2

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/frizinak/photos/importer"
)

const bin = "gphoto2"

var (
	folderRE       = regexp.MustCompile(`in folder '(.*?)'.`)
	filenameRE     = regexp.MustCompile(`^#(\d+)\s+([^\.]+\.[^\s]+)`)
	fullInfoPathRE = regexp.MustCompile(`Information on file '(.*?)'.*?folder '(.*?)'`)
	sizeRE         = regexp.MustCompile(`Size:\s+(\d+) byte`)
)

type GPhoto2 struct {
	exts map[string]struct{}
}

func New(exts []string) *GPhoto2 {
	m := map[string]struct{}{}
	for _, e := range exts {
		e = strings.ToLower(e)
		m[e] = struct{}{}
	}
	return &GPhoto2{exts: m}
}

type scanCloser struct {
	*bufio.Scanner
	err *bytes.Buffer
	cmd *exec.Cmd
}

func (s *scanCloser) Close() error {
	if err := s.cmd.Wait(); err != nil {
		err = fmt.Errorf("%w: %s", err, s.err.String())
		return err
	}
	return s.Err()
}

func (g *GPhoto2) cmd(cmd *exec.Cmd) (*scanCloser, error) {
	buf := bytes.NewBuffer(nil)
	cmd.Stderr = buf

	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanLines)
	return &scanCloser{scanner, buf, cmd}, nil
}

func (g *GPhoto2) Available() (bool, error) {
	scanner, err := g.cmd(exec.Command(bin, "--auto-detect"))
	if err != nil {
		return false, err
	}

	header := true
	hasCamera := false
	for scanner.Scan() {
		b := scanner.Bytes()
		allDash := true
		if !header {
			hasCamera = strings.TrimSpace(string(b)) != ""
			break
		}

		if len(b) == 0 {
			continue
		}

		for i := 0; i < len(b); i++ {
			if b[i] != 45 {
				allDash = false
			}
		}

		if allDash {
			header = false
		}
	}

	return hasCamera, scanner.Close()
}

func (g *GPhoto2) Import(log *log.Logger, destination string, imp *importer.Import) error {
	scanner, err := g.cmd(exec.Command(bin, "-L"))
	if err != nil {
		return err
	}

	items := []string{}
	fnMap := map[string]string{}
	folder := ""

	for scanner.Scan() {
		s := scanner.Text()
		f := folderRE.FindStringSubmatch(s)
		if len(f) == 2 {
			folder = f[1]
			continue
		}

		if len(s) == 0 {
			continue
		}

		if s[0] != '#' {
			continue
		}

		fn := filenameRE.FindStringSubmatch(s)
		if len(fn) != 3 {
			continue
		}

		ext := filepath.Ext(fn[2])
		if _, ok := g.exts[strings.ToLower(ext)]; !ok {
			continue
		}

		p := path.Join(folder, fn[2])
		fnMap[p] = fn[1]
		items = append(items, fn[1])
	}

	if err := scanner.Close(); err != nil {
		return err
	}

	scanner, err = g.cmd(exec.Command(bin, "--show-info", strings.Join(items, ",")))
	if err != nil {
		return err
	}

	currentDir := ""
	currentFile := ""
	indices := make([]string, 0, len(items))
	files := make([]*importer.File, 0, len(items))
	for scanner.Scan() {
		s := scanner.Text()
		f := fullInfoPathRE.FindStringSubmatch(s)
		if len(f) == 3 {
			currentDir, currentFile = f[2], f[1]
			continue
		}

		b := sizeRE.FindStringSubmatch(s)
		if len(b) != 2 {
			continue
		}

		byt, err := strconv.ParseInt(b[1], 10, 64)
		if err != nil {
			return err
		}

		file := importer.NewFile(currentDir, byt, currentFile)
		exists, err := imp.Exists(file, nil, 10)
		if err != nil {
			return err
		}

		if exists {
			continue
		}
		file = importer.NewFile(destination, byt, currentFile)
		files = append(files, file)
		imp.Progress(0, len(files))

		internalFile := path.Join(currentDir, currentFile)
		ix := fnMap[internalFile]
		if ix == "" {
			return fmt.Errorf("could not find index for file '%s'", internalFile)
		}

		indices = append(indices, ix)
	}

	if err := scanner.Close(); err != nil {
		return err
	}

	cmd := exec.Command(bin, "-p", strings.Join(indices, ","))
	cmd.Dir = destination
	scanner, err = g.cmd(cmd)
	if err != nil {
		return err
	}

	n := 0
	for scanner.Scan() {
		n++
		imp.Progress(n, len(files))
		log.Println(scanner.Text())
	}
	if err := scanner.Close(); err != nil {
		return err
	}

	for _, f := range files {
		if err := imp.Add(f.BasePath(), f); err != nil {
			return err
		}
	}

	return nil
}
