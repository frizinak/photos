package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/frizinak/phodo/phodo"
	"github.com/frizinak/photos/cmd/cli"
	"github.com/frizinak/photos/cmd/flags"
	"github.com/frizinak/photos/gphotos"
	"github.com/frizinak/photos/gtimeline"
	"github.com/frizinak/photos/importer"
	"github.com/frizinak/photos/importer/fs"
	"github.com/frizinak/photos/importer/gphoto2"
	"github.com/frizinak/photos/importer/libgphoto2"
	"github.com/frizinak/photos/meta"
	"github.com/frizinak/photos/rate"
)

type FileMeta struct {
	d time.Time
	m *meta.Meta
	f *importer.File
}

type FileMetas []*FileMeta

func (f FileMetas) Len() int      { return len(f) }
func (f FileMetas) Swap(i, j int) { f[i], f[j] = f[j], f[i] }
func (f FileMetas) Less(i, j int) bool {
	if f[i].d == f[j].d {
		if f[i].f.BaseFilename() < f[j].f.BaseFilename() {
			return true
		}
	}

	return f[i].d.Before(f[j].d)
}

func combine(files importer.Files) (FileMetas, error) {
	m := make(FileMetas, len(files))
	for i, f := range files {
		met, err := importer.GetMeta(f)
		if err != nil {
			return nil, err
		}

		m[i] = &FileMeta{met.CreatedTime(), &met, f}
	}

	return m, nil
}

func ask() string {
	sc := bufio.NewScanner(os.Stdin)
	sc.Split(bufio.ScanLines)
	sc.Scan()
	return sc.Text()
}

func main() {
	l := log.New(os.Stderr, "", log.LstdFlags)
	flag := cli.NewFlags()
	flag.Parse()
	imp := importer.New(
		l,
		flag.Log(),
		flag.PhodoConf,
		flag.RawDir(),
		flag.CollectionDir(),
		flag.JPEGDir(),
	)

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

	const pbarSize = 17
	const pbarChar = '#'
	progress := func(n, total int) {
		pbar := [pbarSize]rune{}
		var pct float32
		if total != 0 {
			pct = (float32(n) / float32(total))
		}
		x := int(pbarSize * pct)
		for i := 0; i < pbarSize; i++ {
			if i < x {
				pbar[i] = pbarChar
				continue
			}
			pbar[i] = 32
		}
		fmt.Fprintf(os.Stderr, "\033[K\r[%s] %4d/%-4d ", string(pbar[:]), n, total)
	}

	progress32 := func(n, total uint32) { progress(int(n), int(total)) }
	progressDone := func() {
		fmt.Fprintln(os.Stderr)
	}

	if flag.Verbose() {
		progress = func(n, total int) {}
		progressDone = func() {}
	}

	allCounted := func(it func(f *importer.File, n, total int) (bool, error)) {
		flag.Exit(
			imp.AllCounted(func(f *importer.File, n, total int) (bool, error) {
				if !filter(f) {
					f = nil
				}
				return it(f, n, total)
			}),
		)
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

	allMeta := func() FileMetas {
		all := allList()
		l, err := combine(all)
		flag.Exit(err)
		return l
	}

	type workCB func() error
	type workCheckCB func(*importer.File) (workCB, error)

	_work := func(counted bool) func(int, workCheckCB) {
		return func(workers int, do workCheckCB) {
			if workers < 1 {
				workers = runtime.NumCPU()
			}
			if workers > flag.MaxWorkers() {
				workers = flag.MaxWorkers()
			}
			work := make(chan *importer.File, workers)
			todo := make([]workCB, 0)
			var sm sync.Mutex
			var wg sync.WaitGroup
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					for f := range work {
						cb, err := do(f)
						flag.Exit(err)
						if cb != nil {
							sm.Lock()
							todo = append(todo, cb)
							sm.Unlock()
						}
					}
					wg.Done()
				}()
			}

			var tot int
			run := func() {
				allCounted(func(f *importer.File, n, total int) (bool, error) {
					if f != nil {
						work <- f
					}
					progress(n-len(todo), total)
					tot = total
					return true, nil
				})
			}
			if !counted {
				run = func() {
					all(func(f *importer.File) (bool, error) {
						work <- f
						return true, nil
					})
				}
			}

			run()
			close(work)
			wg.Wait()

			var wg2 sync.WaitGroup
			todos := make(chan workCB, workers)
			for i := 0; i < workers; i++ {
				wg2.Add(1)
				go func() {
					for f := range todos {
						flag.Exit(f())
					}
					wg2.Done()
				}()
			}

			for n, cb := range todo {
				todos <- cb
				if counted {
					progress(tot-len(todo)+n+1-workers, tot)
				}
			}

			close(todos)
			wg2.Wait()
			if counted {
				progress(tot, tot)
				progressDone()
			}
		}
	}

	editor := func(file string) error {
		c, err := flag.PhodoConf()
		if err != nil {
			return err
		}
		return phodo.Editor(context.Background(), c, file)
	}

	work := _work(true)
	workNoProgress := _work(false)

	cmds := map[string]func(){
		flags.ActionImport: func() {
			l.Println("importing")
			exts := make([]string, 0)
			exts = imp.RawExtList(exts)
			exts = imp.VideoExtList(exts)
			n := "raw"
			if flag.ImportJPEG() {
				n = "all"
				exts = imp.ImageExtList(exts)
			}

			fsi := false
			for _, path := range flag.SourceDirs() {
				fsi = true
				importer.Register(
					fmt.Sprintf("filesystem:%s:%s", n, path),
					fs.New(path, true, exts),
				)
			}

			if !fsi {
				var gp importer.Backend = libgphoto2.New(exts)
				n = "libgphoto2"
				if ok, _ := gp.Available(); !ok {
					gp = gphoto2.New(exts)
					n = "gphoto2"
				}
				importer.Register(n, gp)
			}

			flag.Exit(imp.Import(flag.Checksum(), progress))
			progressDone()
		},
		flags.ActionShow: func() {
			all(func(f *importer.File) (bool, error) {
				flag.Output(f.Path())
				return true, nil
			})
		},
		flags.ActionShowPreviews: func() {
			all(func(f *importer.File) (bool, error) {
				p := importer.PreviewFile(f)
				_, err := os.Stat(p)
				if err != nil {
					if os.IsNotExist(err) {
						return true, nil
					}
					return false, err
				}

				if flag.NoRawPrefix() {
					flag.Output(p)
					return true, nil
				}
				flag.Output(fmt.Sprintf("%s: %s", f.Path(), p))
				return true, nil
			})
		},
		flags.ActionShowJPEGs: func() {
			list := allMeta()
			sort.Sort(list)
			sizes := flag.Sizes()
			smap := make(map[int]struct{}, len(sizes))
			for _, s := range sizes {
				smap[s] = struct{}{}
			}
			for _, f := range list {
				for jpg, conv := range f.m.Conv {
					if len(sizes) != 0 {
						if _, ok := smap[conv.Size]; !ok {
							continue
						}
					}
					p := filepath.Join(flag.JPEGDir(), jpg)
					if flag.NoRawPrefix() {
						flag.Output(p)
						continue
					}
					flag.Output(fmt.Sprintf("%s: %s", f.f.Path(), p))
				}
			}
		},
		flags.ActionShowLinks: func() {
			list := allMeta()
			sort.Sort(list)
			for _, f := range list {
				links, err := imp.FindLinks(f.f)
				flag.Exit(err)
				for _, l := range links {
					if flag.NoRawPrefix() {
						flag.Output(l)
						continue
					}
					flag.Output(fmt.Sprintf("%s: %s", f.f.Path(), l))
				}
			}
		},
		flags.ActionShowTags: func() {
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
		flags.ActionTagsAdd: func() {
			t := flags.CommaSep(strings.Join(flag.Args(), ","))
			if len(t) == 0 {
				return
			}
			work(-1, func(f *importer.File) (workCB, error) {
				m, err := importer.GetMeta(f)
				if err != nil {
					return nil, err
				}

				c := true
				for _, t := range t {
					if !m.Tags.Contains(t) {
						c = false
						break
					}
				}

				if c {
					return nil, nil
				}

				return func() error {
					m.Tags = append(m.Tags, t...)
					return importer.SaveMeta(f, m)
				}, nil
			})
		},
		flags.ActionTagsRemove: func() {
			t := flags.CommaSep(strings.Join(flag.Args(), ","))
			if len(t) == 0 {
				return
			}

			mp := make(map[string]struct{}, 0)
			for _, tag := range t {
				mp[tag] = struct{}{}
			}

			work(-1, func(f *importer.File) (workCB, error) {
				m, err := importer.GetMeta(f)
				if err != nil {
					return nil, err
				}

				change := false
				tags := make(meta.Tags, 0, len(m.Tags))
				for _, tag := range m.Tags.Unique() {
					if _, ok := mp[tag]; !ok {
						change = true
						tags = append(tags, tag)
					}
				}
				if !change {
					return nil, nil
				}

				return func() error {
					m.Tags = tags
					return importer.SaveMeta(f, m)
				}, nil
			})
		},
		flags.ActionLink: func() {
			l.Println("linking")
			imp.ClearCache()
			work(-1, func(f *importer.File) (workCB, error) {
				return func() error { return imp.Link(f) }, nil
			})
			imp.ClearCache()
		},
		flags.ActionPreviews: func() {
			l.Println("creating previews")
			work(-1, func(f *importer.File) (workCB, error) {
				ex, can := imp.HasPreview(f)
				if ex {
					return nil, nil
				}

				if !can {
					l.Println("WARN", f.Filename(), importer.ErrPreviewNotPossible)
					return nil, nil
				}

				return func() error { return imp.EnsurePreview(f) }, nil
			})
		},
		flags.ActionRate: func() {
			_list := allMeta()
			sort.Sort(_list)
			list := make(importer.Files, len(_list))
			for i := range _list {
				list[i] = _list[i].f
			}

			if len(list) == 0 {
				l.Println("no files to rate with given filters")
				return
			}

			rater, err := rate.New(l, list, imp, editor)
			flag.Exit(err)
			flag.Exit(rater.Run())
		},
		flags.ActionEdit: func() {
			list := allMeta()
			sort.Sort(list)
			for _, f := range list {
				links, err := imp.FindLinks(f.f)
				flag.Exit(err)
				for _, l := range links {
					flag.Exit(editor(l))
				}
			}
		},
		flags.ActionSyncMeta: func() {
			l.Println("syncing meta")
			work(-1, func(f *importer.File) (workCB, error) {
				return func() error { return imp.SyncMetaAndPP3(f) }, nil
			})
		},
		flags.ActionRewriteMeta: func() {
			l.Println("rewriting meta")
			work(-1, func(f *importer.File) (workCB, error) {
				return func() error {
					_, err := importer.MakeMeta(f)
					return err
				}, nil
			})
		},
		flags.ActionJPEGFixup: func() {
			work(1, func(f *importer.File) (workCB, error) {
				m, err := importer.GetMeta(f)
				if err != nil {
					return nil, err
				}

				return func() error {
					for p := range m.Conv {
						p = filepath.Join(flag.JPEGDir(), p)
						if err := imp.JPEGTZ(p, m.CreatedTime()); err != nil {
							return err
						}
					}
					return nil
				}, nil
			})
		},
		flags.ActionConvert: func() {
			sizes := flag.Sizes()
			if len(sizes) == 0 {
				flag.Exit(errors.New("no sizes specified"))
			}
			l.Printf("converting (sizes: %v)", sizes)
			work(2, func(f *importer.File) (workCB, error) {
				conv, err := imp.CheckConvert(f, sizes)
				if err != nil || !conv {
					return nil, err
				}

				return func() error { return imp.Convert(f, sizes) }, nil
			})
		},
		flags.ActionCleanup: func() {
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
		flags.ActionInfo: func() {
			files := allMeta()

			for _, f := range files {
				_links, err := imp.FindLinks(f.f)
				flag.Exit(err)
				links := make([]string, len(_links))
				for i := range _links {
					l, err := filepath.Abs(_links[i])
					flag.Exit(err)
					links[i] = fmt.Sprintf("Link[]: %s", l)
				}

				l := strings.Join(links, "\n")
				if l == "" {
					l = "Link[]:"
				}

				converted := make([]string, 0, len(f.m.Conv))
				for i := range f.m.Conv {
					p, err := filepath.Abs(filepath.Join(flag.JPEGDir(), i))
					flag.Exit(err)
					converted = append(converted, fmt.Sprintf("Converted[]: %s", p))
				}

				tags := make([]string, 0, len(f.m.Tags))
				for _, t := range f.m.Tags {
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

				var ll, loc, addr string
				if f.m.Location != nil {
					ll = fmt.Sprintf("%f,%f", f.m.Location.Lat, f.m.Location.Lng)
					loc = f.m.Location.Name
					addr = f.m.Location.Address
				}

				var deviceStr, exposureStr string
				if c := f.m.CameraInfo; c != nil {
					deviceStr = c.DeviceString()
					exposureStr = c.ExposureString()
				}

				flag.Output(
					fmt.Sprintf(`RAW: %s
Size: %d
Deleted: %t
Rank: %d
Date: %s
LatLng: %s
Location: %s
Address: %s
Device: %s
Exposure: %s
%s
%s
%s
`,
						f.f.Filename(),
						f.m.Size,
						f.m.Deleted,
						f.m.Rating,
						f.m.CreatedTime().Format(time.RFC3339),
						ll,
						loc,
						addr,
						deviceStr,
						exposureStr,
						l,
						c,
						t,
					),
				)
			}
		},
		flags.ActionExec: func() {
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

			workNoProgress(-1, func(f *importer.File) (workCB, error) {
				return func() error {
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
				}, nil
			})

			close(results)
			<-done
		},
		flags.ActionGPhotos: func() {
			sizes := flag.Sizes()
			if len(sizes) == 0 {
				flag.Exit(errors.New("please specify all sizes that should be uploaded"))
			}
			smap := make(map[int]struct{}, len(sizes))
			for _, s := range sizes {
				smap[s] = struct{}{}
			}

			creds := flag.GPhotosCredentials()
			if creds == "" {
				flag.Exit(errors.New("no gphotos credentials file specified"))
			}

			l.Println("assembling files")
			var sem sync.Mutex
			list := make([]gphotos.UploadTask, 0)
			work(-1, func(f *importer.File) (workCB, error) {
				return func() error {
					m, err := importer.GetMeta(f)
					if err != nil {
						return err
					}
					for jpg, conv := range m.Conv {
						if _, ok := smap[conv.Size]; !ok {
							continue
						}
						p := filepath.Join(flag.JPEGDir(), jpg)
						tags := []string{}
						for _, t := range m.Tags.Unique() {
							tags = append(tags, "+"+t)
						}
						descr := fmt.Sprintf("sha512:%s\nRAW:%s\n%s",
							m.Checksum,
							f.Filename(),
							strings.Join(tags, " "),
						)
						sem.Lock()
						list = append(list, gphotos.NewFileUploadTask(p, descr))
						sem.Unlock()
					}
					return nil
				}, nil
			})

			if len(list) == 0 {
				l.Println("no files to upload")
				return
			}

			gp := gphotos.New(
				l,
				"530510971074-tdam4676hpg5u82vh8jb1mka23jb06hc.apps.googleusercontent.com",
				secbyobflol("9c24d41fa1de5c7d855223b5640447ee665797b196d11b8575c59e6a507e3fc70053b408ff9bb0a70718e8c54c89e46754167b28878878719fc2a00ea38ffa45ff3e587fcdca63b2f431ed94590581f366ed102d9afa7267a3d6c9628c075466766d65b837a4e6acf5fe432833c9ce05a9"),
				creds,
			)

			flag.Exit(gp.Authenticate(true))
			l.Println("uploading")
			flag.Exit(gp.BatchUpload(8, list, progress32))
			progressDone()
			l.Println("done")
		},
		flags.ActionGLocation: func() {
			l.Println("gathering date information of images")
			glocationDir := strings.TrimSpace(flag.GLocationDirectory())
			if glocationDir == "" {
				flag.Exit(fmt.Errorf("please provide a directory containing your downloaded location kmls"))
			}

			docs := gtimeline.New(glocationDir)
			var first, last time.Time
			work(-1, func(f *importer.File) (workCB, error) {
				m, err := importer.GetMeta(f)
				if err != nil {
					return nil, err
				}
				t := m.CreatedTime()
				if first == (time.Time{}) || t.Before(first) {
					first = t
				}
				if last == (time.Time{}) || t.After(last) {
					last = t
				}
				return nil, nil
			})

			if first == (time.Time{}) {
				l.Println("nothing to do")
				return
			}

			l.Println("fetching google timeline kmls")
			extraDays := 31
			first = first.Add(-time.Hour * 24 * time.Duration(extraDays))
			last = last.Add(time.Hour * 24 * time.Duration(extraDays))
			_, err := docs.GetDayRange(first, last, 8, progress)
			progressDone()
			if err != nil {
				err = fmt.Errorf("%s\nno locations updated", err)
			}
			flag.Exit(err)

			l.Println("updating meta with google timeline location information")
			work(-1, func(f *importer.File) (workCB, error) {
				m, err := importer.GetMeta(f)
				if err != nil {
					return nil, err
				}

				c := m.CreatedTime()
				p, ll, err := docs.GetLatLng(c, extraDays)
				if err != nil {
					if err != gtimeline.ErrNoLatLng && err != gtimeline.ErrNoPlaceMark {
						return nil, err
					}
					l.Printf("could not find location info for %s at %s", f.Path(), c.Local())
					return nil, nil
				}

				m.Location = &meta.Location{
					Lat:     ll.Lat,
					Lng:     ll.Lng,
					Name:    p.Name,
					Address: p.Address,
				}

				return func() error {
					return importer.SaveMeta(f, m)
				}, nil
			})

			l.Println("updating converted jpegs with new location information")
			work(-1, func(f *importer.File) (workCB, error) {
				m, err := importer.GetMeta(f)
				if err != nil {
					return nil, err
				}

				c := m.CreatedTime()
				_, ll, err := docs.GetLatLng(c, extraDays)
				if err != nil {
					return func() error { return nil }, nil
				}

				return func() error {
					for p := range m.Conv {
						p = filepath.Join(flag.JPEGDir(), p)
						err := imp.JPEGGPS(p, c, ll.Lat, ll.Lng)
						if err != nil {
							return err
						}
					}
					return nil
				}, nil

			})
		},
	}

	for i := range flags.AllActions {
		if _, ok := cmds[i]; !ok {
			flag.Exit(fmt.Errorf("[FATAL] unimplemented action %s", i))
		}
	}

	fil := flag.Filter(imp)
	mfil := flag.MetaFilter(imp)
	fcache := map[string]bool{}
	filter = func(f *importer.File) bool {
		p := f.Path()
		if v, ok := fcache[p]; ok {
			return v
		}

		if !fil(f) {
			fcache[p] = false
			return false
		}

		meta, err := importer.GetMeta(f)
		flag.Exit(err)
		fcache[p] = mfil(meta, f)

		return fcache[p]
	}

	for _, action := range flag.Actions() {
		cmds[action]()
	}
}

func secbyobflol(in string) string {
	b := base64.StdEncoding
	r, _ := hex.DecodeString(in)
	k, t, i, p := r[0:32], int(r[32]), r[33:33+aes.BlockSize], r[33+aes.BlockSize:]
	bl, _ := aes.NewCipher(k)
	d := make([]byte, len(p))
	cipher.NewCBCDecrypter(bl, i).CryptBlocks(d, p)
	d = d[:len(d)-t]
	a := make([]byte, b.DecodedLen(len(d)))
	t, _ = b.Decode(a, d)
	h := make([]byte, hex.DecodedLen(t))
	hex.Decode(h, a[:t])
	return string(h)
}
