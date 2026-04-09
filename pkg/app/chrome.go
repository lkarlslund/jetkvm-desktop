package app

import (
	"context"
	"fmt"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/lkarlslund/jetkvm-native/pkg/session"
)

type iconKind string

const (
	iconReconnect  iconKind = "reconnect"
	iconMouse      iconKind = "mouse"
	iconPaste      iconKind = "paste"
	iconStats      iconKind = "stats"
	iconMinus      iconKind = "minus"
	iconPlus       iconKind = "plus"
	iconPower      iconKind = "power"
	iconSettings   iconKind = "settings"
	iconFullscreen iconKind = "fullscreen"
	iconClose      iconKind = "close"
)

type chromeButton struct {
	id      string
	label   string
	hint    string
	icon    iconKind
	enabled bool
	active  bool
	rect    rect
}

type settingsSection string

const (
	sectionGeneral    settingsSection = "general"
	sectionMouse      settingsSection = "mouse"
	sectionKeyboard   settingsSection = "keyboard"
	sectionVideo      settingsSection = "video"
	sectionHardware   settingsSection = "hardware"
	sectionAccess     settingsSection = "access"
	sectionAppearance settingsSection = "appearance"
	sectionMacros     settingsSection = "macros"
	sectionNetwork    settingsSection = "network"
	sectionMQTT       settingsSection = "mqtt"
	sectionAdvanced   settingsSection = "advanced"
)

type settingsSectionDef struct {
	id          settingsSection
	label       string
	description string
	available   bool
	items       []string
}

type sectionData struct {
	Access   accessState
	Hardware hardwareState
	Network  networkState
	Advanced advancedState
}

type accessState struct {
	Loading        bool
	Error          string
	CloudConnected bool
	CloudURL       string
	CloudAppURL    string
	TLSMode        string
}

type hardwareState struct {
	Loading           bool
	Error             string
	USBEmulation      *bool
	USBConfig         string
	USBDevicesSummary string
	DisplayRotation   string
}

type networkState struct {
	Loading  bool
	Error    string
	Hostname string
	IP       string
	DHCP     *bool
}

type advancedState struct {
	Loading       bool
	Error         string
	DevMode       *bool
	USBEmulation  *bool
	AppVersion    string
	SystemVersion string
}

func settingsSections(snap session.Snapshot) []settingsSectionDef {
	return []settingsSectionDef{
		{
			id:          sectionGeneral,
			label:       "General",
			description: "Device identity, connection, updates, reboot",
			available:   true,
			items: []string{
				"Connection state, WebRTC state, and signaling mode",
				fmt.Sprintf("Device: %s", fallbackLabel(snap.DeviceID, snap.Hostname, "Unknown device")),
				"Reconnect now",
				"Reboot device",
			},
		},
		{
			id:          sectionMouse,
			label:       "Mouse",
			description: "Cursor mode, host cursor, scroll, jiggler",
			available:   true,
			items: []string{
				"Absolute or relative mouse mode",
				"Host cursor visibility and scroll throttling",
				"Mouse jiggler presets and custom schedule",
			},
		},
		{
			id:          sectionKeyboard,
			label:       "Keyboard",
			description: "Layout and pressed-key display",
			available:   true,
			items: []string{
				"Keyboard layout selection",
				"Show pressed keys overlay",
			},
		},
		{
			id:          sectionVideo,
			label:       "Video",
			description: "Stream quality and EDID",
			available:   true,
			items: []string{
				"Stream quality presets",
				"Current EDID state",
			},
		},
		{
			id:          sectionHardware,
			label:       "Hardware",
			description: "Display rotation, brightness, USB gadget shape",
			available:   false,
			items: []string{
				"Display rotation and backlight behavior",
				"USB device classes and identifiers",
				"Power-saving HDMI sleep",
			},
		},
		{
			id:          sectionAccess,
			label:       "Access",
			description: "Local auth, TLS mode, cloud adoption",
			available:   false,
			items: []string{
				"Local auth mode and password",
				"HTTPS mode: disabled, self-signed, or custom TLS",
				"Cloud provider registration and deregistration",
			},
		},
		{
			id:          sectionAppearance,
			label:       "Appearance",
			description: "Theme and client chrome behavior",
			available:   true,
			items: []string{
				"Auto-hide top control bar",
				"Fullscreen",
			},
		},
		{
			id:          sectionMacros,
			label:       "Macros",
			description: "Keyboard macro library and reordering",
			available:   false,
			items: []string{
				"Create, edit, duplicate, and reorder macros",
				"Layout-aware key display",
			},
		},
		{
			id:          sectionNetwork,
			label:       "Network",
			description: "IPv4, IPv6, DHCP, DNS, mDNS, tailscale, public IP",
			available:   true,
			items: []string{
				"DHCP or static IPv4 and IPv6",
				"DNS and domain settings",
				"Lease info, public IP, and tailscale state",
			},
		},
		{
			id:          sectionMQTT,
			label:       "MQTT",
			description: "Broker, topics, TLS, HA discovery, actions",
			available:   false,
			items: []string{
				"Broker connection and TLS options",
				"Base topic and Home Assistant discovery",
				"Connection test before save",
			},
		},
		{
			id:          sectionAdvanced,
			label:       "Advanced",
			description: "Developer mode, SSH key, loopback-only, reset config",
			available:   true,
			items: []string{
				"Developer mode and dev channel",
				"USB emulation toggle and loopback-only mode",
				"SSH key, custom version update, and reset config",
			},
		},
	}
}

func fallbackLabel(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (a *App) uiAlpha() float64 {
	if a.prefs.PinChrome {
		return 1
	}
	if a.settingsOpen {
		return 1
	}
	remaining := time.Until(a.uiVisibleUntil)
	if remaining <= 0 {
		return 0
	}
	if remaining >= 180*time.Millisecond {
		return 1
	}
	return float64(remaining) / float64(180*time.Millisecond)
}

func (a *App) revealUIFor(d time.Duration) {
	until := time.Now().Add(d)
	if until.After(a.uiVisibleUntil) {
		a.uiVisibleUntil = until
	}
}

func (a *App) layoutChromeButtons(width int, snap session.Snapshot) []chromeButton {
	defs := make([]chromeButton, 0, 7)
	if snap.Phase != session.PhaseConnected {
		defs = append(defs, chromeButton{id: "reconnect", hint: reconnectLabel(snap.Phase), icon: iconReconnect, enabled: true})
	}
	if snap.Phase == session.PhaseConnected {
		defs = append(defs,
			chromeButton{id: "paste", hint: "Paste text", icon: iconPaste, enabled: true, active: a.pasteOpen},
			chromeButton{id: "mouse", hint: mouseButtonLabel(a.relative), icon: iconMouse, enabled: true, active: a.relative},
		)
	}
	defs = append(defs,
		chromeButton{id: "stats", hint: "Connection stats", icon: iconStats, enabled: true, active: a.statsOpen},
		chromeButton{id: "fullscreen", hint: "Toggle fullscreen", icon: iconFullscreen, enabled: true, active: ebiten.IsFullscreen()},
		chromeButton{id: "settings", hint: "Settings", icon: iconSettings, enabled: true, active: a.settingsOpen},
	)
	if snap.Phase == session.PhaseConnected {
		defs = append(defs, chromeButton{id: "reboot", hint: "Reboot device", icon: iconPower, enabled: true})
	}

	const size = 34.0
	const gap = 8.0
	totalW := (size * float64(len(defs))) + (gap * float64(len(defs)-1))
	x := float64(width) - totalW - 18
	y := 14.0
	out := make([]chromeButton, len(defs))
	for i, def := range defs {
		def.rect = rect{x: x + float64(i)*(size+gap), y: y, w: size, h: size}
		out[i] = def
	}
	return out
}

func drawChromeButton(screen *ebiten.Image, btn chromeButton, alpha float64) {
	fill := rgba(20, 30, 42, 220, alpha)
	stroke := rgba(130, 146, 162, 160, alpha)
	icon := rgba(236, 241, 245, 255, alpha)
	if btn.active {
		fill = rgba(28, 66, 116, 232, alpha)
		stroke = rgba(148, 198, 255, 210, alpha)
	}
	if !btn.enabled {
		fill = rgba(20, 24, 32, 160, alpha)
		stroke = rgba(86, 96, 108, 100, alpha)
		icon = rgba(126, 136, 146, 180, alpha)
	}
	vector.DrawFilledRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), fill, false)
	vector.StrokeRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), 1, stroke, false)
	drawIcon(screen, btn.icon, btn.rect, icon, alpha, btn.active)
}

func drawIcon(screen *ebiten.Image, kind iconKind, r rect, clr color.Color, alpha float64, active bool) {
	cx := float32(r.x + r.w/2)
	cy := float32(r.y + r.h/2)
	left := float32(r.x + 9)
	right := float32(r.x + r.w - 9)
	top := float32(r.y + 9)
	bottom := float32(r.y + r.h - 9)
	mid := float32(r.y + r.h/2)
	switch kind {
	case iconReconnect:
		vector.StrokeLine(screen, left+3, top+1, right-2, top+1, 1.5, clr, true)
		vector.StrokeLine(screen, right-2, top+1, right-2, bottom-4, 1.5, clr, true)
		vector.StrokeLine(screen, right-2, bottom-4, left+5, bottom-4, 1.5, clr, true)
		vector.StrokeLine(screen, left+5, bottom-4, left+5, mid+1, 1.5, clr, true)
		vector.StrokeLine(screen, left+5, mid+1, left+1, mid-3, 1.5, clr, true)
		vector.StrokeLine(screen, left+5, mid+1, left+9, mid-3, 1.5, clr, true)
	case iconMouse:
		if active {
			vector.StrokeLine(screen, cx, top, cx, bottom, 1.5, clr, true)
			vector.StrokeLine(screen, left, cy, right, cy, 1.5, clr, true)
			vector.StrokeLine(screen, cx, top, cx-3, top+3, 1.5, clr, true)
			vector.StrokeLine(screen, cx, top, cx+3, top+3, 1.5, clr, true)
			vector.StrokeLine(screen, cx, bottom, cx-3, bottom-3, 1.5, clr, true)
			vector.StrokeLine(screen, cx, bottom, cx+3, bottom-3, 1.5, clr, true)
		} else {
			vector.StrokeLine(screen, left+2, top, left+2, bottom-1, 1.5, clr, true)
			vector.StrokeLine(screen, left+2, top, right-1, cy, 1.5, clr, true)
			vector.StrokeLine(screen, left+2, top, cx+1, bottom-2, 1.5, clr, true)
		}
	case iconPaste:
		vector.StrokeRect(screen, left, top+2, right-left, bottom-top-2, 1.4, clr, false)
		vector.StrokeLine(screen, left+3, top+6, right-3, top+6, 1.4, clr, true)
		vector.StrokeLine(screen, cx, top+6, cx, top+1, 1.4, clr, true)
	case iconStats:
		vector.StrokeLine(screen, left+2, bottom, left+2, mid+4, 2, clr, true)
		vector.StrokeLine(screen, cx, bottom, cx, top+5, 2, clr, true)
		vector.StrokeLine(screen, right-2, bottom, right-2, mid-1, 2, clr, true)
	case iconMinus:
		vector.StrokeLine(screen, left, cy, right, cy, 2, clr, true)
	case iconPlus:
		vector.StrokeLine(screen, left, cy, right, cy, 2, clr, true)
		vector.StrokeLine(screen, cx, top, cx, bottom, 2, clr, true)
	case iconPower:
		vector.StrokeLine(screen, cx, top-1, cx, cy-2, 2, clr, true)
		vector.StrokeLine(screen, left+3, top+4, left, mid, 1.5, clr, true)
		vector.StrokeLine(screen, left, mid, left+4, bottom-1, 1.5, clr, true)
		vector.StrokeLine(screen, left+4, bottom-1, right-4, bottom-1, 1.5, clr, true)
		vector.StrokeLine(screen, right-4, bottom-1, right, mid, 1.5, clr, true)
		vector.StrokeLine(screen, right, mid, right-3, top+4, 1.5, clr, true)
	case iconSettings:
		vector.StrokeLine(screen, left, top+2, right, top+2, 1.5, clr, true)
		vector.StrokeLine(screen, left, cy, right, cy, 1.5, clr, true)
		vector.StrokeLine(screen, left, bottom-2, right, bottom-2, 1.5, clr, true)
		vector.FillCircle(screen, cx-4, top+2, 2.5, clr, true)
		vector.FillCircle(screen, cx+5, cy, 2.5, clr, true)
		vector.FillCircle(screen, cx-1, bottom-2, 2.5, clr, true)
	case iconFullscreen:
		vector.StrokeLine(screen, left, top+4, left, top, 1.6, clr, true)
		vector.StrokeLine(screen, left, top, left+4, top, 1.6, clr, true)
		vector.StrokeLine(screen, right, top+4, right, top, 1.6, clr, true)
		vector.StrokeLine(screen, right-4, top, right, top, 1.6, clr, true)
		vector.StrokeLine(screen, left, bottom-4, left, bottom, 1.6, clr, true)
		vector.StrokeLine(screen, left, bottom, left+4, bottom, 1.6, clr, true)
		vector.StrokeLine(screen, right, bottom-4, right, bottom, 1.6, clr, true)
		vector.StrokeLine(screen, right-4, bottom, right, bottom, 1.6, clr, true)
	case iconClose:
		vector.StrokeLine(screen, left, top, right, bottom, 1.8, clr, true)
		vector.StrokeLine(screen, right, top, left, bottom, 1.8, clr, true)
	}
}

func (a *App) drawTopBar(screen *ebiten.Image, snap session.Snapshot) {
	alpha := a.uiAlpha()
	if alpha <= 0 {
		return
	}
	buttons := a.layoutChromeButtons(screen.Bounds().Dx(), snap)
	a.chromeButtons = buttons

	label := fallbackLabel(snap.DeviceID, snap.Hostname, "jetkvm-client")
	status := fmt.Sprintf("%s  %s  %s", label, snap.Phase, mouseButtonLabel(a.relative))
	statusW, _ := measureText(status, 13)
	bgW := statusW + 22
	bgX := 14.0
	bgY := 14.0
	bgH := 34.0
	vector.DrawFilledRect(screen, float32(bgX), float32(bgY), float32(bgW), float32(bgH), rgba(14, 24, 36, 200, alpha), false)
	vector.StrokeRect(screen, float32(bgX), float32(bgY), float32(bgW), float32(bgH), 1, rgba(112, 128, 148, 120, alpha), false)
	drawText(screen, status, bgX+11, bgY+10, 13, rgba(228, 236, 244, 255, alpha))

	clusterX := buttons[0].rect.x - 10
	clusterW := buttons[len(buttons)-1].rect.x + buttons[len(buttons)-1].rect.w - clusterX + 10
	vector.DrawFilledRect(screen, float32(clusterX), 10, float32(clusterW), 42, rgba(14, 24, 36, 200, alpha), false)
	vector.StrokeRect(screen, float32(clusterX), 10, float32(clusterW), 42, 1, rgba(112, 128, 148, 120, alpha), false)
	for _, btn := range buttons {
		drawChromeButton(screen, btn, alpha)
	}
}

func (a *App) drawHint(screen *ebiten.Image) {
	alpha := a.uiAlpha()
	if alpha <= 0 {
		return
	}
	x, y := ebiten.CursorPosition()
	for _, btn := range a.chromeButtons {
		if btn.rect.contains(x, y) {
			w, _ := measureText(btn.hint, 13)
			bx := btn.rect.x + (btn.rect.w-w)/2 - 10
			if bx < 12 {
				bx = 12
			}
			bw := w + 20
			by := btn.rect.y + btn.rect.h + 8
			vector.DrawFilledRect(screen, float32(bx), float32(by), float32(bw), 28, rgba(8, 12, 18, 220, alpha), false)
			vector.StrokeRect(screen, float32(bx), float32(by), float32(bw), 28, 1, rgba(112, 128, 148, 120, alpha), false)
			drawText(screen, btn.hint, bx+10, by+8, 13, rgba(236, 241, 245, 255, alpha))
			return
		}
	}
}

func (a *App) drawStatusFooter(screen *ebiten.Image, snap session.Snapshot) {
	alpha := a.uiAlpha()
	if alpha <= 0 && snap.Phase == session.PhaseConnected && snap.LastError == "" {
		return
	}
	left := fmt.Sprintf("RTC %s  HID %s  Video %s  Quality %.0f%%", rtcLabel(snap.RTCState), readyWord(snap.HIDReady), readyWord(snap.VideoReady), snap.Quality*100)
	clr := rgba(164, 176, 188, 255, max(alpha, 0.75))
	y := float64(screen.Bounds().Dy() - 24)
	drawText(screen, left, 14, y, 12, clr)
	if snap.LastError != "" && snap.Phase != session.PhaseConnected {
		msg := trimForFooter(snap.LastError)
		w, _ := measureText(msg, 12)
		drawText(screen, msg, float64(screen.Bounds().Dx())-w-14, y, 12, rgba(228, 142, 142, 255, max(alpha, 0.75)))
	}
}

func readyWord(value bool) string {
	if value {
		return "ready"
	}
	return "pending"
}

func rgba(r, g, b, a uint8, alpha float64) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: uint8(float64(a) * alpha)}
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (a *App) drawSettingsOverlay(screen *ebiten.Image, snap session.Snapshot) {
	if !a.settingsOpen {
		a.settingsButtons = nil
		return
	}

	bounds := screen.Bounds()
	vector.DrawFilledRect(screen, 0, 0, float32(bounds.Dx()), float32(bounds.Dy()), color.RGBA{A: 170}, false)

	panelW := min(920, float64(bounds.Dx()-48))
	panelH := min(620, float64(bounds.Dy()-64))
	panelX := (float64(bounds.Dx()) - panelW) / 2
	panelY := (float64(bounds.Dy()) - panelH) / 2
	sidebarW := 190.0

	a.settingsPanel = rect{x: panelX, y: panelY, w: panelW, h: panelH}
	vector.DrawFilledRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), color.RGBA{R: 13, G: 20, B: 30, A: 246}, false)
	vector.StrokeRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), 1, color.RGBA{R: 88, G: 102, B: 118, A: 180}, false)
	vector.DrawFilledRect(screen, float32(panelX), float32(panelY), float32(sidebarW), float32(panelH), color.RGBA{R: 18, G: 28, B: 40, A: 255}, false)

	drawText(screen, "Settings", panelX+22, panelY+18, 22, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	drawWrappedText(screen, "Settings structure matches the upstream web client and only shows the current native or target capability surface.", panelX+22, panelY+46, panelW-80, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})

	closeBtn := chromeButton{
		id:      "settings_close",
		hint:    "Close settings",
		icon:    iconClose,
		enabled: true,
		rect:    rect{x: panelX + panelW - 44, y: panelY + 14, w: 28, h: 28},
	}
	a.settingsButtons = append(a.settingsButtons[:0], closeBtn)
	drawChromeButton(screen, closeBtn, 1)

	sections := settingsSections(snap)
	sideY := panelY + 84
	for _, section := range sections {
		btn := chromeButton{
			id:      "section:" + string(section.id),
			label:   section.label,
			enabled: true,
			active:  a.settingsSection == section.id,
			rect:    rect{x: panelX + 14, y: sideY, w: sidebarW - 28, h: 30},
		}
		a.settingsButtons = append(a.settingsButtons, btn)
		fill := color.RGBA{R: 18, G: 28, B: 40, A: 255}
		stroke := color.RGBA{R: 54, G: 68, B: 84, A: 180}
		textClr := color.RGBA{R: 184, G: 196, B: 208, A: 255}
		if btn.active {
			fill = color.RGBA{R: 28, G: 66, B: 116, A: 255}
			stroke = color.RGBA{R: 134, G: 186, B: 248, A: 180}
			textClr = color.RGBA{R: 240, G: 244, B: 248, A: 255}
		}
		vector.DrawFilledRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), fill, false)
		vector.StrokeRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), 1, stroke, false)
		drawText(screen, section.label, btn.rect.x+10, btn.rect.y+8, 13, textClr)
		sideY += 36
	}

	contentX := panelX + sidebarW + 24
	contentY := panelY + 84
	contentW := panelW - sidebarW - 44
	section := a.currentSection(sections)
	drawText(screen, section.label, contentX, contentY, 20, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	drawWrappedText(screen, section.description, contentX, contentY+28, contentW, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})

	switch section.id {
	case sectionGeneral:
		a.drawSettingsGeneral(screen, snap, contentX, contentY+74, contentW)
	case sectionMouse:
		a.drawSettingsMouse(screen, snap, contentX, contentY+74, contentW)
	case sectionKeyboard:
		a.drawSettingsKeyboard(screen, snap, contentX, contentY+74, contentW)
	case sectionVideo:
		a.drawSettingsVideo(screen, snap, contentX, contentY+74, contentW)
	case sectionHardware:
		a.drawSettingsHardware(screen, contentX, contentY+74, contentW)
	case sectionAccess:
		a.drawSettingsAccess(screen, contentX, contentY+74, contentW)
	case sectionAppearance:
		a.drawSettingsAppearance(screen, contentX, contentY+74, contentW)
	case sectionNetwork:
		a.drawSettingsNetwork(screen, contentX, contentY+74, contentW)
	case sectionAdvanced:
		a.drawSettingsAdvanced(screen, contentX, contentY+74, contentW)
	default:
		a.drawSettingsPlanned(screen, section, contentX, contentY+74, contentW)
	}
}

func (a *App) refreshSettingsSection(section settingsSection) {
	switch section {
	case sectionAccess:
		a.sectionData.Access.Loading = true
		a.sectionData.Access.Error = ""
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var cloud struct {
				Connected bool   `json:"connected"`
				URL       string `json:"url"`
				AppURL    string `json:"appUrl"`
			}
			var tlsState struct {
				Mode string `json:"mode"`
			}
			access := accessState{Loading: false}
			if err := a.ctrl.Query(ctx, "getCloudState", nil, &cloud); err == nil {
				access.CloudConnected = cloud.Connected
				access.CloudURL = cloud.URL
				access.CloudAppURL = cloud.AppURL
			}
			if err := a.ctrl.Query(ctx, "getTLSState", nil, &tlsState); err == nil {
				access.TLSMode = tlsState.Mode
			}
			if access.CloudURL == "" && access.TLSMode == "" {
				access.Error = "No access RPC state available on this target"
			}
			a.mu.Lock()
			a.sectionData.Access = access
			a.mu.Unlock()
		}()
	case sectionHardware:
		a.sectionData.Hardware.Loading = true
		a.sectionData.Hardware.Error = ""
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var usbEnabled bool
			var usbConfig struct {
				VendorID  string `json:"vendor_id"`
				ProductID string `json:"product_id"`
			}
			var usbDevices []any
			var rotation struct {
				Rotation string `json:"rotation"`
			}
			hw := hardwareState{Loading: false}
			if err := a.ctrl.Query(ctx, "getUsbEmulationState", nil, &usbEnabled); err == nil {
				hw.USBEmulation = &usbEnabled
			}
			if err := a.ctrl.Query(ctx, "getUsbConfig", nil, &usbConfig); err == nil {
				hw.USBConfig = fmt.Sprintf("%s / %s", usbConfig.VendorID, usbConfig.ProductID)
			}
			if err := a.ctrl.Query(ctx, "getUsbDevices", nil, &usbDevices); err == nil {
				hw.USBDevicesSummary = fmt.Sprintf("%d configured classes", len(usbDevices))
			}
			if err := a.ctrl.Query(ctx, "getDisplayRotation", nil, &rotation); err == nil {
				hw.DisplayRotation = rotation.Rotation
			}
			if hw.USBEmulation == nil && hw.USBConfig == "" && hw.DisplayRotation == "" {
				hw.Error = "No hardware RPC state available on this target"
			}
			a.mu.Lock()
			a.sectionData.Hardware = hw
			a.mu.Unlock()
		}()
	case sectionNetwork:
		a.sectionData.Network.Loading = true
		a.sectionData.Network.Error = ""
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var settings struct {
				Hostname string `json:"hostname"`
				IP       string `json:"ip"`
			}
			var state struct {
				Hostname string `json:"hostname"`
				IP       string `json:"ip"`
				DHCP     bool   `json:"dhcp"`
			}
			netState := networkState{Loading: false}
			if err := a.ctrl.Query(ctx, "getNetworkSettings", nil, &settings); err == nil {
				netState.Hostname = settings.Hostname
				netState.IP = settings.IP
			}
			if err := a.ctrl.Query(ctx, "getNetworkState", nil, &state); err == nil {
				if netState.Hostname == "" {
					netState.Hostname = state.Hostname
				}
				if netState.IP == "" {
					netState.IP = state.IP
				}
				netState.DHCP = &state.DHCP
			}
			if netState.Hostname == "" && netState.IP == "" && netState.DHCP == nil {
				netState.Error = "No network RPC state available on this target"
			}
			a.mu.Lock()
			a.sectionData.Network = netState
			a.mu.Unlock()
		}()
	case sectionAdvanced:
		a.sectionData.Advanced.Loading = true
		a.sectionData.Advanced.Error = ""
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var devMode struct {
				Enabled bool `json:"enabled"`
			}
			var usbEnabled bool
			var version struct {
				AppVersion    string `json:"appVersion"`
				SystemVersion string `json:"systemVersion"`
			}
			adv := advancedState{Loading: false}
			if err := a.ctrl.Query(ctx, "getDevModeState", nil, &devMode); err == nil {
				adv.DevMode = &devMode.Enabled
			}
			if err := a.ctrl.Query(ctx, "getUsbEmulationState", nil, &usbEnabled); err == nil {
				adv.USBEmulation = &usbEnabled
			}
			if err := a.ctrl.Query(ctx, "getLocalVersion", nil, &version); err == nil {
				adv.AppVersion = version.AppVersion
				adv.SystemVersion = version.SystemVersion
			}
			if adv.DevMode == nil && adv.USBEmulation == nil && adv.AppVersion == "" {
				adv.Error = "No advanced RPC state available on this target"
			}
			a.mu.Lock()
			a.sectionData.Advanced = adv
			a.mu.Unlock()
		}()
	}
}

func (a *App) currentSection(sections []settingsSectionDef) settingsSectionDef {
	for _, section := range sections {
		if section.id == a.settingsSection {
			return section
		}
	}
	return sections[0]
}

func (a *App) drawSettingsCard(screen *ebiten.Image, x, y, w, h float64, title, desc string) rect {
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), color.RGBA{R: 18, G: 28, B: 40, A: 255}, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 1, color.RGBA{R: 54, G: 68, B: 84, A: 180}, false)
	drawText(screen, title, x+16, y+14, 16, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	drawWrappedText(screen, desc, x+16, y+40, w-32, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	return rect{x: x, y: y, w: w, h: h}
}

func (a *App) drawSettingsAction(screen *ebiten.Image, id, label string, x, y, w float64, enabled, active bool) {
	btn := chromeButton{id: id, label: label, enabled: enabled, active: active, rect: rect{x: x, y: y, w: w, h: 30}}
	a.settingsButtons = append(a.settingsButtons, btn)
	fill := color.RGBA{R: 30, G: 42, B: 58, A: 255}
	stroke := color.RGBA{R: 80, G: 96, B: 112, A: 180}
	textClr := color.RGBA{R: 228, G: 236, B: 244, A: 255}
	if active {
		fill = color.RGBA{R: 28, G: 66, B: 116, A: 255}
		stroke = color.RGBA{R: 134, G: 186, B: 248, A: 180}
	}
	if !enabled {
		fill = color.RGBA{R: 24, G: 30, B: 38, A: 255}
		stroke = color.RGBA{R: 60, G: 68, B: 76, A: 150}
		textClr = color.RGBA{R: 128, G: 136, B: 144, A: 255}
	}
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), 30, fill, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), 30, 1, stroke, false)
	drawText(screen, label, x+12, y+8, 13, textClr)
}

func (a *App) drawSettingsGeneral(screen *ebiten.Image, snap session.Snapshot, x, y, w float64) {
	a.drawSettingsCard(screen, x, y, w, 238, "Connection", "The native client now mirrors the web UI's general section for connection state, versions, updates, and reboot.")
	drawText(screen, "Base URL", x+16, y+72, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, snap.BaseURL, x+128, y+72, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "Phase", x+16, y+96, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, string(snap.Phase), x+128, y+96, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "Signaling", x+16, y+120, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, signalingLabel(snap.SignalingMode), x+128, y+120, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "App Version", x+16, y+152, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(snap.AppVersion, "Unavailable"), x+128, y+152, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "System Version", x+16, y+176, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(snap.SystemVersion, "Unavailable"), x+128, y+176, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	updateLabel := "No updates reported"
	if snap.AppUpdateAvailable || snap.SystemUpdateAvailable {
		updateLabel = "Updates available"
	}
	drawText(screen, "Updates", x+16, y+200, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, updateLabel, x+128, y+200, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	a.drawSettingsAction(screen, "reconnect", reconnectLabel(snap.Phase), x+w-214, y+192, 92, true, false)
	a.drawSettingsAction(screen, "reboot", "Reboot", x+w-110, y+192, 92, snap.Phase != session.PhaseConnecting, false)
}

func (a *App) drawSettingsMouse(screen *ebiten.Image, snap session.Snapshot, x, y, w float64) {
	a.drawSettingsCard(screen, x, y, w, 206, "Mouse Mode", "Absolute and relative modes, host cursor visibility, and wheel throttling.")
	a.drawSettingsAction(screen, "mouse_absolute", "Absolute", x+16, y+104, 110, snap.Phase == session.PhaseConnected, !a.relative)
	a.drawSettingsAction(screen, "mouse_relative", "Relative", x+138, y+104, 110, snap.Phase == session.PhaseConnected, a.relative)
	a.drawSettingsAction(screen, "mouse_hide_cursor", "Hide Host Cursor", x+260, y+104, 154, true, a.hideCursor)
	drawText(screen, "Scroll throttling", x+16, y+152, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	a.drawSettingsAction(screen, "scroll_0", "Off", x+16, y+176, 64, true, a.scrollThrottle == 0)
	a.drawSettingsAction(screen, "scroll_10", "Low", x+92, y+176, 64, true, a.scrollThrottle == 10*time.Millisecond)
	a.drawSettingsAction(screen, "scroll_25", "Medium", x+168, y+176, 84, true, a.scrollThrottle == 25*time.Millisecond)
	a.drawSettingsAction(screen, "scroll_50", "High", x+264, y+176, 72, true, a.scrollThrottle == 50*time.Millisecond)
	a.drawSettingsAction(screen, "scroll_100", "Very High", x+348, y+176, 108, true, a.scrollThrottle == 100*time.Millisecond)
}

func (a *App) drawSettingsKeyboard(screen *ebiten.Image, snap session.Snapshot, x, y, w float64) {
	a.drawSettingsCard(screen, x, y, w, 248, "Keyboard", "Layout selection and a local pressed-key view.")
	drawText(screen, "Active layout", x+16, y+78, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	layout := snap.KeyboardLayout
	if layout == "" {
		layout = "en_US"
	}
	drawText(screen, layout, x+118, y+78, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	a.drawSettingsAction(screen, "toggle_pressed_keys", "Show Pressed Keys", x+w-174, y+64, 158, true, a.showPressedKeys)
	drawText(screen, "Layout presets", x+16, y+118, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	options := []struct {
		id    string
		label string
	}{
		{id: "layout:en_US", label: "US"},
		{id: "layout:en_UK", label: "UK"},
		{id: "layout:da_DK", label: "Danish"},
		{id: "layout:de_DE", label: "German"},
		{id: "layout:fr_FR", label: "French"},
		{id: "layout:es_ES", label: "Spanish"},
		{id: "layout:it_IT", label: "Italian"},
		{id: "layout:ja_JP", label: "Japanese"},
	}
	rowX := x + 16
	rowY := y + 142
	for i, option := range options {
		btnW := 94.0
		if len(option.label) > 7 {
			btnW = 112
		}
		a.drawSettingsAction(screen, option.id, option.label, rowX, rowY, btnW, snap.Phase == session.PhaseConnected, layout == option.id[7:])
		rowX += btnW + 10
		if (i+1)%4 == 0 {
			rowX = x + 16
			rowY += 38
		}
	}
	drawWrappedText(screen, "Paste and keyboard input use physical HID semantics. Unsupported paste characters are skipped and shown before send.", x+16, y+220, w-32, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}

func (a *App) drawSettingsVideo(screen *ebiten.Image, snap session.Snapshot, x, y, w float64) {
	a.drawSettingsCard(screen, x, y, w, 198, "Stream Quality", "The native client exposes quality presets and current EDID state.")
	a.drawSettingsAction(screen, "quality_preset_high", "High", x+16, y+106, 96, snap.Phase == session.PhaseConnected, snap.Quality >= 0.95)
	a.drawSettingsAction(screen, "quality_preset_medium", "Medium", x+124, y+106, 96, snap.Phase == session.PhaseConnected, snap.Quality >= 0.45 && snap.Quality < 0.95)
	a.drawSettingsAction(screen, "quality_preset_low", "Low", x+232, y+106, 96, snap.Phase == session.PhaseConnected, snap.Quality < 0.45)
	drawText(screen, fmt.Sprintf("Current factor %.2f", snap.Quality), x+16, y+146, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "EDID", x+16, y+176, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	edid := snap.EDID
	if edid == "" {
		edid = "Unavailable on current target"
	} else if len(edid) > 60 {
		edid = edid[:60] + "..."
	}
	drawWrappedText(screen, edid, x+72, y+176, w-88, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
}

func (a *App) drawSettingsHardware(screen *ebiten.Image, x, y, w float64) {
	a.mu.RLock()
	state := a.sectionData.Hardware
	a.mu.RUnlock()
	a.drawSettingsCard(screen, x, y, w, 244, "Hardware", "Display orientation and USB emulation state exposed by the current target.")
	if state.Loading {
		drawText(screen, "Loading hardware state…", x+16, y+82, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		return
	}
	drawText(screen, "USB Emulation", x+16, y+82, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	if state.USBEmulation != nil {
		drawText(screen, boolWord(*state.USBEmulation), x+128, y+82, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	} else {
		drawText(screen, "Unavailable", x+128, y+82, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	}
	drawText(screen, "USB Config", x+16, y+108, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(state.USBConfig, "Unavailable"), x+128, y+108, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "USB Devices", x+16, y+134, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(state.USBDevicesSummary, "Unavailable"), x+128, y+134, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "Display Rotation", x+16, y+160, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(state.DisplayRotation, "Unavailable"), x+128, y+160, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	a.drawSettingsAction(screen, "rotate_normal", "Normal", x+16, y+192, 88, state.DisplayRotation != "", state.DisplayRotation == "270")
	a.drawSettingsAction(screen, "rotate_inverted", "Inverted", x+116, y+192, 98, state.DisplayRotation != "", state.DisplayRotation == "90")
	if state.USBEmulation != nil {
		a.drawSettingsAction(screen, "usb_emulation_on", "USB On", x+228, y+192, 86, true, *state.USBEmulation)
		a.drawSettingsAction(screen, "usb_emulation_off", "USB Off", x+326, y+192, 92, true, !*state.USBEmulation)
	}
	if state.Error != "" {
		drawText(screen, state.Error, x+16, y+226, 13, color.RGBA{R: 220, G: 132, B: 132, A: 255})
	}
}

func (a *App) drawSettingsAccess(screen *ebiten.Image, x, y, w float64) {
	a.mu.RLock()
	state := a.sectionData.Access
	a.mu.RUnlock()
	a.drawSettingsCard(screen, x, y, w, 244, "Access", "Local access, TLS mode, and cloud adoption state exposed by the current target.")
	if state.Loading {
		drawText(screen, "Loading access state…", x+16, y+82, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		return
	}
	drawText(screen, "Cloud Connected", x+16, y+82, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, boolWord(state.CloudConnected), x+136, y+82, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "Cloud API", x+16, y+108, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(state.CloudURL, "Unavailable"), x+136, y+108, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "Cloud App", x+16, y+134, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(state.CloudAppURL, "Unavailable"), x+136, y+134, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "TLS Mode", x+16, y+160, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(state.TLSMode, "Unavailable"), x+136, y+160, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	a.drawSettingsAction(screen, "tls_disabled", "Disabled", x+16, y+192, 92, state.TLSMode != "", state.TLSMode == "disabled")
	a.drawSettingsAction(screen, "tls_self_signed", "Self-Signed", x+120, y+192, 114, state.TLSMode != "", state.TLSMode == "self-signed")
	if state.Error != "" {
		drawText(screen, state.Error, x+16, y+226, 13, color.RGBA{R: 220, G: 132, B: 132, A: 255})
	}
}

func (a *App) drawSettingsNetwork(screen *ebiten.Image, x, y, w float64) {
	a.mu.RLock()
	state := a.sectionData.Network
	a.mu.RUnlock()
	a.drawSettingsCard(screen, x, y, w, 174, "Network", "Current hostname, IP, and DHCP state exposed by the target.")
	if state.Loading {
		drawText(screen, "Loading network state…", x+16, y+82, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		return
	}
	drawText(screen, "Hostname", x+16, y+82, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(state.Hostname, "Unavailable"), x+112, y+82, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "IP", x+16, y+108, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(state.IP, "Unavailable"), x+112, y+108, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "DHCP", x+16, y+134, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	if state.DHCP != nil {
		drawText(screen, boolWord(*state.DHCP), x+112, y+134, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	} else {
		drawText(screen, "Unavailable", x+112, y+134, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	}
	if state.Error != "" {
		drawText(screen, state.Error, x+16, y+160, 13, color.RGBA{R: 220, G: 132, B: 132, A: 255})
	}
}

func (a *App) drawSettingsAdvanced(screen *ebiten.Image, x, y, w float64) {
	a.mu.RLock()
	state := a.sectionData.Advanced
	a.mu.RUnlock()
	a.drawSettingsCard(screen, x, y, w, 198, "Advanced", "The web UI uses this section for developer and recovery-oriented controls. The native client now surfaces the advanced state that the current target exposes.")
	if state.Loading {
		drawText(screen, "Loading advanced state…", x+16, y+82, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		return
	}
	drawText(screen, "Developer Mode", x+16, y+82, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	if state.DevMode != nil {
		drawText(screen, boolWord(*state.DevMode), x+142, y+82, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	} else {
		drawText(screen, "Unavailable", x+142, y+82, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	}
	drawText(screen, "USB Emulation", x+16, y+108, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	if state.USBEmulation != nil {
		drawText(screen, boolWord(*state.USBEmulation), x+142, y+108, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	} else {
		drawText(screen, "Unavailable", x+142, y+108, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	}
	drawText(screen, "App Version", x+16, y+134, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(state.AppVersion, "Unavailable"), x+142, y+134, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "System Version", x+16, y+160, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(state.SystemVersion, "Unavailable"), x+142, y+160, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	if state.Error != "" {
		drawText(screen, state.Error, x+16, y+190, 13, color.RGBA{R: 220, G: 132, B: 132, A: 255})
	}
}

func (a *App) drawSettingsAppearance(screen *ebiten.Image, x, y, w float64) {
	a.drawSettingsCard(screen, x, y, w, 188, "Appearance", "Desktop-specific chrome visibility and fullscreen behavior.")
	drawText(screen, "Chrome mode", x+16, y+78, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	a.drawSettingsAction(screen, "pin_chrome_off", "Auto-hide", x+136, y+66, 96, true, !a.prefs.PinChrome)
	a.drawSettingsAction(screen, "pin_chrome_on", "Pinned", x+244, y+66, 84, true, a.prefs.PinChrome)
	drawText(screen, "Fullscreen", x+16, y+118, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	a.drawSettingsAction(screen, "fullscreen", "Toggle Fullscreen", x+136, y+106, 160, true, ebiten.IsFullscreen())
	drawWrappedText(screen, "The pinned setting is saved locally and keeps the top action bar visible even when the pointer is away from the edge.", x+16, y+154, w-32, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}

func boolWord(v bool) string {
	if v {
		return "Enabled"
	}
	return "Disabled"
}

func (a *App) drawSettingsPlanned(screen *ebiten.Image, section settingsSectionDef, x, y, w float64) {
	a.drawSettingsCard(screen, x, y, w, 230, section.label, section.description)
	drawText(screen, "Current upstream surface", x+16, y+78, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	lineY := y + 104
	for _, item := range section.items {
		drawText(screen, "• "+item, x+24, lineY, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		lineY += 24
	}
	drawWrappedText(screen, "This section is not currently exposed by the native client or the current target. It stays in the list so the settings map matches the upstream product structure.", x+16, y+184, w-32, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}
