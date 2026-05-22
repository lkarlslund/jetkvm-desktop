package gtkui

import (
	"context"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// SettingsMQTT provides MQTT broker configuration and status.
type SettingsMQTT struct {
	Box *gtk.Box

	app *Application

	enableSwitch    *gtk.Switch
	brokerEntry     *gtk.Entry
	portEntry       *gtk.Entry
	usernameEntry   *gtk.Entry
	passwordEntry   *gtk.Entry
	baseTopicEntry  *gtk.Entry
	tlsSwitch       *gtk.Switch
	tlsInsecure     *gtk.Switch
	haDiscovery     *gtk.Switch
	enableActions   *gtk.Switch
	debounceEntry   *gtk.Entry

	statusConnLabel *gtk.Label
	statusBrkLabel  *gtk.Label
	statusErrLabel  *gtk.Label
}

func NewSettingsMQTT(app *Application) *SettingsMQTT {
	s := &SettingsMQTT{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 8)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)

	title := gtk.NewLabel("MQTT Settings")
	title.AddCSSClass("title-3")
	title.SetXAlign(0)
	s.Box.Append(title)

	// Enable switch
	enableRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	enableLabel := gtk.NewLabel("Enable MQTT")
	enableLabel.SetHExpand(true)
	enableLabel.SetXAlign(0)
	s.enableSwitch = gtk.NewSwitch()
	enableRow.Append(enableLabel)
	enableRow.Append(s.enableSwitch)
	s.Box.Append(enableRow)

	// Config fields
	s.brokerEntry = s.addEntryRow("Broker")
	s.portEntry = s.addEntryRow("Port")
	s.usernameEntry = s.addEntryRow("Username")
	s.passwordEntry = s.addEntryRow("Password")
	s.passwordEntry.SetVisibility(false)
	s.baseTopicEntry = s.addEntryRow("Base Topic")

	// TLS toggles
	tlsTitle := gtk.NewLabel("TLS")
	tlsTitle.AddCSSClass("title-4")
	tlsTitle.SetXAlign(0)
	s.Box.Append(tlsTitle)

	s.tlsSwitch = s.addSwitchRow("Use TLS")
	s.tlsInsecure = s.addSwitchRow("Allow Insecure TLS")

	// Feature toggles
	featTitle := gtk.NewLabel("Features")
	featTitle.AddCSSClass("title-4")
	featTitle.SetXAlign(0)
	s.Box.Append(featTitle)

	s.haDiscovery = s.addSwitchRow("Home Assistant Discovery")
	s.enableActions = s.addSwitchRow("Enable MQTT Actions")

	// Debounce
	s.debounceEntry = s.addEntryRow("Debounce (ms)")

	// Buttons
	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnBox.SetHAlign(gtk.AlignEnd)
	testBtn := gtk.NewButtonWithLabel("Test Connection")
	testBtn.ConnectClicked(func() { s.testConnection() })
	saveBtn := gtk.NewButtonWithLabel("Save Settings")
	saveBtn.AddCSSClass("suggested-action")
	saveBtn.ConnectClicked(func() { s.saveSettings() })
	btnBox.Append(testBtn)
	btnBox.Append(saveBtn)
	s.Box.Append(btnBox)

	// --- Status Card ---
	statusTitle := gtk.NewLabel("Status")
	statusTitle.AddCSSClass("title-4")
	statusTitle.SetXAlign(0)
	s.Box.Append(statusTitle)

	statusBox := gtk.NewBox(gtk.OrientationVertical, 4)
	s.statusConnLabel = s.addReadOnlyRow(statusBox, "Connected")
	s.statusBrkLabel = s.addReadOnlyRow(statusBox, "Broker")
	s.statusErrLabel = s.addReadOnlyRow(statusBox, "Error")
	s.Box.Append(statusBox)

	s.loadSettings()
	s.loadStatus()
	return s
}

func (s *SettingsMQTT) addEntryRow(label string) *gtk.Entry {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	l := gtk.NewLabel(label)
	l.SetXAlign(0)
	l.SetSizeRequest(160, -1)
	entry := gtk.NewEntry()
	entry.SetHExpand(true)
	row.Append(l)
	row.Append(entry)
	s.Box.Append(row)
	return entry
}

func (s *SettingsMQTT) addSwitchRow(label string) *gtk.Switch {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	l := gtk.NewLabel(label)
	l.SetHExpand(true)
	l.SetXAlign(0)
	sw := gtk.NewSwitch()
	row.Append(l)
	row.Append(sw)
	s.Box.Append(row)
	return sw
}

func (s *SettingsMQTT) addReadOnlyRow(parent *gtk.Box, label string) *gtk.Label {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	l := gtk.NewLabel(label)
	l.AddCSSClass("dim-label")
	l.SetXAlign(0)
	l.SetHExpand(true)
	val := gtk.NewLabel("—")
	val.SetXAlign(1)
	row.Append(l)
	row.Append(val)
	parent.Append(row)
	return val
}

func (s *SettingsMQTT) loadSettings() {
	if s.app.ctrl == nil {
		return
	}
	settings, err := s.app.ctrl.GetMQTTSettings(context.Background())
	if err != nil {
		return
	}
	s.enableSwitch.SetActive(settings.Enabled)
	s.brokerEntry.SetText(settings.Broker)
	s.portEntry.SetText(strconv.Itoa(settings.Port))
	s.usernameEntry.SetText(settings.Username)
	s.passwordEntry.SetText(settings.Password)
	s.baseTopicEntry.SetText(settings.BaseTopic)
	s.tlsSwitch.SetActive(settings.UseTLS)
	s.tlsInsecure.SetActive(settings.TLSInsecure)
	s.haDiscovery.SetActive(settings.EnableHADiscovery)
	s.enableActions.SetActive(settings.EnableActions)
	s.debounceEntry.SetText(strconv.Itoa(settings.DebounceMs))
}

func (s *SettingsMQTT) loadStatus() {
	if s.app.ctrl == nil {
		return
	}
	status, err := s.app.ctrl.GetMQTTStatus(context.Background())
	if err != nil {
		return
	}
	if status.Connected {
		s.statusConnLabel.SetText("Yes")
	} else {
		s.statusConnLabel.SetText("No")
	}
	s.statusBrkLabel.SetText(s.brokerEntry.Text())
	s.statusErrLabel.SetText(status.Error)
}

func (s *SettingsMQTT) buildSettings() session.MQTTSettings {
	port, _ := strconv.Atoi(s.portEntry.Text())
	debounce, _ := strconv.Atoi(s.debounceEntry.Text())
	return session.MQTTSettings{
		Enabled:           s.enableSwitch.State(),
		Broker:            s.brokerEntry.Text(),
		Port:              port,
		Username:          s.usernameEntry.Text(),
		Password:          s.passwordEntry.Text(),
		BaseTopic:         s.baseTopicEntry.Text(),
		UseTLS:            s.tlsSwitch.State(),
		TLSInsecure:       s.tlsInsecure.State(),
		EnableHADiscovery: s.haDiscovery.State(),
		EnableActions:     s.enableActions.State(),
		DebounceMs:        debounce,
	}
}

func (s *SettingsMQTT) saveSettings() {
	if s.app.ctrl == nil {
		return
	}
	_ = s.app.ctrl.SetMQTTSettings(s.buildSettings())
}

func (s *SettingsMQTT) testConnection() {
	if s.app.ctrl == nil {
		return
	}
	result, err := s.app.ctrl.TestMQTTConnection(s.buildSettings())
	if err != nil {
		s.statusErrLabel.SetText(err.Error())
		return
	}
	if result.Success {
		s.statusErrLabel.SetText("Connection successful")
	} else {
		s.statusErrLabel.SetText(result.Error)
	}
}
