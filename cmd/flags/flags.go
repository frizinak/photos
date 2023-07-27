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
	Filters            = "filter"
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

	FilterUndeleted  = "undeleted"
	FilterDeleted    = "deleted"
	FilterUpdated    = "updated"
	FilterEdited     = "edited"
	FilterUnedited   = "unedited"
	FilterRated      = "rated"
	FilterUnrated    = "unrated"
	FilterLocation   = "location"
	FilterNoLocation = "nolocation"
)

var (
	AllFlags = map[string]struct{}{
		Actions:            struct{}{},
		Filters:            struct{}{},
		GT:                 struct{}{},
		LT:                 struct{}{},
		Camera:             struct{}{},
		Lens:               struct{}{},
		Exposure:           struct{}{},
		File:               struct{}{},
		Ext:                struct{}{},
		Since:              struct{}{},
		Until:              struct{}{},
		Tags:               struct{}{},
		Checksum:           struct{}{},
		Sizes:              struct{}{},
		RawDir:             struct{}{},
		CollectionDir:      struct{}{},
		JPEGDir:            struct{}{},
		MaxWorkers:         struct{}{},
		BaseDir:            struct{}{},
		SourceDir:          struct{}{},
		AlwaysYes:          struct{}{},
		Zero:               struct{}{},
		NoRawPrefix:        struct{}{},
		GPhotosCredentials: struct{}{},
		GLocationDirectory: struct{}{},
		Verbose:            struct{}{},
		Editor:             struct{}{},
	}

	AllActions = map[string]struct{}{
		ActionImport:       struct{}{},
		ActionShow:         struct{}{},
		ActionShowPreviews: struct{}{},
		ActionShowJPEGs:    struct{}{},
		ActionShowLinks:    struct{}{},
		ActionShowTags:     struct{}{},
		ActionInfo:         struct{}{},
		ActionLink:         struct{}{},
		ActionPreviews:     struct{}{},
		ActionRate:         struct{}{},
		ActionSyncMeta:     struct{}{},
		ActionRewriteMeta:  struct{}{},
		ActionConvert:      struct{}{},
		ActionJPEGFixup:    struct{}{},
		ActionExec:         struct{}{},
		ActionCleanup:      struct{}{},
		ActionTagsRemove:   struct{}{},
		ActionTagsAdd:      struct{}{},
		ActionGPhotos:      struct{}{},
		ActionGLocation:    struct{}{},
	}

	AllFilters = map[string]struct{}{
		FilterUndeleted:  struct{}{},
		FilterDeleted:    struct{}{},
		FilterUpdated:    struct{}{},
		FilterEdited:     struct{}{},
		FilterUnedited:   struct{}{},
		FilterRated:      struct{}{},
		FilterUnrated:    struct{}{},
		FilterLocation:   struct{}{},
		FilterNoLocation: struct{}{},
	}
)
