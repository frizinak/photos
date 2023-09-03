package gphotos

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

type UploadTask interface {
	Open() (io.Reader, error)
	Close()
	Filename() string
	Mime() string
	Description() string
}

type SimpleUploadTask struct {
	r                           io.Reader
	filename, description, mime string
}

func NewSimpleUploadTask(filename, description, mime string, r io.Reader) *SimpleUploadTask {
	return &SimpleUploadTask{r, filename, description, mime}
}

func (s *SimpleUploadTask) Open() (io.Reader, error) { return s.r, nil }
func (s *SimpleUploadTask) Filename() string         { return s.filename }
func (s *SimpleUploadTask) Description() string      { return s.description }
func (s *SimpleUploadTask) Mime() string             { return s.mime }
func (s *SimpleUploadTask) Close()                   {}

type FileUploadTask struct {
	r           io.ReadCloser
	path        string
	description string
}

func NewFileUploadTask(path, description string) *FileUploadTask {
	return &FileUploadTask{path: path, description: description}
}

func (f FileUploadTask) Open() (io.Reader, error) {
	r, err := os.Open(f.path)
	if err == nil {
		f.r = r
	}
	return r, err
}
func (f FileUploadTask) Close() {
	if f.r != nil {
		f.r.Close()
	}
}

func (f FileUploadTask) Filename() string    { return filepath.Base(f.path) }
func (f FileUploadTask) Description() string { return f.description }
func (f FileUploadTask) Mime() string        { return mime.TypeByExtension(filepath.Ext(f.path)) }

func (g *GPhotos) batchCreate(c BatchCreate) (BatchCreateResult, error) {
	var o BatchCreateResult
	req, err := g.reqJSON(
		"POST",
		"https://photoslibrary.googleapis.com/v1/mediaItems:batchCreate",
		c,
	)

	if err != nil {
		return o, err
	}

	if _, err = g.do(req, &o); err != nil {
		return o, err
	}

	if len(o.NewMediaItems) == 0 {
		return o, errors.New("invalid response, seems no images where created")
	}

	return o, o.Err()
}

func (g *GPhotos) create(uploadID, filename, description string) (BatchCreateResult, error) {
	return g.batchCreate(
		BatchCreate{
			NewMediaItems: []*MediaItem{
				{
					Description: description,
					SimpleMediaItem: SimpleMediaItem{
						Filename:    filename,
						UploadToken: uploadID,
					},
				},
			},
		},
	)
}

func (g *GPhotos) upload(name, mime string, r io.Reader) (string, error) {
	req, err := g.req("POST", "https://photoslibrary.googleapis.com/v1/uploads", r)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Goog-Upload-Content-Type", mime)
	req.Header.Set("X-Goog-Upload-Protocol", "raw")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	d, err := io.ReadAll(res.Body)
	return string(d), err
}

func (g *GPhotos) BatchUpload(parallel int, tasks []UploadTask, progress func(uint32, uint32)) error {
	if progress == nil {
		progress = func(n, total uint32) {}
	}

	const maxBatch = 50
	var wg sync.WaitGroup
	if parallel < 1 {
		parallel = 1
	}

	type token struct {
		task  UploadTask
		token string
	}

	total := uint32(len(tasks))
	var n uint32
	work := make(chan UploadTask, parallel)
	tokens := make(chan *token, maxBatch)
	errs := make(chan error, parallel)
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			for t := range work {
				fh, err := t.Open()
				if err != nil {
					errs <- err
					break
				}
				tok, err := g.upload(
					t.Filename(),
					t.Mime(),
					fh,
				)
				progress(atomic.AddUint32(&n, 1), total)
				t.Close()
				if err != nil {
					errs <- err
					break
				}

				tokens <- &token{t, tok}
			}
			wg.Done()
		}()
	}

	berrs := make(chan error, 1)
	done := make(chan struct{}, 1)
	var result BatchCreateResult
	go func() {
		do := func(list []*token) error {
			if len(list) == 0 {
				return nil
			}
			c := BatchCreate{NewMediaItems: make([]*MediaItem, len(list))}
			for i, item := range list {
				c.NewMediaItems[i] = &MediaItem{
					Description: item.task.Description(),
					SimpleMediaItem: SimpleMediaItem{
						Filename:    item.task.Filename(),
						UploadToken: item.token,
					},
				}
			}

			o, err := g.batchCreate(c)
			result.NewMediaItems = append(result.NewMediaItems, o.NewMediaItems...)
			return err
		}

		list := make([]*token, 0, maxBatch)
		for t := range tokens {
			list = append(list, t)
			if len(list) == maxBatch {
				err := do(list)
				list = list[0:0]
				if err != nil {
					berrs <- err
					done <- struct{}{}
					return
				}
			}
		}

		if err := do(list); err != nil {
			berrs <- err
		}
		done <- struct{}{}
	}()

	var gerr error
outer:
	for _, t := range tasks {
	inner:
		for {
			select {
			case err := <-berrs:
				gerr = err
				break outer
			case err := <-errs:
				gerr = err
				break outer
			case work <- t:
				break inner
			}
		}
	}

	close(work)
	wg.Wait()
	close(tokens)
	if gerr != nil {
		return gerr
	}
	select {
	case err := <-berrs:
		return err
	case <-done:
	}

	return nil
}

func (g *GPhotos) UploadJPEG(name string, r io.Reader) error {
	token, err := g.upload(name, "image/jpeg", r)
	if err != nil {
		return err
	}
	_, err = g.create(token, name, "")
	return err
}
