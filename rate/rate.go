package rate

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/frizinak/photos/importer"
	"github.com/frizinak/photos/meta"
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
)

const (
	fs       = 4
	stride   = 4
	vertices = 4
)

type points [stride * vertices]float32

type update struct {
	Rating  int
	Deleted int
}

func buf(d *points, x0, y0, x1, y1 float32) {
	d[0] = x1
	d[1] = y1
	d[4] = x1
	d[5] = y0
	d[8] = x0
	d[9] = y0
	d[12] = x0
	d[13] = y1
	d[2], d[3] = 1, 1
	d[6], d[7] = 1, 0
	d[10], d[11] = 0, 0
	d[14], d[15] = 0, 1
}

func initialize() (*glfw.Window, error) {
	if err := glfw.Init(); err != nil {
		return nil, err
	}
	glfw.WindowHint(glfw.Resizable, glfw.True)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.DoubleBuffer, 1)
	window, err := glfw.CreateWindow(
		800,
		800,
		"photos rate",
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}

	window.MakeContextCurrent()
	return window, nil
}

type Rater struct {
	window                             *glfw.Window
	monitor                            *glfw.Monitor
	videoMode                          *glfw.VidMode
	windowX, windowY, windowW, windowH int

	realWidth, realHeight int

	fullscreen bool
	zoom       bool
	auto       bool

	index int

	invalidateVAOs bool

	proj mgl32.Mat4

	gErr error

	term struct {
		clrRed, clrRedContrast     string
		clrGreen, clrGreenContrast string
		clrBlue, clrBlueContrast   string
	}

	files []*importer.File
	log   *log.Logger
}

func New(log *log.Logger, files []*importer.File) *Rater {
	r := &Rater{files: files, log: log}
	r.term.clrRed = "\033[48;5;124m"
	r.term.clrRedContrast = "\033[38;5;231m"
	r.term.clrGreen = "\033[48;5;70m"
	r.term.clrGreenContrast = "\033[38;5;16m"
	r.term.clrBlue = "\033[48;5;56m"
	r.term.clrBlueContrast = "\033[38;5;231m"
	if term := os.Getenv("TERM"); !strings.Contains(term, "256color") {
		r.term.clrRed = "\033[41m"
		r.term.clrRedContrast = "\033[37m"
		r.term.clrGreen = "\033[42m"
		r.term.clrGreenContrast = ""
		r.term.clrBlue = "\033[44m"
		r.term.clrBlueContrast = "\033[37m"
	}

	return r
}

func (r *Rater) onKey(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
	if action == glfw.Release {
		return
	}
	// if key == glfw.KeyT && mods == glfw.ModControl {
	// 	tagMode = !tagMode
	// 	enabled := "enabled "
	// 	clr, clrContrast := termClrRed, termClrRedContrast
	// 	if !tagMode {
	// 		clr, clrContrast = termClrGreen, termClrGreenContrast
	// 		enabled = "disabled"
	// 	}
	// 	fmt.Printf("%s%s   tagmode %s    \033[0m\n", clr, clrContrast, enabled)
	// }
	// if tagMode {
	// 	return
	// }

	if key == glfw.KeyQ {
		r.window.SetShouldClose(true)
	}

	li := r.index
	upd := update{-1, -1}
	var changed bool
	var doprint bool
	var next bool

	switch key {
	case glfw.KeyF:
		r.toggleFS()
	case glfw.KeyZ:
		r.zoom = !r.zoom
	case glfw.KeyLeft:
		r.addIndex(-1)
	case glfw.KeyRight, glfw.KeySpace:
		r.addIndex(1)

	case glfw.KeyH:
		r.usage()

	case glfw.KeyA:
		r.auto = !r.auto
		enabled := "enabled"
		if !r.auto {
			enabled = "disabled"
		}
		fmt.Printf("auto switching images %s\n", enabled)

	case glfw.KeyD, glfw.KeyDelete:
		upd.Deleted = 1
		next = true
	case glfw.KeyU:
		upd.Deleted = 0

	case glfw.Key0:
		upd.Rating = 0
	case glfw.Key1:
		upd.Rating = 1
		next = true
	case glfw.Key2:
		upd.Rating = 2
		next = true
	case glfw.Key3:
		upd.Rating = 3
		next = true
	case glfw.Key4:
		upd.Rating = 4
		next = true
	case glfw.Key5:
		upd.Rating = 5
		next = true

	case glfw.KeyP:
		doprint = true
	}

	if next && r.auto {
		r.addIndex(1)
	}

	doprint = doprint || li != r.index

	if upd.Rating > -1 || upd.Deleted > -1 {
		changed = true
		r.updateMeta(r.getFile(li), func(m *meta.Meta) (bool, error) {
			if upd.Deleted > -1 {
				m.Deleted = upd.Deleted == 1
			}
			if upd.Rating > -1 {
				m.Rating = upd.Rating
			}
			return true, nil
		})
	}

	if changed {
		r.print(r.getFile(li), false)
	}
	if doprint {
		r.print(r.file(), true)
	}
}

func (r *Rater) file() *importer.File {
	return r.files[r.index]
}

func (r *Rater) getFile(index int) *importer.File {
	return r.files[index]
}

func (r *Rater) addIndex(i int) {
	r.setIndex(r.index + i)
}

func (r *Rater) setIndex(i int) {
	r.index = i
	if r.index < 0 {
		r.index = 0
	} else if r.index >= len(r.files) {
		r.index = len(r.files) - 1
	}
}

func (r *Rater) onResize(wnd *glfw.Window, width, height int) {
	r.realWidth, r.realHeight = width, height
	r.invalidateVAOs = true
	gl.Viewport(0, 0, int32(width), int32(height))
	r.proj = mgl32.Ortho2D(0, float32(width), float32(height), 0)
	if r.fullscreen {
		return
	}
	r.windowW, r.windowH = width, height
}
func (r *Rater) onPos(wnd *glfw.Window, x, y int) {
	if r.fullscreen {
		return
	}
	r.windowX, r.windowY = x, y
}

func (r *Rater) toggleFS() {
	r.fullscreen = !r.fullscreen
	if r.fullscreen {
		r.window.SetMonitor(r.monitor, 0, 0, r.videoMode.Width, r.videoMode.Height, r.videoMode.RefreshRate)
		return
	}
	r.window.SetMonitor(nil, r.windowX, r.windowY, r.windowW, r.windowH, r.videoMode.RefreshRate)
}

func (r *Rater) usage() {
	fmt.Print(`Usage:
q            : quit
f            : toggle fullscreen
h            : print this
z            : toggle zoom

a            : toggle automatically go to next image after deleting or rating
p            : print filename and meta

d | delete   : delete
u            : undelete

1-5          : rate 1-5
0            : remove rating

left | space : next
right        : previous
`)
}

func (r *Rater) fatal(err error) {
	if err != nil {
		r.window.SetShouldClose(true)
		r.gErr = err
	}
}

func (r *Rater) updateMeta(f *importer.File, mod func(*meta.Meta) (save bool, err error)) {
	m, err := importer.EnsureMeta(f)
	if err != nil {
		r.fatal(err)
		return
	}

	rm := &m
	if save, err := mod(rm); !save || err != nil {
		if err != nil {
			r.fatal(err)
		}
		return
	}

	if err := importer.SaveMeta(f, *rm); err != nil {
		r.fatal(err)
	}
}

func (r *Rater) print(f *importer.File, fn bool) {
	var met meta.Meta
	r.updateMeta(f, func(m *meta.Meta) (bool, error) {
		met = *m
		return false, nil
	})
	if fn {
		fmt.Println()
		fmt.Printf(
			"\033[1m%s%s   %d/%d   \033[0m\n%s [%s]\n",
			r.term.clrBlue,
			r.term.clrBlueContrast,
			r.index+1,
			len(r.files),
			f.Filename(),
			filepath.Base(importer.NicePath("", f, met)),
		)
	}

	delString := fmt.Sprintf("\033[1m%s%s  keep  \033[0m", r.term.clrGreen, r.term.clrGreenContrast)
	if met.Deleted {
		delString = fmt.Sprintf("\033[1m%s%s delete \033[0m", r.term.clrRed, r.term.clrRedContrast)
	}

	color := r.term.clrRed
	colorContrast := r.term.clrRedContrast
	if met.Rating > 2 {
		color = r.term.clrGreen
		colorContrast = r.term.clrGreenContrast
	}

	fmt.Printf("%s %s%s %d/5 \033[0m\n", delString, color, colorContrast, met.Rating)
}

func (r *Rater) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if len(r.files) == 0 {
		return errors.New("no files")
	}
	var err error
	r.window, err = initialize()
	defer glfw.Terminate()
	if err != nil {
		return err
	}
	r.monitor = glfw.GetPrimaryMonitor()
	r.videoMode = r.monitor.GetVideoMode()
	r.windowX, r.windowY = r.window.GetPos()
	r.windowW, r.windowH = r.window.GetSize()
	r.zoom = false
	r.invalidateVAOs = false
	r.fullscreen = false
	r.proj = mgl32.Ortho2D(0, 800, 800, 0)
	r.index = 0

	r.usage()
	r.print(r.file(), true)

	// tagMode := false
	// onText := func(w *glfw.Window, char rune) {
	// 	if !tagMode {
	// 		return
	// 	}

	// 	fmt.Println(string(char))
	// }

	if err := gl.Init(); err != nil {
		return err
	}

	r.window.SetFramebufferSizeCallback(r.onResize)
	r.window.SetPosCallback(r.onPos)
	r.window.SetKeyCallback(r.onKey)
	// window.SetCharCallback(onText)
	w, h := r.window.GetFramebufferSize()
	r.onResize(r.window, w, h)

	program, err := newProgram()
	if err != nil {
		return err
	}
	gl.UseProgram(program)
	gl.Enable(gl.TEXTURE_2D)

	textures := make([]uint32, len(r.files))
	vaos := make([]uint32, len(r.files))
	vbos := make([]uint32, len(r.files))
	vaosState := make([]bool, len(r.files))
	dimensions := make([]image.Point, len(r.files))
	maxTex := 100
	var tex uint32 = 0
	var vao uint32 = 0
	var dimension image.Point
	model := mgl32.Ident4()

	lastProjection := mgl32.Ident4()
	lastI := -1
	var lastTex uint32 = 0

	modelUniform := gl.GetUniformLocation(program, gl.Str("model\x00"))
	projectionUniform := gl.GetUniformLocation(program, gl.Str("projection\x00"))

	var ebo uint32
	indices := []uint32{0, 1, 3, 1, 2, 3}
	gl.GenBuffers(1, &ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, 6*fs, gl.Ptr(indices), gl.STATIC_DRAW)

	newEntry := func(index int, bounds image.Rectangle) {
		if vaos[index] != 0 {
			return
		}

		d := points{}
		buf(&d, 0, 0, float32(bounds.Dx()), float32(bounds.Dy()))
		var vao, vbo uint32
		gl.GenVertexArrays(1, &vao)
		gl.GenBuffers(1, &vbo)

		gl.BindVertexArray(vao)

		gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ebo)
		gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
		gl.BufferData(gl.ARRAY_BUFFER, stride*vertices*fs, gl.Ptr(&d[0]), gl.DYNAMIC_DRAW)

		gl.EnableVertexAttribArray(0)
		gl.VertexAttribPointer(0, 2, gl.FLOAT, false, stride*fs, gl.PtrOffset(0))
		gl.EnableVertexAttribArray(1)
		gl.VertexAttribPointer(1, 2, gl.FLOAT, false, stride*fs, gl.PtrOffset(2*fs))

		gl.BindBuffer(gl.ARRAY_BUFFER, 0)
		gl.BindVertexArray(0)
		gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, 0)

		vaos[index] = vao + 1
		vbos[index] = vbo + 1
		dimensions[index] = image.Pt(bounds.Dx(), bounds.Dy())
	}

	getVAO := func(index int) (uint32, image.Point) {
		s := vaosState[index]
		dims := dimensions[index]
		if r.zoom {
			if dims.X == 0 || dims.Y == 0 {
				dims.X, dims.Y = 1, 1
			}
			rat := float64(dims.X) / float64(dims.Y)
			dims.X, dims.Y = r.realWidth, int(float64(r.realWidth)/rat)
			if float64(r.realHeight)/float64(dims.Y) < float64(r.realWidth)/float64(dims.X) {
				dims.X, dims.Y = int(float64(r.realHeight)*rat), r.realHeight
			}
		}

		if !r.invalidateVAOs && s == r.zoom {
			return vaos[index], dims
		}
		if vbos[index] == 0 {
			return vaos[index], dims
		}

		r.invalidateVAOs = false
		gl.BindBuffer(gl.ARRAY_BUFFER, vbos[index]-1)

		d := points{}
		if !r.zoom {
			vaosState[index] = false
			buf(&d, 0, 0, float32(dims.X), float32(dims.Y))
			gl.BufferData(gl.ARRAY_BUFFER, stride*vertices*fs, gl.Ptr(&d[0]), gl.DYNAMIC_DRAW)
			gl.BindBuffer(gl.ARRAY_BUFFER, 0)
			return vaos[index], dims
		}

		vaosState[index] = true
		buf(&d, 0, 0, float32(dims.X), float32(dims.Y))
		gl.BufferData(gl.ARRAY_BUFFER, stride*vertices*fs, gl.Ptr(&d[0]), gl.DYNAMIC_DRAW)
		gl.BindBuffer(gl.ARRAY_BUFFER, 0)
		return vaos[index], dims
	}

	update := func() error {
		vao, dimension = getVAO(r.index)
		if r.index == lastI {
			return nil
		}

		lastI = r.index
		if textures[r.index] != 0 {
			tex = textures[r.index]
			return nil
		}

		f, err := importer.GetPreview(r.file())
		if err != nil {
			log.Printf("WARN could not get preview for %s: %s", r.file().Path(), err)
			tex = 0
			return nil
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			return err
		}

		bounds := img.Bounds()
		imgRGBA := image.NewRGBA(bounds)
		draw.Draw(imgRGBA, bounds, img, image.Point{}, draw.Src)
		newEntry(r.index, bounds)
		stex, err := imgTexture(imgRGBA)

		tex = stex + 1
		vao, dimension = getVAO(r.index)

		if err != nil {
			return err
		}
		textures[r.index] = tex

		for i := 0; i < len(textures); i++ {
			if textures[i] == 0 {
				continue
			}
			if i > r.index-maxTex/2 && i < r.index+maxTex/2 {
				continue
			}
			err = releaseTexture(textures[i] - 1)
			if err != nil {
				return err
			}
			textures[i] = 0
		}
		return nil
	}

	var lastDim image.Point
	frame := func() error {
		if err = update(); err != nil {
			return err
		}
		if tex == 0 {
			return nil
		}
		recenter := false
		if tex != lastTex {
			lastTex = tex
			gl.BindTexture(gl.TEXTURE_2D, uint32(tex-1))
			gl.BindVertexArray(vao - 1)
		}

		if r.proj != lastProjection {
			gl.UniformMatrix4fv(projectionUniform, 1, false, &r.proj[0])
			lastProjection = r.proj
			recenter = true
		}

		if dimension != lastDim {
			lastDim = dimension
			recenter = true
		}

		if recenter {
			tx := r.realWidth/2 - dimension.X/2
			ty := r.realHeight/2 - dimension.Y/2
			model = mgl32.Translate3D(float32(tx), float32(ty), 0)
			gl.UniformMatrix4fv(modelUniform, 1, false, &model[0])
		}

		gl.DrawElements(gl.TRIANGLES, 6, gl.UNSIGNED_INT, gl.PtrOffset(0))
		return nil
	}

	for !r.window.ShouldClose() {
		gl.Clear(gl.COLOR_BUFFER_BIT)
		if err = frame(); err != nil {
			return err
		}
		r.window.SwapBuffers()
		glfw.PollEvents()
	}

	return r.gErr
}
