package gtkui

import (
	"context"
	"log"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// SettingsHardware configures display rotation, brightness, and USB emulation.
type SettingsHardware struct {
	Box *gtk.Box
	app *Application

	btnRotNormal   *gtk.ToggleButton
	btnRotInverted *gtk.ToggleButton

	btnBrOff  *gtk.ToggleButton
	btnBrLow  *gtk.ToggleButton
	btnBrMed  *gtk.ToggleButton
	btnBrHigh *gtk.ToggleButton

	entDimAfter *gtk.Entry
	entOffAfter *gtk.Entry
	entSleepDur *gtk.Entry

	swUSB *gtk.Switch

	swAbsMouse *gtk.Switch
	swRelMouse *gtk.Switch
	swKeyboard *gtk.Switch
	swStorage  *gtk.Switch
}

func NewSettingsHardware(app *Application) *SettingsHardware {
	s := &SettingsHardware{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 12)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)

	// --- Display ---
	s.Box.Append(sectionTitle("Display"))

	rotRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	rotLbl := gtk.NewLabel("Rotation")
	rotLbl.SetHExpand(true)
	rotLbl.SetXAlign(0)
	s.btnRotNormal = gtk.NewToggleButtonWithLabel("Normal")
	s.btnRotInverted = gtk.NewToggleButtonWithLabel("Inverted")
	s.btnRotInverted.SetGroup(s.btnRotNormal)
	s.btnRotNormal.ConnectClicked(func() { s.setRotation(session.DisplayRotationNormal) })
	s.btnRotInverted.ConnectClicked(func() { s.setRotation(session.DisplayRotationInverted) })
	rotRow.Append(rotLbl)
	rotRow.Append(s.btnRotNormal)
	rotRow.Append(s.btnRotInverted)
	s.Box.Append(rotRow)

	brRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	brLbl := gtk.NewLabel("Brightness")
	brLbl.SetHExpand(true)
	brLbl.SetXAlign(0)
	s.btnBrOff = gtk.NewToggleButtonWithLabel("Off")
	s.btnBrLow = gtk.NewToggleButtonWithLabel("Low")
	s.btnBrMed = gtk.NewToggleButtonWithLabel("Med")
	s.btnBrHigh = gtk.NewToggleButtonWithLabel("High")
	s.btnBrLow.SetGroup(s.btnBrOff)
	s.btnBrMed.SetGroup(s.btnBrOff)
	s.btnBrHigh.SetGroup(s.btnBrOff)
	s.btnBrOff.ConnectClicked(func() { s.setBrightness(0) })
	s.btnBrLow.ConnectClicked(func() { s.setBrightness(30) })
	s.btnBrMed.ConnectClicked(func() { s.setBrightness(60) })
	s.btnBrHigh.ConnectClicked(func() { s.setBrightness(100) })
	brRow.Append(brLbl)
	brRow.Append(s.btnBrOff)
	brRow.Append(s.btnBrLow)
	brRow.Append(s.btnBrMed)
	brRow.Append(s.btnBrHigh)
	s.Box.Append(brRow)

	s.entDimAfter = gtk.NewEntry()
	s.entDimAfter.SetPlaceholderText("seconds")
	s.entDimAfter.SetMaxWidthChars(8)
	s.Box.Append(entryRow("Dim After (s)", s.entDimAfter))

	s.entOffAfter = gtk.NewEntry()
	s.entOffAfter.SetPlaceholderText("seconds")
	s.entOffAfter.SetMaxWidthChars(8)
	s.Box.Append(entryRow("Off After (s)", s.entOffAfter))

	s.entSleepDur = gtk.NewEntry()
	s.entSleepDur.SetPlaceholderText("seconds, 0=disabled")
	s.entSleepDur.SetMaxWidthChars(8)
	s.Box.Append(entryRow("HDMI Sleep Duration (s)", s.entSleepDur))

	btnApplyDisp := gtk.NewButtonWithLabel("Apply Display Settings")
	btnApplyDisp.AddCSSClass("suggested-action")
	btnApplyDisp.ConnectClicked(func() { s.applyDisplaySettings() })
	s.Box.Append(btnApplyDisp)

	// --- USB ---
	s.Box.Append(sectionTitle("USB Emulation"))

	s.swUSB = gtk.NewSwitch()
	s.swUSB.ConnectStateSet(func(state bool) bool {
		s.setUSBEmulation(state)
		return false
	})
	s.Box.Append(switchRow("Enable USB", s.swUSB))

	s.Box.Append(sectionTitle("USB Devices"))

	s.swAbsMouse = gtk.NewSwitch()
	s.swRelMouse = gtk.NewSwitch()
	s.swKeyboard = gtk.NewSwitch()
	s.swStorage = gtk.NewSwitch()
	s.Box.Append(switchRow("Absolute Mouse", s.swAbsMouse))
	s.Box.Append(switchRow("Relative Mouse", s.swRelMouse))
	s.Box.Append(switchRow("Keyboard", s.swKeyboard))
	s.Box.Append(switchRow("Mass Storage", s.swStorage))

	btnApplyUSB := gtk.NewButtonWithLabel("Apply USB Devices")
	btnApplyUSB.AddCSSClass("suggested-action")
	btnApplyUSB.ConnectClicked(func() { s.applyUSBDevices() })
	s.Box.Append(btnApplyUSB)

	return s
}

func (s *SettingsHardware) setRotation(rot session.DisplayRotation) {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	if err := ctrl.SetDisplayRotation(rot); err != nil {
		log.Printf("[settings] set rotation: %v", err)
	}
}

func (s *SettingsHardware) setBrightness(level int) {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	current, err := ctrl.GetBacklightSettings(ctx)
	if err != nil {
		log.Printf("[settings] get backlight: %v", err)
		return
	}
	current.MaxBrightness = level
	if err := ctrl.SetBacklightSettings(current); err != nil {
		log.Printf("[settings] set backlight: %v", err)
	}
}

func (s *SettingsHardware) applyDisplaySettings() {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	current, err := ctrl.GetBacklightSettings(ctx)
	if err != nil {
		current = session.BacklightSettings{}
	}
	if v := parseIntEntry(s.entDimAfter); v >= 0 {
		current.DimAfter = v
	}
	if v := parseIntEntry(s.entOffAfter); v >= 0 {
		current.OffAfter = v
	}
	if err := ctrl.SetBacklightSettings(current); err != nil {
		log.Printf("[settings] set backlight: %v", err)
	}

	sleepDur := parseIntEntry(s.entSleepDur)
	if sleepDur >= 0 {
		if err := ctrl.SetVideoSleepMode(sleepDur); err != nil {
			log.Printf("[settings] set video sleep: %v", err)
		}
	}
}

func (s *SettingsHardware) setUSBEmulation(enabled bool) {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	if err := ctrl.SetUSBEmulation(enabled); err != nil {
		log.Printf("[settings] set USB emulation: %v", err)
	}
}

func (s *SettingsHardware) applyUSBDevices() {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	if err := ctrl.SetUSBDevices(session.USBDevices{
		AbsoluteMouse: s.swAbsMouse.Active(),
		RelativeMouse: s.swRelMouse.Active(),
		Keyboard:      s.swKeyboard.Active(),
		MassStorage:   s.swStorage.Active(),
	}); err != nil {
		log.Printf("[settings] set USB devices: %v", err)
	}
}

func parseIntEntry(e *gtk.Entry) int {
	text := e.Text()
	if text == "" {
		return -1
	}
	var v int
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return -1
		}
		v = v*10 + int(ch-'0')
	}
	return v
}
