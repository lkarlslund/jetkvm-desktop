package gtkui

import (
	"log"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// SettingsKeyboard configures keyboard capture, layout, and hotkeys.
type SettingsKeyboard struct {
	Box *gtk.Box
	app *Application

	swShowKeys    *gtk.Switch
	swRemoteHkeys *gtk.Switch
	ddCaptureKey  *gtk.DropDown
	ddLayout      *gtk.DropDown

	captureKeys []string
	layouts     []input.KeyboardLayout
}

func NewSettingsKeyboard(app *Application) *SettingsKeyboard {
	s := &SettingsKeyboard{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 12)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)

	// --- Show Pressed Keys ---
	s.Box.Append(sectionTitle("Display"))

	s.swShowKeys = gtk.NewSwitch()
	s.swShowKeys.SetActive(app.prefs.ShowPressedKeys)
	s.Box.Append(switchRow("Show Pressed Keys", s.swShowKeys))

	// --- Capture Toggle Key ---
	s.Box.Append(sectionTitle("Capture Toggle Key"))

	s.captureKeys = []string{
		"", "F1", "F2", "F3", "F4", "F5", "F6",
		"F7", "F8", "F9", "F10", "F11", "F12",
		"Pause", "ScrollLock",
	}
	captureLabels := make([]string, len(s.captureKeys))
	captureLabels[0] = "None"
	for i := 1; i < len(s.captureKeys); i++ {
		captureLabels[i] = s.captureKeys[i]
	}
	captureModel := gtk.NewStringList(captureLabels)
	s.ddCaptureKey = gtk.NewDropDown(captureModel, nil)
	for i, k := range s.captureKeys {
		if k == app.prefs.CaptureToggleKey {
			s.ddCaptureKey.SetSelected(uint(i))
			break
		}
	}
	s.Box.Append(s.ddCaptureKey)

	// --- Hotkeys ---
	s.Box.Append(sectionTitle("Hotkeys"))

	s.swRemoteHkeys = gtk.NewSwitch()
	s.swRemoteHkeys.SetActive(app.prefs.ExperimentalGlobalHotkeys)
	s.Box.Append(switchRow("Experimental Remote Hotkeys", s.swRemoteHkeys))

	// --- Keyboard Layout ---
	s.Box.Append(sectionTitle("Keyboard Layout"))

	s.layouts = input.SupportedKeyboardLayouts()
	labels := make([]string, len(s.layouts))
	for i, l := range s.layouts {
		labels[i] = l.Label + " (" + l.Code + ")"
	}
	layoutModel := gtk.NewStringList(labels)
	s.ddLayout = gtk.NewDropDown(layoutModel, nil)
	if app.ctrl != nil {
		snap := app.ctrl.Snapshot()
		for i, l := range s.layouts {
			if l.Code == snap.KeyboardLayout {
				s.ddLayout.SetSelected(uint(i))
				break
			}
		}
	}
	s.Box.Append(s.ddLayout)

	s.Box.Append(sectionTitle("Shortcuts"))
	captureKeyLabel := app.prefs.CaptureToggleKey
	if captureKeyLabel == "" {
		captureKeyLabel = "ScrollLock"
	}
	ref := gtk.NewLabel(captureKeyLabel + ": Toggle fullscreen + capture\nEscape: Close overlay")
	ref.AddCSSClass("dim-label")
	ref.SetXAlign(0)
	s.Box.Append(ref)

	// Apply
	applyBtn := gtk.NewButtonWithLabel("Apply")
	applyBtn.AddCSSClass("suggested-action")
	applyBtn.ConnectClicked(func() { s.apply() })
	s.Box.Append(applyBtn)

	return s
}

// Refresh updates the keyboard layout dropdown to match the target machine's
// reported layout. Called whenever the settings panel becomes visible.
func (s *SettingsKeyboard) Refresh(snap session.Snapshot) {
	if snap.KeyboardLayout == "" {
		return
	}
	for i, l := range s.layouts {
		if l.Code == snap.KeyboardLayout {
			s.ddLayout.SetSelected(uint(i))
			return
		}
	}
}

func (s *SettingsKeyboard) apply() {
	p := &s.app.prefs
	p.ShowPressedKeys = s.swShowKeys.Active()
	p.ExperimentalGlobalHotkeys = s.swRemoteHkeys.Active()

	idx := s.ddCaptureKey.Selected()
	if idx < uint(len(s.captureKeys)) {
		p.CaptureToggleKey = s.captureKeys[idx]
	}

	layoutIdx := s.ddLayout.Selected()
	if layoutIdx < uint(len(s.layouts)) {
		code := s.layouts[layoutIdx].Code
		if s.app.ctrl != nil {
			if err := s.app.ctrl.SetKeyboardLayout(code); err != nil {
				log.Printf("[settings] set keyboard layout: %v", err)
			}
		}
	}

	if err := savePrefs(*p); err != nil {
		log.Printf("[settings] save keyboard prefs: %v", err)
	}
}
