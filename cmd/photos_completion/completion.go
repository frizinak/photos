package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/frizinak/photos/cmd/flags"
)

func filter(opts []string, comp string) []string {
	l := make([]string, 0, len(opts))
	for _, o := range opts {
		if !strings.HasPrefix(o, comp) {
			continue
		}
		l = append(l, o)
	}
	return l
}

func main() {
	if len(os.Args) < 4 {
		os.Exit(0)
	}

	comp := os.Args[2]
	prev := os.Args[3]
	fl := ""
	if strings.HasPrefix(prev, "-") {
		fl = prev[1:]
	}

	comma := false

	opts := make([]string, 0, 100)
	switch fl {
	case flags.BaseDir:
		fallthrough
	case flags.CollectionDir:
		fallthrough
	case flags.JPEGDir:
		fallthrough
	case flags.SourceDir:
		return

	case flags.Actions:
		comma = true
		for i := range flags.AllActions {
			opts = append(opts, i)
		}

	case flags.GT:
		for i := 0; i < 5; i++ {
			opts = append(opts, strconv.Itoa(i))
		}
	case flags.LT:
		for i := 1; i <= 5; i++ {
			opts = append(opts, strconv.Itoa(i))
		}

	case flags.Checksum, flags.AlwaysYes, flags.Zero, flags.NoRawPrefix, flags.Verbose:
		fl = ""

	case flags.Undeleted:
		fl = ""
	case flags.Deleted:
		fl = ""
	case flags.Updated:
		fl = ""
	case flags.Edited:
		fl = ""
	case flags.Unedited:
		fl = ""
	case flags.Rated:
		fl = ""
	case flags.Unrated:
		fl = ""
	case flags.Location:
		fl = ""
	case flags.NoLocation:
		fl = ""
	case flags.Photo:
		fl = ""
	case flags.Video:
		fl = ""
	}

	if fl == "" {
		for i := range flags.AllFlags {
			opts = append(opts, "-"+i)
		}
	}

	cut := ""
	if comma {
		if strings.HasSuffix(comp, ",") {
			cut = comp[0 : len(comp)-1]
			comp = ""
		}
		if comp != "" {
			n := flags.CommaSep(comp)
			comp = n[len(n)-1]
			cut = strings.Join(n[:len(n)-1], ",")
		}
	}

	for _, n := range filter(opts, comp) {
		if cut != "" {
			os.Stdout.WriteString(cut + ",")
		}
		os.Stdout.WriteString(n)
		os.Stdout.WriteString("\n")
	}
}
