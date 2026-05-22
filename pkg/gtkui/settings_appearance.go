package gtkui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// SettingsAppearance provides theme, chrome visibility and window mode controls.
type SettingsAppearance struct {
	Box *gtk.Box

	app *Application

	themeButtons  []*gtk.ToggleButton
	windowButtons []*gtk.ToggleButton
}

func NewSettingsAppearance(app *Application) *SettingsAppearance {
	s := &SettingsAppearance{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 8)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)

	// --- Theme ---
	themeTitle := gtk.NewLabel("Theme")
	themeTitle.AddCSSClass("title-4")
	themeTitle.SetXAlign(0)
	s.Box.Append(themeTitle)

	themeBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	themes := []struct {
		label string
		value string
	}{
		{"System", "system"},
		{"Dark", "dark"},
		{"Light", "light"},
	}
	var firstThemeBtn *gtk.ToggleButton
	for _, t := range themes {
		btn := gtk.NewToggleButtonWithLabel(t.label)
		if firstThemeBtn != nil {
			btn.SetGroup(firstThemeBtn)
		} else {
			firstThemeBtn = btn
		}
		btn.SetActive(app.prefs.Theme == t.value)
		val := t.value
		btn.ConnectClicked(func() {
			s.selectTheme(val)
		})
		themeBox.Append(btn)
		s.themeButtons = append(s.themeButtons, btn)
	}
	s.Box.Append(themeBox)

	// --- Always Visible (PinChrome) ---
	chromeRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	chromeLabel := gtk.NewLabel("Always Visible Chrome")
	chromeLabel.SetHExpand(true)
	chromeLabel.SetXAlign(0)
	chromeSwitch := gtk.NewSwitch()
	chromeSwitch.SetActive(app.prefs.PinChrome)
	chromeSwitch.ConnectStateSet(func(state bool) bool {
		app.prefs.PinChrome = state
		savePrefs(app.prefs)
		return false
	})
	chromeRow.Append(chromeLabel)
	chromeRow.Append(chromeSwitch)
	s.Box.Append(chromeRow)

	// --- Hide Footer Status ---
	statusRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	statusLabel := gtk.NewLabel("Hide Footer Status")
	statusLabel.SetHExpand(true)
	statusLabel.SetXAlign(0)
	statusSwitch := gtk.NewSwitch()
	statusSwitch.SetActive(app.prefs.HideStatusBar)
	statusSwitch.ConnectStateSet(func(state bool) bool {
		app.prefs.HideStatusBar = state
		savePrefs(app.prefs)
		return false
	})
	statusRow.Append(statusLabel)
	statusRow.Append(statusSwitch)
	s.Box.Append(statusRow)

	// --- Toggle Fullscreen ---
	fsBtn := gtk.NewButtonWithLabel("Toggle Fullscreen")
	fsBtn.ConnectClicked(func() {
		app.toggleFullscreen()
	})
	s.Box.Append(fsBtn)

	// --- Initial window mode ---
	modeTitle := gtk.NewLabel("Initial Window Mode")
	modeTitle.AddCSSClass("title-4")
	modeTitle.SetXAlign(0)
	s.Box.Append(modeTitle)

	modeBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	modes := []struct {
		label string
		value string
	}{
		{"Unchanged", "unchanged"},
		{"Maximize", "maximize"},
		{"1:1 Pixels", "pixel"},
		{"Fullscreen", "fullscreen"},
	}
	var firstModeBtn *gtk.ToggleButton
	for _, m := range modes {
		btn := gtk.NewToggleButtonWithLabel(m.label)
		if firstModeBtn != nil {
			btn.SetGroup(firstModeBtn)
		} else {
			firstModeBtn = btn
		}
		btn.SetActive(app.prefs.ConnectWindowMode == m.value)
		val := m.value
		btn.ConnectClicked(func() {
			s.selectWindowMode(val)
		})
		modeBox.Append(btn)
		s.windowButtons = append(s.windowButtons, btn)
	}
	s.Box.Append(modeBox)

	// --- Reset position ---
	resetBtn := gtk.NewButtonWithLabel("Reset Window Position")
	resetBtn.ConnectClicked(func() {
		app.window.SetDefaultSize(1024, 720)
		// TODO: clear stored position if tracked
	})
	s.Box.Append(resetBtn)

	return s
}

func (s *SettingsAppearance) selectTheme(value string) {
	themes := []string{"system", "dark", "light"}
	for i, btn := range s.themeButtons {
		btn.SetActive(themes[i] == value)
	}
	s.app.prefs.Theme = value
	savePrefs(s.app.prefs)
	applyTheme(s.app.prefs)
}

func (s *SettingsAppearance) selectWindowMode(value string) {
	modes := []string{"unchanged", "maximize", "pixel", "fullscreen"}
	for i, btn := range s.windowButtons {
		btn.SetActive(modes[i] == value)
	}
	s.app.prefs.ConnectWindowMode = value
	savePrefs(s.app.prefs)
}
