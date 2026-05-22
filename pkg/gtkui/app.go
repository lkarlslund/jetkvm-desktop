package gtkui

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/capture"
	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

const appID = "org.jetkvm.desktop"

type Config struct {
	BaseURL                string
	Password               string
	RPCTimeout             time.Duration
	ExperimentalUSBNetwork bool
}

type Application struct {
	cfg    Config
	gtkApp *gtk.Application
	window *gtk.ApplicationWindow
	prefs  Preferences

	mainStack *gtk.Stack
	launcher  *Launcher

	// Session view widgets
	video      *VideoView
	chrome     *Chrome
	statusBar  *gtk.Label
	statusBox  *gtk.Box
	overlay    *gtk.Overlay

	// Overlay stack for modals
	overlayRevealer *gtk.Revealer
	overlayStack    *gtk.Stack
	activeOverlay   string

	// Overlay panels
	statsPanel    *StatsOverlay
	pastePanel    *PasteOverlay
	serialPanel   *SerialOverlay
	mediaPanel    *MediaOverlay
	wolPanel      *WoLOverlay
	settingsPanel *Settings

	// 10-second settings hint button
	settingsHint      *gtk.Button
	settingsHintUntil time.Time

	// Paste-in-progress banner
	pasteBanner *gtk.Box

	// Session
	ctrl       *session.Controller
	cancel     context.CancelFunc
	sessionURL string

	// Chrome auto-hide
	uiVisibleUntil time.Time
	lastTitle      string
	lastPhase      session.Phase

	fullscreen   bool
	totalCapture bool
	grabber      capture.Grabber
}

func Run(cfg Config) int {
	app := &Application{
		cfg:     cfg,
		grabber: capture.New(),
	}

	gtk.Init()
	app.gtkApp = gtk.NewApplication(appID, gio.ApplicationFlagsNone)
	app.gtkApp.ConnectActivate(app.activate)
	return app.gtkApp.Run(nil)
}

func (a *Application) activate() {
	a.prefs = loadPrefs()
	a.window = gtk.NewApplicationWindow(a.gtkApp)
	a.window.SetTitle("JetKVM Desktop")
	a.window.SetDefaultSize(480, 640)
	centerWindow(a.window)
	a.uiVisibleUntil = time.Now().Add(3 * time.Second)

	applyTheme(a.prefs)

	a.mainStack = gtk.NewStack()
	a.mainStack.SetTransitionType(gtk.StackTransitionTypeCrossfade)

	a.launcher = NewLauncher(a, &a.prefs)
	a.mainStack.AddNamed(a.launcher.Stack, "launcher")

	a.video = NewVideoView(a, nil)

	// Status bar -- transparent text overlay at bottom, not eating space
	a.statusBar = gtk.NewLabel("")
	a.statusBar.SetXAlign(0)
	a.statusBar.AddCSSClass("status-bar")

	statusRight := gtk.NewLabel("")
	statusRight.SetXAlign(1)
	statusRight.SetHExpand(true)
	statusRight.AddCSSClass("status-bar")

	a.statusBox = gtk.NewBox(gtk.OrientationHorizontal, 8)
	a.statusBox.SetVAlign(gtk.AlignEnd)
	a.statusBox.SetHExpand(true)
	a.statusBox.SetCanTarget(false)
	a.statusBox.Append(a.statusBar)
	a.statusBox.Append(statusRight)
	a.statusBox.AddCSSClass("status-bar-box")

	a.chrome = NewChrome(a)

	// 10-second settings hint (bottom-right gear)
	a.settingsHint = gtk.NewButtonFromIconName("preferences-system-symbolic")
	a.settingsHint.AddCSSClass("settings-hint")
	a.settingsHint.SetTooltipText("Settings")
	a.settingsHint.SetHAlign(gtk.AlignEnd)
	a.settingsHint.SetVAlign(gtk.AlignEnd)
	a.settingsHint.SetVisible(false)
	a.settingsHint.ConnectClicked(func() {
		a.settingsHintUntil = time.Time{}
		a.settingsHint.SetVisible(false)
		a.releaseTotalCapture()
		a.toggleOverlay("settings")
	})

	a.statsPanel = NewStatsOverlay(a)
	a.pastePanel = NewPasteOverlay(a)
	a.serialPanel = NewSerialOverlay(a)
	a.mediaPanel = NewMediaOverlay(a)
	a.wolPanel = NewWoLOverlay(a)
	a.settingsPanel = NewSettings(a)

	a.overlayStack = gtk.NewStack()
	a.overlayStack.SetTransitionType(gtk.StackTransitionTypeCrossfade)
	a.overlayStack.SetHExpand(true)
	a.overlayStack.SetVExpand(true)
	a.overlayStack.SetHAlign(gtk.AlignFill)
	a.overlayStack.SetVAlign(gtk.AlignFill)
	a.overlayStack.AddNamed(a.statsPanel.Box, "stats")
	a.overlayStack.AddNamed(a.pastePanel.Box, "paste")
	a.overlayStack.AddNamed(a.serialPanel.Box, "serial")
	a.overlayStack.AddNamed(a.mediaPanel.Box, "media")
	a.overlayStack.AddNamed(a.wolPanel.Box, "wol")
	a.overlayStack.AddNamed(a.settingsPanel.Box, "settings")

	overlayBackdrop := gtk.NewBox(gtk.OrientationVertical, 0)
	overlayBackdrop.AddCSSClass("overlay-backdrop")
	overlayBackdrop.SetHExpand(true)
	overlayBackdrop.SetVExpand(true)
	overlayBackdrop.Append(a.overlayStack)

	a.overlayRevealer = gtk.NewRevealer()
	a.overlayRevealer.SetChild(overlayBackdrop)
	a.overlayRevealer.SetRevealChild(false)
	a.overlayRevealer.SetTransitionType(gtk.RevealerTransitionTypeCrossfade)
	a.overlayRevealer.SetHAlign(gtk.AlignFill)
	a.overlayRevealer.SetVAlign(gtk.AlignFill)
	a.overlayRevealer.SetHExpand(true)
	a.overlayRevealer.SetVExpand(true)
	a.overlayRevealer.SetCanTarget(false)

	// Paste-in-progress banner (shown when ExecutePaste is running)
	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		if a.ctrl != nil {
			_ = a.ctrl.CancelPaste()
		}
	})
	bannerLabel := gtk.NewLabel("Pasting — input is disabled until complete")
	bannerLabel.SetHExpand(true)
	bannerLabel.SetXAlign(0)
	a.pasteBanner = gtk.NewBox(gtk.OrientationHorizontal, 12)
	a.pasteBanner.AddCSSClass("paste-banner")
	a.pasteBanner.SetHAlign(gtk.AlignCenter)
	a.pasteBanner.SetVAlign(gtk.AlignStart)
	a.pasteBanner.Append(bannerLabel)
	a.pasteBanner.Append(cancelBtn)
	a.pasteBanner.SetVisible(false)

	a.overlay = gtk.NewOverlay()
	a.overlay.SetChild(a.video.GLArea)
	a.overlay.AddOverlay(a.chrome.Box)
	a.overlay.AddOverlay(a.statusBox)
	a.overlay.AddOverlay(a.settingsHint)
	a.overlay.AddOverlay(a.pasteBanner)
	a.overlay.AddOverlay(a.overlayRevealer)
	a.chrome.AttachToOverlay(a.overlay)

	a.mainStack.AddNamed(a.overlay, "session")
	a.window.SetChild(a.mainStack)

	if a.cfg.BaseURL != "" {
		a.startSession(a.cfg.BaseURL, a.cfg.Password)
		a.showSession()
	} else {
		a.mainStack.SetVisibleChildName("launcher")
	}

	glib.TimeoutAdd(33, func() bool {
		a.tick()
		return true
	})

	a.setupShortcuts()

	a.gtkApp.ConnectShutdown(func() {
		a.releaseTotalCapture()
		if a.launcher != nil {
			a.launcher.Stop()
		}
		if a.cancel != nil {
			a.cancel()
		}
		if a.ctrl != nil {
			a.ctrl.Stop()
		}
	})

	a.window.Show()
}

func (a *Application) tick() {
	if a.mainStack.VisibleChildName() != "session" {
		return
	}
	a.syncSessionState()
	a.syncWindowTitle()
	a.syncChromeAlpha()
	a.syncSettingsHint()
	a.pollState()
	a.video.QueueRender()
	a.updateVisibleOverlay()
}

func (a *Application) syncSessionState() {
	if a.ctrl == nil {
		return
	}
	snap := a.ctrl.Snapshot()
	phase := snap.Phase

	if phase == a.lastPhase {
		return
	}

	// Lost connection
	if a.lastPhase == session.PhaseConnected && phase != session.PhaseConnected {
		a.releaseTotalCapture()
		if a.activeOverlay == "paste" || a.activeOverlay == "media" || a.activeOverlay == "serial" {
			a.closeOverlay()
		}
	}

	// Just connected
	if phase == session.PhaseConnected && a.lastPhase != session.PhaseConnected {
		a.revealUIFor(2 * time.Second)
		a.settingsHintUntil = time.Now().Add(10 * time.Second)
		a.settingsHint.SetVisible(true)
	}

	// Auth failed -- go back to launcher
	if phase == session.PhaseAuthFailed && a.lastPhase != session.PhaseAuthFailed {
		a.closeOverlay()
		a.releaseTotalCapture()
	}

	a.lastPhase = phase
}

func (a *Application) syncWindowTitle() {
	if a.mainStack.VisibleChildName() != "session" {
		title := "JetKVM Desktop"
		if title != a.lastTitle {
			a.window.SetTitle(title)
			a.lastTitle = title
		}
		return
	}
	if a.ctrl == nil {
		return
	}
	snap := a.ctrl.Snapshot()
	host := parseHost(a.sessionURL)
	label := host
	if label == "" {
		label = parseHost(a.cfg.BaseURL)
	}
	if label == "" && snap.Hostname != "" {
		label = snap.Hostname
	}
	title := fmt.Sprintf("JetKVM Desktop [%s - %s]", label, snap.Phase.String())
	if title != a.lastTitle {
		a.window.SetTitle(title)
		a.lastTitle = title
	}
}

// overlayTargetSize returns a clamped size for a centered overlay panel that
// will not overflow the application window or the underlying monitor work area.
// preferredW/H is the panel's ideal size; the result is min(preferred,
// window-margin) (with a lower bound so very small windows still show
// something reasonable).
func overlayTargetSize(a *Application, preferredW, preferredH int) (int, int) {
	maxW, maxH := preferredW, preferredH
	if a.window != nil {
		if w := a.window.AllocatedWidth(); w > 0 {
			maxW = w - 60
		}
		if h := a.window.AllocatedHeight(); h > 0 {
			maxH = h - 60
		}
	}
	if mw, mh := monitorWorkarea(); mw > 0 && mh > 0 {
		if maxW > mw-60 {
			maxW = mw - 60
		}
		if maxH > mh-60 {
			maxH = mh - 60
		}
	}
	w := preferredW
	if w > maxW {
		w = maxW
	}
	h := preferredH
	if h > maxH {
		h = maxH
	}
	if w < 400 {
		w = 400
	}
	if h < 300 {
		h = 300
	}
	return w, h
}

func parseHost(baseURL string) string {
	if u, err := url.Parse(baseURL); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	s := strings.TrimPrefix(baseURL, "http://")
	s = strings.TrimPrefix(s, "https://")
	s = strings.Split(s, "/")[0]
	s = strings.Split(s, ":")[0]
	if s == "" {
		return baseURL
	}
	return s
}

// uiAlpha returns the current floating-menu opacity (0..1).
func (a *Application) uiAlpha() float64 {
	if a.prefs.PinChrome {
		return 1
	}
	if a.activeOverlay != "" {
		return 1
	}
	if a.chrome != nil && a.chrome.IsDragging() {
		return 1
	}
	remaining := time.Until(a.uiVisibleUntil)
	if remaining <= 0 {
		return 0
	}
	if remaining >= 180*time.Millisecond {
		return 1
	}
	return float64(remaining) / float64(180*time.Millisecond)
}

func (a *Application) revealUIFor(d time.Duration) {
	until := time.Now().Add(d)
	if until.After(a.uiVisibleUntil) {
		a.uiVisibleUntil = until
	}
}

func (a *Application) syncChromeAlpha() {
	alpha := a.uiAlpha()
	if a.chrome.IsHovering() || a.chrome.IsDragging() {
		alpha = 1
	}
	a.chrome.Box.SetOpacity(alpha)
	a.chrome.Box.SetCanTarget(alpha > 0)

	if a.prefs.HideStatusBar {
		a.statusBox.SetVisible(false)
	} else {
		footerAlpha := alpha
		if a.ctrl != nil {
			snap := a.ctrl.Snapshot()
			if snap.Phase != session.PhaseConnected || snap.LastError != "" {
				if footerAlpha < 0.75 {
					footerAlpha = 0.75
				}
			}
		}
		a.statusBox.SetOpacity(footerAlpha)
		a.statusBox.SetVisible(footerAlpha > 0)
	}
}

func (a *Application) syncSettingsHint() {
	if a.activeOverlay == "settings" || a.ctrl == nil {
		a.settingsHint.SetVisible(false)
		return
	}
	remaining := time.Until(a.settingsHintUntil)
	if remaining <= 0 {
		a.settingsHint.SetVisible(false)
		return
	}
	alpha := 1.0
	if remaining < 2*time.Second {
		alpha = float64(remaining) / float64(2*time.Second)
	}
	a.settingsHint.SetOpacity(alpha)
	a.settingsHint.SetVisible(true)
}

func (a *Application) updateVisibleOverlay() {
	if a.ctrl == nil || a.activeOverlay == "" {
		return
	}
	snap := a.ctrl.Snapshot()
	switch a.activeOverlay {
	case "stats":
		a.statsPanel.Update(snap, a.ctrl.Stats())
	case "serial":
		a.serialPanel.Update(snap)
	case "settings":
		a.settingsPanel.video.Update(snap)
		a.settingsPanel.general.Update(snap)
		a.settingsPanel.extension.Update(snap)
		a.settingsPanel.keyboard.Refresh(snap)
	}
}

func (a *Application) startSession(baseURL, password string) {
	if a.cancel != nil {
		a.cancel()
	}
	if a.ctrl != nil {
		a.ctrl.Stop()
	}

	timeout := a.cfg.RPCTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	a.ctrl = session.New(session.Config{
		BaseURL:         baseURL,
		Password:        password,
		RPCTimeout:      timeout,
		MutationTimeout: 10 * time.Second,
		Reconnect:       true,
		ReconnectBase:   time.Second,
		ReconnectMax:    30 * time.Second,
	})
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.ctrl.Start(ctx)
	a.video.ctrl = a.ctrl
	a.sessionURL = baseURL
	log.Printf("[gtkui] session started for %s", baseURL)
}

func (a *Application) showSession() {
	a.window.SetDefaultSize(1024, 720)
	a.mainStack.SetVisibleChildName("session")
	a.video.GLArea.GrabFocus()
	a.revealUIFor(3 * time.Second)

	centerWindow(a.window)

	glib.IdleAdd(func() {
		a.chrome.ApplyPosition()
		a.applyConnectWindowMode()
	})
}

func (a *Application) applyConnectWindowMode() {
	switch a.prefs.ConnectWindowMode {
	case "maximize":
		a.window.Maximize()
	case "fullscreen":
		a.fullscreen = true
		a.window.Fullscreen()
	case "pixel":
		a.window.SetDefaultSize(1920, 1080)
	}
}

func (a *Application) pollState() {
	if a.ctrl == nil {
		if a.mainStack.VisibleChildName() == "session" {
			a.statusBar.SetText("No connection")
		}
		return
	}
	snap := a.ctrl.Snapshot()

	left := fmt.Sprintf("RTC %s  HID %s  Video %s",
		readyWord(snap.RTCState != 0),
		readyWord(snap.HIDReady),
		readyWord(snap.VideoReady))
	a.statusBar.SetText(left)

	connected := snap.Phase == session.PhaseConnected
	serialActive := snap.ActiveExtension == "serial-console"
	captureSupported := a.grabber.IsSupported()
	a.chrome.UpdateVisibility(connected, serialActive, captureSupported)

	a.pasteBanner.SetVisible(snap.PasteInProgress && a.activeOverlay == "")
}

func readyWord(v bool) string {
	if v {
		return "ready"
	}
	return "pending"
}

func (a *Application) toggleOverlay(name string) {
	if a.activeOverlay == name {
		a.closeOverlay()
		return
	}
	a.activeOverlay = name
	a.overlayStack.SetVisibleChildName(name)
	a.overlayRevealer.SetCanTarget(true)
	a.overlayRevealer.SetRevealChild(true)
	a.releaseTotalCapture()
	a.revealUIFor(500 * time.Millisecond)

	switch name {
	case "wol":
		a.wolPanel.Refresh()
	case "media":
		a.mediaPanel.RefreshStorage()
	case "settings":
		a.settingsPanel.Refresh()
	case "paste":
		a.pastePanel.Refresh()
	}
}

func (a *Application) closeOverlay() {
	a.activeOverlay = ""
	a.overlayRevealer.SetRevealChild(false)
	a.overlayRevealer.SetCanTarget(false)
	a.video.GLArea.GrabFocus()
	a.revealUIFor(1200 * time.Millisecond)
}

func (a *Application) toggleFullscreen() {
	a.fullscreen = !a.fullscreen
	if a.fullscreen {
		a.window.Fullscreen()
	} else {
		a.window.Unfullscreen()
		a.releaseTotalCapture()
	}
}

// toggleCaptureKey handles the configurable capture toggle key (default ScrollLock/F12).
// Entering fullscreen activates the X11 grab; exiting releases it.
// Key events bypass GTK and are sent as HID directly to the KVM.
func (a *Application) toggleCaptureKey() {
	if a.fullscreen {
		a.releaseTotalCapture()
		a.fullscreen = false
		a.window.Unfullscreen()
	} else {
		a.fullscreen = true
		a.window.Fullscreen()
		if a.grabber.IsSupported() && a.ctrl != nil &&
			a.ctrl.Snapshot().Phase == session.PhaseConnected && !a.totalCapture {
			a.activateCapture()
		}
	}
}

func (a *Application) toggleTotalCapture() {
	if a.totalCapture {
		a.releaseTotalCapture()
		return
	}
	a.activateCapture()
}

func (a *Application) activateCapture() {
	toggleKeysym := captureToggleX11Keysym(a.prefs.CaptureToggleKey)

	err := a.grabber.GrabWithCallback(func(evt capture.KeyEvent) bool {
		if evt.Keysym == toggleKeysym && evt.Pressed {
			glib.IdleAdd(func() {
				a.toggleCaptureKey()
			})
			return true
		}

		if a.ctrl == nil || a.ctrl.Snapshot().PasteInProgress {
			return false
		}

		if key, ok := x11KeysymToInputKey(evt.Keysym); ok {
			if hid, ok := input.KeyToHID(key); ok {
				_ = a.ctrl.SendKeypress(hid, evt.Pressed)
				return true
			}
		}
		return false
	})

	if err != nil {
		log.Printf("[gtkui] capture grab failed: %v", err)
		return
	}
	a.totalCapture = a.grabber.IsGrabbed()
}

func (a *Application) releaseTotalCapture() {
	if !a.totalCapture {
		return
	}
	_ = a.grabber.Release()
	a.totalCapture = false
}

// captureToggleX11Keysym maps the pref string to an X11 keysym.
func captureToggleX11Keysym(name string) uint32 {
	switch name {
	case "F1":
		return 0xffbe
	case "F2":
		return 0xffbf
	case "F3":
		return 0xffc0
	case "F4":
		return 0xffc1
	case "F5":
		return 0xffc2
	case "F6":
		return 0xffc3
	case "F7":
		return 0xffc4
	case "F8":
		return 0xffc5
	case "F9":
		return 0xffc6
	case "F10":
		return 0xffc7
	case "F11":
		return 0xffc8
	case "F12":
		return 0xffc9
	case "Pause":
		return 0xff13
	case "ScrollLock":
		return 0xff14
	default:
		return 0xff14
	}
}

// x11KeysymToInputKey maps X11 keysyms to input.Key.
// X11 keysyms match GDK keyvals for the most part.
func x11KeysymToInputKey(keysym uint32) (input.Key, bool) {
	return gdkKeyToInputKey(uint(keysym))
}

