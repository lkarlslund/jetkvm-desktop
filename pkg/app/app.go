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
	screen.Fill(color.RGBA{R: 9, G: 14, B: 22, A: 255})
	vector.DrawFilledRect(screen, 0, 0, float32(screen.Bounds().Dx()), 52, color.RGBA{R: 15, G: 26, B: 42, A: 255}, false)
	vector.DrawFilledRect(screen, 0, float32(screen.Bounds().Dy()-38), float32(screen.Bounds().Dx()), 38, color.RGBA{R: 15, G: 26, B: 42, A: 255}, false)

	videoArea := image.Rect(16, 64, screen.Bounds().Dx()-16, screen.Bounds().Dy()-52)
	a.buttons = layoutButtons(screen.Bounds().Dx(), snap, a.relative)
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
	a.drawHeader(screen, snap)
	for _, btn := range a.buttons {
		drawButton(screen, btn)
	}
	a.drawFooter(screen, snap)
	a.drawOverlay(screen, snap, img != nil)
}

func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func (a *App) syncKeyboard() {
	if !a.focused || a.ctrl.Snapshot().Phase != session.PhaseConnected {
		return
	}
	rawKeys := inpututil.AppendPressedKeys(nil)
	keys := make([]input.Key, 0, len(rawKeys))
	for _, rawKey := range rawKeys {
		if key, ok := toInputKey(rawKey); ok {
			keys = append(keys, key)
		}
	}
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
		if !a.renderRect.contains(x, y) && buttons == 0 && a.lastButtons == 0 {
			a.lastX = x
			a.lastY = y
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

func toInputKey(key ebiten.Key) (input.Key, bool) {
	switch key {
	case ebiten.KeyA:
		return input.KeyA, true
	case ebiten.KeyB:
		return input.KeyB, true
	case ebiten.KeyC:
		return input.KeyC, true
	case ebiten.KeyD:
		return input.KeyD, true
	case ebiten.KeyE:
		return input.KeyE, true
	case ebiten.KeyF:
		return input.KeyF, true
	case ebiten.KeyG:
		return input.KeyG, true
	case ebiten.KeyH:
		return input.KeyH, true
	case ebiten.KeyI:
		return input.KeyI, true
	case ebiten.KeyJ:
		return input.KeyJ, true
	case ebiten.KeyK:
		return input.KeyK, true
	case ebiten.KeyL:
		return input.KeyL, true
	case ebiten.KeyM:
		return input.KeyM, true
	case ebiten.KeyN:
		return input.KeyN, true
	case ebiten.KeyO:
		return input.KeyO, true
	case ebiten.KeyP:
		return input.KeyP, true
	case ebiten.KeyQ:
		return input.KeyQ, true
	case ebiten.KeyR:
		return input.KeyR, true
	case ebiten.KeyS:
		return input.KeyS, true
	case ebiten.KeyT:
		return input.KeyT, true
	case ebiten.KeyU:
		return input.KeyU, true
	case ebiten.KeyV:
		return input.KeyV, true
	case ebiten.KeyW:
		return input.KeyW, true
	case ebiten.KeyX:
		return input.KeyX, true
	case ebiten.KeyY:
		return input.KeyY, true
	case ebiten.KeyZ:
		return input.KeyZ, true
	case ebiten.Key1:
		return input.Key1, true
	case ebiten.Key2:
		return input.Key2, true
	case ebiten.Key3:
		return input.Key3, true
	case ebiten.Key4:
		return input.Key4, true
	case ebiten.Key5:
		return input.Key5, true
	case ebiten.Key6:
		return input.Key6, true
	case ebiten.Key7:
		return input.Key7, true
	case ebiten.Key8:
		return input.Key8, true
	case ebiten.Key9:
		return input.Key9, true
	case ebiten.Key0:
		return input.Key0, true
	case ebiten.KeyEnter:
		return input.KeyEnter, true
	case ebiten.KeyEscape:
		return input.KeyEscape, true
	case ebiten.KeyBackspace:
		return input.KeyBackspace, true
	case ebiten.KeyTab:
		return input.KeyTab, true
	case ebiten.KeySpace:
		return input.KeySpace, true
	case ebiten.KeyMinus:
		return input.KeyMinus, true
	case ebiten.KeyEqual:
		return input.KeyEqual, true
	case ebiten.KeyLeftBracket:
		return input.KeyLeftBracket, true
	case ebiten.KeyRightBracket:
		return input.KeyRightBracket, true
	case ebiten.KeyBackslash:
		return input.KeyBackslash, true
	case ebiten.KeySemicolon:
		return input.KeySemicolon, true
	case ebiten.KeyApostrophe:
		return input.KeyApostrophe, true
	case ebiten.KeyGraveAccent:
		return input.KeyGraveAccent, true
	case ebiten.KeyComma:
		return input.KeyComma, true
	case ebiten.KeyPeriod:
		return input.KeyPeriod, true
	case ebiten.KeySlash:
		return input.KeySlash, true
	case ebiten.KeyCapsLock:
		return input.KeyCapsLock, true
	case ebiten.KeyF1:
		return input.KeyF1, true
	case ebiten.KeyF2:
		return input.KeyF2, true
	case ebiten.KeyF3:
		return input.KeyF3, true
	case ebiten.KeyF4:
		return input.KeyF4, true
	case ebiten.KeyF5:
		return input.KeyF5, true
	case ebiten.KeyF6:
		return input.KeyF6, true
	case ebiten.KeyF7:
		return input.KeyF7, true
	case ebiten.KeyF8:
		return input.KeyF8, true
	case ebiten.KeyF9:
		return input.KeyF9, true
	case ebiten.KeyF10:
		return input.KeyF10, true
	case ebiten.KeyF11:
		return input.KeyF11, true
	case ebiten.KeyF12:
		return input.KeyF12, true
	case ebiten.KeyPrintScreen:
		return input.KeyPrintScreen, true
	case ebiten.KeyScrollLock:
		return input.KeyScrollLock, true
	case ebiten.KeyPause:
		return input.KeyPause, true
	case ebiten.KeyInsert:
		return input.KeyInsert, true
	case ebiten.KeyHome:
		return input.KeyHome, true
	case ebiten.KeyPageUp:
		return input.KeyPageUp, true
	case ebiten.KeyDelete:
		return input.KeyDelete, true
	case ebiten.KeyEnd:
		return input.KeyEnd, true
	case ebiten.KeyPageDown:
		return input.KeyPageDown, true
	case ebiten.KeyRight:
		return input.KeyRight, true
	case ebiten.KeyLeft:
		return input.KeyLeft, true
	case ebiten.KeyDown:
		return input.KeyDown, true
	case ebiten.KeyUp:
		return input.KeyUp, true
	case ebiten.KeyNumLock:
		return input.KeyNumLock, true
	case ebiten.KeyNumpadDivide:
		return input.KeyNumpadDivide, true
	case ebiten.KeyNumpadMultiply:
		return input.KeyNumpadMultiply, true
	case ebiten.KeyNumpadSubtract:
		return input.KeyNumpadSubtract, true
	case ebiten.KeyNumpadAdd:
		return input.KeyNumpadAdd, true
	case ebiten.KeyNumpadEnter:
		return input.KeyNumpadEnter, true
	case ebiten.KeyNumpad1:
		return input.KeyNumpad1, true
	case ebiten.KeyNumpad2:
		return input.KeyNumpad2, true
	case ebiten.KeyNumpad3:
		return input.KeyNumpad3, true
	case ebiten.KeyNumpad4:
		return input.KeyNumpad4, true
	case ebiten.KeyNumpad5:
		return input.KeyNumpad5, true
	case ebiten.KeyNumpad6:
		return input.KeyNumpad6, true
	case ebiten.KeyNumpad7:
		return input.KeyNumpad7, true
	case ebiten.KeyNumpad8:
		return input.KeyNumpad8, true
	case ebiten.KeyNumpad9:
		return input.KeyNumpad9, true
	case ebiten.KeyNumpad0:
		return input.KeyNumpad0, true
	case ebiten.KeyNumpadDecimal:
		return input.KeyNumpadDecimal, true
	case ebiten.KeyControlLeft:
		return input.KeyControlLeft, true
	case ebiten.KeyShiftLeft:
		return input.KeyShiftLeft, true
	case ebiten.KeyAltLeft:
		return input.KeyAltLeft, true
	case ebiten.KeyMetaLeft:
		return input.KeyMetaLeft, true
	case ebiten.KeyControlRight:
		return input.KeyControlRight, true
	case ebiten.KeyShiftRight:
		return input.KeyShiftRight, true
	case ebiten.KeyAltRight:
		return input.KeyAltRight, true
	case ebiten.KeyMetaRight:
		return input.KeyMetaRight, true
	default:
		return input.KeyUnknown, false
	}
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
		if !btn.enabled || !btn.rect.contains(x, y) {
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
	snap := a.ctrl.Snapshot()
	phase := snap.Phase
	if phase == a.lastPhase {
		return
	}
	if a.lastPhase == session.PhaseConnected && phase != session.PhaseConnected {
		a.releaseAllKeys(false)
		a.releasePointerState()
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

func (a *App) releasePointerState() {
	if a.lastButtons == 0 {
		return
	}
	if a.relative {
		_ = a.ctrl.SendRelMouse(0, 0, 0)
		return
	}
	if a.renderRect.valid() {
		nx, ny := a.renderRect.toHID(a.lastX, a.lastY)
		_ = a.ctrl.SendAbsPointer(nx, ny, 0)
	}
}

func (a *App) drawOverlay(screen *ebiten.Image, snap session.Snapshot, hasVideo bool) {
	title := ""
	detail := ""
	switch snap.Phase {
	case session.PhaseConnecting:
		title = "Connecting"
		detail = "Opening auth, WebRTC, and HID channels"
	case session.PhaseReconnecting:
		title = "Reconnecting"
		detail = snap.Status
	case session.PhaseAuthFailed:
		title = "Authentication Failed"
		detail = "Check the password and local auth mode"
	case session.PhaseOtherSession:
		title = "Session Replaced"
		detail = "Another client took over the device"
	case session.PhaseRebooting:
		title = "Rebooting"
		detail = "Waiting for the device to come back"
	case session.PhaseDisconnected:
		title = "Disconnected"
		detail = "The session dropped before it became ready"
	case session.PhaseFatal:
		title = "Fatal Error"
		detail = snap.LastError
	case session.PhaseConnected:
		if !hasVideo || !snap.VideoReady {
			title = "Loading Video"
			detail = "Waiting for the first decoded frame"
		}
	}
	if title == "" {
		return
	}
	vector.DrawFilledRect(screen, 26, 84, float32(screen.Bounds().Dx()-52), 86, color.RGBA{R: 8, G: 12, B: 18, A: 228}, false)
	ebitenutil.DebugPrintAt(screen, title, 42, 108)
	if detail == "" && snap.LastError != "" && snap.Phase != session.PhaseConnected {
		detail = snap.LastError
	}
	if detail != "" {
		ebitenutil.DebugPrintAt(screen, detail, 42, 128)
	}
}

func (a *App) drawHeader(screen *ebiten.Image, snap session.Snapshot) {
	label := "jetkvm-client"
	if snap.DeviceID != "" {
		label = snap.DeviceID
	} else if snap.Hostname != "" {
		label = snap.Hostname
	}
	ebitenutil.DebugPrintAt(screen, label, 16, 10)
	ebitenutil.DebugPrintAt(screen, snap.BaseURL, 16, 28)

	chips := []string{
		"phase: " + string(snap.Phase),
		"rtc: " + rtcLabel(snap.RTCState),
		"signal: " + signalingLabel(snap.SignalingMode),
		boolChip("hid", snap.HIDReady),
		boolChip("video", snap.VideoReady),
		fmt.Sprintf("quality: %.0f%%", snap.Quality*100),
		"mouse: " + a.mouseModeLabel(),
	}
	x := 260
	for _, chip := range chips {
		ebitenutil.DebugPrintAt(screen, chip, x, 18)
		x += (len(chip) * 7) + 16
	}
}

func (a *App) drawFooter(screen *ebiten.Image, snap session.Snapshot) {
	left := "F8 toggle mouse mode  F5 reboot  +/- stream quality"
	if snap.Phase == session.PhaseConnected {
		left = left + "  click inside the video to control the host"
	}
	ebitenutil.DebugPrintAt(screen, left, 16, screen.Bounds().Dy()-26)
	if snap.LastError != "" && snap.Phase != session.PhaseConnected {
		ebitenutil.DebugPrintAt(screen, trimForFooter(snap.LastError), screen.Bounds().Dx()-360, screen.Bounds().Dy()-26)
	}
}

func boolChip(label string, value bool) string {
	if value {
		return label + ": ready"
	}
	return label + ": pending"
}

func rtcLabel(state interface{}) string {
	return fmt.Sprint(state)
}

func signalingLabel(mode interface{}) string {
	label := fmt.Sprint(mode)
	if label == "" {
		return "pending"
	}
	return label
}

func trimForFooter(value string) string {
	if len(value) <= 42 {
		return value
	}
	return value[:39] + "..."
}

func (a *App) mouseModeLabel() string {
	if a.relative {
		return "relative"
	}
	return "absolute"
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

func (r rect) toHID(cursorX, cursorY int) (int32, int32) {
	if !r.valid() {
		return 0, 0
	}
	relX := clamp((float64(cursorX)-r.x)/r.w, 0, 1)
	relY := clamp((float64(cursorY)-r.y)/r.h, 0, 1)
	return int32(relX * 32767.0), int32(relY * 32767.0)
}

type button struct {
	id    string
	label string
	enabled bool
	rect  rect
}

func layoutButtons(width int, snap session.Snapshot, relative bool) []button {
	canAct := snap.Phase == session.PhaseConnected || snap.Phase == session.PhaseDisconnected || snap.Phase == session.PhaseReconnecting
	defs := []struct {
		id    string
		label string
		w     float64
		enabled bool
	}{
		{id: "reconnect", label: reconnectLabel(snap.Phase), w: 92, enabled: true},
		{id: "mouse", label: mouseButtonLabel(relative), w: 104, enabled: snap.Phase == session.PhaseConnected},
		{id: "quality_down", label: "Quality -", w: 82, enabled: snap.Phase == session.PhaseConnected},
		{id: "quality_up", label: "Quality +", w: 82, enabled: snap.Phase == session.PhaseConnected},
		{id: "reboot", label: "Reboot", w: 76, enabled: canAct},
	}
	buttons := make([]button, 0, len(defs))
	x := float64(width) - 24
	for i := len(defs) - 1; i >= 0; i-- {
		x -= defs[i].w
		buttons = append([]button{{
			id:    defs[i].id,
			label: defs[i].label,
			enabled: defs[i].enabled,
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
	fill := color.RGBA{R: 28, G: 48, B: 72, A: 255}
	if !btn.enabled {
		fill = color.RGBA{R: 22, G: 30, B: 42, A: 255}
	}
	vector.DrawFilledRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), fill, false)
	ebitenutil.DebugPrintAt(screen, btn.label, int(btn.rect.x)+10, int(btn.rect.y)+8)
}

func reconnectLabel(phase session.Phase) string {
	switch phase {
	case session.PhaseConnected:
		return "Reconnect"
	case session.PhaseConnecting, session.PhaseReconnecting:
		return "Retry"
	default:
		return "Connect"
	}
}

func mouseButtonLabel(relative bool) string {
	if relative {
		return "Mouse: Relative"
	}
	return "Mouse: Absolute"
}
