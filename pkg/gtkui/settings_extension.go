package gtkui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// SettingsExtension handles ATX / DC / Serial extension configuration.
type SettingsExtension struct {
	Box *gtk.Box
	app *Application

	extStack *gtk.Stack

	// ATX
	atxPowerLED *gtk.Label
	atxHDDLED   *gtk.Label

	// DC
	dcVoltage *gtk.Label
	dcCurrent *gtk.Label
	dcPower   *gtk.Label
}

func NewSettingsExtension(app *Application) *SettingsExtension {
	s := &SettingsExtension{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 8)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)

	title := sectionTitle("Extension")
	s.Box.Append(title)

	// Extension selector
	selectorBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	selectorLabel := gtk.NewLabel("Active Extension:")
	selectorLabel.AddCSSClass("dim-label")
	selectorBox.Append(selectorLabel)

	noneBtn := gtk.NewToggleButton()
	noneBtn.SetLabel("None")
	noneBtn.SetActive(true)
	atxBtn := gtk.NewToggleButton()
	atxBtn.SetLabel("ATX")
	atxBtn.SetGroup(noneBtn)
	dcBtn := gtk.NewToggleButton()
	dcBtn.SetLabel("DC")
	dcBtn.SetGroup(noneBtn)
	serialBtn := gtk.NewToggleButton()
	serialBtn.SetLabel("Serial")
	serialBtn.SetGroup(noneBtn)

	selectorBox.Append(noneBtn)
	selectorBox.Append(atxBtn)
	selectorBox.Append(dcBtn)
	selectorBox.Append(serialBtn)
	s.Box.Append(selectorBox)

	// Extension-specific panels
	s.extStack = gtk.NewStack()
	s.extStack.SetTransitionType(gtk.StackTransitionTypeCrossfade)

	emptyBox := gtk.NewBox(gtk.OrientationVertical, 0)
	emptyLabel := gtk.NewLabel("No extension selected.")
	emptyLabel.AddCSSClass("dim-label")
	emptyBox.Append(emptyLabel)
	s.extStack.AddNamed(emptyBox, "none")

	s.extStack.AddNamed(s.buildATXPanel(), "atx")
	s.extStack.AddNamed(s.buildDCPanel(), "dc")
	s.extStack.AddNamed(s.buildSerialPanel(), "serial")

	s.extStack.SetVisibleChildName("none")
	s.Box.Append(s.extStack)

	setExt := func(name string) {
		s.extStack.SetVisibleChildName(name)
		if app.ctrl != nil {
			ext := ""
			switch name {
			case "atx":
				ext = "atx-power"
			case "dc":
				ext = "dc-power"
			case "serial":
				ext = "serial-console"
			}
			_ = app.ctrl.SetActiveExtension(ext)
		}
	}
	noneBtn.ConnectClicked(func() {
		if noneBtn.Active() {
			setExt("none")
		}
	})
	atxBtn.ConnectClicked(func() {
		if atxBtn.Active() {
			setExt("atx")
		}
	})
	dcBtn.ConnectClicked(func() {
		if dcBtn.Active() {
			setExt("dc")
		}
	})
	serialBtn.ConnectClicked(func() {
		if serialBtn.Active() {
			setExt("serial")
		}
	})

	return s
}

func (s *SettingsExtension) buildATXPanel() *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(8)

	subtitle := gtk.NewLabel("ATX Power Control")
	subtitle.AddCSSClass("title-4")
	subtitle.SetXAlign(0)
	box.Append(subtitle)

	ledBox := gtk.NewBox(gtk.OrientationHorizontal, 16)
	s.atxPowerLED = gtk.NewLabel("Power LED: —")
	s.atxHDDLED = gtk.NewLabel("HDD LED: —")
	ledBox.Append(s.atxPowerLED)
	ledBox.Append(s.atxHDDLED)
	box.Append(ledBox)

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	powerBtn := gtk.NewButtonWithLabel("Power")
	powerBtn.ConnectClicked(func() {
		if s.app.ctrl != nil {
			_ = s.app.ctrl.SetATXPowerAction(session.ATXPowerActionShortPress)
		}
	})
	resetBtn := gtk.NewButtonWithLabel("Reset")
	resetBtn.AddCSSClass("destructive-action")
	resetBtn.ConnectClicked(func() {
		if s.app.ctrl != nil {
			_ = s.app.ctrl.SetATXPowerAction(session.ATXPowerActionReset)
		}
	})
	longBtn := gtk.NewButtonWithLabel("Long Press")
	longBtn.AddCSSClass("destructive-action")
	longBtn.ConnectClicked(func() {
		if s.app.ctrl != nil {
			_ = s.app.ctrl.SetATXPowerAction(session.ATXPowerActionLongPress)
		}
	})
	btnBox.Append(powerBtn)
	btnBox.Append(resetBtn)
	btnBox.Append(longBtn)
	box.Append(btnBox)

	return box
}

func (s *SettingsExtension) buildDCPanel() *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(8)

	subtitle := gtk.NewLabel("DC Power Control")
	subtitle.AddCSSClass("title-4")
	subtitle.SetXAlign(0)
	box.Append(subtitle)

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	onBtn := gtk.NewButtonWithLabel("Power On")
	onBtn.AddCSSClass("suggested-action")
	onBtn.ConnectClicked(func() {
		if s.app.ctrl != nil {
			_ = s.app.ctrl.SetDCPowerState(true)
		}
	})
	offBtn := gtk.NewButtonWithLabel("Power Off")
	offBtn.AddCSSClass("destructive-action")
	offBtn.ConnectClicked(func() {
		if s.app.ctrl != nil {
			_ = s.app.ctrl.SetDCPowerState(false)
		}
	})
	btnBox.Append(onBtn)
	btnBox.Append(offBtn)
	box.Append(btnBox)

	s.dcVoltage = gtk.NewLabel("Voltage: —")
	s.dcCurrent = gtk.NewLabel("Current: —")
	s.dcPower = gtk.NewLabel("Power: —")
	box.Append(s.dcVoltage)
	box.Append(s.dcCurrent)
	box.Append(s.dcPower)

	return box
}

func (s *SettingsExtension) buildSerialPanel() *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(8)

	subtitle := gtk.NewLabel("Serial Console Settings")
	subtitle.AddCSSClass("title-4")
	subtitle.SetXAlign(0)
	box.Append(subtitle)

	openBtn := gtk.NewButtonWithLabel("Open Console")
	openBtn.AddCSSClass("suggested-action")
	openBtn.ConnectClicked(func() {
		s.app.toggleOverlay("serial")
	})
	box.Append(openBtn)

	desc := gtk.NewLabel("Configure baud rate, data bits, parity, and stop bits in the serial console overlay.")
	desc.SetWrap(true)
	desc.AddCSSClass("dim-label")
	desc.SetXAlign(0)
	box.Append(desc)

	return box
}

func (s *SettingsExtension) Update(snap session.Snapshot) {
	if snap.ATXState != nil {
		on := "Off"
		if snap.ATXState.Power {
			on = "On"
		}
		s.atxPowerLED.SetText("Power LED: " + on)
		hdd := "Off"
		if snap.ATXState.HDD {
			hdd = "On"
		}
		s.atxHDDLED.SetText("HDD LED: " + hdd)
	}
}
