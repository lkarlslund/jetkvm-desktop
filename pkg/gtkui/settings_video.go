package gtkui

import (
	"context"
	"log"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// SettingsVideo configures stream quality, codec, and EDID.
type SettingsVideo struct {
	Box *gtk.Box
	app *Application

	btnQHigh *gtk.ToggleButton
	btnQMed  *gtk.ToggleButton
	btnQLow  *gtk.ToggleButton

	btnCAuto *gtk.ToggleButton
	btnCH265 *gtk.ToggleButton
	btnCH264 *gtk.ToggleButton

	lblEDID    *gtk.Label
	entCustom  *gtk.Entry
	btnApplyED *gtk.Button
	btnClearED *gtk.Button
}

type edidPreset struct {
	label string
	hex   string
}

var edidPresets = []edidPreset{
	{"JetKVM", ""},
	{"Acer", "00ffffffffffff000472cb0a624b2e03211e0104b53c2278fb4ea5a756529f270d5054bfef8081808140818090409500a940b300d1c04dd000a0f0703e803020350055502100001a565e00a0a0a029503020350055502100001a000000fd00304b73733c010a202020202020000000fc0058423237304855200a20202020017d020345f1439005040302070601061102130426150723091f0783010000e305e301e30f0003681a00000101304be6e6e5018b849001e200cf67030c001000383ce305e30f0003e3060d01a36600a0f0701f800890350055502100001a023a801871382d40582c450055502100001e0000000000000000002a"},
	{"ASUS", "00ffffffffffff000469b127010101012b200104b53c22783a76f5a655519f270d5054bfef00d1c081809500a9c0b3000101010101014dd000a0f0703e8030203a0055502100001a286800a0f0703e800890350055502100001a000000fd00304b72723c010a202020202020000000fc005647323839510a2020202020200161020330f1439005040302070601061102130415071006230907078301000067030c001000383ce305e301e30f0003e6060701606000023a801871382d40582c450055502100001e565e00a0a0a029503020350055502100001a0000000000000000000000000000000000000000000000000012"},
	{"Dell", "00ffffffffffff0010acea414c4e4d30281e010380502178ea40e5a8554fa3260b5054a54b00714f8180a9c0d1c00101010101010101565e00a0a0a029503020350055502100001a000000ff004d374d5056383331304d4e4c0a000000fc0044454c4c20553334323148570a000000fd0030551e5920000a20202020202001ce020322f144900504030201111213060715161023091f0783010000681a00000101304be6023a801871382d40582c450055502100001e7e3900a080381f4030203a0055502100001a866f80a07038404030203500a0502100001a011d007251d01e206e28550055502100001e00000000000000000000000027"},
	{"iDRAC", "00ffffffffffff001220200000000000001a0104a5301e783aa6a5a2544f9f260d50540000000101010101010101010101010101010140e7006aa0a067500820980c30203100001a286800a0a0a055500820980c30203100001a000000fd00324b1e5111000a202020202020000000fc005669727475616c204d6f6e690a00b2"},
}

func NewSettingsVideo(app *Application) *SettingsVideo {
	s := &SettingsVideo{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 12)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)

	// --- Quality ---
	s.Box.Append(sectionTitle("Stream Quality"))

	qRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	s.btnQHigh = gtk.NewToggleButtonWithLabel("High")
	s.btnQMed = gtk.NewToggleButtonWithLabel("Medium")
	s.btnQLow = gtk.NewToggleButtonWithLabel("Low")
	s.btnQMed.SetGroup(s.btnQHigh)
	s.btnQLow.SetGroup(s.btnQHigh)
	s.btnQHigh.ConnectClicked(func() { s.setQuality(1.0) })
	s.btnQMed.ConnectClicked(func() { s.setQuality(0.5) })
	s.btnQLow.ConnectClicked(func() { s.setQuality(0.25) })
	qRow.Append(s.btnQHigh)
	qRow.Append(s.btnQMed)
	qRow.Append(s.btnQLow)
	s.Box.Append(qRow)

	// --- Codec ---
	s.Box.Append(sectionTitle("Video Codec"))

	cRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	s.btnCAuto = gtk.NewToggleButtonWithLabel("Auto")
	s.btnCH265 = gtk.NewToggleButtonWithLabel("H.265")
	s.btnCH264 = gtk.NewToggleButtonWithLabel("H.264")
	s.btnCH265.SetGroup(s.btnCAuto)
	s.btnCH264.SetGroup(s.btnCAuto)
	s.btnCAuto.ConnectClicked(func() { s.setCodec(session.VideoCodecAuto) })
	s.btnCH265.ConnectClicked(func() { s.setCodec(session.VideoCodecH265) })
	s.btnCH264.ConnectClicked(func() { s.setCodec(session.VideoCodecH264) })
	cRow.Append(s.btnCAuto)
	cRow.Append(s.btnCH265)
	cRow.Append(s.btnCH264)
	s.Box.Append(cRow)

	// --- EDID ---
	s.Box.Append(sectionTitle("EDID"))

	s.lblEDID = gtk.NewLabel("—")
	s.lblEDID.SetXAlign(0)
	s.lblEDID.SetWrap(true)
	s.lblEDID.SetWrapMode(pango.WrapWordChar)
	s.lblEDID.SetMaxWidthChars(60)
	s.lblEDID.SetSelectable(true)
	s.lblEDID.AddCSSClass("monospace")
	s.lblEDID.AddCSSClass("dim-label")
	s.Box.Append(s.lblEDID)

	presetRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	for _, p := range edidPresets {
		btn := gtk.NewButtonWithLabel(p.label)
		hex := p.hex
		btn.ConnectClicked(func() { s.applyEDID(hex) })
		presetRow.Append(btn)
	}
	s.Box.Append(presetRow)

	customRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	s.entCustom = gtk.NewEntry()
	s.entCustom.SetPlaceholderText("Custom EDID hex")
	s.entCustom.SetHExpand(true)
	s.btnApplyED = gtk.NewButtonWithLabel("Apply")
	s.btnApplyED.AddCSSClass("suggested-action")
	s.btnApplyED.ConnectClicked(func() {
		s.applyEDID(s.entCustom.Text())
	})
	s.btnClearED = gtk.NewButtonWithLabel("Clear")
	s.btnClearED.ConnectClicked(func() {
		s.applyEDID("")
	})
	customRow.Append(s.entCustom)
	customRow.Append(s.btnApplyED)
	customRow.Append(s.btnClearED)
	s.Box.Append(customRow)

	return s
}

// Update refreshes displayed EDID and selects the active quality/codec toggles.
func (s *SettingsVideo) Update(snap session.Snapshot) {
	if snap.EDID != "" {
		s.lblEDID.SetText(snap.EDID)
	} else {
		s.lblEDID.SetText("(default)")
	}

	switch {
	case snap.Quality >= 0.9:
		s.btnQHigh.SetActive(true)
	case snap.Quality >= 0.4:
		s.btnQMed.SetActive(true)
	default:
		s.btnQLow.SetActive(true)
	}
}

// Refresh asks the controller for the current codec preference and selects
// the matching button. Should be called when the panel becomes visible.
func (s *SettingsVideo) Refresh() {
	if s.app.ctrl == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	codec, err := s.app.ctrl.GetVideoCodec(ctx)
	if err != nil {
		return
	}
	switch codec {
	case session.VideoCodecH265:
		s.btnCH265.SetActive(true)
	case session.VideoCodecH264:
		s.btnCH264.SetActive(true)
	default:
		s.btnCAuto.SetActive(true)
	}
}

func (s *SettingsVideo) setQuality(q float64) {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	if err := ctrl.SetQuality(q); err != nil {
		log.Printf("[settings] set quality: %v", err)
	}
}

func (s *SettingsVideo) setCodec(codec session.VideoCodec) {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	if err := ctrl.SetVideoCodec(codec); err != nil {
		log.Printf("[settings] set codec: %v", err)
	}
}

func (s *SettingsVideo) applyEDID(hex string) {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	if err := ctrl.SetEDID(hex); err != nil {
		log.Printf("[settings] set EDID: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if edid, err := ctrl.GetEDID(ctx); err == nil {
		if edid == "" {
			s.lblEDID.SetText("(default)")
		} else {
			s.lblEDID.SetText(edid)
		}
	}
}
