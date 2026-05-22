package gtkui

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// SettingsGeneral shows device info, update controls, and session actions.
type SettingsGeneral struct {
	Box *gtk.Box
	app *Application

	lblBaseURL     *gtk.Label
	lblPhase       *gtk.Label
	lblSignaling   *gtk.Label
	lblAppVersion  *gtk.Label
	lblSysVersion  *gtk.Label
	lblUpdateAvail *gtk.Label
	swAutoUpdate   *gtk.Switch
	btnCheckUpdate *gtk.Button
	btnInstall     *gtk.Button
}

func NewSettingsGeneral(app *Application) *SettingsGeneral {
	s := &SettingsGeneral{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 12)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)

	// --- Device info card ---
	s.Box.Append(sectionTitle("Device Info"))

	s.lblBaseURL = gtk.NewLabel("—")
	s.lblPhase = gtk.NewLabel("—")
	s.lblSignaling = gtk.NewLabel("—")
	appendLabelRow(s.Box, "Base URL", s.lblBaseURL)
	appendLabelRow(s.Box, "Phase", s.lblPhase)
	appendLabelRow(s.Box, "Signaling", s.lblSignaling)

	// --- Updates card ---
	s.Box.Append(sectionTitle("Updates"))

	s.lblAppVersion = gtk.NewLabel("—")
	s.lblSysVersion = gtk.NewLabel("—")
	s.lblUpdateAvail = gtk.NewLabel("")
	appendLabelRow(s.Box, "App Version", s.lblAppVersion)
	appendLabelRow(s.Box, "System Version", s.lblSysVersion)
	appendLabelRow(s.Box, "Update Available", s.lblUpdateAvail)

	btnRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	s.btnCheckUpdate = gtk.NewButtonWithLabel("Check for Updates")
	s.btnCheckUpdate.ConnectClicked(func() { s.checkUpdate() })
	s.btnInstall = gtk.NewButtonWithLabel("Install Updates")
	s.btnInstall.AddCSSClass("suggested-action")
	s.btnInstall.ConnectClicked(func() { s.installUpdate() })
	btnRow.Append(s.btnCheckUpdate)
	btnRow.Append(s.btnInstall)

	autoRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	autoLbl := gtk.NewLabel("Auto Updates")
	autoLbl.SetHExpand(true)
	autoLbl.SetXAlign(0)
	s.swAutoUpdate = gtk.NewSwitch()
	s.swAutoUpdate.ConnectStateSet(func(state bool) bool {
		s.setAutoUpdate(state)
		return false
	})
	autoRow.Append(autoLbl)
	autoRow.Append(s.swAutoUpdate)

	s.Box.Append(btnRow)
	s.Box.Append(autoRow)

	// --- Actions card ---
	s.Box.Append(sectionTitle("Actions"))

	actRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnReconnect := gtk.NewButtonWithLabel("Reconnect")
	btnReconnect.ConnectClicked(func() {
		if app.ctrl != nil {
			app.ctrl.ReconnectNow()
		}
	})
	btnReboot := gtk.NewButtonWithLabel("Reboot Device")
	btnReboot.AddCSSClass("destructive-action")
	btnReboot.ConnectClicked(func() {
		if app.ctrl != nil {
			if err := app.ctrl.Reboot(); err != nil {
				log.Printf("[settings] reboot: %v", err)
			}
		}
	})
	actRow.Append(btnReconnect)
	actRow.Append(btnReboot)
	s.Box.Append(actRow)

	return s
}

// Update refreshes all displayed values from the current snapshot.
func (s *SettingsGeneral) Update(snap session.Snapshot) {
	s.lblBaseURL.SetText(snap.BaseURL)
	s.lblPhase.SetText(snap.Phase.String())
	s.lblSignaling.SetText(string(snap.SignalingMode))
	s.lblAppVersion.SetText(orDash(snap.AppVersion))
	s.lblSysVersion.SetText(orDash(snap.SystemVersion))

	avail := ""
	if snap.AppUpdateAvailable {
		avail += "App "
	}
	if snap.SystemUpdateAvailable {
		avail += "System "
	}
	if avail == "" {
		avail = "No"
	}
	s.lblUpdateAvail.SetText(avail)
}

func (s *SettingsGeneral) checkUpdate() {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	status, err := ctrl.GetUpdateStatus(ctx)
	if err != nil {
		log.Printf("[settings] check update: %v", err)
		return
	}
	s.lblAppVersion.SetText(fmt.Sprintf("%s → %s", status.Local.AppVersion, status.Remote.AppVersion))
	s.lblSysVersion.SetText(fmt.Sprintf("%s → %s", status.Local.SystemVersion, status.Remote.SystemVersion))

	avail := ""
	if status.AppUpdateAvailable {
		avail += "App "
	}
	if status.SystemUpdateAvailable {
		avail += "System "
	}
	if avail == "" {
		avail = "No"
	}
	s.lblUpdateAvail.SetText(avail)
}

func (s *SettingsGeneral) installUpdate() {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	if err := ctrl.TryUpdate(); err != nil {
		log.Printf("[settings] install update: %v", err)
	}
}

func (s *SettingsGeneral) setAutoUpdate(enabled bool) {
	ctrl := s.app.ctrl
	if ctrl == nil {
		return
	}
	if err := ctrl.SetAutoUpdateState(enabled); err != nil {
		log.Printf("[settings] set auto update: %v", err)
	}
}

// --- helpers ---

func sectionTitle(text string) *gtk.Label {
	l := gtk.NewLabel(text)
	l.AddCSSClass("title-4")
	l.SetXAlign(0)
	l.SetMarginTop(8)
	return l
}

func appendLabelRow(parent *gtk.Box, name string, value *gtk.Label) {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	lbl := gtk.NewLabel(name)
	lbl.AddCSSClass("dim-label")
	lbl.SetXAlign(0)
	lbl.SetHExpand(true)
	value.SetXAlign(1)
	row.Append(lbl)
	row.Append(value)
	parent.Append(row)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
