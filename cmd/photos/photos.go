package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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
	flag := NewFlags()
	flag.Parse()
	l := log.New(os.Stderr, "", log.LstdFlags)
	imp := importer.New(l, flag.RawDir(), flag.CollectionDir(), flag.JPEGDir())

	var filter func(f *importer.File) bool
	all := func(it func(f *importer.File) (bool, error)) {
		flag.Exit(
			imp.All(func(f *importer.File) (bool, error) {
				if !filter(f) {
					return true, nil
				}
				return it(f)
			}),
		)
	}

	allCounted := func(it func(f *importer.File, n, total int) (bool, error)) {
		flag.Exit(
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
		flag.Exit(
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

	_work := func(counted bool) func(int, func(*importer.File) error) {
		return func(workers int, do func(*importer.File) error) {
			if workers < 1 {
				workers = runtime.NumCPU()
			}
			if workers > flag.MaxWorkers() {
				workers = flag.MaxWorkers()
			}
			work := make(chan *importer.File, workers)
			var wg sync.WaitGroup
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					for f := range work {
						flag.Exit(do(f))
					}
					wg.Done()
				}()
			}

			if counted {
				allCounted(func(f *importer.File, n, total int) (bool, error) {
					work <- f
					return true, nil
				})
				close(work)
				wg.Wait()
				return
			}

			all(func(f *importer.File) (bool, error) {
				work <- f
				return true, nil
			})
			close(work)
			wg.Wait()

		}
	}

	work := _work(true)
	workNoProgress := _work(false)

	cmds := map[string]func(){
		ActionImport: func() {
			l.Println("importing")
			for _, path := range flag.SourceDirs() {
				importer.Register(
					"filesystem:"+path,
					fs.New(path, true, imp.SupportedExtList()),
				)
			}

			flag.Exit(imp.Import(flag.Checksum()))

		},
		ActionShow: func() {
			all(func(f *importer.File) (bool, error) {
				flag.Output(f.Path())
				return true, nil
			})
		},
		ActionShowJPEGs: func() {
			all(func(f *importer.File) (bool, error) {
				m, err := importer.GetMeta(f)
				if err != nil {
					return false, err
				}
				for jpg := range m.Converted {
					p := filepath.Join(flag.JPEGDir(), jpg)
					if flag.NoRawPrefix() {
						flag.Output(p)
						continue
					}
					flag.Output(fmt.Sprintf("%s: %s", f.Path(), p))
				}
				return true, nil
			})
		},
		ActionShowLinks: func() {
			all(func(f *importer.File) (bool, error) {
				links, err := imp.FindLinks(f)
				if err != nil {
					return false, err
				}
				for _, l := range links {
					if flag.NoRawPrefix() {
						flag.Output(l)
						continue
					}
					flag.Output(fmt.Sprintf("%s: %s", f.Path(), l))
				}
				return true, nil
			})
		},
		ActionShowTags: func() {
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
				flag.Output(t)
			}
		},
		ActionTagsAdd: func() {
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
		ActionTagsRemove: func() {
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
		ActionLink: func() {
			l.Println("linking")
			flag.Exit(imp.Link())
		},
		ActionPreviews: func() {
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
		ActionRate: func() {
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

			flag.Exit(rate.New(l, list, imp).Run())
		},
		ActionSyncMeta: func() {
			l.Println("syncing meta")
			work(-1, func(f *importer.File) error {
				return imp.SyncMetaAndPP3(f)
			})
		},
		ActionConvert: func() {
			sizes := flag.Sizes()
			if len(sizes) == 0 {
				flag.Exit(errors.New("no sizes specified"))
			}
			work(2, func(f *importer.File) error {
				return imp.Convert(f, sizes)
			})
		},
		ActionCleanup: func() {
			list, err := imp.Cleanup(flag.RatingGT())
			flag.Exit(err)

			for _, p := range list {
				flag.Output(p)
			}
			answer := "y"
			if len(list) != 0 && !flag.Yes() {
				fmt.Print("Delete all? [y/N]: ")
				answer = ask()
			}
			if answer != "y" && answer != "Y" {
				list = []string{}
			}
			flag.Exit(imp.DoCleanup(list))
		},
		ActionInfo: func() {
			files := flag.Args()
			var err error
			for i := range files {
				files[i], err = importer.Abs(files[i])
				flag.Exit(err)
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
					p, err := filepath.Abs(filepath.Join(flag.JPEGDir(), i))
					flag.Exit(err)
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

				flag.Output(
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
		ActionExec: func() {
			args := flag.Args()
			if len(args) == 0 {
				flag.Exit(errors.New("no exec command given"))
			}
			bin, err := exec.LookPath(args[0])
			flag.Exit(err)

			type w struct {
				out, err *bytes.Buffer
				e        error
			}
			results := make(chan w, 50)
			done := make(chan struct{})
			go func() {
				for d := range results {
					_, err := io.Copy(os.Stdout, d.out)
					flag.Exit(err)
					_, err = io.Copy(os.Stderr, d.err)
					flag.Exit(err)
					if d.e != nil {
						flag.Exit(d.e)
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

	for i := range AllActions {
		if _, ok := cmds[i]; !ok {
			flag.Exit(fmt.Errorf("[FATAL] unimplemented action %s", i))
		}
	}

	for _, action := range flag.Actions() {
		filter = func(f *importer.File) bool {
			meta, err := importer.GetMeta(f)
			flag.Exit(err)
			return flag.Filter(imp)(meta, f)
		}

		cmds[action]()
	}
}
