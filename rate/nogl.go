//go:build nogl
// +build nogl

package rate

import (
	"errors"
	"log"

	"github.com/frizinak/photos/importer"
)

type Rater struct{}

var err = errors.New("rater can't be launched as no GL backend available")

func New(log *log.Logger, tzOffset int, files []*importer.File, imp *importer.Importer) (*Rater, error) {
	return nil, err
}

func (r Rater) Run() error { return err }
