package gtimeline

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Document struct {
	Name      string `xml:"name"`
	Placemark []Placemark
}

type Placemark struct {
	Name       string `xml:"name"`
	Address    string `xml:"address"`
	Point      Point
	LineString LineString
	TimeSpan   TimeSpan
}

type LineString struct {
	Point
}

type TimeSpan struct {
	Begin time.Time `xml:"begin"`
	End   time.Time `xml:"end"`
}

type Point struct {
	Coordinates Coordinates `xml:"coordinates"`
}

type Coordinates string

func (p Placemark) LatLngs() []LatLng {
	n := p.Point.Coordinates.LatLng()
	n = append(n, p.LineString.Coordinates.LatLng()...)
	return n
}

func (p Placemark) LatLng(moment time.Time) (LatLng, bool) {
	l := p.LatLngs()
	if len(l) < 1 {
		return LatLng{}, false
	}
	if len(l) < 2 {
		return l[0], true
	}
	m := float64(moment.Unix())
	b := float64(p.TimeSpan.Begin.Unix())
	e := float64(p.TimeSpan.End.Unix())
	if m < b || m > e {
		return l[0], false
	}
	if b == e {
		return l[0], true
	}

	pct := 0.0
	pct = (m - b) / (e - b)
	max := 1.0 - 1e-5
	if pct > max {
		pct = max
	}
	return l[int(float64(len(l))*pct)], true
}

func (c Coordinates) LatLng() []LatLng {
	n := strings.Split(string(c), " ")
	ll := make([]LatLng, 0, len(n))
	var llf [2]float64
	for _, s := range n {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}

		lls := strings.Split(s, ",")
		if len(lls) < 2 {
			panic(fmt.Sprintf("invalid latlng string: %s", s))
		}

		for i := 0; i < 2; i++ {
			n, err := strconv.ParseFloat(lls[i], 64)
			if err != nil {
				panic(err)
			}
			llf[i] = n
		}

		ll = append(ll, LatLng{llf[1], llf[0]})
	}

	return ll
}

type LatLng struct {
	Lat, Lng float64
}
