package gtkui

import (
	"context"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// WoLOverlay manages Wake-on-LAN saved devices and sending magic packets.
type WoLOverlay struct {
	Box *gtk.Box

	app        *Application
	deviceList *gtk.ListBox
	nameEntry  *gtk.Entry
	macEntry   *gtk.Entry
	addBtn     *gtk.Button
	statusLabel *gtk.Label
	closeBtn   *gtk.Button

	devices []session.WakeOnLanDevice
}

func NewWoLOverlay(app *Application) *WoLOverlay {
	w := &WoLOverlay{app: app}
	w.Box = gtk.NewBox(gtk.OrientationVertical, 8)
	w.Box.AddCSSClass("overlay-panel")
	w.Box.SetHAlign(gtk.AlignCenter)
	w.Box.SetVAlign(gtk.AlignCenter)

	title := gtk.NewLabel("Wake on LAN")
	title.AddCSSClass("title-3")
	title.SetXAlign(0)
	w.Box.Append(title)

	desc := gtk.NewLabel("Send Wake-on-LAN magic packets to start remote machines.")
	desc.SetWrap(true)
	desc.AddCSSClass("dim-label")
	desc.SetXAlign(0)
	w.Box.Append(desc)

	// Saved devices
	w.deviceList = gtk.NewListBox()
	w.deviceList.SetSelectionMode(gtk.SelectionNone)
	w.deviceList.AddCSSClass("boxed-list")

	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(w.deviceList)
	scroll.SetMinContentHeight(100)
	scroll.SetMaxContentHeight(200)
	w.Box.Append(scroll)

	// Add new device
	addLabel := gtk.NewLabel("Add New Device")
	addLabel.AddCSSClass("title-4")
	addLabel.SetXAlign(0)
	w.Box.Append(addLabel)

	w.nameEntry = gtk.NewEntry()
	w.nameEntry.SetPlaceholderText("Device name")
	w.Box.Append(w.nameEntry)

	w.macEntry = gtk.NewEntry()
	w.macEntry.SetPlaceholderText("MAC address (e.g. AA:BB:CC:DD:EE:FF)")
	w.Box.Append(w.macEntry)

	w.addBtn = gtk.NewButtonWithLabel("+ Add Device")
	w.addBtn.AddCSSClass("suggested-action")
	w.addBtn.ConnectClicked(func() { w.addDevice() })
	w.Box.Append(w.addBtn)

	w.statusLabel = gtk.NewLabel("")
	w.statusLabel.AddCSSClass("dim-label")
	w.statusLabel.SetXAlign(0)
	w.Box.Append(w.statusLabel)

	w.closeBtn = gtk.NewButtonWithLabel("Close")
	w.closeBtn.ConnectClicked(func() { app.closeOverlay() })
	w.Box.Append(w.closeBtn)

	return w
}

func (w *WoLOverlay) Refresh() {
	tw, th := overlayTargetSize(w.app, 560, 520)
	w.Box.SetSizeRequest(tw, th)
	if w.app.ctrl == nil {
		return
	}
	devices, err := w.app.ctrl.GetWakeOnLanDevices(context.Background())
	if err != nil {
		w.statusLabel.SetText("Error loading devices: " + err.Error())
		return
	}
	w.devices = devices
	w.rebuildList()
}

func (w *WoLOverlay) rebuildList() {
	removeAllChildren(w.deviceList)

	for _, dev := range w.devices {
		row := w.makeDeviceRow(dev)
		w.deviceList.Append(row)
	}
}

func (w *WoLOverlay) makeDeviceRow(dev session.WakeOnLanDevice) *gtk.ListBoxRow {
	box := gtk.NewBox(gtk.OrientationHorizontal, 8)
	box.SetMarginTop(4)
	box.SetMarginBottom(4)
	box.SetMarginStart(8)
	box.SetMarginEnd(8)

	nameLabel := gtk.NewLabel(dev.Name)
	nameLabel.SetHExpand(true)
	nameLabel.SetXAlign(0)

	macLabel := gtk.NewLabel(dev.MacAddress)
	macLabel.AddCSSClass("dim-label")

	wakeBtn := gtk.NewButtonWithLabel("Wake")
	wakeBtn.AddCSSClass("suggested-action")
	mac := dev.MacAddress
	wakeBtn.ConnectClicked(func() {
		if w.app.ctrl != nil {
			err := w.app.ctrl.SendWakeOnLan(mac, "")
			if err != nil {
				w.statusLabel.SetText("Error: " + err.Error())
			} else {
				w.statusLabel.SetText("Magic packet sent to " + mac)
			}
		}
	})

	delBtn := gtk.NewButtonFromIconName("window-close-symbolic")
	delBtn.AddCSSClass("flat")
	devName := dev.Name
	delBtn.ConnectClicked(func() { w.deleteDevice(devName) })

	box.Append(nameLabel)
	box.Append(macLabel)
	box.Append(wakeBtn)
	box.Append(delBtn)

	row := gtk.NewListBoxRow()
	row.SetChild(box)
	return row
}

func (w *WoLOverlay) addDevice() {
	name := w.nameEntry.Text()
	mac := w.macEntry.Text()
	if name == "" || mac == "" {
		return
	}
	w.devices = append(w.devices, session.WakeOnLanDevice{
		Name:       name,
		MacAddress: mac,
	})
	w.saveDevices()
	w.nameEntry.SetText("")
	w.macEntry.SetText("")
	w.rebuildList()
}

func (w *WoLOverlay) deleteDevice(name string) {
	var filtered []session.WakeOnLanDevice
	for _, d := range w.devices {
		if d.Name != name {
			filtered = append(filtered, d)
		}
	}
	w.devices = filtered
	w.saveDevices()
	w.rebuildList()
}

func (w *WoLOverlay) saveDevices() {
	if w.app.ctrl == nil {
		return
	}
	items := make([]struct {
		Name        string
		MacAddress  string
		BroadcastIP string
	}, len(w.devices))
	for i, d := range w.devices {
		items[i].Name = d.Name
		items[i].MacAddress = d.MacAddress
		items[i].BroadcastIP = d.BroadcastIP
	}
	_ = w.app.ctrl.SetWakeOnLanDevices(items)
}
