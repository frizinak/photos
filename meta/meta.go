package meta

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"time"

	"github.com/frizinak/binary"
	"github.com/frizinak/photos/tags"
	jsoniter "github.com/json-iterator/go"
)

var (
	metaVersion    = []byte{'M', 0}
	oldJSONVersion = []byte{'{', '"'}
)

type Tags []string

func (t Tags) Map() map[string]struct{} {
	tm := make(map[string]struct{}, len(t))
	for _, tag := range t {
		tm[tag] = struct{}{}
	}
	return tm
}

func (t Tags) Unique() Tags {
	nt := make(Tags, 0, len(t))
	for i := range t.Map() {
		nt = append(nt, i)
	}
	sort.Strings(nt)
	return nt
}

func (t Tags) Contains(tag string) bool {
	for _, t := range t {
		if t == tag {
			return true
		}
	}

	return false
}

func (t Tags) decode(r *binary.Reader) Tags {
	n := int(r.ReadUint32())
	if t == nil {
		t = make(Tags, 0, n)
	}
	for i := 0; i < n; i++ {
		t = append(t, r.ReadString(16))
	}

	return t
}

func (t Tags) encode(w *binary.Writer) {
	w.WriteUint32(uint32(len(t)))
	for _, tag := range t {
		w.WriteString(tag, 16)
	}
}

type Location struct {
	Lat     float64
	Lng     float64
	Name    string
	Address string
}

func (l Location) decode(r *binary.Reader) Location {
	l.Lat = math.Float64frombits(r.ReadUint64())
	l.Lng = math.Float64frombits(r.ReadUint64())
	l.Name = r.ReadString(32)
	l.Address = r.ReadString(32)

	return l
}

func (l Location) encode(w *binary.Writer) {
	w.WriteUint64(math.Float64bits(l.Lat))
	w.WriteUint64(math.Float64bits(l.Lng))
	w.WriteString(l.Name, 32)
	w.WriteString(l.Address, 32)
}

type Converted struct {
	Hash string
	Size int
}

func (c Converted) decode(r *binary.Reader) Converted {
	c.Hash = r.ReadString(16)
	c.Size = int(r.ReadUint32())
	return c
}

func (c Converted) encode(w *binary.Writer) {
	w.WriteString(c.Hash, 16)
	w.WriteUint32(uint32(c.Size))
}

type Meta struct {
	Checksum     string
	Size         int64
	RealFilename string
	BaseFilename string
	Created      int64

	Deleted bool
	Rating  uint8

	Conv map[string]Converted

	Tags     Tags
	Location *Location

	CameraInfo *tags.CameraInfo
}

func (m Meta) decode(r *binary.Reader) Meta {
	m.Checksum = r.ReadString(16)
	m.Size = int64(r.ReadUint32())
	m.RealFilename = r.ReadString(16)
	m.BaseFilename = r.ReadString(16)
	m.Created = int64(r.ReadUint32())
	m.Deleted = r.ReadUint8() == 1
	m.Rating = r.ReadUint8()
	nconv := int(r.ReadUint32())
	m.Conv = make(map[string]Converted, nconv)
	for i := 0; i < nconv; i++ {
		k := r.ReadString(16)
		m.Conv[k] = Converted{}.decode(r)
	}

	m.Tags = m.Tags.decode(r)

	if r.ReadUint8() == 1 {
		loc := Location{}.decode(r)
		m.Location = &loc
	}
	if r.ReadUint8() == 1 {
		cam := tags.CameraInfo{}.BinaryDecode(r)
		m.CameraInfo = &cam
	}

	return m
}

func (m Meta) encode(w *binary.Writer) {
	w.WriteString(m.Checksum, 16)
	w.WriteUint32(uint32(m.Size))

	w.WriteString(m.RealFilename, 16)
	w.WriteString(m.BaseFilename, 16)
	w.WriteUint32(uint32(m.Created))

	var del uint8
	if m.Deleted {
		del = 1
	}

	w.WriteUint8(del)

	w.WriteUint8(m.Rating)

	srt := make([]string, 0, len(m.Conv))
	for k := range m.Conv {
		srt = append(srt, k)
	}
	sort.Strings(srt)

	w.WriteUint32(uint32(len(srt)))
	for _, k := range srt {
		w.WriteString(k, 16)
		m.Conv[k].encode(w)
	}

	m.Tags.encode(w)

	func() {
		if m.Location == nil {
			w.WriteUint8(0)
			return
		}
		w.WriteUint8(1)
		m.Location.encode(w)
	}()

	func() {
		if m.CameraInfo == nil {
			w.WriteUint8(0)
			return
		}
		w.WriteUint8(1)
		m.CameraInfo.BinaryEncode(w)
	}()
}

func New(size int64, real string, base string) Meta {
	m := Meta{}
	m.Size = size
	m.RealFilename = real
	m.BaseFilename = base
	return m
}

func (m Meta) CreatedTime() time.Time {
	return time.Unix(m.Created, 0)
}

var j = jsoniter.Config{
	EscapeHTML:                    false,
	ObjectFieldMustBeSimpleString: true,
}.Froze()

func loadOld(r io.Reader) (Meta, error) {
	var m Meta
	err := j.NewDecoder(r).Decode(&m)
	return m, err
}

func Load(path string) (Meta, error) {
	var m Meta
	// files should be tiny. so just alloc once.
	d, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}

	if len(d) < len(metaVersion) {
		return m, fmt.Errorf("invalid meta file '%s'", path)
	}
	version := d[:len(metaVersion)]
	d = d[len(version):]

	buf := bytes.NewReader(d)
	if bytes.Equal(version, oldJSONVersion) {
		return loadOld(buf)
	}

	if !bytes.Equal(version, metaVersion) {
		return m, fmt.Errorf("meta version mismatch: %+v != expected %+v in '%s'", version, metaVersion, path)
	}

	r := binary.NewReader(buf)
	m = m.decode(r)

	if err = r.Err(); err != nil {
		err = fmt.Errorf("could not load meta %s: %w", path, err)
	}
	return m, err
}

func (m Meta) Save(path string) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	m.Tags = m.Tags.Unique()
	_, err = f.Write(metaVersion)
	if err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}

	w := binary.NewWriter(f)
	m.encode(w)
	f.Close()
	if err := w.Err(); err != nil {
		os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, path)
}
