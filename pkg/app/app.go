package app

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/lkarlslund/jetkvm-native/pkg/input"
	"github.com/lkarlslund/jetkvm-native/pkg/session"
)

type Config struct {
	BaseURL    string
	Password   string
	RPCTimeout time.Duration
}

type App struct {
	cfg  Config
	ctrl *session.Controller

	mu          sync.RWMutex
	lastImg     *ebiten.Image
	keyboard    *input.Keyboard
	lastX       int
	lastY       int
	lastButtons byte
	lastPhase   session.Phase
	lastTitle   string
	relative    bool
	buttons     []button
	renderRect  rect
	focused     bool
}

func New(cfg Config) (*App, error) {
	ctrl := session.New(session.Config{
		BaseURL:    cfg.BaseURL,
		Password:   cfg.Password,
		RPCTimeout: cfg.RPCTimeout,
		Reconnect:  true,
	})
	return &App{
		cfg:       cfg,
		ctrl:      ctrl,
		keyboard:  input.NewKeyboard(),
		lastPhase: session.PhaseIdle,
		focused:   true,
	}, nil
}

func (a *App) Start(ctx context.Context) {
	a.ctrl.Start(ctx)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			frame := a.ctrl.LatestFrame()
			if frame == nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			img := ebiten.NewImageFromImage(frame)
			a.mu.Lock()
			a.lastImg = img
			a.mu.Unlock()
			time.Sleep(16 * time.Millisecond)
		}
	}()
}

func (a *App) Update() error {
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	a.syncSessionState()
	a.syncWindowTitle()
	nowFocused := ebiten.IsFocused()
	if a.focused && !nowFocused {
		a.releaseAllKeys(true)
		if a.relative {
			a.relative = false
			ebiten.SetCursorMode(ebiten.CursorModeVisible)
		}
	}
	a.focused = nowFocused

	if inpututil.IsKeyJustPressed(ebiten.KeyF8) {
		a.relative = !a.relative
		if a.relative {
			ebiten.SetCursorMode(ebiten.CursorModeCaptured)
		} else {
			ebiten.SetCursorMode(ebiten.CursorModeVisible)
		}
		a.lastX, a.lastY = ebiten.CursorPosition()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF5) {
		_ = a.ctrl.Reboot()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) {
		a.adjustStreamQuality(+0.05)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) {
		a.adjustStreamQuality(-0.05)
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		a.handleClick()
	}

	a.syncKeyboard()
	a.syncMouse()
	return nil
}

func (a *App) Draw(screen *ebiten.Image) {
	snap := a.ctrl.Snapshot()
	screen.Fill(color.RGBA{R: 10, G: 15, B: 24, A: 255})
	vector.DrawFilledRect(screen, 0, 0, float32(screen.Bounds().Dx()), 44, color.RGBA{R: 16, G: 28, B: 44, A: 255}, false)
	vector.DrawFilledRect(screen, 0, float32(screen.Bounds().Dy()-32), float32(screen.Bounds().Dx()), 32, color.RGBA{R: 16, G: 28, B: 44, A: 255}, false)

	videoArea := image.Rect(16, 56, screen.Bounds().Dx()-16, screen.Bounds().Dy()-44)
	a.buttons = layoutButtons(screen.Bounds().Dx())
	a.mu.RLock()
	img := a.lastImg
	a.mu.RUnlock()
	if img != nil {
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		op := &ebiten.DrawImageOptions{}
		scale := min(float64(videoArea.Dx())/float64(w), float64(videoArea.Dy())/float64(h))
		drawW := float64(w) * scale
		drawH := float64(h) * scale
		x := float64(videoArea.Min.X) + (float64(videoArea.Dx())-drawW)/2
		y := float64(videoArea.Min.Y) + (float64(videoArea.Dy())-drawH)/2
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(x, y)
		screen.DrawImage(img, op)
		a.renderRect = rect{
			x: x,
			y: y,
			w: drawW,
			h: drawH,
		}
	} else {
		a.renderRect = rect{}
	}
	mode := "absolute"
	if a.relative {
		mode = "relative"
	}
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("jetkvm-client  %s", snap.BaseURL), 16, 14)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("phase: %s  hid: %t  rtc: %s  quality: %.2f  mouse: %s", snap.Phase, snap.HIDReady, snap.RTCState, snap.Quality, mode), 16, screen.Bounds().Dy()-24)
	if snap.DeviceID != "" || snap.Hostname != "" {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s  %s", snap.DeviceID, snap.Hostname), screen.Bounds().Dx()-320, 14)
	}
	for _, btn := range a.buttons {
		drawButton(screen, btn)
	}
	a.drawOverlay(screen, snap, img != nil)
}

func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func (a *App) syncKeyboard() {
	if a.ctrl.Snapshot().Phase != session.PhaseConnected {
		return
	}
	keys := inpututil.AppendPressedKeys(nil)
	for _, evt := range a.keyboard.Update(keys) {
		_ = a.ctrl.SendKeypress(evt.HID, evt.Press)
	}
}

func (a *App) syncMouse() {
	if a.ctrl.Snapshot().Phase != session.PhaseConnected {
		return
	}
	x, y := ebiten.CursorPosition()
	buttons := byte(0)
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		buttons |= 1
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		buttons |= 2
	}
	if a.relative {
		dx := int8(clamp(float64(x-a.lastX), -127, 127))
		dy := int8(clamp(float64(y-a.lastY), -127, 127))
		if dx != 0 || dy != 0 || buttons != a.lastButtons {
			_ = a.ctrl.SendRelMouse(dx, dy, buttons)
		}
	} else {
		if !a.renderRect.valid() {
			return
		}
		if x != a.lastX || y != a.lastY || buttons != a.lastButtons {
			nx, ny := a.renderRect.toHID(x, y)
			_ = a.ctrl.SendAbsPointer(nx, ny, buttons)
		}
	}
	_, wheelY := ebiten.Wheel()
	if wheelY != 0 {
		_ = a.ctrl.SendWheel(int8(clamp(-wheelY, -127, 127)))
	}
	a.lastX = x
	a.lastY = y
	a.lastButtons = buttons
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func (a *App) adjustStreamQuality(delta float64) {
	snap := a.ctrl.Snapshot()
	next := clamp(snap.Quality+delta, 0.1, 1.0)

	go func(value float64) {
		_ = a.ctrl.SetQuality(value)
	}(next)
}

func (a *App) handleClick() {
	x, y := ebiten.CursorPosition()
	for _, btn := range a.buttons {
		if !btn.rect.contains(x, y) {
			continue
		}
		switch btn.id {
		case "reconnect":
			a.releaseAllKeys(true)
			a.ctrl.ReconnectNow()
		case "mouse":
			a.relative = !a.relative
			if a.relative {
				ebiten.SetCursorMode(ebiten.CursorModeCaptured)
			} else {
				ebiten.SetCursorMode(ebiten.CursorModeVisible)
			}
			a.lastX, a.lastY = ebiten.CursorPosition()
		case "quality_down":
			a.adjustStreamQuality(-0.05)
		case "quality_up":
			a.adjustStreamQuality(+0.05)
		case "reboot":
			_ = a.ctrl.Reboot()
		}
		return
	}
}

func (a *App) syncSessionState() {
	phase := a.ctrl.Snapshot().Phase
	if phase == a.lastPhase {
		return
	}
	if a.lastPhase == session.PhaseConnected && phase != session.PhaseConnected {
		a.releaseAllKeys(false)
		a.lastButtons = 0
		if a.relative {
			a.relative = false
			ebiten.SetCursorMode(ebiten.CursorModeVisible)
		}
	}
	if phase == session.PhaseConnected && a.lastPhase != session.PhaseConnected {
		a.lastX, a.lastY = ebiten.CursorPosition()
		a.lastButtons = 0
	}
	a.lastPhase = phase
}

func (a *App) syncWindowTitle() {
	snap := a.ctrl.Snapshot()
	title := "jetkvm-client"
	if snap.DeviceID != "" {
		title = snap.DeviceID
	} else if snap.Hostname != "" {
		title = snap.Hostname
	}
	title = fmt.Sprintf("%s [%s]", title, snap.Phase)
	if title == a.lastTitle {
		return
	}
	ebiten.SetWindowTitle(title)
	a.lastTitle = title
}

func (a *App) releaseAllKeys(send bool) {
	if send {
		for _, evt := range a.keyboard.ReleaseAll() {
			_ = a.ctrl.SendKeypress(evt.HID, evt.Press)
		}
		return
	}
	_ = a.keyboard.ReleaseAll()
}

func (a *App) drawOverlay(screen *ebiten.Image, snap session.Snapshot, hasVideo bool) {
	message := ""
	switch snap.Phase {
	case session.PhaseConnecting:
		message = "Connecting to device"
	case session.PhaseReconnecting:
		message = "Reconnecting to device"
	case session.PhaseAuthFailed:
		message = "Authentication failed"
	case session.PhaseOtherSession:
		message = "Another session took over"
	case session.PhaseRebooting:
		message = "Device rebooting"
	case session.PhaseDisconnected:
		message = "Connection lost"
	case session.PhaseFatal:
		message = "Fatal error"
	case session.PhaseConnected:
		if !hasVideo || !snap.VideoReady {
			message = "Loading video stream"
		}
	}
	if message == "" {
		return
	}
	vector.DrawFilledRect(screen, 24, 72, float32(screen.Bounds().Dx()-48), 72, color.RGBA{R: 8, G: 12, B: 18, A: 224}, false)
	ebitenutil.DebugPrintAt(screen, message, 40, 96)
	if snap.LastError != "" && snap.Phase != session.PhaseConnected {
		ebitenutil.DebugPrintAt(screen, snap.LastError, 40, 116)
	}
}

type rect struct {
	x float64
	y float64
	w float64
	h float64
}

func (r rect) valid() bool {
	return r.w > 0 && r.h > 0
}

func (r rect) contains(cursorX, cursorY int) bool {
	return float64(cursorX) >= r.x && float64(cursorX) <= r.x+r.w &&
		float64(cursorY) >= r.y && float64(cursorY) <= r.y+r.h
}

func (r rect) toHID(cursorX, cursorY int) (uint16, uint16) {
	if !r.valid() {
		return 0, 0
	}
	relX := clamp((float64(cursorX)-r.x)/r.w, 0, 1)
	relY := clamp((float64(cursorY)-r.y)/r.h, 0, 1)
	return uint16(relX * 32767.0), uint16(relY * 32767.0)
}

type button struct {
	id    string
	label string
	rect  rect
}

func layoutButtons(width int) []button {
	defs := []struct {
		id    string
		label string
		w     float64
	}{
		{id: "reconnect", label: "Reconnect", w: 92},
		{id: "mouse", label: "Mouse Mode", w: 96},
		{id: "quality_down", label: "Quality -", w: 82},
		{id: "quality_up", label: "Quality +", w: 82},
		{id: "reboot", label: "Reboot", w: 76},
	}
	buttons := make([]button, 0, len(defs))
	x := float64(width) - 24
	for i := len(defs) - 1; i >= 0; i-- {
		x -= defs[i].w
		buttons = append([]button{{
			id:    defs[i].id,
			label: defs[i].label,
			rect: rect{
				x: x,
				y: 8,
				w: defs[i].w,
				h: 26,
			},
		}}, buttons...)
		x -= 8
	}
	return buttons
}

func drawButton(screen *ebiten.Image, btn button) {
	vector.DrawFilledRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), color.RGBA{R: 28, G: 48, B: 72, A: 255}, false)
	ebitenutil.DebugPrintAt(screen, btn.label, int(btn.rect.x)+10, int(btn.rect.y)+8)
}
