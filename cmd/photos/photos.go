package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/frizinak/photos/importer"
	"github.com/frizinak/photos/importer/fs"
	_ "github.com/frizinak/photos/importer/gphoto2"
	"github.com/frizinak/photos/meta"
	"github.com/frizinak/photos/rate"
)

func ask() string {
	sc := bufio.NewScanner(os.Stdin)
	sc.Split(bufio.ScanLines)
	sc.Scan()
	return sc.Text()
}

type flagStrs []string

func (i *flagStrs) String() string {
	return "my string representation"
}

func (i *flagStrs) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func exit(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func main() {
	var actions string
	var itemFilter string
	var ratingGTFilter int
	var ratingLTFilter int
	var baseDir string
	var rawDir string
	var collectionDir string
	var jpegDir string
	var fsSources flagStrs
	var checksum bool
	var sizes string
	var alwaysYes bool
	var zero bool
	var maxWorkers int
	var noRawPrefix bool
	var edited bool

	flag.StringVar(
		&actions,
		"actions",
		"import,link,sync-meta,link",
		`comma separated list of actions:
- import         Import media from connected camera (gphoto2) and any given directory (-source) to the directory specified with -raws
- show           Show raws (filter with -filter)
- show-jpegs     Show jpegs (filter with -filter) (see -no-raw)
- show-links     Show links (filter with -filter) (see -no-raw)
- info           Show info for given RAWs
- update-meta    Rewrite .meta file (filter with -filter)
- link           Create collection symlinks in the given directory (-collection)
- previews       Generate simple jpeg previews (used by -actions rate)
- rate           Simple opengl window to rate / trash images (filter with -filter)
- sync-meta      Sync .meta file with .pp3 (file mtime determines which one is the authority)
- convert        Convert images to jpegs to the given directory (-jpegs) and sizes (-sizes) (filter with -filter and -edited)
- exec           Run an external command for each file (first non flag and any further arguments, {} is replaced with the filepath)
                 e.g.: photos -base . -actions exec -filter all wc -c {}
- cleanup        Remove pp3s and jpegs for deleted RAWs 
                 -filter and -lt are ignored
				 Images whose rating is not higher than -gt will also have their jpegs deleted.
`)

	flag.StringVar(&itemFilter, "filter", "normal", "[any] filter (normal / all / deleted / unrated / unedited)")
	flag.IntVar(&ratingGTFilter, "gt", -1, "[any] additional greater than given rating filter")
	flag.IntVar(&ratingLTFilter, "lt", 6, "[any] additional less than given rating filter")
	flag.BoolVar(&edited, "edited", false, "[convert] only convert images that have been edited with rawtherapee")
	flag.BoolVar(&checksum, "sum", false, "[import] dry-run and report non-identical files with duplicate filenames")
	flag.StringVar(&sizes, "sizes", "1920", "[convert] comma separated list of widths (e.g.: 3840,1920,800)")

	flag.StringVar(&rawDir, "raws", "", "[any] Raw directory")
	flag.StringVar(&collectionDir, "collection", "", "[any] Collection directory")
	flag.StringVar(&jpegDir, "jpegs", "", "[convert] JPEG directory")

	flag.IntVar(&maxWorkers, "workers", 100, "[all] maximum amount of threads")

	flag.StringVar(
		&baseDir,
		"base",
		"",
		`[all] Set a basedir which implies:
-raws (if not given)       = <basedir>/Originals
-collection (if not given) = <basedir>/Collection
-jpegs (if not given)      = <basedir>/Converted
`,
	)

	flag.Var(&fsSources, "source", "[import] filesystem paths to import from, can be specified multiple times")

	flag.BoolVar(&alwaysYes, "y", false, "always answer yes")
	flag.BoolVar(&zero, "0", false, `all stdout output will be separated by a null byte
e.g.: photos -base . -0 -actions show-jpegs | xargs -0 feh`)
	flag.BoolVar(&noRawPrefix, "no-raw", false, "[show-*] don't prefix output with the corresponding raw file")

	flag.Parse()

	stdout := func(str string) {
		fmt.Println(str)
	}
	if zero {
		stdout = func(str string) {
			fmt.Print(string(append([]byte(str), 0)))
		}
	}

	if baseDir != "" {
		if rawDir == "" {
			rawDir = filepath.Join(baseDir, "Originals")
		}
		if collectionDir == "" {
			collectionDir = filepath.Join(baseDir, "Collection")
		}
		if jpegDir == "" {
			jpegDir = filepath.Join(baseDir, "Converted")
		}
	}

	if rawDir == "" {
		exit(errors.New("please provide a raw directory"))
	}
	if collectionDir == "" {
		exit(errors.New("please provide a collection directory"))
	}

	_filter := func(meta meta.Meta, f *importer.File) bool {
		return false
	}

	l := log.New(os.Stderr, "", log.LstdFlags)
	imp := importer.New(l, rawDir, collectionDir, jpegDir)

	switch itemFilter {
	case "normal":
		_filter = func(meta meta.Meta, f *importer.File) bool {
			return !meta.Deleted
		}

	case "all":
		_filter = func(meta meta.Meta, f *importer.File) bool { return true }

	case "deleted":
		_filter = func(meta meta.Meta, f *importer.File) bool {
			return meta.Deleted
		}
	case "unrated":
		_filter = func(meta meta.Meta, f *importer.File) bool {
			return !meta.Deleted && (meta.Rating < 1 || meta.Rating > 5)
		}
	case "unedited":
		_filter = func(meta meta.Meta, f *importer.File) bool {
			b, err := imp.Unedited(f)
			exit(err)
			return b && !meta.Deleted
		}
	}

	var filter func(f *importer.File) bool

	all := func(it func(f *importer.File) (bool, error)) {
		exit(
			imp.All(func(f *importer.File) (bool, error) {
				if !filter(f) {
					return true, nil
				}
				return it(f)
			}),
		)
	}

	allCounted := func(it func(f *importer.File, n, total int) (bool, error)) {
		exit(
			imp.AllCounted(func(f *importer.File, n, total int) (bool, error) {
				if !filter(f) {
					return true, nil
				}
				return it(f, n, total)
			}),
		)
	}

	allList := func() []*importer.File {
		l := make([]*importer.File, 0, 100)
		exit(
			imp.All(func(f *importer.File) (bool, error) {
				if !filter(f) {
					return true, nil
				}
				l = append(l, f)
				return true, nil
			}),
		)
		return l
	}

	work := func(workers int, do func(*importer.File) error) {
		if workers < 1 {
			workers = runtime.NumCPU()
		}
		if workers > maxWorkers {
			workers = maxWorkers
		}
		work := make(chan *importer.File, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				for f := range work {
					exit(do(f))
				}
				wg.Done()
			}()
		}

		output := false
		allCounted(func(f *importer.File, n, total int) (bool, error) {
			work <- f
			output = true
			fmt.Fprintf(os.Stderr, "\033[K\r%4d/%-4d", n+1, total)
			return true, nil
		})

		close(work)
		wg.Wait()
		if output {
			fmt.Fprintln(os.Stderr)
		}

	}

	workNoProgress := func(workers int, do func(*importer.File) error) {
		if workers < 1 {
			workers = runtime.NumCPU()
		}
		if workers > maxWorkers {
			workers = maxWorkers
		}
		work := make(chan *importer.File, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				for f := range work {
					exit(do(f))
				}
				wg.Done()
			}()
		}

		all(func(f *importer.File) (bool, error) {
			work <- f
			return true, nil
		})

		close(work)
		wg.Wait()
	}

	cmds := map[string]func(){
		"import": func() {
			l.Println("importing")
			for _, path := range fsSources {
				importer.Register(
					"filesystem:"+path,
					fs.New(path, true, imp.SupportedExtList()),
				)
			}

			exit(imp.Import(checksum))

		},
		"show": func() {
			all(func(f *importer.File) (bool, error) {
				stdout(f.Path())
				return true, nil
			})
		},
		"show-jpegs": func() {
			all(func(f *importer.File) (bool, error) {
				m, err := importer.GetMeta(f)
				if err != nil {
					return false, err
				}
				for jpg := range m.Converted {
					p := filepath.Join(jpegDir, jpg)
					if noRawPrefix {
						stdout(p)
						continue
					}
					stdout(fmt.Sprintf("%s: %s", f.Path(), p))
				}
				return true, nil
			})
		},
		"show-links": func() {
			all(func(f *importer.File) (bool, error) {
				links, err := imp.FindLinks(f)
				if err != nil {
					return false, err
				}
				for _, l := range links {
					if noRawPrefix {
						stdout(l)
						continue
					}
					stdout(fmt.Sprintf("%s: %s", f.Path(), l))
				}
				return true, nil
			})
		},
		"update-meta": func() {
			l.Println("updating meta")
			work(100, func(f *importer.File) error {
				_, err := importer.MakeMeta(f)
				return err
			})
		},
		"link": func() {
			l.Println("linking")
			exit(imp.Link())
		},
		"previews": func() {
			l.Println("creating previews")
			work(-1, func(f *importer.File) error {
				err := importer.EnsurePreview(f)
				if err == importer.ErrPreviewNotPossible {
					l.Println("WARN", f.Filename(), err)
					return nil
				}

				return err
			})
		},
		"rate": func() {
			exit(rate.Run(l, allList()))
		},
		"sync-meta": func() {
			l.Println("syncing meta")
			work(-1, func(f *importer.File) error {
				return imp.SyncMetaAndPP3(f)
			})
		},
		"convert": func() {
			sl := strings.Split(sizes, ",")
			rs := make([]int, 0, len(sl))
			for _, s := range sl {
				i, err := strconv.Atoi(strings.TrimSpace(s))
				exit(err)
				rs = append(rs, i)
			}

			allCounted(func(f *importer.File, n, total int) (bool, error) {
				fmt.Fprintf(os.Stderr, "\033[K\rConverting %4d/%-4d", n+1, total)
				return true, imp.Convert(f, rs, edited)
			})
			fmt.Fprintln(os.Stderr)
		},
		"cleanup": func() {
			list, err := imp.Cleanup(ratingGTFilter)
			exit(err)
			if len(list) == 0 {
				return
			}

			for _, p := range list {
				stdout(p)
			}
			answer := "y"
			if !alwaysYes {
				fmt.Print("Delete all? [y/N]: ")
				answer = ask()
			}
			if answer == "y" || answer == "Y" {
				for _, p := range list {
					exit(os.Remove(p))
				}
				fmt.Println("done")
			}
		},
		"info": func() {
			files := flag.Args()
			var err error
			for i := range files {
				files[i], err = importer.Abs(files[i])
				exit(err)
			}

			var sem sync.Mutex
			type m struct {
				m     meta.Meta
				links []string
			}
			fmap := make(map[string]m)

			filter = func(f *importer.File) bool {
				return true
			}

			workNoProgress(100, func(f *importer.File) error {
				fp, err := importer.Abs(f.Path())
				if err != nil {
					return err
				}

				met, err := importer.GetMeta(f)
				if err != nil {
					return err
				}

				links, err := imp.FindLinks(f)
				if err != nil {
					return err
				}
				for i := range links {
					links[i], err = filepath.Abs(links[i])
					if err != nil {
						return err
					}
				}

				m := m{
					met,
					links,
				}
				sem.Lock()
				fmap[fp] = m
				sem.Unlock()
				return nil
			})

			for _, f := range files {
				info, ok := fmap[f]
				if !ok {
					l.Printf("%s does not exist", f)
					continue
				}

				links := make([]string, len(info.links))
				for i := range info.links {
					links[i] = fmt.Sprintf("Link[]: %s", info.links[i])
				}

				l := strings.Join(links, "\n")
				if l == "" {
					l = "Link[]:"
				}

				converted := make([]string, 0, len(info.m.Converted))
				for i := range info.m.Converted {
					p, err := filepath.Abs(filepath.Join(jpegDir, i))
					exit(err)
					converted = append(converted, fmt.Sprintf("Converted[]: %s", p))
				}
				c := strings.Join(converted, "\n")
				if c == "" {
					c = "Converted[]:"
				}

				stdout(
					fmt.Sprintf(`RAW: %s
Size: %d
Deleted: %t
Rank: %d
Date: %s
%s
%s
`,
						f,
						info.m.Size,
						info.m.Deleted,
						info.m.Rating,
						info.m.CreatedTime().Format(time.RFC3339),
						l,
						c,
					),
				)
			}
		},
		"exec": func() {
			args := flag.Args()
			if len(args) == 0 {
				exit(errors.New("no exec command given"))
			}
			bin, err := exec.LookPath(args[0])
			exit(err)

			type w struct {
				out, err *bytes.Buffer
				e        error
			}
			results := make(chan w, 50)
			done := make(chan struct{})
			go func() {
				for d := range results {
					_, err := io.Copy(os.Stdout, d.out)
					exit(err)
					_, err = io.Copy(os.Stderr, d.err)
					exit(err)
					if d.e != nil {
						exit(d.e)
					}
				}
				done <- struct{}{}
			}()

			workNoProgress(100, func(f *importer.File) error {
				l := make([]string, len(args)-1)
				for i := 1; i < len(args); i++ {
					l[i-1] = strings.ReplaceAll(args[i], "{}", f.Path())
				}
				cmd := exec.Command(bin, l...)
				w := w{bytes.NewBuffer(nil), bytes.NewBuffer(nil), nil}
				cmd.Stdout = w.out
				cmd.Stderr = w.err
				w.e = cmd.Run()
				results <- w
				return nil
			})

			close(results)
			<-done
		},
	}

	act := strings.Split(actions, ",")
	for _, action := range act {
		filter = func(f *importer.File) bool {
			meta, err := importer.GetMeta(f)
			exit(err)
			if meta.Rating <= ratingGTFilter {
				return false
			}
			if meta.Rating >= ratingLTFilter {
				return false
			}

			return _filter(meta, f)
		}

		action = strings.TrimSpace(action)
		if f, ok := cmds[action]; ok {
			f()
			continue
		}

		exit(fmt.Errorf("action '%s' does not exist", action))
	}
}
