package cli

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/frizinak/photos/cmd/flags"
	"github.com/frizinak/photos/gtimeline"
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

func (l Lists) Help(cmd string) string { return l[cmd].String() }

var lists = Lists{
	flags.Actions: {
		help: "list of actions (comma separated and/or specified multiple times)",
		list: map[string][]string{
			flags.ActionImport:       {"Import media from connected camera (gphoto2) and any given directory (-source) to the directory specified with -raws"},
			flags.ActionShow:         {"Show raws (filter with -filter)"},
			flags.ActionShowPreviews: {"Show jpegs (filter with -filter) (see -no-raw)"},
			flags.ActionShowJPEGs:    {"Show previews (filter with -filter) (see -no-raw)"},
			flags.ActionShowLinks:    {"Show links (filter with -filter) (see -no-raw)"},
			flags.ActionShowTags:     {"Show all tags"},
			flags.ActionInfo:         {"Show info for given RAWs"},
			flags.ActionLink:         {"Create collection symlinks in the given directory (-collection)"},
			flags.ActionPreviews:     {"Generate simple jpeg previews (used by -action rate)"},
			flags.ActionRate:         {"Simple opengl window to rate / trash images (filter with -filter)"},
			flags.ActionSyncMeta:     {"Sync .meta file with .pp3 (file mtime determines which one is the authority) and filesystem"},
			flags.ActionRewriteMeta:  {"Rewrite .meta, make sure you synced first so newer pp3s are not overwritten."},
			flags.ActionConvert: {
				"Convert images to jpegs resized with -sizes (filter with -filter)",
				"These conversions are tracked in .meta i.e.:",
				"running",
				"photos ... -action convert -sizes 3840,1920 and later",
				"photos ... -action convert -sizes 1920 will result in only the 1920 image being tracked",
				"an -action cleanup will result in the deletion of all 3840 images",
			},
			flags.ActionExec: {
				"Run an external command for each file (first non flag and any further arguments, {} is replaced with the filepath)",
				"e.g.: photos -base . -action exec -filter all wc -c {}",
			},
			flags.ActionCleanup: {
				"Remove pp3s and jpegs for deleted RAWs",
				"-filter and -lt are ignored",
				"Images whose rating is not higher than -gt will also have their jpegs deleted.",
				"!Note: .meta files are seen as the single source of truth, so run sync-meta before",
			},
			flags.ActionTagsRemove: {"Remove tags (first non flag argument are the tags that will be removed)"},
			flags.ActionTagsAdd:    {"Add tag (first non flag argument are the tags that will be removed)"},
			flags.ActionGPhotos:    {"Upload converted photos to google photos"},
			flags.ActionGLocation: {
				"Update meta with location information extracted from google timeline kml",
				fmt.Sprintf("requires -glocation flag with your %s cookie value", gtimeline.SessID),
				"!!! there is nothing safe about this, this is your google session id",
			},
		},
	},

	flags.Filters: {
		help: "[any] filters (comma separated and/or specified multiple times)",
		list: map[string][]string{
			flags.FilterUndeleted: {"ignore trashed/deleted files"},
			flags.FilterDeleted:   {"only include trashed/deleted files"},
			flags.FilterUnrated:   {"only include unrated files"},
			flags.FilterUnedited:  {"only include files with incomplete pp3s (never opened in rawtherapee)"},
			flags.FilterEdited:    {"only include files with complete pp3s (have been opened in rawtherapee)"},
		},
	},

	flags.GT:    {help: "[any] greater than given rating filter"},
	flags.LT:    {help: "[any] less than given rating filter"},
	flags.Ext:   {help: "[any] filter original file extension (case insensitive)"},
	flags.Since: {help: "[any] since time filter [Y-m-d (H:M)]"},
	flags.Until: {help: "[any] until time filter [Y-m-d (H:M)]"},
	flags.Tags: {
		help: `[any] tag filter, comma separated <or> can be specified multiple times <and>, ^ to negate a single tag
e.g:
photo must be tagged: (outside || sunny) && dog && !tree
-tags 'outside,sunny' -tags 'dog' -tags '^tree'

special case: '-' only matches files with no tags
special case: '*' only matches files with tags
`,
	},

	flags.Checksum: {help: "[import] dry-run and report non-identical files with duplicate filenames"},
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

	flags.CameraFixedTZ: {
		help: `[import,rewrite-meta] timezone offset in minutes.
Since there is no standard in exif timezone data the time we store in .meta will be off unless your camera is set to UTC.
Set this to the timezone your camera is ALWAYS in, won't work if your camera has correcly applied daylight saving times (set either automatically or manually).
e.g.: daylight saving time always off in brussels: -tz 120
e.g.: daylight saving time always on             : -tz 60`,
	},

	flags.RawDir:        {help: "[any] Raw directory"},
	flags.CollectionDir: {help: "[any] Collection directory"},
	flags.JPEGDir:       {help: "[convert] JPEG directory"},
	flags.BaseDir: {help: `[all] Set a basedir which implies:
-raws (if not given)       = <basedir>/Originals
-collection (if not given) = <basedir>/Collection
-jpegs (if not given)      = <basedir>/Converted
-gphotos (if not given)    = <basedir>/gphotos.credentials
`},
	flags.GPhotosCredentials: {help: "[gphotos] path to the google credentials file"},
	flags.GLocationCredentials: {
		help: fmt.Sprintf(
			"[glocation] your %s cookie value (see -action glocation)",
			gtimeline.SessID,
		),
	},

	flags.MaxWorkers: {help: "[all] maximum amount of threads"},
	flags.SourceDir:  {help: "[import] filesystem paths to import from, can be specified multiple times"},

	flags.AlwaysYes: {help: "always answer yes"},
	flags.Zero: {help: `all stdout output will be separated by a null byte
e.g.: photos -base . -0 -action show-jpegs -no-raw | xargs -0 feh`},
	flags.NoRawPrefix: {help: "[show-*] don't prefix output with the corresponding raw file"},
	flags.Verbose:     {help: "enable verbose stderr logging"},
}

type Flags struct {
	fs    *flag.FlagSet
	lists Lists

	actions []string
	filters []string
	ext     string
	tags    []map[string]struct{}
	rating  struct {
		gt, lt int
	}

	time struct {
		since, until *time.Time
	}

	tz *int

	rawDir, collectionDir, jpegDir string

	sourceDirs []string

	checksum bool

	alwaysYes bool

	verbose bool

	sizes []int

	noRawPrefix bool
	zero        bool

	maxWorkers int

	gphotos   string
	glocation string

	log    *log.Logger
	output func(string)

	filter     Filter
	metafilter MetaFilter

	filterFuncs  []Filter
	mfilterFuncs []MetaFilter
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
func (f *Flags) Yes() bool         { return f.alwaysYes }
func (f *Flags) NoRawPrefix() bool { return f.noRawPrefix }

func (f *Flags) Args() []string { return f.fs.Args() }

func (f *Flags) Sizes() []int { return f.sizes }

func (f *Flags) RatingGT() int { return f.rating.gt }
func (f *Flags) RatingLT() int { return f.rating.lt }

func (f *Flags) Verbose() bool { return f.verbose }

func (f *Flags) GPhotosCredentials() string   { return f.gphotos }
func (f *Flags) GLocationCredentials() string { return f.glocation }

func (f *Flags) Log() *log.Logger { return f.log }

func (f *Flags) TZOffset() (offset int, ok bool) {
	if f.tz == nil {
		return
	}
	ok = true
	offset = *f.tz
	return
}

func (f *Flags) makeFilters(imp *importer.Importer) {
	if f.mfilterFuncs != nil {
		return
	}
	mlist := make([]MetaFilter, 0, len(f.filters))
	list := make([]Filter, 0, len(f.filters))
	for _, filter := range f.filters {
		var _mf MetaFilter
		var _f Filter
		switch filter {
		case flags.FilterUndeleted:
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				return !meta.Deleted
			}

		case flags.FilterDeleted:
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				return meta.Deleted
			}
		case flags.FilterRated:
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				return meta.Rating > 0 && meta.Rating < 6
			}
		case flags.FilterUnrated:
			_mf = func(meta meta.Meta, fl *importer.File) bool {
				return meta.Rating < 1 || meta.Rating > 5
			}
		case flags.FilterEdited:
			_f = func(fl *importer.File) bool {
				b, err := imp.Unedited(fl)
				f.Exit(err)
				return !b
			}
		case flags.FilterUnedited:
			_f = func(fl *importer.File) bool {
				b, err := imp.Unedited(fl)
				f.Exit(err)
				return b
			}
		default:
			f.Exit(fmt.Errorf("unknown filter %s", filter))
		}

		if _f != nil {
			list = append(list, _f)
		}
		if _mf != nil {
			mlist = append(mlist, _mf)
		}
	}
	f.mfilterFuncs = mlist
	f.filterFuncs = list
}

func (f *Flags) Filter(imp *importer.Importer) Filter {
	if f.filter != nil {
		return f.filter
	}
	f.makeFilters(imp)
	f.filter = func(fl *importer.File) bool {
		if f.ext != "" && f.ext != strings.ToLower(filepath.Ext(fl.BaseFilename())) {
			return false
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
		if m.Rating <= f.rating.gt || m.Rating >= f.rating.lt {
			return false
		}
		for _, f := range f.mfilterFuncs {
			if !f(m, fl) {
				return false
			}
		}

		if f.time.since != nil && f.time.since.After(m.CreatedTime()) {
			return false
		}

		if f.time.until != nil && f.time.until.Before(m.CreatedTime()) {
			return false
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
	var filters flagStrs
	var ratingGTFilter int
	var ratingLTFilter int
	var ext string
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
	var tz int
	var help bool
	var verbose bool

	f.fs.BoolVar(&help, "h", false, "help")
	f.fs.Var(&actions, flags.Actions, f.lists.Help(flags.Actions))
	f.fs.Var(&filters, flags.Filters, f.lists.Help(flags.Filters))
	f.fs.IntVar(&ratingGTFilter, flags.GT, -1, f.lists.Help(flags.GT))
	f.fs.IntVar(&ratingLTFilter, flags.LT, 6, f.lists.Help(flags.LT))
	f.fs.StringVar(&ext, flags.Ext, "", f.lists.Help(flags.Ext))
	f.fs.StringVar(&since, flags.Since, "", f.lists.Help(flags.Since))
	f.fs.StringVar(&until, flags.Until, "", f.lists.Help(flags.Until))

	f.fs.Var(&tags, flags.Tags, f.lists.Help(flags.Tags))

	f.fs.BoolVar(&checksum, flags.Checksum, false, f.lists.Help(flags.Checksum))
	f.fs.Var(&sizes, flags.Sizes, f.lists.Help(flags.Sizes))

	f.fs.StringVar(&rawDir, flags.RawDir, "", f.lists.Help(flags.RawDir))
	f.fs.StringVar(&collectionDir, flags.CollectionDir, "", f.lists.Help(flags.CollectionDir))
	f.fs.StringVar(&jpegDir, flags.JPEGDir, "", f.lists.Help(flags.JPEGDir))

	f.fs.StringVar(&gphotos, flags.GPhotosCredentials, "", f.lists.Help(flags.GPhotosCredentials))
	f.fs.StringVar(&glocation, flags.GLocationCredentials, "", f.lists.Help(flags.GLocationCredentials))

	f.fs.IntVar(&tz, flags.CameraFixedTZ, 0, f.lists.Help(flags.CameraFixedTZ))

	f.fs.IntVar(&maxWorkers, flags.MaxWorkers, 100, f.lists.Help(flags.MaxWorkers))

	f.fs.StringVar(&baseDir, flags.BaseDir, "", f.lists.Help(flags.BaseDir))

	f.fs.Var(&fsSources, flags.SourceDir, f.lists.Help(flags.SourceDir))

	f.fs.BoolVar(&alwaysYes, flags.AlwaysYes, false, f.lists.Help(flags.AlwaysYes))
	f.fs.BoolVar(&zero, flags.Zero, false, f.lists.Help(flags.Zero))
	f.fs.BoolVar(&noRawPrefix, flags.NoRawPrefix, false, f.lists.Help(flags.NoRawPrefix))

	f.fs.BoolVar(&verbose, flags.Verbose, false, f.lists.Help(flags.Verbose))

	f.Err(f.fs.Parse(os.Args[1:]))

	if help {
		f.fs.PrintDefaults()
		os.Exit(0)
	}

	tzSet := false
	f.fs.Visit(func(f *flag.Flag) {
		if f.Name == flags.CameraFixedTZ {
			tzSet = true
		}
	})

	f.actions = flags.CommaSep(strings.Join(actions, ","))
	if len(f.actions) == 0 {
		f.Err(errors.New("no actions provided"))
	}

	for _, a := range f.actions {
		if _, ok := flags.AllActions[a]; !ok {
			f.Err(fmt.Errorf("action %s does not exist", a))
		}
	}

	f.filters = flags.CommaSep(strings.Join(filters, ","))
	for _, fi := range f.filters {
		if _, ok := flags.AllFilters[fi]; !ok {
			f.Err(fmt.Errorf("filter %s does not exist", fi))
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
	var err error
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

	if f.ext = strings.TrimLeft(strings.ToLower(ext), "."); f.ext != "" {
		f.ext = "." + f.ext
	}

	f.sourceDirs = fsSources
	f.rating.gt = ratingGTFilter
	f.rating.lt = ratingLTFilter
	f.checksum = checksum
	f.alwaysYes = alwaysYes
	f.noRawPrefix = noRawPrefix
	f.zero = zero
	f.maxWorkers = maxWorkers
	f.rawDir, f.collectionDir, f.jpegDir = rawDir, collectionDir, jpegDir
	f.gphotos = gphotos
	f.glocation = glocation
	f.verbose = verbose
	f.tz = &tz
	if !tzSet {
		f.tz = nil
	}

	f.log = log.New(os.Stderr, "", log.LstdFlags)
	if !verbose {
		f.log = log.New(ioutil.Discard, "", 0)
	}
}
