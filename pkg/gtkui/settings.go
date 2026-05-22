package gtkui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Settings holds the stack+sidebar for all 12 settings sections.
type Settings struct {
	Box *gtk.Box
	app *Application

	stack   *gtk.Stack
	sidebar *gtk.StackSidebar

	general    *SettingsGeneral
	mouse      *SettingsMouse
	keyboard   *SettingsKeyboard
	video      *SettingsVideo
	hardware   *SettingsHardware
	extension  *SettingsExtension
	access     *SettingsAccess
	appearance *SettingsAppearance
	macros     *SettingsMacros
	network    *SettingsNetwork
	mqtt       *SettingsMQTT
	advanced   *SettingsAdvanced
}

func NewSettings(app *Application) *Settings {
	s := &Settings{app: app}

	s.stack = gtk.NewStack()
	s.stack.SetTransitionType(gtk.StackTransitionTypeCrossfade)
	s.stack.SetHExpand(true)
	s.stack.SetVExpand(true)

	s.sidebar = gtk.NewStackSidebar()
	s.sidebar.SetStack(s.stack)
	s.sidebar.SetSizeRequest(160, -1)

	s.general = NewSettingsGeneral(app)
	s.mouse = NewSettingsMouse(app)
	s.keyboard = NewSettingsKeyboard(app)
	s.video = NewSettingsVideo(app)
	s.hardware = NewSettingsHardware(app)
	s.extension = NewSettingsExtension(app)
	s.access = NewSettingsAccess(app)
	s.appearance = NewSettingsAppearance(app)
	s.macros = NewSettingsMacros(app)
	s.network = NewSettingsNetwork(app)
	s.mqtt = NewSettingsMQTT(app)
	s.advanced = NewSettingsAdvanced(app)

	addSection := func(name, title string, child *gtk.Box) {
		scroll := gtk.NewScrolledWindow()
		scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
		scroll.SetChild(child)
		s.stack.AddTitled(scroll, name, title)
	}

	addSection("general", "General", s.general.Box)
	addSection("mouse", "Mouse", s.mouse.Box)
	addSection("keyboard", "Keyboard", s.keyboard.Box)
	addSection("video", "Video", s.video.Box)
	addSection("hardware", "Hardware", s.hardware.Box)
	addSection("extension", "Extension", s.extension.Box)
	addSection("access", "Access", s.access.Box)
	addSection("appearance", "Appearance", s.appearance.Box)
	addSection("macros", "Macros", s.macros.Box)
	addSection("network", "Network", s.network.Box)
	addSection("mqtt", "MQTT", s.mqtt.Box)
	addSection("advanced", "Advanced", s.advanced.Box)

	// Header with close button
	header := gtk.NewBox(gtk.OrientationHorizontal, 8)
	headerTitle := gtk.NewLabel("Settings")
	headerTitle.AddCSSClass("title-3")
	headerTitle.SetHExpand(true)
	headerTitle.SetXAlign(0)

	closeBtn := gtk.NewButtonFromIconName("window-close-symbolic")
	closeBtn.AddCSSClass("flat")
	closeBtn.SetTooltipText("Close settings")
	closeBtn.ConnectClicked(func() { app.closeOverlay() })

	header.Append(headerTitle)
	header.Append(closeBtn)
	header.SetMarginStart(16)
	header.SetMarginEnd(16)
	header.SetMarginTop(12)
	header.SetMarginBottom(4)

	content := gtk.NewBox(gtk.OrientationHorizontal, 0)
	content.Append(s.sidebar)
	content.Append(gtk.NewSeparator(gtk.OrientationVertical))
	content.Append(s.stack)
	content.SetVExpand(true)

	s.Box = gtk.NewBox(gtk.OrientationVertical, 0)
	s.Box.AddCSSClass("settings-panel")
	s.Box.SetHAlign(gtk.AlignCenter)
	s.Box.SetVAlign(gtk.AlignCenter)
	s.Box.Append(header)
	s.Box.Append(content)

	return s
}

// Refresh asks each sub-panel to pull current state from the controller.
// Called when the settings overlay is opened. Also clamps the panel size
// to the available window/work area so it never overflows the screen.
func (s *Settings) Refresh() {
	s.applySize()
	if s.app.ctrl == nil {
		return
	}
	snap := s.app.ctrl.Snapshot()
	s.video.Update(snap)
	s.video.Refresh()
	s.general.Update(snap)
	s.extension.Update(snap)
	s.keyboard.Refresh(snap)
}

func (s *Settings) applySize() {
	w, h := overlayTargetSize(s.app, 960, 680)
	s.Box.SetSizeRequest(w, h)
	// Subtract header+padding so the inner stack fits without scrollbars.
	if h > 120 {
		s.stack.SetSizeRequest(-1, h-120)
	}
}
