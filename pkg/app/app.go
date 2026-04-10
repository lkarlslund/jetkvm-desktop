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
	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
	"github.com/lkarlslund/jetkvm-desktop/pkg/discovery"
	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
	"github.com/lkarlslund/jetkvm-desktop/pkg/virtualmedia"
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

	mu                     sync.RWMutex
	lastImg                *ebiten.Image
	lastFrameAt            time.Time
	keyboard               *input.Keyboard
	lastX                  int
	lastY                  int
	lastButtons            byte
	lastPhase              session.Phase
	lastTitle              string
	relative               bool
	renderRect             rect
	focused                bool
	lastWidth              int
	lastHeight             int
	resizeUntil            time.Time
	lastUIX                int
	lastUIY                int
	uiVisibleUntil         time.Time
	settingsOpen           bool
	pasteOpen              bool
	statsOpen              bool
	mediaOpen              bool
	settingsSection        settingsSection
	chromeButtons          []chromeButton
	overlayButtons         []chromeButton
	settingsButtons        []chromeButton
	settingsPanel          rect
	pasteButtons           []chromeButton
	pastePanel             rect
	mediaButtons           []chromeButton
	mediaPanel             rect
	launcherButtons        []chromeButton
	prefs                  Preferences
	hideCursor             bool
	showPressedKeys        bool
	scrollThrottle         time.Duration
	lastWheelAt            time.Time
	suppressKeysUntilClear bool
	suppressMouseUntilUp   bool
	sectionData            sectionData
	pasteText              string
	pasteDelay             uint16
	pasteInvalid           string
	pasteError             string
	stats                  client.StatsSnapshot
	statsHistory           []statsPoint
	lastStatsPoll          time.Time
	launcherOpen           bool
	launcherMode           launcherMode
	launcherInput          string
	launcherPassword       string
	launcherError          string
	pendingTarget          string
	discovery              *discovery.Scanner
	discovered             []discovery.Device
	settingsActions        map[settingsActionGroup]settingsActionState
	sectionLoadSeq         map[settingsSection]uint64
	mediaView              mediaView
	mediaURL               string
	mediaMode              virtualmedia.Mode
	mediaURLFocused        bool
	mediaState             *virtualmedia.State
	mediaFiles             []mediaFileRow
	mediaSpace             mediaSpaceSnapshot
	mediaSelectedFile      string
	mediaLoading           bool
	mediaError             string
	mediaUploadPath        string
	mediaUploadFocused     bool
	mediaUploading         bool
	mediaUploadProgress    float64
	mediaUploadSent        int64
	mediaUploadTotal       int64
	mediaUploadSpeed       float64
	mediaStorageLoaded     bool
}

type statsPoint struct {
	At              time.Time
	BitrateKbps     float64
	JitterMs        float64
	RoundTripMs     float64
	FramesPerSecond float64
}

//go:generate go tool github.com/dmarkham/enumer -type=launcherMode,settingsActionGroup,mediaView -linecomment -json -text -output app_enums.go

type launcherMode uint8

const (
	launcherModeBrowse   launcherMode = iota // browse
	launcherModePassword                     // password
)

type settingsActionGroup uint8

const (
	settingsGroupKeyboardLayout settingsActionGroup = iota // keyboard_layout
	settingsGroupVideoQuality                              // video_quality
	settingsGroupTLSMode                                   // tls_mode
	settingsGroupDisplayRotate                             // display_rotation
	settingsGroupUSBEmulation                              // usb_emulation
)

type settingsActionState struct {
	Pending       bool
	PendingChoice string
	Error         string
	RequestSeq    uint64
}

type mediaView uint8

const (
	mediaViewHome    mediaView = iota // home
	mediaViewURL                      // url
	mediaViewStorage                  // storage
	mediaViewUpload                   // upload
)

type mediaFileRow struct {
	Filename  string
	Size      int64
	CreatedAt time.Time
}

type mediaSpaceSnapshot struct {
	BytesUsed int64
	BytesFree int64
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
		launcherMode:    launcherModeBrowse,
		discovery:       discovery.NewScanner(),
		settingsActions: make(map[settingsActionGroup]settingsActionState),
		sectionLoadSeq:  make(map[settingsSection]uint64),
		mediaView:       mediaViewHome,
		mediaMode:       virtualmedia.ModeCDROM,
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
		if a.pasteOpen {
			a.closePasteOverlay()
			a.revealUIFor(1200 * time.Millisecond)
			return nil
		}
		if a.mediaOpen {
			a.closeMediaOverlay()
			a.revealUIFor(1200 * time.Millisecond)
			return nil
		}
		if a.settingsOpen {
			a.closeSettingsOverlay()
			a.revealUIFor(1200 * time.Millisecond)
			return nil
		}
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
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		a.handleClick()
	}

	a.syncPasteInput()
	a.syncMediaInput()
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
	a.drawMediaOverlay(screen, snap)
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
	if !a.focused || a.settingsOpen || a.pasteOpen || a.mediaOpen || a.ctrl.Snapshot().Phase != session.PhaseConnected {
		return
	}
	rawKeys := inpututil.AppendPressedKeys(nil)
	if a.suppressKeysUntilClear {
		if len(rawKeys) == 0 {
			a.suppressKeysUntilClear = false
		} else {
			a.releaseAllKeys(false)
			return
		}
	}
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
	if a.settingsOpen || a.pasteOpen || a.mediaOpen || a.ctrl.Snapshot().Phase != session.PhaseConnected {
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
	if a.suppressMouseUntilUp {
		a.lastX = x
		a.lastY = y
		if buttons == 0 {
			a.suppressMouseUntilUp = false
			a.lastButtons = 0
		}
		return
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
	case ebiten.KeyIntlBackslash:
		return input.KeyIntlBackslash, true
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
	case ebiten.KeyF13:
		return input.KeyF13, true
	case ebiten.KeyF14:
		return input.KeyF14, true
	case ebiten.KeyF15:
		return input.KeyF15, true
	case ebiten.KeyF16:
		return input.KeyF16, true
	case ebiten.KeyF17:
		return input.KeyF17, true
	case ebiten.KeyF18:
		return input.KeyF18, true
	case ebiten.KeyF19:
		return input.KeyF19, true
	case ebiten.KeyF20:
		return input.KeyF20, true
	case ebiten.KeyF21:
		return input.KeyF21, true
	case ebiten.KeyF22:
		return input.KeyF22, true
	case ebiten.KeyF23:
		return input.KeyF23, true
	case ebiten.KeyF24:
		return input.KeyF24, true
	case ebiten.KeyPrintScreen:
		return input.KeyPrintScreen, true
	case ebiten.KeyScrollLock:
		return input.KeyScrollLock, true
	case ebiten.KeyPause:
		return input.KeyPause, true
	case ebiten.KeyContextMenu:
		return input.KeyContextMenu, true
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
	case ebiten.KeyNumpadEqual:
		return input.KeyNumpadEqual, true
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

func (a *App) runAsync(fn func()) {
	go fn()
}

func (a *App) settingsAction(group settingsActionGroup) settingsActionState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settingsActions[group]
}

func (a *App) settingsActionPending(group settingsActionGroup) bool {
	return a.settingsAction(group).Pending
}

func (a *App) beginSettingsAction(group settingsActionGroup, choice string) uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.settingsActions[group]
	state.Pending = true
	state.PendingChoice = choice
	state.Error = ""
	state.RequestSeq++
	a.settingsActions[group] = state
	return state.RequestSeq
}

func (a *App) finishSettingsAction(group settingsActionGroup, seq uint64, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.settingsActions[group]
	if state.RequestSeq != seq {
		return
	}
	state.Pending = false
	state.PendingChoice = ""
	if err != nil {
		state.Error = err.Error()
	} else {
		state.Error = ""
	}
	a.settingsActions[group] = state
}

func (a *App) withSettingsAction(group settingsActionGroup, choice string, fn func() error) {
	seq := a.beginSettingsAction(group, choice)
	go func() {
		a.finishSettingsAction(group, seq, fn())
	}()
}

func (a *App) nextSectionLoadSeq(section settingsSection) uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sectionLoadSeq[section]++
	return a.sectionLoadSeq[section]
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
	case a.mediaOpen:
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
	for _, btn := range a.mediaButtons {
		if !btn.enabled || !btn.rect.contains(x, y) {
			continue
		}
		a.invokeAction(btn.id)
		return
	}
	if a.mediaOpen && !a.mediaPanel.contains(x, y) {
		a.closeMediaOverlay()
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
		a.closePasteOverlay()
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
		a.closeSettingsOverlay()
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
	case "launcher_retry_password":
		a.connectFromLauncher(a.pendingTarget)
	case "launcher_back":
		a.launcherMode = launcherModeBrowse
		a.launcherPassword = ""
		a.launcherError = ""
		if a.cfg.BaseURL != "" && a.ctrl != nil {
			a.ctrl.Stop()
			a.ctrl = nil
		}
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
		if a.pasteOpen {
			a.closePasteOverlay()
		} else {
			a.pasteOpen = true
			a.loadClipboardText()
			a.settingsOpen = false
			a.mediaOpen = false
			a.applyCursorMode()
		}
	case "media":
		if a.mediaOpen {
			a.closeMediaOverlay()
		} else {
			a.openMediaOverlay()
		}
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
		a.closePasteOverlay()
	case "mouse_absolute":
		a.setMouseRelative(false)
	case "mouse_relative":
		a.setMouseRelative(true)
	case "quality_preset_high":
		if a.settingsActionPending(settingsGroupVideoQuality) {
			return
		}
		a.withSettingsAction(settingsGroupVideoQuality, "high", func() error {
			return a.ctrl.SetQuality(1.0)
		})
	case "quality_preset_medium":
		if a.settingsActionPending(settingsGroupVideoQuality) {
			return
		}
		a.withSettingsAction(settingsGroupVideoQuality, "medium", func() error {
			return a.ctrl.SetQuality(0.5)
		})
	case "quality_preset_low":
		if a.settingsActionPending(settingsGroupVideoQuality) {
			return
		}
		a.withSettingsAction(settingsGroupVideoQuality, "low", func() error {
			return a.ctrl.SetQuality(0.1)
		})
	case "reboot":
		a.runAsync(func() {
			_ = a.ctrl.Reboot()
		})
	case "settings":
		if a.settingsOpen {
			a.closeSettingsOverlay()
		} else {
			a.settingsOpen = true
			a.pasteOpen = false
			a.mediaOpen = false
			a.refreshSettingsSection(a.settingsSection)
			a.applyCursorMode()
		}
		a.revealUIFor(1200 * time.Millisecond)
	case "settings_close":
		a.closeSettingsOverlay()
	case "media_close":
		a.closeMediaOverlay()
	default:
		if a.invokeMediaAction(id) {
			return
		}
	}
	switch id {
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
	case "chrome_anchor:top_left":
		a.prefs.ChromeAnchor = chromeAnchorTopLeft
		a.savePreferences()
	case "chrome_anchor:top_center":
		a.prefs.ChromeAnchor = chromeAnchorTopCenter
		a.savePreferences()
	case "chrome_anchor:top_right":
		a.prefs.ChromeAnchor = chromeAnchorTopRight
		a.savePreferences()
	case "chrome_anchor:left_center":
		a.prefs.ChromeAnchor = chromeAnchorLeftCenter
		a.savePreferences()
	case "chrome_anchor:right_center":
		a.prefs.ChromeAnchor = chromeAnchorRightCenter
		a.savePreferences()
	case "chrome_anchor:bottom_left":
		a.prefs.ChromeAnchor = chromeAnchorBottomLeft
		a.savePreferences()
	case "chrome_anchor:bottom_center":
		a.prefs.ChromeAnchor = chromeAnchorBottomCenter
		a.savePreferences()
	case "chrome_anchor:bottom_right":
		a.prefs.ChromeAnchor = chromeAnchorBottomRight
		a.savePreferences()
	case "chrome_layout:horizontal":
		a.prefs.ChromeLayout = chromeLayoutHorizontal
		a.savePreferences()
	case "chrome_layout:vertical":
		a.prefs.ChromeLayout = chromeLayoutVertical
		a.savePreferences()
	case "fullscreen":
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	case "tls_disabled":
		if a.settingsActionPending(settingsGroupTLSMode) {
			return
		}
		a.withSettingsAction(settingsGroupTLSMode, "disabled", func() error {
			if err := a.ctrl.SetTLSMode(session.TLSModeDisabled); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionAccess)
		})
	case "tls_self_signed":
		if a.settingsActionPending(settingsGroupTLSMode) {
			return
		}
		a.withSettingsAction(settingsGroupTLSMode, "self-signed", func() error {
			if err := a.ctrl.SetTLSMode(session.TLSModeSelfSigned); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionAccess)
		})
	case "rotate_normal":
		if a.settingsActionPending(settingsGroupDisplayRotate) {
			return
		}
		a.withSettingsAction(settingsGroupDisplayRotate, "270", func() error {
			if err := a.ctrl.SetDisplayRotation(session.DisplayRotationNormal); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionHardware)
		})
	case "rotate_inverted":
		if a.settingsActionPending(settingsGroupDisplayRotate) {
			return
		}
		a.withSettingsAction(settingsGroupDisplayRotate, "90", func() error {
			if err := a.ctrl.SetDisplayRotation(session.DisplayRotationInverted); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionHardware)
		})
	case "usb_emulation_on":
		if a.settingsActionPending(settingsGroupUSBEmulation) {
			return
		}
		a.withSettingsAction(settingsGroupUSBEmulation, "on", func() error {
			if err := a.ctrl.SetUSBEmulation(true); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionHardware)
		})
	case "usb_emulation_off":
		if a.settingsActionPending(settingsGroupUSBEmulation) {
			return
		}
		a.withSettingsAction(settingsGroupUSBEmulation, "off", func() error {
			if err := a.ctrl.SetUSBEmulation(false); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionHardware)
		})
	case "layout:en-US":
		a.invokeKeyboardLayoutAction("en-US")
	case "layout:en-UK":
		a.invokeKeyboardLayoutAction("en-UK")
	case "layout:da-DK":
		a.invokeKeyboardLayoutAction("da-DK")
	case "layout:de-DE":
		a.invokeKeyboardLayoutAction("de-DE")
	case "layout:fr-FR":
		a.invokeKeyboardLayoutAction("fr-FR")
	case "layout:es-ES":
		a.invokeKeyboardLayoutAction("es-ES")
	case "layout:it-IT":
		a.invokeKeyboardLayoutAction("it-IT")
	case "layout:ja-JP":
		a.invokeKeyboardLayoutAction("ja-JP")
	default:
		if strings.HasPrefix(id, "discover:") {
			a.connectFromLauncher(strings.TrimPrefix(id, "discover:"))
			return
		}
		if len(id) > 8 && id[:8] == "section:" {
			section, ok := parseSettingsSection(id[8:])
			if !ok {
				return
			}
			a.settingsSection = section
			a.refreshSettingsSection(a.settingsSection)
		}
	}
}

func (a *App) invokeKeyboardLayoutAction(layout string) {
	if a.settingsActionPending(settingsGroupKeyboardLayout) {
		return
	}
	a.withSettingsAction(settingsGroupKeyboardLayout, layout, func() error {
		return a.ctrl.SetKeyboardLayout(layout)
	})
}

func (a *App) syncChromeVisibility() {
	if a.ctrl == nil {
		return
	}
	snap := a.ctrl.Snapshot()
	hotZone := a.chromeRevealZone(a.lastWidth, a.lastHeight, snap)
	x, y := ebiten.CursorPosition()
	if x != a.lastUIX || y != a.lastUIY {
		if hotZone.contains(x, y) || a.settingsOpen || a.pasteOpen || a.mediaOpen {
			a.revealUIFor(1600 * time.Millisecond)
		}
		a.lastUIX = x
		a.lastUIY = y
	}
	if a.settingsOpen || a.pasteOpen || a.mediaOpen {
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
	if phase == session.PhaseAuthFailed && a.lastPhase != session.PhaseAuthFailed {
		errMsg := ""
		if a.launcherMode == launcherModePassword {
			errMsg = authPromptError(snap.LastError)
		}
		a.showPasswordPrompt(a.cfg.BaseURL, errMsg)
		a.settingsOpen = false
		a.pasteOpen = false
		a.statsOpen = false
		a.mediaOpen = false
		a.relative = false
		a.applyCursorMode()
	}
	if phase == a.lastPhase {
		return
	}
	if a.lastPhase == session.PhaseConnected && phase != session.PhaseConnected {
		if a.pasteOpen {
			a.pasteOpen = false
		}
		if a.mediaOpen {
			a.mediaOpen = false
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
		a.launcherOpen = false
		a.launcherMode = launcherModeBrowse
		a.launcherError = ""
		a.launcherPassword = ""
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
	title = fmt.Sprintf("%s [%s]", title, snap.Phase.String())
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

func (a *App) armOverlayDismissSuppression() {
	a.releaseAllKeys(false)
	a.suppressKeysUntilClear = true
	a.suppressMouseUntilUp = true
	a.lastButtons = 0
	a.lastX, a.lastY = ebiten.CursorPosition()
}

func (a *App) closePasteOverlay() {
	if !a.pasteOpen {
		return
	}
	a.pasteOpen = false
	a.armOverlayDismissSuppression()
	a.applyCursorMode()
}

func (a *App) closeSettingsOverlay() {
	if !a.settingsOpen {
		return
	}
	a.settingsOpen = false
	a.armOverlayDismissSuppression()
	a.applyCursorMode()
}

func (a *App) closeMediaOverlay() {
	if !a.mediaOpen || a.mediaUploading {
		return
	}
	a.mediaOpen = false
	a.mediaURLFocused = false
	a.mediaUploadFocused = false
	a.armOverlayDismissSuppression()
	a.applyCursorMode()
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
	vector.FillRect(screen, 26, 84, float32(screen.Bounds().Dx()-52), 86, color.RGBA{R: 8, G: 12, B: 18, A: 228}, false)
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
		vector.FillRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), fill, false)
		vector.StrokeRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), 1, stroke, false)
		drawText(screen, btn.label, btn.rect.x+12, btn.rect.y+9, 13, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	}
}

func (a *App) drawPressedKeysOverlay(screen *ebiten.Image) {
	if !a.showPressedKeys || a.settingsOpen || a.mediaOpen {
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
	vector.FillRect(screen, float32(x), float32(y), float32(w+20), 28, color.RGBA{R: 8, G: 12, B: 18, A: 212}, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w+20), 28, 1, color.RGBA{R: 112, G: 128, B: 148, A: 120}, false)
	drawText(screen, line, x+10, y+8, 12, color.RGBA{R: 236, G: 241, B: 245, A: 255})
}

func rtcLabel(state webrtc.PeerConnectionState) string {
	return state.String()
}

func signalingLabel(mode client.SignalingMode) string {
	if mode == client.SignalingModeUnknown {
		return "pending"
	}
	return mode.String()
}

func trimForFooter(value string) string {
	if len(value) <= 42 {
		return value
	}
	return value[:39] + "..."
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
	a.pendingTarget = baseURL
	a.launcherInput = baseURL
	a.launcherOpen = false
	a.connectTo(baseURL)
}

func (a *App) showPasswordPrompt(target, errMsg string) {
	a.pendingTarget = target
	a.launcherOpen = true
	a.launcherMode = launcherModePassword
	a.launcherError = errMsg
}

func authPromptError(lastError string) string {
	lastError = strings.TrimSpace(lastError)
	if lastError == "" {
		return "Authentication failed"
	}
	return lastError
}

func (a *App) connectTo(target string) {
	baseURL, err := normalizeBaseURL(target)
	if err != nil {
		a.launcherError = err.Error()
		a.launcherOpen = true
		a.launcherMode = launcherModeBrowse
		return
	}
	if a.ctrl != nil {
		a.ctrl.Stop()
	}
	a.cfg.BaseURL = baseURL
	a.cfg.Password = a.launcherPassword
	a.lastImg = nil
	a.lastFrameAt = time.Time{}
	a.lastPhase = session.PhaseIdle
	a.stats = client.StatsSnapshot{}
	a.statsHistory = nil
	a.ctrl = session.New(session.Config{
		BaseURL:    baseURL,
		Password:   a.launcherPassword,
		RPCTimeout: a.cfg.RPCTimeout,
		Reconnect:  true,
	})
	if a.ctx != nil {
		a.ctrl.Start(a.ctx)
	}
}
