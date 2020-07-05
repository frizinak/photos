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
	"unsafe"

	"github.com/frizinak/photos/importer"
	"github.com/frizinak/photos/meta"
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
)

func init() {
	runtime.LockOSThread()
}

const (
	FS       = 4
	Stride   = 4
	Vertices = 4
)

type Points [Stride * Vertices]float32

func Buf(d *Points, x0, y0, x1, y1 float32) {
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

func Run(log *log.Logger, files []*importer.File) error {
	var gErr error
	if len(files) == 0 {
		return errors.New("no files")
	}
	window, err := initialize()
	defer glfw.Terminate()
	if err != nil {
		return err
	}
	monitor := glfw.GetPrimaryMonitor()
	videoMode := monitor.GetVideoMode()
	var windowX, windowY int = window.GetPos()
	var windowW, windowH int = window.GetSize()
	fullscreen := false
	proj := mgl32.Ortho2D(0, 800, 800, 0)
	index := 0

	doError := func(err error) {
		if err != nil {
			window.SetShouldClose(true)
			gErr = err
		}
	}

	updateMeta := func(f *importer.File, mod func(*meta.Meta) (save bool, err error)) {
		m, err := importer.EnsureMeta(f)
		if err != nil {
			doError(err)
			return
		}

		rm := &m
		if save, err := mod(rm); !save || err != nil {
			if err != nil {
				doError(err)
			}
			return
		}

		if err := importer.SaveMeta(f, *rm); err != nil {
			doError(err)
		}
	}

	termClrRed := "\033[48;5;124m"
	termClrRedContrast := "\033[38;5;231m"
	termClrGreen := "\033[48;5;70m"
	termClrGreenContrast := "\033[38;5;16m"
	termClrBlue := "\033[48;5;56m"
	termClrBlueContrast := "\033[38;5;231m"
	if term := os.Getenv("TERM"); !strings.Contains(term, "256color") {
		termClrRed = "\033[41m"
		termClrRedContrast = "\033[37m"
		termClrGreen = "\033[42m"
		termClrGreenContrast = ""
		termClrBlue = "\033[44m"
		termClrBlueContrast = "\033[37m"
	}

	print := func(f *importer.File, fn bool) {
		var met meta.Meta
		updateMeta(f, func(m *meta.Meta) (bool, error) {
			met = *m
			return false, nil
		})
		if fn {
			fmt.Println()
			fmt.Printf(
				"\033[1m%s%s   %d/%d   \033[0m\n%s [%s]\n",
				termClrBlue,
				termClrBlueContrast,
				index+1,
				len(files),
				f.Filename(),
				filepath.Base(importer.NicePath("", f, met)),
			)
		}

		delString := fmt.Sprintf("\033[1m%s%s  keep  \033[0m", termClrGreen, termClrGreenContrast)
		if met.Deleted {
			delString = fmt.Sprintf("\033[1m%s%s delete \033[0m", termClrRed, termClrRedContrast)
		}

		color := termClrRed
		colorContrast := termClrRedContrast
		if met.Rating > 2 {
			color = termClrGreen
			colorContrast = termClrGreenContrast
		}

		fmt.Printf("%s %s%s %d/5 \033[0m\n", delString, color, colorContrast, met.Rating)
	}

	type Update struct {
		Rating  int
		Deleted int
	}
	var auto bool

	usage := func() {
		fmt.Print(`Usage:
q            : quit
f            : toggle fullscreen
h            : print this

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
	usage()

	print(files[0], true)

	var realWidth, realHeight int
	onResize := func(wnd *glfw.Window, width, height int) {
		realWidth, realHeight = width, height
		gl.Viewport(0, 0, int32(width), int32(height))
		proj = mgl32.Ortho2D(0, float32(width), float32(height), 0)
		if fullscreen {
			return
		}
		windowW, windowH = width, height
	}
	onPos := func(wnd *glfw.Window, x, y int) {
		if fullscreen {
			return
		}
		windowX, windowY = x, y
	}
	onScroll := func(wnd *glfw.Window, x, y float64) {
	}
	toggleFS := func() {
		fullscreen = !fullscreen
		if fullscreen {
			window.SetMonitor(monitor, 0, 0, videoMode.Width, videoMode.Height, videoMode.RefreshRate)
			return
		}
		window.SetMonitor(nil, windowX, windowY, windowW, windowH, videoMode.RefreshRate)
	}

	checkIndex := func() {
		if index < 0 {
			index = 0
		} else if index >= len(files) {
			index = len(files) - 1
		}
	}

	onKey := func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action == glfw.Release {
			return
		}

		if key == glfw.KeyQ {
			window.SetShouldClose(true)
		}

		if key == glfw.KeyF {
			toggleFS()
		}

		li := index
		update := Update{-1, -1}
		var changed bool
		var doprint bool
		var next bool

		switch key {
		case glfw.KeyLeft:
			index--
		case glfw.KeyRight, glfw.KeySpace:
			index++

		case glfw.KeyH:
			usage()

		case glfw.KeyA:
			auto = !auto
			enabled := "enabled"
			if !auto {
				enabled = "disabled"
			}
			fmt.Printf("auto switching images %s\n", enabled)

		case glfw.KeyD, glfw.KeyDelete:
			update.Deleted = 1
			next = true
		case glfw.KeyU:
			update.Deleted = 0

		case glfw.Key0:
			update.Rating = 0
		case glfw.Key1:
			update.Rating = 1
			next = true
		case glfw.Key2:
			update.Rating = 2
			next = true
		case glfw.Key3:
			update.Rating = 3
			next = true
		case glfw.Key4:
			update.Rating = 4
			next = true
		case glfw.Key5:
			update.Rating = 5
			next = true

		case glfw.KeyP:
			doprint = true
		}

		if next && auto {
			index++
		}

		checkIndex()

		doprint = doprint || li != index

		if update.Rating > -1 || update.Deleted > -1 {
			changed = true
			updateMeta(files[li], func(m *meta.Meta) (bool, error) {
				if update.Deleted > -1 {
					m.Deleted = update.Deleted == 1
				}
				if update.Rating > -1 {
					m.Rating = update.Rating
				}
				return true, nil
			})
		}

		if changed {
			print(files[li], false)
		}
		if doprint {
			print(files[index], true)
		}
	}

	if err := gl.Init(); err != nil {
		return err
	}

	window.SetFramebufferSizeCallback(onResize)
	window.SetPosCallback(onPos)
	window.SetKeyCallback(onKey)
	window.SetScrollCallback(onScroll)
	w, h := window.GetFramebufferSize()
	onResize(window, w, h)

	program, err := newProgram()
	if err != nil {
		return err
	}
	gl.UseProgram(program)
	gl.Enable(gl.TEXTURE_2D)

	onGLError := func(source uint32, gltype uint32, id uint32, severity uint32, length int32, message string, userParam unsafe.Pointer) {
		log.Printf("GL debug message: %s\n", message)
	}
	gl.DebugMessageCallback(onGLError, nil)
	gl.Enable(gl.DEBUG_OUTPUT)

	textures := make([]uint32, len(files))
	vaos := make([]uint32, len(files))
	dimensions := make([]image.Point, len(files))
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
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, 6*FS, gl.Ptr(indices), gl.STATIC_DRAW)

	newEntry := func(index int, bounds image.Rectangle) {
		if vaos[index] != 0 {
			return
		}

		d := Points{}
		Buf(&d, 0, 0, float32(bounds.Dx()), float32(bounds.Dy()))
		var vao, vbo uint32
		gl.GenVertexArrays(1, &vao)
		gl.GenBuffers(1, &vbo)

		gl.BindVertexArray(vao)

		gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ebo)
		gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
		gl.BufferData(gl.ARRAY_BUFFER, Stride*Vertices*FS, gl.Ptr(&d[0]), gl.DYNAMIC_DRAW)

		gl.EnableVertexAttribArray(0)
		gl.VertexAttribPointer(0, 2, gl.FLOAT, false, Stride*FS, gl.PtrOffset(0))
		gl.EnableVertexAttribArray(1)
		gl.VertexAttribPointer(1, 2, gl.FLOAT, false, Stride*FS, gl.PtrOffset(2*FS))

		gl.BindBuffer(gl.ARRAY_BUFFER, 0)
		gl.BindVertexArray(0)
		gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, 0)

		vaos[index] = vao + 1
		dimensions[index] = image.Pt(bounds.Dx(), bounds.Dy())
	}

	update := func() error {
		if index == lastI {
			return nil
		}
		vao = vaos[index]
		dimension = dimensions[index]

		lastI = index
		if textures[index] != 0 {
			tex = textures[index]
			return nil
		}

		f, err := importer.GetPreview(files[index])
		if err != nil {
			log.Printf("WARN could not get preview for %s: %s", files[index].Path(), err)
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
		newEntry(index, bounds)
		stex, err := ImgTexture(imgRGBA)

		tex = stex + 1
		vao = vaos[index]
		dimension = dimensions[index]

		if err != nil {
			return err
		}
		textures[index] = tex

		for i := 0; i < len(textures); i++ {
			if textures[i] == 0 {
				continue
			}
			if i > index-maxTex/2 && i < index+maxTex/2 {
				continue
			}
			err = ReleaseTexture(textures[i] - 1)
			if err != nil {
				return err
			}
			textures[i] = 0
		}
		return nil
	}

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
			recenter = true
		}

		if proj != lastProjection {
			gl.UniformMatrix4fv(projectionUniform, 1, false, &proj[0])
			lastProjection = proj
			recenter = true
		}

		if recenter {
			tx := realWidth/2 - dimension.X/2
			ty := realHeight/2 - dimension.Y/2
			model = mgl32.Translate3D(float32(tx), float32(ty), 0)
			gl.UniformMatrix4fv(modelUniform, 1, false, &model[0])
		}

		gl.DrawElements(gl.TRIANGLES, 6, gl.UNSIGNED_INT, gl.PtrOffset(0))
		return nil
	}

	for !window.ShouldClose() {
		gl.Clear(gl.COLOR_BUFFER_BIT)
		if err = frame(); err != nil {
			return err
		}
		window.SwapBuffers()
		glfw.PollEvents()
	}

	return gErr
}
