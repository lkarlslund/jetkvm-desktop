package gtkui

import (
	"context"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// SettingsAccess handles local auth, TLS, and cloud status.
type SettingsAccess struct {
	Box *gtk.Box
	app *Application

	authModeLabel *gtk.Label
	loopbackLabel *gtk.Label
	tlsModeLabel  *gtk.Label
	cloudLabel    *gtk.Label

	passwordBtnBox *gtk.Box
}

func NewSettingsAccess(app *Application) *SettingsAccess {
	s := &SettingsAccess{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 8)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)

	// Local Access
	s.Box.Append(sectionTitle("Local Access"))

	s.authModeLabel = gtk.NewLabel("Mode: —")
	s.authModeLabel.SetXAlign(0)
	s.Box.Append(s.authModeLabel)

	s.loopbackLabel = gtk.NewLabel("Loopback Only: —")
	s.loopbackLabel.SetXAlign(0)
	s.loopbackLabel.AddCSSClass("dim-label")
	s.Box.Append(s.loopbackLabel)

	s.passwordBtnBox = gtk.NewBox(gtk.OrientationHorizontal, 8)

	changePassBtn := gtk.NewButtonWithLabel("Change Password")
	changePassBtn.ConnectClicked(func() {
		// TODO: password change dialog
	})

	enablePassBtn := gtk.NewButtonWithLabel("Enable Password")
	enablePassBtn.AddCSSClass("suggested-action")
	enablePassBtn.ConnectClicked(func() {
		// TODO: enable password dialog
	})

	disablePassBtn := gtk.NewButtonWithLabel("Disable Password")
	disablePassBtn.AddCSSClass("destructive-action")
	disablePassBtn.ConnectClicked(func() {
		// TODO: disable password dialog
	})

	s.passwordBtnBox.Append(changePassBtn)
	s.passwordBtnBox.Append(enablePassBtn)
	s.passwordBtnBox.Append(disablePassBtn)
	s.Box.Append(s.passwordBtnBox)

	// TLS
	s.Box.Append(sectionTitle("TLS"))

	s.tlsModeLabel = gtk.NewLabel("Mode: —")
	s.tlsModeLabel.SetXAlign(0)
	s.Box.Append(s.tlsModeLabel)

	tlsBtnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	for _, mode := range []struct {
		label string
		mode  session.TLSMode
	}{
		{"Disabled", session.TLSModeDisabled},
		{"Self-Signed", session.TLSModeSelfSigned},
		{"Custom", session.TLSModeCustom},
	} {
		btn := gtk.NewButtonWithLabel(mode.label)
		m := mode.mode
		btn.ConnectClicked(func() {
			if app.ctrl != nil {
				_ = app.ctrl.SetTLSMode(m)
			}
		})
		tlsBtnBox.Append(btn)
	}
	s.Box.Append(tlsBtnBox)

	// Cloud
	s.Box.Append(sectionTitle("Cloud"))
	s.cloudLabel = gtk.NewLabel("—")
	s.cloudLabel.SetXAlign(0)
	s.cloudLabel.AddCSSClass("dim-label")
	s.Box.Append(s.cloudLabel)

	return s
}

func (s *SettingsAccess) Refresh() {
	if s.app.ctrl == nil {
		return
	}
	authMode, loopback, err := s.app.ctrl.GetLocalAccessState(context.Background())
	if err != nil {
		return
	}
	switch authMode {
	case session.LocalAuthModeNoPassword:
		s.authModeLabel.SetText("Mode: No Password")
	case session.LocalAuthModePassword:
		s.authModeLabel.SetText("Mode: Password")
	default:
		s.authModeLabel.SetText("Mode: Unknown")
	}
	if loopback {
		s.loopbackLabel.SetText("Loopback Only: Yes")
	} else {
		s.loopbackLabel.SetText("Loopback Only: No")
	}

	tlsState, err := s.app.ctrl.GetTLSState(context.Background())
	if err == nil {
		s.tlsModeLabel.SetText("TLS Mode: " + string(tlsState.Mode))
	}

	cloud, err := s.app.ctrl.GetCloudState(context.Background())
	if err == nil {
		if cloud.Connected {
			s.cloudLabel.SetText("Connected — " + cloud.URL)
		} else {
			s.cloudLabel.SetText("Not connected")
		}
	}
}
