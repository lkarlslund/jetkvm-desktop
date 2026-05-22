package gtkui

import (
	"context"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// SettingsAdvanced provides developer mode, dev channel, loopback, SSH key, and factory reset.
type SettingsAdvanced struct {
	Box *gtk.Box

	app *Application

	devModeLabel    *gtk.Label
	devModeSwitch   *gtk.Switch
	devChannelLabel *gtk.Label
	devChannelSwitch *gtk.Switch
	loopbackLabel   *gtk.Label
	loopbackSwitch  *gtk.Switch
	versionLabel    *gtk.Label

	sshEntry *gtk.Entry

	confirmBox *gtk.Box
}

func NewSettingsAdvanced(app *Application) *SettingsAdvanced {
	s := &SettingsAdvanced{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 8)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)

	title := gtk.NewLabel("Advanced Settings")
	title.AddCSSClass("title-3")
	title.SetXAlign(0)
	s.Box.Append(title)

	// --- Info section ---
	infoTitle := gtk.NewLabel("Device Info")
	infoTitle.AddCSSClass("title-4")
	infoTitle.SetXAlign(0)
	s.Box.Append(infoTitle)

	s.versionLabel = s.addInfoRow("Version")

	// --- Developer Mode ---
	devTitle := gtk.NewLabel("Developer Options")
	devTitle.AddCSSClass("title-4")
	devTitle.SetXAlign(0)
	s.Box.Append(devTitle)

	devModeRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	s.devModeLabel = gtk.NewLabel("Developer Mode")
	s.devModeLabel.SetHExpand(true)
	s.devModeLabel.SetXAlign(0)
	s.devModeSwitch = gtk.NewSwitch()
	s.devModeSwitch.ConnectStateSet(func(state bool) bool {
		if app.ctrl != nil {
			_ = app.ctrl.SetDeveloperModeState(state)
		}
		return false
	})
	devModeRow.Append(s.devModeLabel)
	devModeRow.Append(s.devModeSwitch)
	s.Box.Append(devModeRow)

	// --- Dev Channel ---
	devChRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	s.devChannelLabel = gtk.NewLabel("Dev Channel")
	s.devChannelLabel.SetHExpand(true)
	s.devChannelLabel.SetXAlign(0)
	s.devChannelSwitch = gtk.NewSwitch()
	s.devChannelSwitch.ConnectStateSet(func(state bool) bool {
		if app.ctrl != nil {
			_ = app.ctrl.SetDevChannelState(state)
		}
		return false
	})
	devChRow.Append(s.devChannelLabel)
	devChRow.Append(s.devChannelSwitch)
	s.Box.Append(devChRow)

	// --- Loopback Only ---
	loopRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	s.loopbackLabel = gtk.NewLabel("Loopback Only")
	s.loopbackLabel.SetHExpand(true)
	s.loopbackLabel.SetXAlign(0)
	s.loopbackSwitch = gtk.NewSwitch()
	s.loopbackSwitch.ConnectStateSet(func(state bool) bool {
		if app.ctrl != nil {
			_ = app.ctrl.SetLocalLoopbackOnly(state)
		}
		return false
	})
	loopRow.Append(s.loopbackLabel)
	loopRow.Append(s.loopbackSwitch)
	s.Box.Append(loopRow)

	// --- SSH Key ---
	sshTitle := gtk.NewLabel("SSH Key")
	sshTitle.AddCSSClass("title-4")
	sshTitle.SetXAlign(0)
	s.Box.Append(sshTitle)

	sshDesc := gtk.NewLabel("Paste an authorized public key to enable SSH access.")
	sshDesc.AddCSSClass("dim-label")
	sshDesc.SetWrap(true)
	sshDesc.SetXAlign(0)
	s.Box.Append(sshDesc)

	s.sshEntry = gtk.NewEntry()
	s.sshEntry.SetPlaceholderText("ssh-ed25519 AAAA...")
	s.sshEntry.SetHExpand(true)
	s.Box.Append(s.sshEntry)

	sshSaveBtn := gtk.NewButtonWithLabel("Save SSH Key")
	sshSaveBtn.ConnectClicked(func() {
		if app.ctrl != nil {
			_ = app.ctrl.SetSSHKeyState(s.sshEntry.Text())
		}
	})
	s.Box.Append(sshSaveBtn)

	// --- Factory Reset ---
	resetTitle := gtk.NewLabel("Factory Reset")
	resetTitle.AddCSSClass("title-4")
	resetTitle.SetXAlign(0)
	s.Box.Append(resetTitle)

	resetDesc := gtk.NewLabel("This will erase all settings and restore factory defaults. This cannot be undone.")
	resetDesc.AddCSSClass("dim-label")
	resetDesc.SetWrap(true)
	resetDesc.SetXAlign(0)
	s.Box.Append(resetDesc)

	resetBtn := gtk.NewButtonWithLabel("Factory Reset")
	resetBtn.AddCSSClass("destructive-action")
	resetBtn.ConnectClicked(func() {
		s.confirmBox.SetVisible(true)
	})
	s.Box.Append(resetBtn)

	// Confirm panel (hidden by default)
	s.confirmBox = gtk.NewBox(gtk.OrientationHorizontal, 8)
	s.confirmBox.SetVisible(false)
	confirmLabel := gtk.NewLabel("Are you sure?")
	confirmLabel.SetHExpand(true)
	confirmLabel.SetXAlign(0)
	confirmBtn := gtk.NewButtonWithLabel("Confirm Reset")
	confirmBtn.AddCSSClass("destructive-action")
	confirmBtn.ConnectClicked(func() {
		if app.ctrl != nil {
			_ = app.ctrl.FactoryReset()
		}
		s.confirmBox.SetVisible(false)
	})
	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		s.confirmBox.SetVisible(false)
	})
	s.confirmBox.Append(confirmLabel)
	s.confirmBox.Append(cancelBtn)
	s.confirmBox.Append(confirmBtn)
	s.Box.Append(s.confirmBox)

	s.loadState()
	return s
}

func (s *SettingsAdvanced) addInfoRow(label string) *gtk.Label {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	l := gtk.NewLabel(label)
	l.AddCSSClass("dim-label")
	l.SetXAlign(0)
	l.SetHExpand(true)
	val := gtk.NewLabel("—")
	val.SetXAlign(1)
	row.Append(l)
	row.Append(val)
	s.Box.Append(row)
	return val
}

func (s *SettingsAdvanced) loadState() {
	if s.app.ctrl == nil {
		return
	}
	ctx := context.Background()

	devMode, err := s.app.ctrl.GetDeveloperModeState(ctx)
	if err == nil && devMode != nil {
		s.devModeSwitch.SetActive(*devMode)
	}

	devCh, err := s.app.ctrl.GetDevChannelState(ctx)
	if err == nil && devCh != nil {
		s.devChannelSwitch.SetActive(*devCh)
	}

	loopback, err := s.app.ctrl.GetLocalLoopbackOnly(ctx)
	if err == nil && loopback != nil {
		s.loopbackSwitch.SetActive(*loopback)
	}

	ver, err := s.app.ctrl.GetLocalVersion(ctx)
	if err == nil {
		s.versionLabel.SetText(ver.AppVersion + " / " + ver.SystemVersion)
	}

	sshKey, err := s.app.ctrl.GetSSHKeyState(ctx)
	if err == nil {
		s.sshEntry.SetText(sshKey)
	}
}
