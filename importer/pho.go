package importer

import (
	"io"

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

func (p Pho) Path() string     { return p.path }
func (p Pho) Hash(w io.Writer) { w.Write(p.hash) }

func (i *Importer) GetPho(link string) (Pho, error) {
	var pho Pho
	conf, err := i.phodoConf()
	if err != nil {
		return pho, err
	}
	root, err := phodo.LoadSidecar(conf, link)
	if err != nil {
		return pho, err
	}

	pho.root = root
	pho.path, err = phodo.SidecarPath(conf, link)
	if err != nil {
		return pho, err
	}

	for _, v := range root.List() {
		pho.hash = append(pho.hash, v.Hash...)
	}

	return pho, err
}
