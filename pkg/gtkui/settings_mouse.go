package gtkui

import (
	"log"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// SettingsMouse configures pointer mode, cursor, scroll, and jiggler.
type SettingsMouse struct {
	Box *gtk.Box
	app *Application

	btnAbsolute    *gtk.ToggleButton
	btnRelative    *gtk.ToggleButton
	swHideCursor   *gtk.Switch
	swInvertScroll *gtk.Switch
	swSideButtons  *gtk.Switch
	entScrollMs    *gtk.Entry
	entMoveMs      *gtk.Entry

	lblJigglerState *gtk.Label
}

func NewSettingsMouse(app *Application) *SettingsMouse {
	s := &SettingsMouse{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 12)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)

	// --- Pointer card ---
	s.Box.Append(sectionTitle("Pointer"))

	modeRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	modeLbl := gtk.NewLabel("Mode")
	modeLbl.SetHExpand(true)
	modeLbl.SetXAlign(0)

	s.btnAbsolute = gtk.NewToggleButtonWithLabel("Absolute")
	s.btnRelative = gtk.NewToggleButtonWithLabel("Relative")
	s.btnRelative.SetGroup(s.btnAbsolute)

	modeRow.Append(modeLbl)
	modeRow.Append(s.btnAbsolute)
	modeRow.Append(s.btnRelative)
	s.Box.Append(modeRow)

	s.swHideCursor = gtk.NewSwitch()
	s.Box.Append(switchRow("Hide Host Cursor", s.swHideCursor))

	s.swInvertScroll = gtk.NewSwitch()
	s.Box.Append(switchRow("Invert Scroll", s.swInvertScroll))

	s.swSideButtons = gtk.NewSwitch()
	s.Box.Append(switchRow("Side Buttons via Relative", s.swSideButtons))

	s.entScrollMs = gtk.NewEntry()
	s.entScrollMs.SetPlaceholderText("ms")
	s.entScrollMs.SetMaxWidthChars(6)
	s.Box.Append(entryRow("Scroll Throttle (ms)", s.entScrollMs))

	s.entMoveMs = gtk.NewEntry()
	s.entMoveMs.SetPlaceholderText("ms")
	s.entMoveMs.SetMaxWidthChars(6)
	s.Box.Append(entryRow("Move Throttle (ms)", s.entMoveMs))

	// --- Jiggler card ---
	s.Box.Append(sectionTitle("Mouse Jiggler"))

	s.lblJigglerState = gtk.NewLabel("—")
	appendLabelRow(s.Box, "State", s.lblJigglerState)

	jigglerRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	for _, preset := range []struct {
		label string
		on    bool
	}{
		{"Disabled", false},
		{"Enabled", true},
	} {
		btn := gtk.NewButtonWithLabel(preset.label)
		enabled := preset.on
		btn.ConnectClicked(func() { s.setJiggler(enabled) })
		jigglerRow.Append(btn)
	}
	s.Box.Append(jigglerRow)

	// Apply button
	applyBtn := gtk.NewButtonWithLabel("Apply")
	applyBtn.AddCSSClass("suggested-action")
	applyBtn.ConnectClicked(func() { s.applyPrefs() })
	s.Box.Append(applyBtn)

	s.loadPrefs()
	return s
}

func (s *SettingsMouse) loadPrefs() {
	p := &s.app.prefs
	s.swHideCursor.SetActive(p.HideCursor)
	s.swInvertScroll.SetActive(p.InvertScroll)
	s.swSideButtons.SetActive(p.AbsoluteSideButtonsViaRel)
	if p.ScrollThrottleMs > 0 {
		s.entScrollMs.SetText(strconv.Itoa(p.ScrollThrottleMs))
	}
	if p.PointerMoveThrottleMs > 0 {
		s.entMoveMs.SetText(strconv.Itoa(p.PointerMoveThrottleMs))
	}
}

func (s *SettingsMouse) applyPrefs() {
	p := &s.app.prefs
	p.HideCursor = s.swHideCursor.Active()
	p.InvertScroll = s.swInvertScroll.Active()
	p.AbsoluteSideButtonsViaRel = s.swSideButtons.Active()
	if v, err := strconv.Atoi(s.entScrollMs.Text()); err == nil {
		p.ScrollThrottleMs = v
	}
	if v, err := strconv.Atoi(s.entMoveMs.Text()); err == nil {
		p.PointerMoveThrottleMs = v
	}
	if err := savePrefs(*p); err != nil {
		log.Printf("[settings] save mouse prefs: %v", err)
	}
}

func (s *SettingsMouse) setJiggler(enabled bool) {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	if err := ctrl.SetJigglerState(enabled); err != nil {
		log.Printf("[settings] set jiggler: %v", err)
		return
	}
	if enabled {
		s.lblJigglerState.SetText("Enabled")
	} else {
		s.lblJigglerState.SetText("Disabled")
	}
}

// --- helpers ---

func switchRow(label string, sw *gtk.Switch) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	lbl := gtk.NewLabel(label)
	lbl.SetHExpand(true)
	lbl.SetXAlign(0)
	row.Append(lbl)
	row.Append(sw)
	return row
}

func entryRow(label string, entry *gtk.Entry) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	lbl := gtk.NewLabel(label)
	lbl.SetHExpand(true)
	lbl.SetXAlign(0)
	row.Append(lbl)
	row.Append(entry)
	return row
}
