package gphotos

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
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

type Errorable interface {
	Err() error
}

func (g *GPhotos) do(req *http.Request, d Errorable) (*http.Response, error) {
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return res, err
	}

	defer res.Body.Close()

	err = json.NewDecoder(res.Body).Decode(d)
	if err != nil {
		return res, err
	}

	// buf := bytes.NewBuffer(nil)
	// rr := io.TeeReader(res.Body, buf)
	// err = json.NewDecoder(rr).Decode(d)
	// if err != nil {
	// 	return res, err
	// }

	return res, d.Err()
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
	return o, err
}

func (g *GPhotos) list(amount int, pageToken string) (ListResult, error) {
	req, err := g.req(
		"GET",
		"https://photoslibrary.googleapis.com/v1/mediaItems",
		nil,
	)

	var l ListResult
	if err != nil {
		return l, err
	}

	q := req.URL.Query()
	q.Set("pageSize", strconv.Itoa(amount))
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
	req.URL.RawQuery = q.Encode()
	_, err = g.do(req, &l)
	return l, err
}

func (g *GPhotos) List(minAmount int) ([]RealMediaItemResult, error) {
	all := make([]RealMediaItemResult, 0, minAmount)
	var pageToken string
	n := 100
	if n > minAmount {
		n = minAmount
	}
	for {
		d, err := g.list(n, pageToken)
		if err != nil {
			return all, err
		}

		all = append(all, d.MediaItems...)
		if d.NextPageToken == "" {
			break
		}
		if len(all) >= minAmount {
			break
		}

		pageToken = d.NextPageToken
	}

	return all, nil
}
