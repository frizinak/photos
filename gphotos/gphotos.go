package gphotos

import (
	"log"
)

type GPhotos struct {
	l          *log.Logger
	id, secret string
	cache      string
	state      struct {
		redirect           string
		codeVerifier       []byte
		codeVerifierLength int
	}
	t Token
}

func New(log *log.Logger, id, secret string, cacheFile string) *GPhotos {
	g := &GPhotos{l: log, id: id, secret: secret, cache: cacheFile}
	g.state.codeVerifierLength = 128
	return g
}
