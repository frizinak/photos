package gphotos

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
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
	t   Token
	sem sync.Mutex
}

func New(log *log.Logger, id, secret string, cacheFile string) *GPhotos {
	g := &GPhotos{l: log, id: id, secret: secret, cache: cacheFile}
	g.state.codeVerifierLength = 128
	return g
}

func (g *GPhotos) req(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return req, err
	}

	err = g.AuthHeader(req.Header)
	return req, err
}

func (g *GPhotos) body(d interface{}) (io.Reader, error) {
	buf := bytes.NewBuffer(nil)
	return buf, json.NewEncoder(buf).Encode(d)
}

func (g *GPhotos) reqJSON(method, url string, d interface{}) (*http.Request, error) {
	body, err := g.body(d)
	if err != nil {
		return nil, err
	}

	return g.req(method, url, body)
}

func (g *GPhotos) do(req *http.Request, d interface{}) (*http.Response, error) {
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return res, err
	}

	defer res.Body.Close()
	// raw, err := ioutil.ReadAll(res.Body)
	// if err != nil {
	// 	return res, err
	// }
	// fmt.Println(string(raw))
	// return res, errors.New("debug")
	return res, json.NewDecoder(res.Body).Decode(d)
}

func (g *GPhotos) Get(id string) (MediaItemResult, error) {
	var m MediaItemResult
	o, err := g.GetBatch([]string{id})
	if err != nil {
		return m, err
	}

	return o.MediaItemResults[0], nil
}

func (g *GPhotos) GetBatch(ids []string) (BatchGetResult, error) {
	var o BatchGetResult
	req, err := g.req(
		"GET",
		"https://photoslibrary.googleapis.com/v1/mediaItems:batchGet",
		nil,
	)
	if err != nil {
		return o, err
	}

	q := req.URL.Query()
	for _, id := range ids {
		q.Add("mediaItemIds", id)
	}
	req.URL.RawQuery = q.Encode()
	_, err = g.do(req, &o)
	return o, nil
}

//func (g*GPhotos) List
