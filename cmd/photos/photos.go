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
	"sort"
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

func commaSep(v string) []string {
	s := strings.Split(v, ",")
	c := make([]string, 0, len(s))
	for _, t := range s {
		t = strings.TrimSpace(t)
		if t != "" {
			c = append(c, t)
		}
	}
	return c
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
	var tags flagStrs

	flag.StringVar(
		&actions,
		"actions",
		"",
		`comma separated list of actions:
- import         Import media from connected camera (gphoto2) and any given directory (-source) to the directory specified with -raws
- show           Show raws (filter with -filter)
- show-jpegs     Show jpegs (filter with -filter) (see -no-raw)
- show-links     Show links (filter with -filter) (see -no-raw)
- show-tags      Show all tags
- info           Show info for given RAWs
- link           Create collection symlinks in the given directory (-collection)
- previews       Generate simple jpeg previews (used by -actions rate)
- rate           Simple opengl window to rate / trash images (filter with -filter)
- sync-meta      Sync .meta file with .pp3 (file mtime determines which one is the authority) and filesystem
- convert        Convert images to jpegs to the given directory (-jpegs) and sizes (-sizes) (filter with -filter and -edited)
- exec           Run an external command for each file (first non flag and any further arguments, {} is replaced with the filepath)
                 e.g.: photos -base . -actions exec -filter all wc -c {}
- cleanup        Remove pp3s and jpegs for deleted RAWs
                 -filter and -lt are ignored
				 Images whose rating is not higher than -gt will also have their jpegs deleted.
				 !Note: .meta files are seen as the single source of truth, so run sync-meta before.

- remove-tags    Remove tags (first non flag argument are the tags that will be removed)
- add-tags       Add tag (first non flag argument are the tags that will be removed)
`)

	flag.StringVar(&itemFilter, "filter", "normal", "[any] filter (normal / all / deleted / unrated / unedited)")
	flag.IntVar(&ratingGTFilter, "gt", -1, "[any] additional greater than given rating filter")
	flag.IntVar(&ratingLTFilter, "lt", 6, "[any] additional less than given rating filter")
	flag.Var(&tags, "tags", `[any] additional tag filter, comma separated <or> can be specified multiple times <and>
e.g:
photo must be tagged: (outside || sunny) && dog
-tags 'outside,sunny' -tags 'dog'

special case: '-' only matches files with no tags
special case: '*' only matches files with tags
`)
	flag.BoolVar(&edited, "edited", false, "[convert] only convert images that have been edited with rawtherapee")
	flag.BoolVar(&checksum, "sum", false, "[import] dry-run and report non-identical files with duplicate filenames")
	flag.StringVar(&sizes, "sizes", "1920", "[convert] comma separated list of sizes (longest image dimension will be scaled to this size) (e.g.: 3840,1920,800)")

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
e.g.: photos -base . -0 -actions show-jpegs -no-raw | xargs -0 feh`)
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
				fmt.Fprintf(os.Stderr, "\033[K\r%4d/%-4d ", n+1, total)
				if !filter(f) {
					return true, nil
				}
				return it(f, n, total)
			}),
		)
		fmt.Fprintln(os.Stderr)
	}

	allList := func() []*importer.File {
		l := make(importer.Files, 0, 100)
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

		allCounted(func(f *importer.File, n, total int) (bool, error) {
			work <- f
			return true, nil
		})

		close(work)
		wg.Wait()
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
		"show-tags": func() {
			tags := make(meta.Tags, 0)
			all(func(f *importer.File) (bool, error) {
				m, err := importer.GetMeta(f)
				if err != nil {
					return false, err
				}
				tags = append(tags, m.Tags...)
				return true, nil
			})
			for _, t := range tags.Unique() {
				stdout(t)
			}
		},
		"add-tags": func() {
			t := commaSep(strings.Join(flag.Args(), ","))
			if len(t) == 0 {
				return
			}
			work(100, func(f *importer.File) error {
				m, err := importer.GetMeta(f)
				if err != nil {
					return err
				}

				m.Tags = append(m.Tags, t...)
				return importer.SaveMeta(f, m)
			})
		},
		"remove-tags": func() {
			t := commaSep(strings.Join(flag.Args(), ","))
			if len(t) == 0 {
				return
			}

			mp := make(map[string]struct{}, 0)
			for _, tag := range t {
				mp[tag] = struct{}{}
			}

			work(100, func(f *importer.File) error {
				m, err := importer.GetMeta(f)
				if err != nil {
					return err
				}
				tags := make(meta.Tags, 0, len(m.Tags))
				for _, tag := range m.Tags.Unique() {
					if _, ok := mp[tag]; !ok {
						tags = append(tags, tag)
					}
				}

				m.Tags = tags
				return importer.SaveMeta(f, m)
			})
		},
		"link": func() {
			l.Println("linking")
			exit(imp.Link())
		},
		"previews": func() {
			l.Println("creating previews")
			work(2, func(f *importer.File) error {
				err := imp.EnsurePreview(f)
				if err == importer.ErrPreviewNotPossible {
					l.Println("WARN", f.Filename(), err)
					return nil
				}

				return err
			})
		},
		"rate": func() {
			flist := allList()
			list := make(importer.Files, 0, len(flist))
			for _, f := range flist {
				if !imp.IsImage(f.Filename()) {
					continue
				}
				list = append(list, f)
			}
			sort.Sort(list)

			if len(list) == 0 {
				l.Println("no files to rate with given filters")
				return
			}

			exit(rate.New(l, list, imp).Run())
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

			work(2, func(f *importer.File) error {
				return imp.Convert(f, rs, edited)
			})
		},
		"cleanup": func() {
			list, err := imp.Cleanup(ratingGTFilter)
			exit(err)

			for _, p := range list {
				stdout(p)
			}
			answer := "y"
			if len(list) != 0 && !alwaysYes {
				fmt.Print("Delete all? [y/N]: ")
				answer = ask()
			}
			if answer != "y" && answer != "Y" {
				list = []string{}
			}
			exit(imp.DoCleanup(list))
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

				tags := make([]string, 0, len(info.m.Tags))
				for _, t := range info.m.Tags {
					tags = append(tags, fmt.Sprintf("Tags[]: %s", t))
				}
				t := strings.Join(tags, "\n")
				if t == "" {
					t = "Tags[]:"
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
%s
`,
						f,
						info.m.Size,
						info.m.Deleted,
						info.m.Rating,
						info.m.CreatedTime().Format(time.RFC3339),
						l,
						c,
						t,
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

	if actions == "" {
		exit(errors.New("no actions provided"))
	}

	tagslist := make([]map[string]struct{}, 0, len(tags))
	for _, t := range tags {
		_ors := strings.Split(t, ",")
		ors := make(map[string]struct{}, len(_ors))
		for _, ot := range _ors {
			ot = strings.TrimSpace(ot)
			if ot == "" {
				continue
			}
			ors[ot] = struct{}{}
		}
		if len(ors) != 0 {
			tagslist = append(tagslist, ors)
		}
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

			for _, and := range tagslist {
				match := false
				if _, ok := and["-"]; ok {
					match = len(meta.Tags) == 0
				}
				if _, ok := and["*"]; ok {
					if len(meta.Tags) != 0 {
						match = true
					}
				}

				for _, t := range meta.Tags {
					if _, ok := and[t]; ok {
						match = true
						break
					}
				}
				if !match {
					return false
				}
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
