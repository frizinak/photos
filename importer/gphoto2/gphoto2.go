package gphoto2

import (
	"bufio"
	"fmt"
	"log"
	"os/exec"
	"path"
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

func init() {
	importer.Register(bin, New())
}

type GPhoto2 struct {
}

func New() *GPhoto2 {
	return &GPhoto2{}
}

type scanCloser struct {
	*bufio.Scanner
	cmd *exec.Cmd
}

func (s *scanCloser) Close() error {
	if err := s.cmd.Wait(); err != nil {
		return err
	}
	return s.Err()
}

func (g *GPhoto2) cmd(cmd *exec.Cmd) (*scanCloser, error) {
	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanLines)
	return &scanCloser{scanner, cmd}, nil
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

func (g *GPhoto2) Import(log *log.Logger, destination string, exists importer.Exists, add importer.Add, prog importer.Progress) error {
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

		if s[0] != '#' {
			continue
		}

		fn := filenameRE.FindStringSubmatch(s)
		if len(fn) != 3 {
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
		if exists(file) {
			continue
		}
		file = importer.NewFile(destination, byt, currentFile)
		files = append(files, file)
		prog(0, len(files))

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
		prog(n, len(files))
		log.Println(scanner.Text())
	}

	for _, f := range files {
		if err := add(f.BasePath(), f); err != nil {
			return err
		}
	}

	return scanner.Close()
}
