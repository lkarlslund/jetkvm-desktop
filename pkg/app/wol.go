package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/lkarlslund/jetkvm-desktop/pkg/ui"
)

type wolDevice struct {
	Name        string `json:"name"`
	MacAddress  string `json:"macAddress"`
	BroadcastIP string `json:"broadcastIP,omitempty"`
}

func (a *App) openWoLOverlay() {
	a.wolOpen = true
	a.wolLabel = ""
	a.wolMAC = ""
	a.wolLabelFocused = false
	a.wolMACFocused = false
	a.wolError = ""
	a.wolSuccess = ""
	a.wolDeleteConfirm = ""
	a.wolDevices = nil
	a.wolLoading = true

	a.pasteOpen = false
	a.mediaOpen = false
	a.settingsOpen = false
	a.serialConsoleOpen = false
	a.releaseTotalCapture()
	a.applyCursorMode()

	go a.loadWoLDevices()
}

func (a *App) closeWoLOverlay() {
	a.wolOpen = false
	a.wolLabelFocused = false
	a.wolMACFocused = false
	a.revealUIFor(1200 * time.Millisecond)
}

func (a *App) loadWoLDevices() {
	if a.ctrl == nil {
		a.mu.Lock()
		a.wolLoading = false
		a.wolError = "Not connected to a device"
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	devices, err := a.ctrl.GetWakeOnLanDevices(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.wolLoading = false
	if err != nil {
		a.wolError = fmt.Sprintf("Failed to load devices: %v", err)
		return
	}
	a.wolDevices = make([]wolDevice, len(devices))
	for i, d := range devices {
		a.wolDevices[i] = wolDevice{
			Name:        d.Name,
			MacAddress:  d.MacAddress,
			BroadcastIP: d.BroadcastIP,
		}
	}
}

func (a *App) wolAddDevice() {
	name := strings.TrimSpace(a.wolLabel)
	mac := strings.TrimSpace(a.wolMAC)
	if name == "" {
		a.wolError = "Name is required"
		return
	}
	if mac == "" {
		a.wolError = "MAC address is required"
		return
	}

	newDev := wolDevice{Name: name, MacAddress: mac}
	updated := append(a.wolDevices, newDev)

	a.wolError = ""
	go func() {
		if a.ctrl == nil {
			return
		}
		if err := a.ctrl.SetWakeOnLanDevices(wolDevicesToSession(updated)); err != nil {
			a.mu.Lock()
			a.wolError = fmt.Sprintf("Failed to save: %v", err)
			a.mu.Unlock()
			return
		}
		a.mu.Lock()
		a.wolDevices = updated
		a.wolLabel = ""
		a.wolMAC = ""
		a.wolSuccess = fmt.Sprintf("Added %s", name)
		a.mu.Unlock()
	}()
}

func (a *App) wolSendPacket(mac string) {
	if a.ctrl == nil {
		a.wolError = "Not connected"
		return
	}
	var broadcastIP string
	for _, d := range a.wolDevices {
		if d.MacAddress == mac {
			broadcastIP = d.BroadcastIP
			break
		}
	}
	a.wolError = ""
	go func() {
		if err := a.ctrl.SendWakeOnLan(mac, broadcastIP); err != nil {
			a.mu.Lock()
			a.wolError = fmt.Sprintf("Failed to send WoL: %v", err)
			a.mu.Unlock()
			return
		}
		a.mu.Lock()
		a.wolSuccess = fmt.Sprintf("Magic packet sent to %s", mac)
		a.mu.Unlock()
	}()
}

func (a *App) wolDeleteDevice(mac string) {
	updated := make([]wolDevice, 0, len(a.wolDevices))
	for _, d := range a.wolDevices {
		if d.MacAddress != mac {
			updated = append(updated, d)
		}
	}
	a.wolDeleteConfirm = ""
	go func() {
		if a.ctrl == nil {
			return
		}
		if err := a.ctrl.SetWakeOnLanDevices(wolDevicesToSession(updated)); err != nil {
			a.mu.Lock()
			a.wolError = fmt.Sprintf("Failed to delete: %v", err)
			a.mu.Unlock()
			return
		}
		a.mu.Lock()
		a.wolDevices = updated
		a.wolSuccess = "Device removed"
		a.mu.Unlock()
	}()
}

func (a *App) syncWoLInput() {
	if !a.wolOpen {
		return
	}
	if a.wolMACFocused {
		if a.textInput.FieldID != "wol_focus_mac" {
			a.textInput.Sync(&ui.TextInputBinding{
				ID:       "wol_focus_mac",
				Value:    a.wolMAC,
				TextSize: 13,
			})
		}
	} else if a.wolLabelFocused {
		if a.textInput.FieldID != "wol_focus_label" {
			a.textInput.Sync(&ui.TextInputBinding{
				ID:       "wol_focus_label",
				Value:    a.wolLabel,
				TextSize: 13,
			})
		}
	}
	a.syncFocusedTextInput()

	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		if a.wolLabelFocused {
			a.wolLabelFocused = false
			a.wolMACFocused = true
		} else if a.wolMACFocused {
			a.wolMACFocused = false
			a.wolLabelFocused = true
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		a.wolAddDevice()
	}
}

func (a *App) drawWoLOverlay(screen *ebiten.Image) {
	if !a.wolOpen {
		a.wolRuntime.BeginFrame()
		return
	}
	a.drawUIRoot(screen, &a.wolRuntime, func(chromeButton) {}, wolOverlayRootElement{app: a})
}

// wolDevicesToSession converts local wolDevice slices to session-layer types.
func wolDevicesToSession(devices []wolDevice) []struct {
	Name        string
	MacAddress  string
	BroadcastIP string
} {
	out := make([]struct {
		Name        string
		MacAddress  string
		BroadcastIP string
	}, len(devices))
	for i, d := range devices {
		out[i].Name = d.Name
		out[i].MacAddress = d.MacAddress
		out[i].BroadcastIP = d.BroadcastIP
	}
	return out
}

func (a *App) invokeWoLAction(id string) bool {
	switch {
	case id == "wol_add":
		a.wolAddDevice()
		return true
	case id == "wol_close":
		a.closeWoLOverlay()
		return true
	case strings.HasPrefix(id, "wol_wake:"):
		mac := strings.TrimPrefix(id, "wol_wake:")
		a.wolSendPacket(mac)
		return true
	case strings.HasPrefix(id, "wol_delete_confirm:"):
		mac := strings.TrimPrefix(id, "wol_delete_confirm:")
		if a.wolDeleteConfirm == mac {
			a.wolDeleteConfirm = ""
		} else {
			a.wolDeleteConfirm = mac
		}
		return true
	case strings.HasPrefix(id, "wol_delete:"):
		mac := strings.TrimPrefix(id, "wol_delete:")
		a.wolDeleteDevice(mac)
		return true
	case id == "wol_focus_label":
		a.wolLabelFocused = true
		a.wolMACFocused = false
		return true
	case id == "wol_focus_mac":
		a.wolMACFocused = true
		a.wolLabelFocused = false
		return true
	}
	return false
}

// --- UI elements ---

type wolOverlayRootElement struct {
	app *App
}

func (wolOverlayRootElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e wolOverlayRootElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ctx.FillRect(bounds, ctx.Theme.Backdrop)
	panelW := min(520.0, bounds.W-48)
	panelH := min(520.0, bounds.H-48)
	panelX := bounds.X + (bounds.W-panelW)/2
	panelY := bounds.Y + (bounds.H-panelH)/2
	panel := ui.Rect{X: panelX, Y: panelY, W: panelW, H: panelH}
	ctx.FillRect(panel, ctx.Theme.ModalFill)
	ctx.StrokeRect(panel, 1, ctx.Theme.ModalStroke)

	content := ui.Rect{X: panel.X + 20, Y: panel.Y + 20, W: panel.W - 40, H: panel.H - 40}
	wolContentElement{app: e.app}.Draw(ctx, content)
}

type wolContentElement struct {
	app *App
}

func (wolContentElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e wolContentElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	a := e.app
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: "Wake On LAN", Size: 24, Color: ctx.Theme.Title}),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{Text: "Manage Wake-on-LAN devices on this KVM. Send magic packets to wake remote machines.", Size: 12, Color: ctx.Theme.Muted}),
		ui.Fixed(ui.Spacer{H: 16}),
	}

	a.mu.RLock()
	loading := a.wolLoading
	devices := a.wolDevices
	a.mu.RUnlock()

	if loading {
		children = append(children,
			ui.Fixed(ui.Label{Text: "Loading devices...", Size: 13, Color: ctx.Theme.Muted}),
		)
	} else if len(devices) > 0 {
		children = append(children,
			ui.Fixed(ui.Label{Text: "Saved Devices", Size: 14, Color: ctx.Theme.Body}),
			ui.Fixed(ui.Spacer{H: 8}),
		)
		for _, dev := range devices {
			children = append(children, ui.Fixed(wolDeviceRowElement{app: a, device: dev}))
			if a.wolDeleteConfirm == dev.MacAddress {
				children = append(children, ui.Fixed(wolDeleteConfirmRowElement{app: a, mac: dev.MacAddress}))
			}
			children = append(children, ui.Fixed(ui.Spacer{H: 6}))
		}
		children = append(children, ui.Fixed(ui.Spacer{H: 10}))
	}

	children = append(children,
		ui.Fixed(ui.Label{Text: "Add New Device", Size: 14, Color: ctx.Theme.Body}),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Label{Text: "Name", Size: 12, Color: ctx.Theme.Muted}),
		ui.Fixed(ui.Spacer{H: 4}),
		ui.Fixed(a.decorateTextField(ui.TextField{
			ID:               "wol_focus_label",
			Value:            a.wolLabel,
			Placeholder:      "My Server",
			Focused:          a.wolLabelFocused,
			Enabled:          true,
			TextSize:         13,
			FillColor:        ctx.Theme.InputFill,
			StrokeColor:      ctx.Theme.InputStroke,
			FocusColor:       ctx.Theme.InputFocus,
			TextColor:        ctx.Theme.Body,
			PlaceholderColor: ctx.Theme.DisabledText,
		})),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Label{Text: "MAC Address", Size: 12, Color: ctx.Theme.Muted}),
		ui.Fixed(ui.Spacer{H: 4}),
		ui.Fixed(a.decorateTextField(ui.TextField{
			ID:               "wol_focus_mac",
			Value:            a.wolMAC,
			Placeholder:      "00:11:22:33:44:55",
			Focused:          a.wolMACFocused,
			Enabled:          true,
			TextSize:         13,
			FillColor:        ctx.Theme.InputFill,
			StrokeColor:      ctx.Theme.InputStroke,
			FocusColor:       ctx.Theme.InputFocus,
			TextColor:        ctx.Theme.Body,
			PlaceholderColor: ctx.Theme.DisabledText,
		})),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(ui.Button{ID: "wol_add", Label: "+ Add Device", Enabled: strings.TrimSpace(a.wolLabel) != "" && strings.TrimSpace(a.wolMAC) != "", OnClick: func() {
			a.wolAddDevice()
		}}),
	)

	if a.wolError != "" {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Paragraph{Text: a.wolError, Size: 12, Color: ctx.Theme.Error}),
		)
	}
	if a.wolSuccess != "" {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Label{Text: a.wolSuccess, Size: 12, Color: ctx.Theme.AccentText}),
		)
	}

	children = append(children, ui.Flex(ui.Spacer{}, 1))
	children = append(children, ui.Fixed(ui.Button{ID: "wol_close", Label: "Close", Enabled: true, OnClick: func() {
		a.closeWoLOverlay()
	}}))

	ui.Column{Children: children}.Draw(ctx, bounds)
}

type wolDeviceRowElement struct {
	app    *App
	device wolDevice
}

func (wolDeviceRowElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 42})
}

func (e wolDeviceRowElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Panel{
		Fill:   ctx.Theme.SectionFill,
		Stroke: ctx.Theme.PanelStroke,
		Insets: ui.SymmetricInsets(12, 8),
		Child: ui.Row{
			AlignY: ui.AlignCenter,
			Children: []ui.Child{
				ui.Flex(ui.Column{
					Children: []ui.Child{
						ui.Fixed(ui.Label{Text: e.device.Name, Size: 14, Color: ctx.Theme.Title}),
						ui.Fixed(ui.Spacer{H: 2}),
						ui.Fixed(ui.Label{Text: e.device.MacAddress, Size: 11, Color: ctx.Theme.Muted}),
					},
				}, 1),
				ui.Fixed(ui.Button{
					ID:      "wol_wake:" + e.device.MacAddress,
					Label:   "Wake",
					Enabled: true,
					MinW:    56,
					OnClick: func() { e.app.wolSendPacket(e.device.MacAddress) },
				}),
				ui.Fixed(ui.Spacer{W: 6}),
				ui.Fixed(wolDeleteButtonElement{app: e.app, mac: e.device.MacAddress}),
			},
			Spacing: 8,
		},
	}.Draw(ctx, bounds)
}

type wolDeleteButtonElement struct {
	app *App
	mac string
}

func (wolDeleteButtonElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: 28, H: 26})
}

func (e wolDeleteButtonElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	fill := ctx.Theme.Error
	ctx.FillRect(bounds, fill)
	ctx.StrokeRect(bounds, 1, ctx.Theme.ButtonStroke)
	if ctx.DrawText != nil {
		tw, _ := ctx.MeasureText("\u2716", 13)
		th := ui.LineHeight(13)
		ctx.DrawText(ctx.Screen, "\u2716", bounds.X+(bounds.W-tw)/2, bounds.Y+(bounds.H-th)/2, 13, ctx.Theme.ButtonText)
	}
	mac := e.mac
	if ctx.Runtime != nil {
		ctx.Runtime.Register(ui.Control{
			ID:      "wol_delete_confirm:" + mac,
			Rect:    bounds,
			Enabled: true,
			OnClick: func(ui.PointerEvent) {
				if e.app.wolDeleteConfirm == mac {
					e.app.wolDeleteConfirm = ""
				} else {
					e.app.wolDeleteConfirm = mac
				}
			},
		})
	}
}

type wolDeleteConfirmRowElement struct {
	app *App
	mac string
}

func (wolDeleteConfirmRowElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 30})
}

func (e wolDeleteConfirmRowElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Row{
		AlignY: ui.AlignCenter,
		Children: []ui.Child{
			ui.Fixed(ui.Label{Text: "Delete this device?", Size: 12, Color: ctx.Theme.Error}),
			ui.Flex(ui.Spacer{}, 1),
			ui.Fixed(ui.Button{
				ID:      "wol_delete:" + e.mac,
				Label:   "Yes, Delete",
				Enabled: true,
				MinW:    86,
				OnClick: func() { e.app.wolDeleteDevice(e.mac) },
			}),
			ui.Fixed(ui.Spacer{W: 6}),
			ui.Fixed(ui.Button{
				ID:      "wol_delete_confirm:" + e.mac,
				Label:   "Cancel",
				Enabled: true,
				MinW:    64,
				OnClick: func() { e.app.wolDeleteConfirm = "" },
			}),
		},
		Spacing: 8,
	}.Draw(ctx, bounds)
}
