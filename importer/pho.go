package importer

import (
	"io"
	"os"

	"github.com/frizinak/phodo/phodo"
	"github.com/frizinak/phodo/pipeline"
)

const PhoConvertTarget = ".convert"

type Pho struct {
	path string
	hash []byte
	root *pipeline.Root
}

func (p Pho) Edited() bool {
	_, ok := p.Convert()
	return ok
}

func (p Pho) Convert() (pipeline.NamedElement, bool) {
	return p.root.Get(PhoConvertTarget)
}

func (p Pho) Path() string { return p.path }

func (p Pho) Hash(w io.Writer) {
	w.Write(p.hash)
}

func (i *Importer) GetPho(link string) (Pho, error) {
	// TODO pass conf / verbose flag
	var pho Pho
	c := phodo.NewConf(os.Stderr, nil)
	root, err := phodo.LoadSidecar(c, link)
	pho.root = root
	pho.path = c.Script
	if err != nil {
		return pho, err
	}

	for _, v := range root.List() {
		pho.hash = append(pho.hash, v.Hash...)
	}

	return pho, err
}
