package gtimeline

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const SessID = "__Secure-3PSID"
const klmURL = "https://www.google.com/maps/timeline/kml?authuser=0&pb=!1m8!1m3!1i%d!2i%d!3i%d!2m3!1i%d!2i%d!3i%d"

var day = 24 * time.Hour

type data struct {
	Document Document
}

func URL(day time.Time) string {
	y, m, d := day.Year(), day.Month()-1, day.Day()
	return fmt.Sprintf(klmURL, y, m, d, y, m, d)
}

func Get(day time.Time, secure3PSID string) (Document, error) {
	d := data{}
	req, err := http.NewRequest("GET", URL(day), nil)
	if err != nil {
		return d.Document, err
	}
	req.AddCookie(&http.Cookie{Name: SessID, Value: secure3PSID})

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return d.Document, err
	}
	defer res.Body.Close()

	dec := xml.NewDecoder(res.Body)
	if _, err = dec.Token(); err != nil {
		return d.Document, err
	}
	err = dec.Decode(&d)
	return d.Document, err
}

type Documents struct {
	sessid string
	d      map[string]Document
	rw     sync.RWMutex
}

func New(secure3PSID string) *Documents {
	return &Documents{sessid: secure3PSID, d: make(map[string]Document)}
}

func (d *Documents) key(day time.Time) string { return day.Format("20060102") }
func (d *Documents) get(day time.Time) (Document, bool) {
	doc, ok := d.d[d.key(day)]
	return doc, ok
}

var ErrNoPlaceMark = errors.New("no placemark found")
var ErrNoLatLng = errors.New("no latlng found")

func (d *Documents) GetPlace(moment time.Time, extraDays int) (Placemark, time.Duration, error) {
	var p Placemark
	margin := time.Hour * 24 * time.Duration(extraDays)
	docs, err := d.GetDayRange(moment.Add(-margin), moment.Add(margin), 8, nil)
	if err != nil {
		return p, 0, err
	}

	closest := margin + time.Second
	chk := func(t time.Time) bool {
		d := moment.Sub(t)
		if d < 0 {
			d = -d
		}
		if d < closest && d < margin {
			closest = d
			return true
		}
		return false
	}

	for _, d := range docs {
		for _, pm := range d.Placemark {
			if !moment.Before(pm.TimeSpan.Begin) && !moment.After(pm.TimeSpan.End) {
				return pm, 0, nil
			}
			if chk(pm.TimeSpan.Begin) {
				p = pm
			}
			if chk(pm.TimeSpan.End) {
				p = pm
			}
		}
	}

	if closest > margin {
		err = ErrNoPlaceMark
	}

	return p, closest, err
}

func (d *Documents) GetLatLng(moment time.Time, extraDays int) (Placemark, LatLng, error) {
	var p Placemark
	var ll LatLng
	var err error
	var margin time.Duration
	p, margin, err = d.GetPlace(moment, extraDays)
	if err != nil {
		return p, ll, err
	}

	// exact only
	if margin != 0 {
		return p, ll, ErrNoPlaceMark
	}

	var ok bool
	if ll, ok = p.LatLng(moment); ok {
		return p, ll, nil
	}

	return p, ll, ErrNoLatLng
}

func (d *Documents) Get(day time.Time) (Document, error) {
	d.rw.RLock()
	if doc, ok := d.get(day); ok {
		d.rw.RUnlock()
		return doc, nil
	}
	d.rw.RUnlock()

	doc, err := Get(day, d.sessid)
	if err != nil {
		return doc, err
	}

	d.rw.Lock()
	d.d[d.key(day)] = doc
	d.rw.Unlock()
	return doc, err
}

func (d *Documents) GetDayRange(start, end time.Time, concurrent int, progress func(int, int)) ([]Document, error) {
	if progress == nil {
		progress = func(n, total int) {}
	}

	if concurrent < 1 {
		concurrent = 1
	}

	type result struct {
		doc Document
		err error
	}
	work := make(chan time.Time, concurrent)
	results := make(chan result, concurrent)
	var wg sync.WaitGroup
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func() {
			for w := range work {
				doc, err := d.Get(w)
				results <- result{doc, err}
			}
			wg.Done()
		}()
	}

	docs := make([]Document, 0)
	done := make(chan struct{}, 1)
	errs := make(chan error, 1)
	var total int
	n := 0
	go func() {
		for r := range results {
			n++
			progress(n, total)
			if r.err != nil {
				errs <- r.err
				continue
			}
			docs = append(docs, r.doc)
		}
		done <- struct{}{}
	}()

	var gerr error

	total = int(float64(end.Sub(start))/float64(day)) + 1
	progress(0, total)

outer:
	for s := start; !s.After(end); s = s.Add(day) {
	inner:
		for {
			select {
			case gerr = <-errs:
				break outer
			case work <- s:
				break inner
			}
		}
	}

	close(work)
	wg.Wait()
	close(results)
	if gerr != nil {
		return docs, gerr
	}

	select {
	case err := <-errs:
		return docs, err
	case <-done:
	}

	return docs, nil
}
