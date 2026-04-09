package app

import (
	"fmt"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/lkarlslund/jetkvm-native/pkg/session"
)

type iconKind string

const (
	iconReconnect iconKind = "reconnect"
	iconMouse     iconKind = "mouse"
	iconMinus     iconKind = "minus"
	iconPlus      iconKind = "plus"
	iconPower     iconKind = "power"
	iconSettings  iconKind = "settings"
	iconClose     iconKind = "close"
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
			description: "Layout, paste behavior, pressed-key display",
			available:   true,
			items: []string{
				"Keyboard layout selection",
				"Show pressed keys overlay",
				"Paste-text behavior tied to selected layout",
			},
		},
		{
			id:          sectionVideo,
			label:       "Video",
			description: "Stream quality, EDID, and client-side video tuning",
			available:   true,
			items: []string{
				"Stream quality presets",
				"EDID presets and custom EDID",
				"Brightness, contrast, and saturation tuning",
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
				"Dark overlay styling and UI density",
				"Future theme selection",
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
			available:   false,
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
			available:   false,
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
	canAct := snap.Phase == session.PhaseConnected || snap.Phase == session.PhaseDisconnected || snap.Phase == session.PhaseReconnecting
	defs := []chromeButton{
		{id: "reconnect", hint: reconnectLabel(snap.Phase), icon: iconReconnect, enabled: true},
		{id: "mouse", hint: mouseButtonLabel(a.relative), icon: iconMouse, enabled: snap.Phase == session.PhaseConnected, active: a.relative},
		{id: "quality_down", hint: "Lower stream quality", icon: iconMinus, enabled: snap.Phase == session.PhaseConnected},
		{id: "quality_up", hint: "Raise stream quality", icon: iconPlus, enabled: snap.Phase == session.PhaseConnected},
		{id: "reboot", hint: "Reboot device", icon: iconPower, enabled: canAct},
		{id: "settings", hint: "Settings", icon: iconSettings, enabled: true, active: a.settingsOpen},
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
	drawText(screen, "Mirrors the web UI structure, starting with the controls the native client already supports.", panelX+22, panelY+46, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})

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
	drawText(screen, section.description, contentX, contentY+28, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})

	switch section.id {
	case sectionGeneral:
		a.drawSettingsGeneral(screen, snap, contentX, contentY+74, contentW)
	case sectionMouse:
		a.drawSettingsMouse(screen, snap, contentX, contentY+74, contentW)
	case sectionKeyboard:
		a.drawSettingsKeyboard(screen, snap, contentX, contentY+74, contentW)
	case sectionVideo:
		a.drawSettingsVideo(screen, snap, contentX, contentY+74, contentW)
	case sectionAppearance:
		a.drawSettingsAppearance(screen, contentX, contentY+74, contentW)
	default:
		a.drawSettingsPlanned(screen, section, contentX, contentY+74, contentW)
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
	drawText(screen, desc, x+16, y+40, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
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
	a.drawSettingsCard(screen, x, y, w, 226, "Mouse Mode", "The web UI exposes absolute and relative modes, cursor behavior, scroll throttling, and a jiggler. The native client now exposes mode switching plus local cursor and wheel behavior.")
	a.drawSettingsAction(screen, "mouse_absolute", "Absolute", x+16, y+104, 110, snap.Phase == session.PhaseConnected, !a.relative)
	a.drawSettingsAction(screen, "mouse_relative", "Relative", x+138, y+104, 110, snap.Phase == session.PhaseConnected, a.relative)
	a.drawSettingsAction(screen, "mouse_hide_cursor", "Hide Host Cursor", x+260, y+104, 154, true, a.hideCursor)
	drawText(screen, "Scroll throttling", x+16, y+152, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	a.drawSettingsAction(screen, "scroll_0", "Off", x+16, y+176, 64, true, a.scrollThrottle == 0)
	a.drawSettingsAction(screen, "scroll_10", "Low", x+92, y+176, 64, true, a.scrollThrottle == 10*time.Millisecond)
	a.drawSettingsAction(screen, "scroll_25", "Medium", x+168, y+176, 84, true, a.scrollThrottle == 25*time.Millisecond)
	a.drawSettingsAction(screen, "scroll_50", "High", x+264, y+176, 72, true, a.scrollThrottle == 50*time.Millisecond)
	a.drawSettingsAction(screen, "scroll_100", "Very High", x+348, y+176, 108, true, a.scrollThrottle == 100*time.Millisecond)
	drawText(screen, "Jiggler remains planned until the native client has a dedicated UI for scheduling and safety confirmations.", x+16, y+212, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}

func (a *App) drawSettingsKeyboard(screen *ebiten.Image, snap session.Snapshot, x, y, w float64) {
	a.drawSettingsCard(screen, x, y, w, 248, "Keyboard", "The web UI exposes layout selection and a pressed-key view. The native client now supports the same structure, with layout written over RPC and the key overlay rendered locally.")
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
	drawText(screen, "The native client still uses physical-HID semantics first. Non-US punctuation remains best-effort until a deeper layout pass lands.", x+16, y+220, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}

func (a *App) drawSettingsVideo(screen *ebiten.Image, snap session.Snapshot, x, y, w float64) {
	a.drawSettingsCard(screen, x, y, w, 214, "Stream Quality", "The web UI exposes high, medium, and low quality plus EDID and image tuning. The native client supports quality directly and exposes the current EDID as device state.")
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
	drawText(screen, edid, x+72, y+176, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "EDID write support and image tuning controls remain planned for a later native pass.", x+16, y+202, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}

func (a *App) drawSettingsAppearance(screen *ebiten.Image, x, y, w float64) {
	a.drawSettingsCard(screen, x, y, w, 154, "Appearance", "The web UI only exposes theme selection here. In the native client this section is where chrome density and auto-hide behavior belong.")
	drawText(screen, "Current behavior", x+16, y+78, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, "Top icon bar fades away when idle and returns when the pointer moves near the top edge.", x+136, y+78, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "Planned next: density presets and optional persistent chrome mode.", x+16, y+112, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}

func (a *App) drawSettingsPlanned(screen *ebiten.Image, section settingsSectionDef, x, y, w float64) {
	a.drawSettingsCard(screen, x, y, w, 230, section.label, section.description)
	status := "Planned"
	if section.available {
		status = "In progress"
	}
	drawText(screen, "Status", x+16, y+78, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, status, x+96, y+78, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "Web UI surface", x+16, y+106, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	lineY := y + 132
	for _, item := range section.items {
		drawText(screen, "• "+item, x+24, lineY, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		lineY += 24
	}
	drawText(screen, "This section is listed so the native settings map stays aligned with the upstream web interface as features land.", x+16, y+194, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}
