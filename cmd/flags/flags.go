package flags

import (
	"strings"
)

func CommaSep(v string) []string {
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

const (
	Actions            = "action"
	Undeleted          = "undeleted"
	Deleted            = "deleted"
	Updated            = "updated"
	Edited             = "edited"
	Unedited           = "unedited"
	Rated              = "rated"
	Unrated            = "unrated"
	Location           = "location"
	NoLocation         = "nolocation"
	Photo              = "photo"
	Video              = "video"
	GT                 = "gt"
	LT                 = "lt"
	Camera             = "camera"
	Lens               = "lens"
	Exposure           = "exposure"
	File               = "file"
	Ext                = "ext"
	Since              = "since"
	Until              = "until"
	Tags               = "tag"
	Checksum           = "sum"
	ImportJPEG         = "import-jpegs"
	Sizes              = "sizes"
	RawDir             = "raws"
	CollectionDir      = "collection"
	JPEGDir            = "jpegs"
	MaxWorkers         = "workers"
	BaseDir            = "base"
	SourceDir          = "source"
	AlwaysYes          = "y"
	Zero               = "0"
	NoRawPrefix        = "no-raw"
	GPhotosCredentials = "gphotos"
	GLocationDirectory = "glocation"
	Verbose            = "v"
	Editor             = "editor"
)

const (
	ActionImport       = "import"
	ActionShow         = "show"
	ActionShowJPEGs    = "show-jpegs"
	ActionShowPreviews = "show-previews"
	ActionShowLinks    = "show-links"
	ActionShowTags     = "show-tags"
	ActionInfo         = "info"
	ActionLink         = "link"
	ActionPreviews     = "previews"
	ActionRate         = "rate"
	ActionEdit         = "edit"
	ActionSyncMeta     = "sync-meta"
	ActionRewriteMeta  = "rewrite-meta"
	ActionConvert      = "convert"
	ActionJPEGFixup    = "jpeg-fixup"
	ActionExec         = "exec"
	ActionCleanup      = "cleanup"
	ActionTagsRemove   = "remove-tags"
	ActionTagsAdd      = "add-tags"
	ActionGPhotos      = "gphotos"
	ActionGLocation    = "glocation"
)

var (
	AllFlags = map[string]struct{}{
		Actions:            {},
		Undeleted:          {},
		Deleted:            {},
		Updated:            {},
		Edited:             {},
		Unedited:           {},
		Rated:              {},
		Unrated:            {},
		Location:           {},
		NoLocation:         {},
		Photo:              {},
		Video:              {},
		GT:                 {},
		LT:                 {},
		Camera:             {},
		Lens:               {},
		Exposure:           {},
		File:               {},
		Ext:                {},
		Since:              {},
		Until:              {},
		Tags:               {},
		Checksum:           {},
		ImportJPEG:         {},
		Sizes:              {},
		RawDir:             {},
		CollectionDir:      {},
		JPEGDir:            {},
		MaxWorkers:         {},
		BaseDir:            {},
		SourceDir:          {},
		AlwaysYes:          {},
		Zero:               {},
		NoRawPrefix:        {},
		GPhotosCredentials: {},
		GLocationDirectory: {},
		Verbose:            {},
		Editor:             {},
	}

	AllActions = map[string]struct{}{
		ActionImport:       {},
		ActionShow:         {},
		ActionShowPreviews: {},
		ActionShowJPEGs:    {},
		ActionShowLinks:    {},
		ActionShowTags:     {},
		ActionInfo:         {},
		ActionLink:         {},
		ActionPreviews:     {},
		ActionRate:         {},
		ActionEdit:         {},
		ActionSyncMeta:     {},
		ActionRewriteMeta:  {},
		ActionConvert:      {},
		ActionJPEGFixup:    {},
		ActionExec:         {},
		ActionCleanup:      {},
		ActionTagsRemove:   {},
		ActionTagsAdd:      {},
		ActionGPhotos:      {},
		ActionGLocation:    {},
	}
)
