package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

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

const (
	FlagActions            = "action"
	FlagFilters            = "filter"
	FlagGT                 = "gt"
	FlagLT                 = "lt"
	FlagSince              = "since"
	FlagUntil              = "until"
	FlagTags               = "tag"
	FlagChecksum           = "sum"
	FlagSizes              = "sizes"
	FlagRawDir             = "raws"
	FlagCollectionDir      = "collection"
	FlagJPEGDir            = "jpegs"
	FlagMaxWorkers         = "workers"
	FlagBaseDir            = "base"
	FlagSourceDir          = "source"
	FlagAlwaysYes          = "y"
	FlagZero               = "0"
	FlagNoRawPrefix        = "no-raw"
	FlagGPhotosCredentials = "gphotos"
)

const (
	ActionImport     = "import"
	ActionShow       = "show"
	ActionShowJPEGs  = "show-jpegs"
	ActionShowLinks  = "show-links"
	ActionShowTags   = "show-tags"
	ActionInfo       = "info"
	ActionLink       = "link"
	ActionPreviews   = "previews"
	ActionRate       = "rate"
	ActionSyncMeta   = "sync-meta"
	ActionConvert    = "convert"
	ActionExec       = "exec"
	ActionCleanup    = "cleanup"
	ActionTagsRemove = "remove-tags"
	ActionTagsAdd    = "add-tags"
	ActionGPhotos    = "gphotos"

	FilterUndeleted = "undeleted"
	FilterDeleted   = "deleted"
	FilterEdited    = "edited"
	FilterUnedited  = "unedited"
	FilterRated     = "rated"
	FilterUnrated   = "unrated"
)

var (
	AllActions = map[string]struct{}{
		ActionImport:     struct{}{},
		ActionShow:       struct{}{},
		ActionShowJPEGs:  struct{}{},
		ActionShowLinks:  struct{}{},
		ActionShowTags:   struct{}{},
		ActionInfo:       struct{}{},
		ActionLink:       struct{}{},
		ActionPreviews:   struct{}{},
		ActionRate:       struct{}{},
		ActionSyncMeta:   struct{}{},
		ActionConvert:    struct{}{},
		ActionExec:       struct{}{},
		ActionCleanup:    struct{}{},
		ActionTagsRemove: struct{}{},
		ActionTagsAdd:    struct{}{},
		ActionGPhotos:    struct{}{},
	}

	allFilters = map[string]struct{}{
		FilterUndeleted: struct{}{},
		FilterDeleted:   struct{}{},
		FilterEdited:    struct{}{},
		FilterUnedited:  struct{}{},
		FilterRated:     struct{}{},
		FilterUnrated:   struct{}{},
	}
)

type Filter func(meta.Meta, *importer.File) bool

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
	FlagActions: {
		help: "list of actions (comma separated and/or specified multiple times)",
		list: map[string][]string{
			ActionImport:    {"Import media from connected camera (gphoto2) and any given directory (-source) to the directory specified with -raws"},
			ActionShow:      {"Show raws (filter with -filter)"},
			ActionShowJPEGs: {"Show jpegs (filter with -filter) (see -no-raw)"},
			ActionShowLinks: {"Show links (filter with -filter) (see -no-raw)"},
			ActionShowTags:  {"Show all tags"},
			ActionInfo:      {"Show info for given RAWs"},
			ActionLink:      {"Create collection symlinks in the given directory (-collection)"},
			ActionPreviews:  {"Generate simple jpeg previews (used by -action rate)"},
			ActionRate:      {"Simple opengl window to rate / trash images (filter with -filter)"},
			ActionSyncMeta:  {"Sync .meta file with .pp3 (file mtime determines which one is the authority) and filesystem"},
			ActionConvert: {
				"Convert images to jpegs resized with -sizes (filter with -filter)",
				"These conversions are tracked in .meta i.e.:",
				"running",
				"photos ... -action convert -sizes 3840,1920 and later",
				"photos ... -action convert -sizes 1920 will result in only the 1920 image being tracked",
				"an -action cleanup will result in the deletion of all 3840 images",
			},
			ActionExec: {
				"Run an external command for each file (first non flag and any further arguments, {} is replaced with the filepath)",
				"e.g.: photos -base . -action exec -filter all wc -c {}",
			},
			ActionCleanup: {
				"Remove pp3s and jpegs for deleted RAWs",
				"-filter and -lt are ignored",
				"Images whose rating is not higher than -gt will also have their jpegs deleted.",
				"!Note: .meta files are seen as the single source of truth, so run sync-meta before",
			},
			ActionTagsRemove: {"Remove tags (first non flag argument are the tags that will be removed)"},
			ActionTagsAdd:    {"Add tag (first non flag argument are the tags that will be removed)"},
			ActionGPhotos:    {"Upload converted photos to google photos"},
		},
	},

	FlagFilters: {
		help: "[any] filters (comma separated and/or specified multiple times)",
		list: map[string][]string{
			FilterUndeleted: {"ignore trashed/deleted files"},
			FilterDeleted:   {"only include trashed/deleted files"},
			FilterUnrated:   {"only include unrated files"},
			FilterUnedited:  {"only include files with incomplete pp3s (never opened in rawtherapee)"},
			FilterEdited:    {"only include files with complete pp3s (have been opened in rawtherapee)"},
		},
	},

	FlagGT:    {help: "[any] additional greater than given rating filter"},
	FlagLT:    {help: "[any] additional less than given rating filter"},
	FlagSince: {help: "[any] additional since time filter [Y-m-d (H:M)]"},
	FlagUntil: {help: "[any] additional until time filter [Y-m-d (H:M)]"},
	FlagTags: {
		help: `[any] additional tag filter, comma separated <or> can be specified multiple times <and>, ^ to negate a single tag
e.g:
photo must be tagged: (outside || sunny) && dog && !tree
-tags 'outside,sunny' -tags 'dog' -tags '^tree'

special case: '-' only matches files with no tags
special case: '*' only matches files with tags
`,
	},

	FlagChecksum: {help: "[import] dry-run and report non-identical files with duplicate filenames"},
	FlagSizes:    {help: "[convert] longest image dimension will be scaled to this size (comma separated and/or specified multiple times (e.g.: 3840,1920,800)"},

	FlagRawDir:        {help: "[any] Raw directory"},
	FlagCollectionDir: {help: "[any] Collection directory"},
	FlagJPEGDir:       {help: "[convert] JPEG directory"},
	FlagBaseDir: {help: `[all] Set a basedir which implies:
-raws (if not given)       = <basedir>/Originals
-collection (if not given) = <basedir>/Collection
-jpegs (if not given)      = <basedir>/Converted
-gphotos (if not given)    = <basedir>/gphotos.credentials
`},
	FlagGPhotosCredentials: {help: "[gphotos] path to the google credentials file"},

	FlagMaxWorkers: {help: "[all] maximum amount of threads"},
	FlagSourceDir:  {help: "[import] filesystem paths to import from, can be specified multiple times"},

	FlagAlwaysYes: {help: "always answer yes"},
	FlagZero: {help: `all stdout output will be separated by a null byte
e.g.: photos -base . -0 -action show-jpegs -no-raw | xargs -0 feh`},
	FlagNoRawPrefix: {help: "[show-*] don't prefix output with the corresponding raw file"},
}

type Flags struct {
	fs    *flag.FlagSet
	lists Lists

	actions []string
	filters []string
	tags    []map[string]struct{}
	rating  struct {
		gt, lt int
	}

	time struct {
		since, until *time.Time
	}

	rawDir, collectionDir, jpegDir string

	sourceDirs []string

	checksum bool

	alwaysYes bool

	sizes []int

	noRawPrefix bool
	zero        bool

	maxWorkers int

	gphotos string

	output func(string)
	filter Filter
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

func (f *Flags) GPhotosCredentials() string { return f.gphotos }

func (f *Flags) Filter(imp *importer.Importer) Filter {
	if f.filter == nil {
		list := make([]Filter, 0, len(f.filters))
		for _, filter := range f.filters {
			var _f Filter
			switch filter {
			case FilterUndeleted:
				_f = func(meta meta.Meta, fl *importer.File) bool {
					return !meta.Deleted
				}

			case FilterDeleted:
				_f = func(meta meta.Meta, fl *importer.File) bool {
					return meta.Deleted
				}
			case FilterRated:
				_f = func(meta meta.Meta, fl *importer.File) bool {
					return meta.Rating > 0 && meta.Rating < 6
				}
			case FilterUnrated:
				_f = func(meta meta.Meta, fl *importer.File) bool {
					return meta.Rating < 1 || meta.Rating > 5
				}
			case FilterEdited:
				_f = func(meta meta.Meta, fl *importer.File) bool {
					b, err := imp.Unedited(fl)
					f.Exit(err)
					return !b
				}
			case FilterUnedited:
				_f = func(meta meta.Meta, fl *importer.File) bool {
					b, err := imp.Unedited(fl)
					f.Exit(err)
					return b
				}
			default:
				f.Exit(fmt.Errorf("unknown filter %s", filter))
			}

			list = append(list, _f)
		}

		f.filter = func(m meta.Meta, fl *importer.File) bool {
			if m.Rating <= f.rating.gt || m.Rating >= f.rating.lt {
				return false
			}
			for _, f := range list {
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
	}

	return f.filter
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
	var since, until string
	var help bool

	f.fs.BoolVar(&help, "h", false, "help")
	f.fs.Var(&actions, FlagActions, f.lists.Help(FlagActions))
	f.fs.Var(&filters, FlagFilters, f.lists.Help(FlagFilters))
	f.fs.IntVar(&ratingGTFilter, FlagGT, -1, f.lists.Help(FlagGT))
	f.fs.IntVar(&ratingLTFilter, FlagLT, 6, f.lists.Help(FlagLT))
	f.fs.StringVar(&since, FlagSince, "", f.lists.Help(FlagSince))
	f.fs.StringVar(&until, FlagUntil, "", f.lists.Help(FlagUntil))

	f.fs.Var(&tags, FlagTags, f.lists.Help(FlagTags))

	f.fs.BoolVar(&checksum, FlagChecksum, false, f.lists.Help(FlagChecksum))
	f.fs.Var(&sizes, FlagSizes, f.lists.Help(FlagSizes))

	f.fs.StringVar(&rawDir, FlagRawDir, "", f.lists.Help(FlagRawDir))
	f.fs.StringVar(&collectionDir, FlagCollectionDir, "", f.lists.Help(FlagCollectionDir))
	f.fs.StringVar(&jpegDir, FlagJPEGDir, "", f.lists.Help(FlagJPEGDir))

	f.fs.StringVar(&gphotos, FlagGPhotosCredentials, "", f.lists.Help(FlagGPhotosCredentials))

	f.fs.IntVar(&maxWorkers, FlagMaxWorkers, 100, f.lists.Help(FlagMaxWorkers))

	f.fs.StringVar(&baseDir, FlagBaseDir, "", f.lists.Help(FlagBaseDir))

	f.fs.Var(&fsSources, FlagSourceDir, f.lists.Help(FlagSourceDir))

	f.fs.BoolVar(&alwaysYes, FlagAlwaysYes, false, f.lists.Help(FlagAlwaysYes))
	f.fs.BoolVar(&zero, FlagZero, false, f.lists.Help(FlagZero))
	f.fs.BoolVar(&noRawPrefix, FlagNoRawPrefix, false, f.lists.Help(FlagNoRawPrefix))

	f.Err(f.fs.Parse(os.Args[1:]))

	if help {
		f.fs.PrintDefaults()
		os.Exit(0)
	}

	f.actions = commaSep(strings.Join(actions, ","))
	if len(f.actions) == 0 {
		f.Err(errors.New("no actions provided"))
	}

	for _, a := range f.actions {
		if _, ok := AllActions[a]; !ok {
			f.Err(fmt.Errorf("action %s does not exist", a))
		}
	}

	f.filters = commaSep(strings.Join(filters, ","))
	for _, fi := range f.filters {
		if _, ok := allFilters[fi]; !ok {
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

	sints := commaSep(strings.Join(sizes, ","))
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
}
