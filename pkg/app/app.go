package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
	"github.com/lkarlslund/jetkvm-desktop/pkg/discovery"
	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

type Config struct {
	BaseURL    string
	Password   string
	RPCTimeout time.Duration
}

type App struct {
	cfg  Config
	ctrl *session.Controller
	ctx  context.Context

	mu              sync.RWMutex
	lastImg         *ebiten.Image
	lastFrameAt     time.Time
	keyboard        *input.Keyboard
	lastX           int
	lastY           int
	lastButtons     byte
	lastPhase       session.Phase
	lastTitle       string
	relative        bool
	renderRect      rect
	focused         bool
	lastWidth       int
	lastHeight      int
	resizeUntil     time.Time
	lastUIX         int
	lastUIY         int
	uiVisibleUntil  time.Time
	settingsOpen    bool
	pasteOpen       bool
	statsOpen       bool
	settingsSection settingsSection
	chromeButtons   []chromeButton
	overlayButtons  []chromeButton
	settingsButtons []chromeButton
	settingsPanel   rect
	pasteButtons    []chromeButton
	pastePanel      rect
	launcherButtons []chromeButton
	prefs           Preferences
	hideCursor      bool
	showPressedKeys bool
	scrollThrottle  time.Duration
	lastWheelAt     time.Time
	sectionData     sectionData
	pasteText       string
	pasteDelay      uint16
	pasteInvalid    string
	pasteError      string
	stats           client.StatsSnapshot
	statsHistory    []statsPoint
	lastStatsPoll   time.Time
	launcherOpen    bool
	launcherInput   string
	launcherError   string
	discovery       *discovery.Scanner
	discovered      []discovery.Device
}

type statsPoint struct {
	At              time.Time
	BitrateKbps     float64
	JitterMs        float64
	RoundTripMs     float64
	FramesPerSecond float64
}

func New(cfg Config) (*App, error) {
	prefs := loadPreferences()
	launcherOpen := strings.TrimSpace(cfg.BaseURL) == ""
	return &App{
		cfg:             cfg,
		keyboard:        input.NewKeyboard(),
		lastPhase:       session.PhaseIdle,
		focused:         true,
		uiVisibleUntil:  time.Now().Add(3 * time.Second),
		settingsSection: sectionGeneral,
		prefs:           prefs,
		hideCursor:      prefs.HideCursor,
		showPressedKeys: prefs.ShowPressedKeys,
		scrollThrottle:  scrollThrottleFromPref(prefs.ScrollThrottle),
		pasteDelay:      100,
		launcherOpen:    launcherOpen,
		discovery:       discovery.NewScanner(),
	}, nil
}

func (a *App) Start(ctx context.Context) {
	a.ctx = ctx
	if a.discovery != nil {
		a.discovery.Start(ctx)
	}
	if strings.TrimSpace(a.cfg.BaseURL) != "" {
		a.connectTo(a.cfg.BaseURL)
	}
}

func (a *App) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if a.launcherOpen {
			return ebiten.Termination
		}
		if a.pasteOpen {
			a.pasteOpen = false
			a.applyCursorMode()
			a.revealUIFor(1200 * time.Millisecond)
			return nil
		}
		if a.settingsOpen {
			a.settingsOpen = false
			a.revealUIFor(1200 * time.Millisecond)
			return nil
		}
		return ebiten.Termination
	}
	if a.launcherOpen {
		a.syncDiscovery()
		a.syncLauncherInput()
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			a.handleClick()
		}
		return nil
	}
	if a.ctrl == nil {
		return nil
	}
	a.syncSessionState()
	a.syncWindowTitle()
	a.syncChromeVisibility()
	a.syncStats()
	nowFocused := ebiten.IsFocused()
	if a.focused && !nowFocused {
		a.releaseAllKeys(true)
		if a.relative {
			a.relative = false
			a.applyCursorMode()
		}
	}
	a.focused = nowFocused

	if inpututil.IsKeyJustPressed(ebiten.KeyF8) {
		a.setMouseRelative(!a.relative)
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

	a.syncPasteInput()
	a.syncVideoFrame()
	a.syncKeyboard()
	a.syncMouse()
	return nil
}

func (a *App) syncVideoFrame() {
	frame, at := a.ctrl.LatestFrameInfo()
	if frame == nil || !at.After(a.lastFrameAt) {
		return
	}
	img := ebiten.NewImageFromImage(frame)
	a.mu.Lock()
	a.lastImg = img
	a.lastFrameAt = at
	a.mu.Unlock()
}

func (a *App) Draw(screen *ebiten.Image) {
	if a.launcherOpen {
		a.drawLauncher(screen)
		return
	}
	if a.ctrl == nil {
		screen.Fill(color.RGBA{R: 9, G: 14, B: 22, A: 255})
		return
	}
	snap := a.ctrl.Snapshot()
	screen.Fill(color.RGBA{R: 9, G: 14, B: 22, A: 255})
	videoArea := image.Rect(8, 8, screen.Bounds().Dx()-8, screen.Bounds().Dy()-8)
	a.mu.RLock()
	img := a.lastImg
	a.mu.RUnlock()
	if img != nil {
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		op := &ebiten.DrawImageOptions{}
		op.Filter = ebiten.FilterLinear
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
	a.drawTopBar(screen, snap)
	a.drawStatusFooter(screen, snap)
	a.drawPressedKeysOverlay(screen)
	a.drawOverlay(screen, snap, img != nil)
	a.drawStatsOverlay(screen)
	a.drawHint(screen)
	a.drawSettingsOverlay(screen, snap)
	a.drawPasteOverlay(screen, snap)
}

func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	if a.lastWidth != 0 && a.lastHeight != 0 && (a.lastWidth != outsideWidth || a.lastHeight != outsideHeight) {
		a.resizeUntil = time.Now().Add(200 * time.Millisecond)
	}
	a.lastWidth = outsideWidth
	a.lastHeight = outsideHeight
	return outsideWidth, outsideHeight
}

func (a *App) syncKeyboard() {
	if !a.focused || a.settingsOpen || a.pasteOpen || a.ctrl.Snapshot().Phase != session.PhaseConnected {
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
	if a.settingsOpen || a.pasteOpen || a.ctrl.Snapshot().Phase != session.PhaseConnected {
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
		if time.Now().Before(a.resizeUntil) {
			a.lastX = x
			a.lastY = y
			a.lastButtons = buttons
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
	if wheelY != 0 && (a.scrollThrottle == 0 || time.Since(a.lastWheelAt) >= a.scrollThrottle) {
		delta := normalizeWheelDelta(wheelY)
		if delta != 0 {
			a.runAsync(func() {
				_ = a.ctrl.SendWheel(delta)
			})
		}
		a.lastWheelAt = time.Now()
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

func normalizeWheelDelta(value float64) int8 {
	if value == 0 {
		return 0
	}
	magnitude := math.Abs(value)
	switch {
	case magnitude < 1:
		value = math.Copysign(1, value)
	default:
		value = math.Round(value)
	}
	return int8(clamp(-value, -127, 127))
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

	a.runAsync(func() {
		_ = a.ctrl.SetQuality(next)
	})
}

func (a *App) runAsync(fn func()) {
	go fn()
}

func (a *App) setMouseRelative(relative bool) {
	a.relative = relative
	a.applyCursorMode()
	a.lastX, a.lastY = ebiten.CursorPosition()
	a.revealUIFor(1200 * time.Millisecond)
}

func (a *App) applyCursorMode() {
	switch {
	case a.settingsOpen:
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
	case a.pasteOpen:
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
	case a.relative:
		ebiten.SetCursorMode(ebiten.CursorModeCaptured)
	case a.hideCursor:
		ebiten.SetCursorMode(ebiten.CursorModeHidden)
	default:
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
	}
}

func (a *App) savePreferences() {
	a.prefs.HideCursor = a.hideCursor
	a.prefs.ShowPressedKeys = a.showPressedKeys
	a.prefs.ScrollThrottle = scrollThrottlePref(a.scrollThrottle)
	_ = savePreferences(a.prefs)
}

func (a *App) handleClick() {
	x, y := ebiten.CursorPosition()
	if a.launcherOpen {
		for _, btn := range a.launcherButtons {
			if !btn.enabled || !btn.rect.contains(x, y) {
				continue
			}
			a.invokeAction(btn.id)
			return
		}
		return
	}
	for _, btn := range a.overlayButtons {
		if !btn.enabled || !btn.rect.contains(x, y) {
			continue
		}
		a.invokeAction(btn.id)
		return
	}
	for _, btn := range a.pasteButtons {
		if !btn.enabled || !btn.rect.contains(x, y) {
			continue
		}
		a.invokeAction(btn.id)
		return
	}
	if a.pasteOpen && !a.pastePanel.contains(x, y) {
		a.pasteOpen = false
		a.applyCursorMode()
		return
	}
	for _, btn := range a.settingsButtons {
		if !btn.enabled || !btn.rect.contains(x, y) {
			continue
		}
		a.invokeAction(btn.id)
		return
	}
	if a.settingsOpen && !a.settingsPanel.contains(x, y) {
		a.settingsOpen = false
		a.applyCursorMode()
		return
	}
	for _, btn := range a.chromeButtons {
		if !btn.enabled || !btn.rect.contains(x, y) {
			continue
		}
		a.invokeAction(btn.id)
		return
	}
}

func (a *App) invokeAction(id string) {
	switch id {
	case "launcher_connect":
		a.connectFromLauncher(a.launcherInput)
	case "reconnect":
		if a.ctrl == nil {
			return
		}
		a.releaseAllKeys(true)
		a.ctrl.ReconnectNow()
	case "take_back_control":
		a.releaseAllKeys(true)
		a.ctrl.ReconnectNow()
		a.revealUIFor(2 * time.Second)
	case "mouse":
		a.setMouseRelative(!a.relative)
	case "paste":
		a.pasteOpen = !a.pasteOpen
		if a.pasteOpen {
			a.loadClipboardText()
			a.settingsOpen = false
		}
		a.applyCursorMode()
	case "stats":
		a.statsOpen = !a.statsOpen
	case "paste_load_clipboard":
		a.loadClipboardText()
	case "paste_send":
		go a.submitPaste()
	case "paste_cancel":
		a.runAsync(func() {
			_ = a.ctrl.CancelPaste()
		})
		a.pasteOpen = false
		a.applyCursorMode()
	case "mouse_absolute":
		a.setMouseRelative(false)
	case "mouse_relative":
		a.setMouseRelative(true)
	case "quality_preset_high":
		a.runAsync(func() {
			_ = a.ctrl.SetQuality(1.0)
		})
	case "quality_preset_medium":
		a.runAsync(func() {
			_ = a.ctrl.SetQuality(0.5)
		})
	case "quality_preset_low":
		a.runAsync(func() {
			_ = a.ctrl.SetQuality(0.1)
		})
	case "reboot":
		a.runAsync(func() {
			_ = a.ctrl.Reboot()
		})
	case "settings":
		a.settingsOpen = !a.settingsOpen
		if a.settingsOpen {
			a.pasteOpen = false
			a.refreshSettingsSection(a.settingsSection)
		}
		a.applyCursorMode()
		a.revealUIFor(1200 * time.Millisecond)
	case "settings_close":
		a.settingsOpen = false
		a.applyCursorMode()
	case "mouse_hide_cursor":
		a.hideCursor = !a.hideCursor
		a.applyCursorMode()
		a.savePreferences()
	case "scroll_0":
		a.scrollThrottle = 0
		a.savePreferences()
	case "scroll_10":
		a.scrollThrottle = 10 * time.Millisecond
		a.savePreferences()
	case "scroll_25":
		a.scrollThrottle = 25 * time.Millisecond
		a.savePreferences()
	case "scroll_50":
		a.scrollThrottle = 50 * time.Millisecond
		a.savePreferences()
	case "scroll_100":
		a.scrollThrottle = 100 * time.Millisecond
		a.savePreferences()
	case "toggle_pressed_keys":
		a.showPressedKeys = !a.showPressedKeys
		a.savePreferences()
	case "pin_chrome_on":
		a.prefs.PinChrome = true
		a.savePreferences()
	case "pin_chrome_off":
		a.prefs.PinChrome = false
		a.savePreferences()
	case "fullscreen":
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	case "tls_disabled":
		a.runAsync(func() {
			_ = a.ctrl.SetTLSMode("disabled")
			a.refreshSettingsSection(sectionAccess)
		})
	case "tls_self_signed":
		a.runAsync(func() {
			_ = a.ctrl.SetTLSMode("self-signed")
			a.refreshSettingsSection(sectionAccess)
		})
	case "rotate_normal":
		a.runAsync(func() {
			_ = a.ctrl.SetDisplayRotation("270")
			a.refreshSettingsSection(sectionHardware)
		})
	case "rotate_inverted":
		a.runAsync(func() {
			_ = a.ctrl.SetDisplayRotation("90")
			a.refreshSettingsSection(sectionHardware)
		})
	case "usb_emulation_on":
		a.runAsync(func() {
			_ = a.ctrl.SetUSBEmulation(true)
			a.refreshSettingsSection(sectionHardware)
		})
	case "usb_emulation_off":
		a.runAsync(func() {
			_ = a.ctrl.SetUSBEmulation(false)
			a.refreshSettingsSection(sectionHardware)
		})
	case "layout:en_US":
		a.runAsync(func() {
			_ = a.ctrl.SetKeyboardLayout("en_US")
		})
	case "layout:en_UK":
		a.runAsync(func() {
			_ = a.ctrl.SetKeyboardLayout("en_UK")
		})
	case "layout:da_DK":
		a.runAsync(func() {
			_ = a.ctrl.SetKeyboardLayout("da_DK")
		})
	case "layout:de_DE":
		a.runAsync(func() {
			_ = a.ctrl.SetKeyboardLayout("de_DE")
		})
	case "layout:fr_FR":
		a.runAsync(func() {
			_ = a.ctrl.SetKeyboardLayout("fr_FR")
		})
	case "layout:es_ES":
		a.runAsync(func() {
			_ = a.ctrl.SetKeyboardLayout("es_ES")
		})
	case "layout:it_IT":
		a.runAsync(func() {
			_ = a.ctrl.SetKeyboardLayout("it_IT")
		})
	case "layout:ja_JP":
		a.runAsync(func() {
			_ = a.ctrl.SetKeyboardLayout("ja_JP")
		})
	default:
		if strings.HasPrefix(id, "discover:") {
			a.connectFromLauncher(strings.TrimPrefix(id, "discover:"))
			return
		}
		if len(id) > 8 && id[:8] == "section:" {
			a.settingsSection = settingsSection(id[8:])
			a.refreshSettingsSection(a.settingsSection)
		}
	}
}

func (a *App) syncChromeVisibility() {
	if a.ctrl == nil {
		return
	}
	x, y := ebiten.CursorPosition()
	if x != a.lastUIX || y != a.lastUIY {
		if y <= 72 || a.settingsOpen || a.pasteOpen {
			a.revealUIFor(1600 * time.Millisecond)
		}
		a.lastUIX = x
		a.lastUIY = y
	}
	if a.settingsOpen || a.pasteOpen {
		a.applyCursorMode()
		a.revealUIFor(500 * time.Millisecond)
	}
}

func (a *App) syncSessionState() {
	if a.ctrl == nil {
		return
	}
	snap := a.ctrl.Snapshot()
	phase := snap.Phase
	if phase == a.lastPhase {
		return
	}
	if a.lastPhase == session.PhaseConnected && phase != session.PhaseConnected {
		if a.pasteOpen {
			a.pasteOpen = false
		}
		a.releaseAllKeys(false)
		a.releasePointerState()
		a.lastButtons = 0
		if a.relative {
			a.relative = false
			a.applyCursorMode()
		}
	}
	if phase == session.PhaseConnected && a.lastPhase != session.PhaseConnected {
		a.lastX, a.lastY = ebiten.CursorPosition()
		a.lastButtons = 0
		a.revealUIFor(2 * time.Second)
	}
	a.lastPhase = phase
}

func (a *App) syncWindowTitle() {
	if a.launcherOpen {
		title := "jetkvm-desktop"
		if title != a.lastTitle {
			ebiten.SetWindowTitle(title)
			a.lastTitle = title
		}
		return
	}
	if a.ctrl == nil {
		return
	}
	snap := a.ctrl.Snapshot()
	title := "jetkvm-desktop"
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
	a.overlayButtons = nil
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
	drawText(screen, title, 42, 104, 22, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	if detail == "" && snap.LastError != "" && snap.Phase != session.PhaseConnected {
		detail = snap.LastError
	}
	if detail != "" {
		drawText(screen, detail, 42, 132, 14, color.RGBA{R: 178, G: 188, B: 198, A: 255})
	}
	if snap.Phase == session.PhaseOtherSession {
		btn := chromeButton{
			id:      "take_back_control",
			label:   "Take Back Control",
			enabled: true,
			rect:    rect{x: 42, y: 148, w: 170, h: 32},
		}
		a.overlayButtons = append(a.overlayButtons, btn)
		fill := color.RGBA{R: 28, G: 66, B: 116, A: 255}
		stroke := color.RGBA{R: 134, G: 186, B: 248, A: 180}
		vector.DrawFilledRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), fill, false)
		vector.StrokeRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), 1, stroke, false)
		drawText(screen, btn.label, btn.rect.x+12, btn.rect.y+9, 13, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	}
}

func (a *App) drawPressedKeysOverlay(screen *ebiten.Image) {
	if !a.showPressedKeys || a.settingsOpen {
		return
	}
	pressed := a.keyboard.Pressed()
	if len(pressed) == 0 {
		return
	}
	line := "Keys: "
	for i, key := range pressed {
		if i > 0 {
			line += "  "
		}
		line += key.String()
	}
	w, _ := measureText(line, 12)
	x := 14.0
	y := float64(screen.Bounds().Dy()) - 58
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w+20), 28, color.RGBA{R: 8, G: 12, B: 18, A: 212}, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w+20), 28, 1, color.RGBA{R: 112, G: 128, B: 148, A: 120}, false)
	drawText(screen, line, x+10, y+8, 12, color.RGBA{R: 236, G: 241, B: 245, A: 255})
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
	id      string
	label   string
	enabled bool
	rect    rect
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

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("host or URL is required")
	}
	if !strings.Contains(raw, "://") {
		if !isValidConnectHost(raw) {
			return "", errors.New("enter a valid hostname or IP address")
		}
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid target")
	}
	host := parsed.Hostname()
	if host == "" || !isValidConnectHost(host) {
		return "", errors.New("enter a valid hostname or IP address")
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func isValidConnectHost(raw string) bool {
	if raw == "" {
		return false
	}
	if strings.ContainsAny(raw, "/?#") {
		return false
	}
	if ip := net.ParseIP(raw); ip != nil {
		return true
	}
	return isValidHostname(raw)
}

func isValidHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		host = strings.Trim(host, ".")
	}
	labels := strings.Split(host, ".")
	if len(labels) == 0 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func (a *App) connectFromLauncher(target string) {
	baseURL, err := normalizeBaseURL(target)
	if err != nil {
		a.launcherError = err.Error()
		return
	}
	a.launcherError = ""
	a.launcherOpen = false
	a.launcherInput = baseURL
	a.connectTo(baseURL)
}

func (a *App) connectTo(target string) {
	baseURL, err := normalizeBaseURL(target)
	if err != nil {
		a.launcherError = err.Error()
		a.launcherOpen = true
		return
	}
	if a.ctrl != nil {
		a.ctrl.Stop()
	}
	a.cfg.BaseURL = baseURL
	a.lastImg = nil
	a.lastFrameAt = time.Time{}
	a.lastPhase = session.PhaseIdle
	a.stats = client.StatsSnapshot{}
	a.statsHistory = nil
	a.ctrl = session.New(session.Config{
		BaseURL:    baseURL,
		Password:   a.cfg.Password,
		RPCTimeout: a.cfg.RPCTimeout,
		Reconnect:  true,
	})
	if a.ctx != nil {
		a.ctrl.Start(a.ctx)
	}
}

func mouseButtonLabel(relative bool) string {
	if relative {
		return "Mouse: Relative"
	}
	return "Mouse: Absolute"
}
