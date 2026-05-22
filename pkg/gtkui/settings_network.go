package gtkui

import (
	"context"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// SettingsNetwork provides network configuration and state display.
type SettingsNetwork struct {
	Box *gtk.Box

	app *Application

	hostnameEntry  *gtk.Entry
	domainEntry    *gtk.Entry
	httpProxyEntry *gtk.Entry

	ipv4Mode       *gtk.DropDown
	ipv4AddrEntry  *gtk.Entry
	ipv4MaskEntry  *gtk.Entry
	ipv4GwEntry    *gtk.Entry
	ipv4DNSEntry   *gtk.Entry
	ipv4StaticBox  *gtk.Box

	ipv6Mode       *gtk.DropDown
	ipv6PrefixEntry *gtk.Entry
	ipv6GwEntry    *gtk.Entry
	ipv6DNSEntry   *gtk.Entry
	ipv6StaticBox  *gtk.Box

	mdnsMode     *gtk.DropDown
	timeSyncMode *gtk.DropDown

	stateBox *gtk.Box
	ifLabel  *gtk.Label
	macLabel *gtk.Label
	ipLabel  *gtk.Label

	dhcpBox     *gtk.Box
	dhcpIPLabel *gtk.Label
	dhcpGwLabel *gtk.Label
	dhcpDNSLabel *gtk.Label
}

func NewSettingsNetwork(app *Application) *SettingsNetwork {
	s := &SettingsNetwork{app: app}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 8)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)

	// --- Settings Card ---
	settingsTitle := gtk.NewLabel("Network Settings")
	settingsTitle.AddCSSClass("title-3")
	settingsTitle.SetXAlign(0)
	s.Box.Append(settingsTitle)

	s.hostnameEntry = s.addEntryRow("Hostname")
	s.domainEntry = s.addEntryRow("Domain")
	s.httpProxyEntry = s.addEntryRow("HTTP Proxy")

	// IPv4
	ipv4Label := gtk.NewLabel("IPv4 Mode")
	ipv4Label.AddCSSClass("title-4")
	ipv4Label.SetXAlign(0)
	s.Box.Append(ipv4Label)

	ipv4Modes := gtk.NewStringList([]string{"dhcp", "static", "disabled"})
	s.ipv4Mode = gtk.NewDropDown(ipv4Modes, nil)
	s.ipv4Mode.SetSelected(0)
	s.Box.Append(s.ipv4Mode)

	s.ipv4StaticBox = gtk.NewBox(gtk.OrientationVertical, 4)
	s.ipv4AddrEntry = s.addEntryRowTo(s.ipv4StaticBox, "Address")
	s.ipv4MaskEntry = s.addEntryRowTo(s.ipv4StaticBox, "Netmask")
	s.ipv4GwEntry = s.addEntryRowTo(s.ipv4StaticBox, "Gateway")
	s.ipv4DNSEntry = s.addEntryRowTo(s.ipv4StaticBox, "DNS (comma-sep)")
	s.ipv4StaticBox.SetVisible(false)
	s.Box.Append(s.ipv4StaticBox)

	// IPv6
	ipv6Label := gtk.NewLabel("IPv6 Mode")
	ipv6Label.AddCSSClass("title-4")
	ipv6Label.SetXAlign(0)
	s.Box.Append(ipv6Label)

	ipv6Modes := gtk.NewStringList([]string{"dhcp", "static", "disabled"})
	s.ipv6Mode = gtk.NewDropDown(ipv6Modes, nil)
	s.ipv6Mode.SetSelected(0)
	s.Box.Append(s.ipv6Mode)

	s.ipv6StaticBox = gtk.NewBox(gtk.OrientationVertical, 4)
	s.ipv6PrefixEntry = s.addEntryRowTo(s.ipv6StaticBox, "Prefix")
	s.ipv6GwEntry = s.addEntryRowTo(s.ipv6StaticBox, "Gateway")
	s.ipv6DNSEntry = s.addEntryRowTo(s.ipv6StaticBox, "DNS (comma-sep)")
	s.ipv6StaticBox.SetVisible(false)
	s.Box.Append(s.ipv6StaticBox)

	// mDNS
	mdnsLabel := gtk.NewLabel("mDNS Mode")
	mdnsLabel.SetXAlign(0)
	s.Box.Append(mdnsLabel)
	mdnsModes := gtk.NewStringList([]string{"enabled", "disabled"})
	s.mdnsMode = gtk.NewDropDown(mdnsModes, nil)
	s.Box.Append(s.mdnsMode)

	// Time sync
	tsLabel := gtk.NewLabel("Time Sync Mode")
	tsLabel.SetXAlign(0)
	s.Box.Append(tsLabel)
	tsModes := gtk.NewStringList([]string{"ntp", "http", "disabled"})
	s.timeSyncMode = gtk.NewDropDown(tsModes, nil)
	s.Box.Append(s.timeSyncMode)

	// Buttons
	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnBox.SetHAlign(gtk.AlignEnd)
	saveBtn := gtk.NewButtonWithLabel("Save Settings")
	saveBtn.AddCSSClass("suggested-action")
	saveBtn.ConnectClicked(func() { s.saveSettings() })
	renewBtn := gtk.NewButtonWithLabel("Renew DHCP Lease")
	renewBtn.ConnectClicked(func() {
		if app.ctrl != nil {
			_ = app.ctrl.RenewDHCPLease()
		}
	})
	btnBox.Append(renewBtn)
	btnBox.Append(saveBtn)
	s.Box.Append(btnBox)

	// --- Current State Card ---
	stateTitle := gtk.NewLabel("Current State")
	stateTitle.AddCSSClass("title-3")
	stateTitle.SetXAlign(0)
	s.Box.Append(stateTitle)

	s.stateBox = gtk.NewBox(gtk.OrientationVertical, 4)
	s.ifLabel = s.addReadOnlyRow(s.stateBox, "Interface")
	s.macLabel = s.addReadOnlyRow(s.stateBox, "MAC Address")
	s.ipLabel = s.addReadOnlyRow(s.stateBox, "IP Addresses")
	s.Box.Append(s.stateBox)

	// --- DHCP Lease Card ---
	dhcpTitle := gtk.NewLabel("DHCP Lease")
	dhcpTitle.AddCSSClass("title-4")
	dhcpTitle.SetXAlign(0)
	s.Box.Append(dhcpTitle)

	s.dhcpBox = gtk.NewBox(gtk.OrientationVertical, 4)
	s.dhcpIPLabel = s.addReadOnlyRow(s.dhcpBox, "IP")
	s.dhcpGwLabel = s.addReadOnlyRow(s.dhcpBox, "Gateway")
	s.dhcpDNSLabel = s.addReadOnlyRow(s.dhcpBox, "DNS")
	s.Box.Append(s.dhcpBox)

	s.loadSettings()
	s.loadState()
	return s
}

func (s *SettingsNetwork) addEntryRow(label string) *gtk.Entry {
	return s.addEntryRowTo(s.Box, label)
}

func (s *SettingsNetwork) addEntryRowTo(parent *gtk.Box, label string) *gtk.Entry {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	l := gtk.NewLabel(label)
	l.SetXAlign(0)
	l.SetSizeRequest(140, -1)
	entry := gtk.NewEntry()
	entry.SetHExpand(true)
	row.Append(l)
	row.Append(entry)
	parent.Append(row)
	return entry
}

func (s *SettingsNetwork) addReadOnlyRow(parent *gtk.Box, label string) *gtk.Label {
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

func (s *SettingsNetwork) loadSettings() {
	if s.app.ctrl == nil {
		return
	}
	settings, err := s.app.ctrl.GetNetworkSettings(context.Background())
	if err != nil {
		return
	}
	s.hostnameEntry.SetText(settings.Hostname)
	s.domainEntry.SetText(settings.Domain)
	s.httpProxyEntry.SetText(settings.HTTPProxy)

	s.setDropDownByValue(s.ipv4Mode, settings.IPv4Mode, []string{"dhcp", "static", "disabled"})
	if settings.IPv4Static != nil {
		s.ipv4AddrEntry.SetText(settings.IPv4Static.Address)
		s.ipv4MaskEntry.SetText(settings.IPv4Static.Netmask)
		s.ipv4GwEntry.SetText(settings.IPv4Static.Gateway)
		s.ipv4DNSEntry.SetText(strings.Join(settings.IPv4Static.DNS, ","))
		s.ipv4StaticBox.SetVisible(true)
	}

	s.setDropDownByValue(s.ipv6Mode, settings.IPv6Mode, []string{"dhcp", "static", "disabled"})
	if settings.IPv6Static != nil {
		s.ipv6PrefixEntry.SetText(settings.IPv6Static.Prefix)
		s.ipv6GwEntry.SetText(settings.IPv6Static.Gateway)
		s.ipv6DNSEntry.SetText(strings.Join(settings.IPv6Static.DNS, ","))
		s.ipv6StaticBox.SetVisible(true)
	}

	s.setDropDownByValue(s.mdnsMode, settings.MDNSMode, []string{"enabled", "disabled"})
	s.setDropDownByValue(s.timeSyncMode, settings.TimeSyncMode, []string{"ntp", "http", "disabled"})
}

func (s *SettingsNetwork) loadState() {
	if s.app.ctrl == nil {
		return
	}
	state, err := s.app.ctrl.GetNetworkState(context.Background())
	if err != nil {
		return
	}
	s.ifLabel.SetText(state.InterfaceName)
	s.macLabel.SetText(state.MACAddress)

	ips := state.IPv4
	if state.IPv6 != "" {
		ips += ", " + state.IPv6
	}
	s.ipLabel.SetText(ips)

	if state.DHCPLease != nil {
		s.dhcpIPLabel.SetText(state.DHCPLease.IP)
		s.dhcpGwLabel.SetText(strings.Join(state.DHCPLease.Routers, ", "))
		s.dhcpDNSLabel.SetText(strings.Join(state.DHCPLease.DNSServers, ", "))
	}
}

func (s *SettingsNetwork) saveSettings() {
	if s.app.ctrl == nil {
		return
	}
	settings := session.NetworkSettings{
		Hostname:  s.hostnameEntry.Text(),
		Domain:    s.domainEntry.Text(),
		HTTPProxy: s.httpProxyEntry.Text(),
		IPv4Mode:  s.getDropDownValue(s.ipv4Mode, []string{"dhcp", "static", "disabled"}),
		IPv6Mode:  s.getDropDownValue(s.ipv6Mode, []string{"dhcp", "static", "disabled"}),
		MDNSMode:  s.getDropDownValue(s.mdnsMode, []string{"enabled", "disabled"}),
		TimeSyncMode: s.getDropDownValue(s.timeSyncMode, []string{"ntp", "http", "disabled"}),
	}

	if settings.IPv4Mode == "static" {
		settings.IPv4Static = &session.IPv4StaticConfig{
			Address: s.ipv4AddrEntry.Text(),
			Netmask: s.ipv4MaskEntry.Text(),
			Gateway: s.ipv4GwEntry.Text(),
			DNS:     splitComma(s.ipv4DNSEntry.Text()),
		}
	}

	if settings.IPv6Mode == "static" {
		settings.IPv6Static = &session.IPv6StaticConfig{
			Prefix:  s.ipv6PrefixEntry.Text(),
			Gateway: s.ipv6GwEntry.Text(),
			DNS:     splitComma(s.ipv6DNSEntry.Text()),
		}
	}

	_ = s.app.ctrl.SetNetworkSettings(settings)
}

func (s *SettingsNetwork) setDropDownByValue(dd *gtk.DropDown, value string, options []string) {
	for i, opt := range options {
		if opt == value {
			dd.SetSelected(uint(i))
			return
		}
	}
}

func (s *SettingsNetwork) getDropDownValue(dd *gtk.DropDown, options []string) string {
	idx := dd.Selected()
	if int(idx) < len(options) {
		return options[idx]
	}
	return options[0]
}
