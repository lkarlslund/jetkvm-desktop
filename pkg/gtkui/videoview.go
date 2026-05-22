package gtkui

import (
	"image"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
	"github.com/lkarlslund/jetkvm-desktop/pkg/video"
)

// VideoView wraps a GtkGLArea that renders decoded video frames and
// forwards pointer/keyboard events to a session.Controller.
type VideoView struct {
	GLArea *gtk.GLArea

	mu        sync.Mutex
	gl        glState
	ready     bool
	lastStamp time.Time
	frameW    int
	frameH    int
	ycbcr     bool
	keyboard  *input.Keyboard
	buttons   byte
	lastMX    float64
	lastMY    float64

	app  *Application
	ctrl *session.Controller
}

func NewVideoView(app *Application, ctrl *session.Controller) *VideoView {
	v := &VideoView{
		GLArea:   gtk.NewGLArea(),
		app:      app,
		ctrl:     ctrl,
		keyboard: input.NewKeyboard(),
	}
	v.GLArea.SetAutoRender(false)
	v.GLArea.SetUseES(true)
	v.GLArea.SetHExpand(true)
	v.GLArea.SetVExpand(true)
	v.GLArea.SetFocusable(true)

	v.GLArea.ConnectRealize(v.onRealize)
	v.GLArea.ConnectUnrealize(v.onUnrealize)
	v.GLArea.ConnectRender(v.onRender)

	v.setupInput()
	return v
}

func (v *VideoView) onRealize() {
	v.GLArea.MakeCurrent()
	if err := v.GLArea.Error(); err != nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ready = glInit(&v.gl)
}

func (v *VideoView) onUnrealize() {
	v.GLArea.MakeCurrent()
	v.mu.Lock()
	defer v.mu.Unlock()
	glCleanup(&v.gl)
	v.ready = false
}

func (v *VideoView) onRender(_ gdk.GLContexter) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.ready {
		return true
	}

	if v.ctrl != nil {
		img, at := v.ctrl.LatestFrameInfo()
		if img != nil && at.After(v.lastStamp) {
			v.lastStamp = at
			v.uploadFrame(img)
		}
	}

	w := v.GLArea.AllocatedWidth()
	h := v.GLArea.AllocatedHeight()
	prog := v.gl.rgbaProgram
	if v.ycbcr {
		prog = v.gl.ycbcrProgram
	}
	glDraw(&v.gl, prog, w, h, v.frameW, v.frameH)
	return true
}

func (v *VideoView) uploadFrame(img image.Image) {
	switch f := img.(type) {
	case *video.PackedYCbCr:
		v.ycbcr = true
		rgba := f.RGBA
		v.frameW = rgba.Rect.Dx()
		v.frameH = rgba.Rect.Dy()
		glUploadRGBA(&v.gl, v.frameW, v.frameH, rgba.Pix)
	case *image.RGBA:
		v.ycbcr = false
		v.frameW = f.Rect.Dx()
		v.frameH = f.Rect.Dy()
		glUploadRGBA(&v.gl, v.frameW, v.frameH, f.Pix)
	case *image.YCbCr:
		v.ycbcr = false
		rgba := ycbcrToRGBA(f)
		v.frameW = rgba.Rect.Dx()
		v.frameH = rgba.Rect.Dy()
		glUploadRGBA(&v.gl, v.frameW, v.frameH, rgba.Pix)
	}
}

func ycbcrToRGBA(src *image.YCbCr) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = uint8(r >> 8)
			dst.Pix[i+1] = uint8(g >> 8)
			dst.Pix[i+2] = uint8(bl >> 8)
			dst.Pix[i+3] = uint8(a >> 8)
		}
	}
	return dst
}

// QueueRender triggers a redraw on the next GTK frame.
func (v *VideoView) QueueRender() {
	v.GLArea.QueueRender()
}

// setupInput wires GTK event controllers for mouse and keyboard.
func (v *VideoView) setupInput() {
	// Keyboard
	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if v.ctrl == nil || v.isPasteInProgress() {
			return true
		}
		if key, ok := gdkKeyToInputKey(keyval); ok {
			if hid, ok := input.KeyToHID(key); ok {
				_ = v.ctrl.SendKeypress(hid, true)
				return true
			}
		}
		return false
	})
	keyCtrl.ConnectKeyReleased(func(keyval, keycode uint, state gdk.ModifierType) {
		if v.ctrl == nil || v.isPasteInProgress() {
			return
		}
		if key, ok := gdkKeyToInputKey(keyval); ok {
			if hid, ok := input.KeyToHID(key); ok {
				_ = v.ctrl.SendKeypress(hid, false)
			}
		}
	})
	v.GLArea.AddController(keyCtrl)

	motionCtrl := gtk.NewEventControllerMotion()
	motionCtrl.ConnectMotion(func(x, y float64) {
		if v.app != nil && (x != v.lastMX || y != v.lastMY) {
			v.app.revealUIFor(1600 * time.Millisecond)
		}
		v.lastMX = x
		v.lastMY = y
		if v.ctrl == nil || v.isPasteInProgress() {
			return
		}
		v.sendAbsPointer(x, y, v.buttons)
	})
	v.GLArea.AddController(motionCtrl)

	for _, btn := range []uint{1, 2, 3} {
		clickCtrl := gtk.NewGestureClick()
		clickCtrl.SetButton(btn)
		capturedBtn := btn
		clickCtrl.ConnectPressed(func(n int, x, y float64) {
			if v.ctrl == nil || v.isPasteInProgress() {
				return
			}
			v.GLArea.GrabFocus()
			v.buttons |= mouseButton(capturedBtn)
			v.sendAbsPointer(x, y, v.buttons)
		})
		clickCtrl.ConnectReleased(func(n int, x, y float64) {
			if v.ctrl == nil || v.isPasteInProgress() {
				return
			}
			v.buttons &^= mouseButton(capturedBtn)
			v.sendAbsPointer(x, y, v.buttons)
		})
		v.GLArea.AddController(clickCtrl)
	}

	// Scroll wheel
	scrollCtrl := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical | gtk.EventControllerScrollDiscrete)
	scrollCtrl.ConnectScroll(func(dx, dy float64) bool {
		if v.ctrl == nil || v.isPasteInProgress() {
			return true
		}
		wy := int8(0)
		if dy < 0 {
			wy = 1
		} else if dy > 0 {
			wy = -1
		}
		if wy != 0 {
			_ = v.ctrl.SendWheel(wy, 0)
		}
		return true
	})
	v.GLArea.AddController(scrollCtrl)
}

func (v *VideoView) sendAbsPointer(x, y float64, buttons byte) {
	v.mu.Lock()
	fw, fh := v.frameW, v.frameH
	v.mu.Unlock()

	if fw == 0 || fh == 0 {
		return
	}

	w := float64(v.GLArea.AllocatedWidth())
	h := float64(v.GLArea.AllocatedHeight())

	srcAspect := float64(fw) / float64(fh)
	dstAspect := w / h
	var vpX, vpY, vpW, vpH float64
	if srcAspect > dstAspect {
		vpW = w
		vpH = w / srcAspect
		vpX = 0
		vpY = (h - vpH) / 2
	} else {
		vpH = h
		vpW = h * srcAspect
		vpX = (w - vpW) / 2
		vpY = 0
	}

	relX := (x - vpX) / vpW
	relY := (y - vpY) / vpH
	if relX < 0 {
		relX = 0
	}
	if relX > 1 {
		relX = 1
	}
	if relY < 0 {
		relY = 0
	}
	if relY > 1 {
		relY = 1
	}

	absX := int32(relX * 32767)
	absY := int32(relY * 32767)
	_ = v.ctrl.SendAbsPointer(absX, absY, buttons)
}

func mouseButton(gtkBtn uint) byte {
	switch gtkBtn {
	case 1:
		return 1 // left
	case 2:
		return 4 // middle
	case 3:
		return 2 // right
	default:
		return 0
	}
}

func (v *VideoView) isPasteInProgress() bool {
	if v.ctrl == nil {
		return false
	}
	return v.ctrl.Snapshot().PasteInProgress
}
