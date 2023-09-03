package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/frizinak/phodo/phodo"
	"github.com/frizinak/phodo/pipeline"
	"github.com/frizinak/photos/cmd/flags"
	"github.com/frizinak/photos/importer"
	"github.com/frizinak/photos/meta"
)

type flagStrs []string

func (i *flagStrs) String() string {
	return "my string representation"
}

func (i *flagStrs) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var timeFormats = []string{
	"2006-01-02 15:04",
	"2006-01-02",
}

func parseTime(str string, eod bool) (*time.Time, error) {
	if str == "" {
		return nil, nil
	}

	var t time.Time
	var err error
	for i, f := range timeFormats {
		t, err = time.ParseInLocation(f, str, time.Local)
		if err == nil {
			if i != 0 && eod {
				y, m, d := t.Date()
				t = time.Date(y, m, d+1, 0, 0, 0, 0, time.Local)
			}
			return &t, nil
		}
	}

	return nil, err
}

type MetaFilter func(meta.Meta, *importer.File) bool
type Filter func(*importer.File) bool

type MetaFilterWeight struct {
	MetaFilter
	Weight int
}

type FilterWeight struct {
	Filter
	Weight int
}

type fws []FilterWeight

func (f fws) Len() int           { return len(f) }
func (f fws) Less(i, j int) bool { return f[i].Weight < f[j].Weight }
func (f fws) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }

type mfws []MetaFilterWeight

func (f mfws) Len() int           { return len(f) }
func (f mfws) Less(i, j int) bool { return f[i].Weight < f[j].Weight }
func (f mfws) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }

type Help struct {
	help string
	list map[string][]string
}

func (h Help) String() string {
	if len(h.list) == 0 {
		return h.help
	}

	ls := make([]string, 0, len(h.list))
	for i := range h.list {
		str := fmt.Sprintf("- %-15s %s", i, h.list[i][0])
		for _, l := range h.list[i][1:] {
			str += fmt.Sprintf("\n  %-15s %s", "", l)
		}
		ls = append(ls, str)
	}

	sort.Strings(ls)
	return h.help + "\n" + strings.Join(ls, "\n")
}

type Lists map[string]Help

func (l Lists) Help(cmd string) string { return l[cmd].String() + "\n" }

var lists = Lists{
	flags.Actions: {
		help: "list of actions (comma separated and/or specified multiple times)",
		list: map[string][]string{
			flags.ActionImport: {
				"Import media from connected camera (gphoto2) and any given directory (-source) to the directory specified with -raws",
			},
			flags.ActionShow: {
				"Show raws",
			},
			flags.ActionShowPreviews: {
				"Show previews",
			},
			flags.ActionShowJPEGs: {
				"Show jpegs",
			},
			flags.ActionShowLinks: {
				"Show links",
			},
			flags.ActionShowTags: {
				"Show all tags",
			},
			flags.ActionInfo: {
				"Show info",
			},
			flags.ActionLink: {
				"Create collection symlinks in the given directory (-collection)",
			},
			flags.ActionPreviews: {
				"Generate simple jpeg previews (used by -action rate)",
			},
			flags.ActionRate: {
				"Simple opengl window to rate / trash images",
			},
			flags.ActionEdit: {
				"Run the phodo editor for each image.",
			},
			flags.ActionSyncMeta: {
				"Sync .meta file with .pp3 (file mtime determines which one is the authority) and filesystem",
			},
			flags.ActionRewriteMeta: {
				"Rewrite .meta, make sure you synced first so newer pp3s are not overwritten.",
			},
			flags.ActionConvert: {
				"Convert images to jpegs resized with -sizes",
				"These conversions are tracked in .meta i.e.:",
				"running",
				"photos ... -action convert -sizes 3840,1920 and later",
				"photos ... -action convert -sizes 1920 will result in only the 1920 image being tracked",
				"an -action cleanup will result in the deletion of all 3840 images",
			},
			flags.ActionExec: {
				"Run an external command for each file (first non flag and any further arguments, {} is replaced with the filepath)",
				"e.g.: photos -base . -action exec wc -c {}",
			},
			flags.ActionCleanup: {
				"Remove pp3s and jpegs for deleted RAWs",
				"all filters and -lt are ignored",
				"Images whose rating is not higher than -gt will also have their jpegs deleted.",
				"!Note: .meta files are seen as the single source of truth, so run sync-meta before",
			},
			flags.ActionTagsRemove: {
				"Remove tags (first non flag argument are the tags that will be removed)",
			},
			flags.ActionTagsAdd: {
				"Add tag (first non flag argument are the tags that will be removed)",
			},
			flags.ActionGPhotos: {
				"Upload converted photos to google photos",
			},
			flags.ActionGLocation: {
				"Update meta with location information extracted from google timeline kmls",
				"requires -glocation flag with a directory where you downloaded history-YYYY-MM-DD.kml files",
			},
		},
	},
	flags.Undeleted: {
		help: "[any] ignore trashed/deleted files",
	},
	flags.Deleted: {
		help: "[any] only include trashed/deleted files",
	},
	flags.Updated: {
		help: "[any] only include files that need to be converted (updated pp3). be sure to pass the correct -sizes",
	},
	flags.Unedited: {
		help: "[any] only include files with incomplete pp3s (never opened in rawtherapee)",
	},
	flags.Edited: {
		help: "[any] only include files with complete pp3s (have been opened in rawtherapee)",
	},
	flags.Rated: {
		help: "[any] only include rated files",
	},
	flags.Unrated: {
		help: "[any] only include unrated files",
	},
	flags.Location: {
		help: "[any] only include files with a location",
	},
	flags.NoLocation: {
		help: "[any] only include files with no location",
	},
	flags.Photo: {
		help: "[any] only include photos",
	},
	flags.Video: {
		help: "[any] only include videos",
	},
	flags.GT: {
		help: "[any] only files with a rating greater than the one specified",
	},
	flags.LT: {
		help: "[any] only files with a rating less than the one specified",
	},
	flags.Camera: {
		help: "[any] filter camera make and model (* as wildcard, case insensitive)",
	},
	flags.Lens: {
		help: "[any] filter lens make and model (* as wildcard, case insensitive)",
	},
	flags.Exposure: {
		help: "[any] filter exposure settings",
		list: map[string][]string{
			"example": []string{
				"+f/2.0,-f/4.0,+1/5s,iso6400,+32mm:",
				"  aperture between 2.0 and 4.0",
				"  shutter speed faster than 1/5s",
				"  iso exactly 6400",
				"  focal length larger than 32mm",
			},
		},
	},
	flags.File: {
		help: "[any] filter original filename (* as wildcard, case insensitive)",
	},
	flags.Ext: {
		help: "[any] filter original file extension (case insensitive)",
	},
	flags.Since: {
		help: "[any] since time filter [Y-m-d (H:M)]",
	},
	flags.Until: {
		help: "[any] until time filter [Y-m-d (H:M)]",
	},
	flags.Tags: {
		help: `[any] tag filter, comma separated <or> can be specified multiple times <and>, ^ to negate a single tag
e.g:
photo must be tagged: (outside || sunny) && dog && !tree
-tags 'outside,sunny' -tags 'dog' -tags '^tree'

special case: '-' only matches files with no tags
special case: '*' only matches files with tags`,
	},

	flags.Checksum: {
		help: "[import] dry-run and report non-identical files with duplicate filenames",
	},
	flags.ImportJPEG: {
		help: "[import] also import jpegs",
	},
	flags.Sizes: {
		help: "comma separated and/or specified multiple times (e.g.: 3840,1920,800)",
		list: map[string][]string{
			"[convert]": {
				"longest image dimension will be scaled to this size ",
			},
			"[show-jpegs]": {"filter on jpeg sizes"},
			"[gphotos]":    {"filter on jpeg sizes"},
		},
	},
	flags.RawDir: {
		help: "[any] Raw directory",
	},
	flags.CollectionDir: {
		help: "[any] Collection directory",
	},
	flags.JPEGDir: {
		help: "[convert] JPEG directory",
	},
	flags.BaseDir: {
		help: `[all] Set a basedir which implies:
-raws (if not given)       = <basedir>/Originals
-collection (if not given) = <basedir>/Collection
-jpegs (if not given)      = <basedir>/Converted
-gphotos (if not given)    = <basedir>/gphotos.credentials`,
	},
	flags.GPhotosCredentials: {
		help: "[gphotos] path to the google credentials file",
	},
	flags.GLocationDirectory: {
		help: "[glocation] directory holding history-YYYY-MM-DD.kml files",
	},
	flags.MaxWorkers: {
		help: "[all] maximum amount of threads",
	},
	flags.SourceDir: {
		help: "[import] filesystem paths to import from, can be specified multiple times",
	},
	flags.AlwaysYes: {
		help: "always answer yes",
	},
	flags.Zero: {
		help: `all stdout output will be separated by a null byte
e.g.: photos -base . -0 -action show-jpegs -no-raw | xargs -0 feh`,
	},
	flags.NoRawPrefix: {
		help: "[show-*] don't prefix output with the corresponding raw file",
	},
	flags.Verbose: {
		help: "enable verbose stderr logging",
	},
	flags.Editor: {
		help: "command to run when editing images using phodo",
	},
}

type Flags struct {
	fs    *flag.FlagSet
	lists Lists

	actions  []string
	filters  map[string]bool
	ext      string
	file     []string
	camera   []string
	lens     []string
	exposure []string
	tags     []map[string]struct{}
	rating   struct {
		gt, lt int
	}

	time struct {
		since, until *time.Time
	}

	rawDir, collectionDir, jpegDir string

	sourceDirs []string

	checksum bool

	importJPEG bool

	alwaysYes bool

	verbose bool

	editor string

	sizes []int

	noRawPrefix bool
	zero        bool

	maxWorkers int

	gphotos   string
	glocation string

	phodoConf    *phodo.Conf
	phodoDefault string

	log    *log.Logger
	output func(string)

	filter     Filter
	metafilter MetaFilter

	filterFuncs  fws
	mfilterFuncs mfws
}

func (f *Flags) Actions() []string { return f.actions }

func NewFlags() *Flags {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	return &Flags{fs: fs, lists: lists}
}

func (f *Flags) Output(str string) {
	if f.output == nil {
		f.output = func(str string) { fmt.Println(str) }
		if f.zero {
			f.output = func(str string) {
				fmt.Print(string(append([]byte(str), 0)))
			}
		}
	}
	f.output(str)
}

func (f *Flags) MaxWorkers() int { return f.maxWorkers }

func (f *Flags) RawDir() string        { return f.rawDir }
func (f *Flags) CollectionDir() string { return f.collectionDir }
func (f *Flags) JPEGDir() string       { return f.jpegDir }

func (f *Flags) SourceDirs() []string { return f.sourceDirs }

func (f *Flags) Checksum() bool    { return f.checksum }
func (f *Flags) ImportJPEG() bool  { return f.importJPEG }
func (f *Flags) Yes() bool         { return f.alwaysYes }
func (f *Flags) NoRawPrefix() bool { return f.noRawPrefix }

func (f *Flags) Args() []string { return f.fs.Args() }

func (f *Flags) Sizes() []int { return f.sizes }

func (f *Flags) RatingGT() int { return f.rating.gt }
func (f *Flags) RatingLT() int { return f.rating.lt }

func (f *Flags) Verbose() bool { return f.verbose }

func (f *Flags) PhodoConf() (phodo.Conf, error) {
	if f.phodoConf != nil {
		return *f.phodoConf, nil
	}
	conf := phodo.NewConf(os.Stderr, nil)
	conf.EditorString = f.editor
	if f.Verbose() {
		conf.Verbose = int(pipeline.VerboseTime)
	}
	var err error
	conf, err = conf.Parse()
	if err != nil {
		return conf, err
	}

	c, err := ioutil.ReadFile(f.phodoDefault)
	if err != nil {
		return conf, err
	}

	conf.DefaultPipelines = func() string {
		return string(c)
	}

	f.phodoConf = &conf

	return conf, nil
}

func (f *Flags) GPhotosCredentials() string { return f.gphotos }
func (f *Flags) GLocationDirectory() string { return f.glocation }

func (f *Flags) Log() *log.Logger { return f.log }

func (f *Flags) makeFilters(imp *importer.Importer) {
	if f.mfilterFuncs != nil {
		return
	}
	mlist := make(mfws, 0, len(f.filters))
	list := make(fws, 0, len(f.filters))
	for filter, enabled := range f.filters {
		if !enabled {
			continue
		}
		var _mf MetaFilter
		var _f Filter
		var weight int
		switch filter {
		case flags.Undeleted:
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				return !meta.Deleted
			}
		case flags.Deleted:
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				return meta.Deleted
			}
		case flags.Rated:
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				return meta.Rating > 0
			}
		case flags.Unrated:
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				return meta.Rating == 0
			}
		case flags.Updated:
			sizes := f.Sizes()
			if len(sizes) == 0 {
				f.Exit(errors.New("no sizes specified"))
			}
			weight = 99
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				c, err := imp.CheckConvert(fl, sizes)
				f.Exit(err)
				return c
			}
		case flags.Edited:
			weight = 50
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				b, err := imp.Unedited(fl)
				f.Exit(err)
				return !b
			}
		case flags.Unedited:
			weight = 50
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				b, err := imp.Unedited(fl)
				f.Exit(err)
				return b
			}
		case flags.Location:
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				return meta.Location != nil
			}
		case flags.NoLocation:
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				return meta.Location == nil
			}
		case flags.Photo:
			_f = func(fl *importer.File) bool { return fl.TypeImage() || fl.TypeRAW() }
		case flags.Video:
			_f = func(fl *importer.File) bool { return fl.TypeVideo() }
		default:
			f.Exit(fmt.Errorf("unknown filter %s", filter))
		}

		if _f != nil {
			list = append(list, FilterWeight{_f, weight})
		}
		if _mf != nil {
			mlist = append(mlist, MetaFilterWeight{_mf, weight})
		}
	}

	f.mfilterFuncs = mlist
	f.filterFuncs = list

	sort.Stable(f.mfilterFuncs)
	sort.Stable(f.filterFuncs)
}

func (f *Flags) Filter(imp *importer.Importer) Filter {
	if f.filter != nil {
		return f.filter
	}
	f.makeFilters(imp)

	fn := func(fl *importer.File) bool {
		return filterString(fl.Filename(), f.file)
	}

	if len(f.file) == 0 {
		fn = func(fl *importer.File) bool { return true }
	}

	f.filter = func(fl *importer.File) bool {
		if f.ext != "" && f.ext != strings.ToLower(filepath.Ext(fl.BaseFilename())) {
			return false
		}

		if !fn(fl) {
			return false
		}

		for _, f := range f.filterFuncs {
			if !f.Filter(fl) {
				return false
			}
		}

		return true
	}

	return f.filter
}

func (f *Flags) MetaFilter(imp *importer.Importer) MetaFilter {
	if f.metafilter != nil {
		return f.metafilter
	}
	f.makeFilters(imp)
	f.metafilter = func(m meta.Meta, fl *importer.File) bool {
		r := int(m.Rating)
		if r <= f.rating.gt || r >= f.rating.lt {
			return false
		}
		if f.time.since != nil && f.time.since.After(m.CreatedTime()) {
			return false
		}
		if f.time.until != nil && f.time.until.Before(m.CreatedTime()) {
			return false
		}

		for _, f := range f.mfilterFuncs {
			if !f.MetaFilter(m, fl) {
				return false
			}
		}

		cnil := func() {
			f.log.Printf("%s has no camera info", fl.Filename())
		}

		if len(f.camera) != 0 {
			if m.CameraInfo == nil {
				cnil()
				return false
			}

			str := fmt.Sprintf("%s %s", m.CameraInfo.Make, m.CameraInfo.Model)
			if !filterString(str, f.camera) {
				return false
			}
		}

		if len(f.lens) != 0 {
			if m.CameraInfo == nil {
				cnil()
				return false
			}

			str := fmt.Sprintf("%s %s", m.CameraInfo.Lens.Make, m.CameraInfo.Lens.Model)
			if !filterString(str, f.lens) {
				return false
			}
		}

		if len(f.exposure) != 0 {
			if m.CameraInfo == nil {
				cnil()
				return false
			}

			for _, rule := range f.exposure {
				if len(rule) < 3 {
					f.Err(fmt.Errorf("invalid exposure rule: '%s'", rule))
				}

				rule = strings.ToLower(rule)
				comp := 0
				if rule[0] == '+' {
					rule = rule[1:]
					comp = 1
				} else if rule[0] == '-' {
					rule = rule[1:]
					comp = 2
				}

				switch {
				case strings.Contains(rule, "f/"):
					rule = strings.Replace(rule, "f/", "", 1)
					fnum, err := strconv.ParseFloat(rule, 32)
					if err != nil {
						f.Err(fmt.Errorf("invalid aperture value: '%s'", rule))
					}
					switch {
					case comp == 0 && fnum != m.CameraInfo.Aperture.Float():
						return false
					case comp == 1 && fnum > m.CameraInfo.Aperture.Float():
						return false
					case comp == 2 && fnum < m.CameraInfo.Aperture.Float():
						return false
					}
				case strings.Contains(rule, "iso"):
					rule = strings.Replace(rule, "iso", "", 1)
					iso, err := strconv.ParseFloat(rule, 32)
					if err != nil {
						f.Err(fmt.Errorf("invalid iso value: '%s'", rule))
					}
					switch {
					case comp == 0 && iso != float64(m.CameraInfo.ISO):
						return false
					case comp == 1 && iso > float64(m.CameraInfo.ISO):
						return false
					case comp == 2 && iso < float64(m.CameraInfo.ISO):
						return false
					}
				case strings.Contains(rule, "s"):
					rule = strings.Replace(rule, "s", "", 1)
					p := strings.SplitN(rule, "/", 2)
					if len(p) > 2 {
						f.Err(fmt.Errorf("invalid shutter speed value: '%s'", rule))
					}

					nom, err := strconv.ParseFloat(p[0], 32)
					if err != nil {
						f.Err(fmt.Errorf("invalid shutters speed value: '%s'", rule))
					}
					denom := 1.0
					if len(p) == 2 {
						denom, err = strconv.ParseFloat(p[1], 32)
						if err != nil {
							f.Err(fmt.Errorf("invalid shutters speed value: '%s'", rule))
						}
					}

					ss := nom / denom
					switch {
					case comp == 0 && math.Abs(ss-m.CameraInfo.ShutterSpeed.Float()) > 1.0/256000:
						return false
					case comp == 1 && ss < m.CameraInfo.ShutterSpeed.Float():
						return false
					case comp == 2 && ss > m.CameraInfo.ShutterSpeed.Float():
						return false
					}
				case strings.Contains(rule, "mm"):
					rule = strings.Replace(rule, "mm", "", 1)
					fl, err := strconv.ParseFloat(rule, 32)
					if err != nil {
						f.Err(fmt.Errorf("invalid focal length value: '%s'", rule))
					}
					switch {
					case comp == 0 && fl != m.CameraInfo.FocalLength.Float():
						return false
					case comp == 1 && fl > m.CameraInfo.FocalLength.Float():
						return false
					case comp == 2 && fl < m.CameraInfo.FocalLength.Float():
						return false
					}
				default:
					f.Err(fmt.Errorf("invalid exposure rule: '%s'", rule))
				}

			}
		}

		tmap := m.Tags.Map()
		for _, and := range f.tags {
			match := false
			if _, ok := and["-"]; ok {
				match = len(m.Tags) == 0
			}
			if _, ok := and["*"]; ok {
				if len(m.Tags) != 0 {
					match = true
				}
			}

			for not := range and {
				if !strings.HasPrefix(not, "^") {
					continue
				}
				match = true
				if _, ok := tmap[not[1:]]; ok {
					match = false
				}
			}

			for _, t := range m.Tags {
				if _, ok := and[t]; ok {
					match = true
					break
				}
			}

			if !match {
				return false
			}
		}

		return true
	}

	return f.metafilter
}

func (f *Flags) Err(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func (f *Flags) Exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (f *Flags) Parse() {
	var actions flagStrs
	var ratingGT int
	var ratingLT int
	var ext string
	var file string
	var camera string
	var lens string
	var exposure string
	var baseDir string
	var rawDir, collectionDir, jpegDir string
	var fsSources flagStrs
	var checksum bool
	var sizes flagStrs
	var alwaysYes bool
	var zero bool
	var maxWorkers int
	var noRawPrefix bool
	var tags flagStrs
	var gphotos string
	var glocation string
	var since, until string
	var help bool
	var importJPEG bool
	var verbose bool
	var editor string

	var undeleted bool
	var deleted bool
	var updated bool
	var edited bool
	var unedited bool
	var rated bool
	var unrated bool
	var location bool
	var noLocation bool
	var photo bool
	var video bool

	f.fs.BoolVar(&help, "h", false, "\nhelp\n")
	f.fs.Var(&actions, flags.Actions, f.lists.Help(flags.Actions))

	f.fs.BoolVar(&undeleted, flags.Undeleted, false, f.lists.Help(flags.Undeleted))
	f.fs.BoolVar(&deleted, flags.Deleted, false, f.lists.Help(flags.Deleted))
	f.fs.BoolVar(&updated, flags.Updated, false, f.lists.Help(flags.Updated))
	f.fs.BoolVar(&edited, flags.Edited, false, f.lists.Help(flags.Edited))
	f.fs.BoolVar(&unedited, flags.Unedited, false, f.lists.Help(flags.Unedited))
	f.fs.BoolVar(&rated, flags.Rated, false, f.lists.Help(flags.Rated))
	f.fs.BoolVar(&unrated, flags.Unrated, false, f.lists.Help(flags.Unrated))
	f.fs.BoolVar(&location, flags.Location, false, f.lists.Help(flags.Location))
	f.fs.BoolVar(&noLocation, flags.NoLocation, false, f.lists.Help(flags.NoLocation))
	f.fs.BoolVar(&photo, flags.Photo, false, f.lists.Help(flags.Photo))
	f.fs.BoolVar(&video, flags.Video, false, f.lists.Help(flags.Video))

	f.fs.IntVar(&ratingGT, flags.GT, -1, f.lists.Help(flags.GT))
	f.fs.IntVar(&ratingLT, flags.LT, 6, f.lists.Help(flags.LT))
	f.fs.StringVar(&ext, flags.Ext, "", f.lists.Help(flags.Ext))
	f.fs.StringVar(&file, flags.File, "", f.lists.Help(flags.File))
	f.fs.StringVar(&camera, flags.Camera, "", f.lists.Help(flags.Camera))
	f.fs.StringVar(&lens, flags.Lens, "", f.lists.Help(flags.Lens))
	f.fs.StringVar(&exposure, flags.Exposure, "", f.lists.Help(flags.Exposure))
	f.fs.StringVar(&since, flags.Since, "", f.lists.Help(flags.Since))
	f.fs.StringVar(&until, flags.Until, "", f.lists.Help(flags.Until))

	f.fs.Var(&tags, flags.Tags, f.lists.Help(flags.Tags))

	f.fs.BoolVar(&checksum, flags.Checksum, false, f.lists.Help(flags.Checksum))
	f.fs.BoolVar(&importJPEG, flags.ImportJPEG, false, f.lists.Help(flags.ImportJPEG))
	f.fs.Var(&sizes, flags.Sizes, f.lists.Help(flags.Sizes))

	f.fs.StringVar(&rawDir, flags.RawDir, "", f.lists.Help(flags.RawDir))
	f.fs.StringVar(&collectionDir, flags.CollectionDir, "", f.lists.Help(flags.CollectionDir))
	f.fs.StringVar(&jpegDir, flags.JPEGDir, "", f.lists.Help(flags.JPEGDir))

	f.fs.StringVar(&gphotos, flags.GPhotosCredentials, "", f.lists.Help(flags.GPhotosCredentials))
	f.fs.StringVar(&glocation, flags.GLocationDirectory, "", f.lists.Help(flags.GLocationDirectory))

	f.fs.IntVar(&maxWorkers, flags.MaxWorkers, 100, f.lists.Help(flags.MaxWorkers))

	f.fs.StringVar(&baseDir, flags.BaseDir, "", f.lists.Help(flags.BaseDir))

	f.fs.Var(&fsSources, flags.SourceDir, f.lists.Help(flags.SourceDir))

	f.fs.BoolVar(&alwaysYes, flags.AlwaysYes, false, f.lists.Help(flags.AlwaysYes))
	f.fs.BoolVar(&zero, flags.Zero, false, f.lists.Help(flags.Zero))
	f.fs.BoolVar(&noRawPrefix, flags.NoRawPrefix, false, f.lists.Help(flags.NoRawPrefix))

	f.fs.BoolVar(&verbose, flags.Verbose, false, f.lists.Help(flags.Verbose))

	f.fs.StringVar(&editor, flags.Editor, "vim", f.lists.Help(flags.Editor))

	uconfdir, err := os.UserConfigDir()
	confArgs := make([]string, 0)
	if err == nil {
		confdir := filepath.Join(uconfdir, "photos")
		conffile := filepath.Join(confdir, "photos.conf")
		fl, err := os.Open(conffile)
		if os.IsNotExist(err) {
			os.MkdirAll(confdir, 0755)
			fl, err = os.Create(conffile)
			f.Err(err)
			fmt.Fprintln(fl, "# -base /home/user/RAW")
			//fl.Seek(0, io.SeekStart)
		}
		if err != nil {
			f.Err(fmt.Errorf("failed opening config file '%s': %w", conffile, err))
		}

		s := bufio.NewScanner(fl)
		s.Split(bufio.ScanLines)
		for s.Scan() {
			t := strings.TrimSpace(s.Text())
			p := strings.SplitN(t, " ", 2)
			confArgs = append(confArgs, p[0])
			if len(p) == 2 {
				confArgs = append(confArgs, strings.TrimSpace(p[1]))
			}
		}

		f.Err(s.Err())

		f.phodoDefault = filepath.Join(confdir, "default.pho")
		phodoPreview := filepath.Join(confdir, "preview.pho")
		cnot := func(path string, def string) error {
			_, err := os.Stat(path)
			if !os.IsNotExist(err) {
				return err
			}
			f, err := os.Create(path)
			if err != nil {
				return err
			}

			_, err = fmt.Fprint(f, def)
			f.Close()
			return err
		}

		f.Err(cnot(f.phodoDefault, `.main(
    orientation()
)

// Uncomment the line below to mark the image edit as 'finished'
// allowing the image to be converted.
// .convert(.main())`))

		f.Err(cnot(phodoPreview, `
.main(
    resize-fit(1920 1920)
    orientation()

    exif-allow()

    extend(310 0 0 0)
    compose(
        pos(0 0 (
            histogram(rgb "width-10" 300 2)
            extend(5)
            border(2 hex(fff))
        ))
    )
)
`))

		importer.RegisterPreviewGen(&importer.PhoPreviewGen{phodoPreview})
	}

	importer.RegisterPreviewGen(&importer.RTPreviewGen{})
	importer.RegisterPreviewGen(&importer.IMPreviewGen{})
	importer.RegisterPreviewGen(&importer.VidPreviewGen{})

	if len(confArgs) != 0 {
		f.Err(f.fs.Parse(confArgs))
	}

	f.Err(f.fs.Parse(os.Args[1:]))

	if help {
		f.fs.PrintDefaults()
		os.Exit(0)
	}

	f.actions = flags.CommaSep(strings.Join(actions, ","))
	if len(f.actions) == 0 {
		f.Err(errors.New("no actions provided"))
	}

	for _, a := range f.actions {
		if _, ok := flags.AllActions[a]; !ok {
			f.Err(fmt.Errorf("action %s does not exist", a))
		}
	}

	f.filters = map[string]bool{
		flags.Undeleted:  undeleted,
		flags.Deleted:    deleted,
		flags.Updated:    updated,
		flags.Edited:     edited,
		flags.Unedited:   unedited,
		flags.Rated:      rated,
		flags.Unrated:    unrated,
		flags.Location:   location,
		flags.NoLocation: noLocation,
		flags.Photo:      photo,
		flags.Video:      video,
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
		if gphotos == "" {
			gphotos = filepath.Join(baseDir, "gphotos.credentials")
		}
	}

	if rawDir == "" {
		f.Err(errors.New("please provide a raw dir"))
	} else if collectionDir == "" {
		f.Err(errors.New("please provide a collection dir"))
	} else if jpegDir == "" {
		f.Err(errors.New("please provide a jpeg dir"))
	}

	sints := flags.CommaSep(strings.Join(sizes, ","))
	f.sizes = make([]int, len(sints))
	for i, s := range sints {
		f.sizes[i], err = strconv.Atoi(s)
		f.Err(err)
	}

	f.tags = make([]map[string]struct{}, 0, len(tags))
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
			f.tags = append(f.tags, ors)
		}
	}

	f.time.since, err = parseTime(since, false)
	f.Err(err)
	f.time.until, err = parseTime(until, true)
	f.Err(err)

	split := func(s string, wc string, cb func(string) bool) []string {
		_list := strings.Split(strings.ToLower(s), wc)
		list := make([]string, 0, len(_list))
		for _, i := range _list {
			if cb(i) {
				list = append(list, i)
			}
		}
		return list
	}
	always := func(string) bool { return true }
	nonempty := func(s string) bool { return s != "" }
	f.exposure = split(exposure, ",", nonempty)
	if camera != "" && camera != "*" {
		f.camera = split(camera, "*", always)
	}
	if lens != "" && lens != "*" {
		f.lens = split(lens, "*", always)
	}
	if file != "" && file != "*" {
		f.file = split(file, "*", func(string) bool { return true })
	}

	if f.ext = strings.TrimLeft(strings.ToLower(ext), "."); f.ext != "" {
		f.ext = "." + f.ext
	}

	f.sourceDirs = fsSources
	f.rating.gt = ratingGT
	f.rating.lt = ratingLT
	f.checksum = checksum
	f.importJPEG = importJPEG
	f.alwaysYes = alwaysYes
	f.noRawPrefix = noRawPrefix
	f.zero = zero
	f.maxWorkers = maxWorkers
	f.rawDir, f.collectionDir, f.jpegDir = rawDir, collectionDir, jpegDir
	f.gphotos = gphotos
	f.glocation = glocation
	f.verbose = verbose
	f.editor = editor

	f.log = log.New(os.Stderr, "", log.LstdFlags)
	if !verbose {
		f.log = log.New(ioutil.Discard, "", 0)
	}
}

func filterString(s string, filter []string) bool {
	lc := strings.ToLower(s)
	for i, p := range filter {
		method := strings.Contains
		if i == 0 {
			method = strings.HasPrefix
		}
		if i == len(filter)-1 {
			method = strings.HasSuffix
		}

		if p == "" {
			continue
		}

		if len(filter) == 1 {
			return lc == p
		}

		if !method(lc, p) {
			return false
		}
	}
	return true
}
