package app

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

//go:generate go tool github.com/dmarkham/enumer -type=iconKind,settingsSection -linecomment -json -text -output chrome_enums.go

type iconKind uint8

const (
	iconReconnect  iconKind = iota // reconnect
	iconMouse                      // mouse
	iconPaste                      // paste
	iconMedia                      // media
	iconStats                      // stats
	iconMinus                      // minus
	iconPlus                       // plus
	iconPower                      // power
	iconSettings                   // settings
	iconFullscreen                 // fullscreen
	iconClose                      // close
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

type settingsSection uint8

const (
	sectionGeneral    settingsSection = iota // general
	sectionMouse                             // mouse
	sectionKeyboard                          // keyboard
	sectionVideo                             // video
	sectionHardware                          // hardware
	sectionAccess                            // access
	sectionAppearance                        // appearance
	sectionMacros                            // macros
	sectionNetwork                           // network
	sectionMQTT                              // mqtt
	sectionAdvanced                          // advanced
)

type settingsSectionDef struct {
	id          settingsSection
	label       string
	description string
	available   bool
	items       []string
}

type sectionData struct {
	General  generalState
	Mouse    mouseState
	Access   accessState
	Hardware hardwareState
	Network  networkState
	Advanced advancedState
}

type generalState struct {
	Loading    bool
	Error      string
	AutoUpdate *bool
}

type mouseState struct {
	Loading        bool
	Error          string
	JigglerEnabled *bool
	JigglerConfig  *session.JigglerConfig
}

type accessState struct {
	Loading bool
	Error   string
	State   session.AccessState
}

type hardwareState struct {
	Loading bool
	Error   string
	State   session.HardwareState
}

type networkState struct {
	Loading bool
	Error   string
	State   session.NetworkState
}

type advancedState struct {
	Loading bool
	Error   string
	State   session.AdvancedState
}

type settingsActionVisual struct {
	Enabled bool
	Active  bool
	Pending bool
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
			available:   true,
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
			available:   true,
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

func (a *App) layoutChromeButtons(width, height int, snap session.Snapshot) []chromeButton {
	defs := make([]chromeButton, 0, 5)
	if snap.Phase != session.PhaseConnected {
		defs = append(defs, chromeButton{id: "reconnect", hint: reconnectLabel(snap.Phase), icon: iconReconnect, enabled: true})
	}
	if snap.Phase == session.PhaseConnected {
		defs = append(defs, chromeButton{id: "paste", hint: "Paste text", icon: iconPaste, enabled: true, active: a.pasteOpen})
		defs = append(defs, chromeButton{id: "media", hint: "Virtual media", icon: iconMedia, enabled: true, active: a.mediaOpen})
	}
	defs = append(defs,
		chromeButton{id: "stats", hint: "Connection stats", icon: iconStats, enabled: true, active: a.statsOpen},
		chromeButton{id: "fullscreen", hint: "Toggle fullscreen", icon: iconFullscreen, enabled: true, active: ebiten.IsFullscreen()},
		chromeButton{id: "settings", hint: "Settings", icon: iconSettings, enabled: true, active: a.settingsOpen},
	)

	const size = 34.0
	const gap = 8.0
	totalW := size
	totalH := size
	horizontal := a.prefs.ChromeLayout != chromeLayoutVertical
	if horizontal {
		totalW = (size * float64(len(defs))) + (gap * float64(len(defs)-1))
	} else {
		totalH = (size * float64(len(defs))) + (gap * float64(len(defs)-1))
	}
	x, y := chromeAnchorOrigin(a.prefs.ChromeAnchor, float64(width), float64(height), totalW, totalH)
	out := make([]chromeButton, len(defs))
	for i, def := range defs {
		btnX := x
		btnY := y
		if horizontal {
			btnX += float64(i) * (size + gap)
		} else {
			btnY += float64(i) * (size + gap)
		}
		def.rect = rect{x: btnX, y: btnY, w: size, h: size}
		out[i] = def
	}
	return out
}

func (a *App) chromeRevealZone(width, height int, snap session.Snapshot) rect {
	buttons := a.layoutChromeButtons(width, height, snap)
	if len(buttons) == 0 {
		return rect{}
	}
	left := buttons[0].rect.x
	top := buttons[0].rect.y
	right := buttons[0].rect.x + buttons[0].rect.w
	bottom := buttons[0].rect.y + buttons[0].rect.h
	for _, btn := range buttons[1:] {
		if btn.rect.x < left {
			left = btn.rect.x
		}
		if btn.rect.y < top {
			top = btn.rect.y
		}
		if btn.rect.x+btn.rect.w > right {
			right = btn.rect.x + btn.rect.w
		}
		if btn.rect.y+btn.rect.h > bottom {
			bottom = btn.rect.y + btn.rect.h
		}
	}
	const pad = 28.0
	return rect{
		x: max(0, left-pad),
		y: max(0, top-pad),
		w: min(float64(width), right+pad) - max(0, left-pad),
		h: min(float64(height), bottom+pad) - max(0, top-pad),
	}
}

func chromeAnchorOrigin(anchor ChromeAnchor, width, height, clusterW, clusterH float64) (float64, float64) {
	const margin = 18.0
	switch anchor {
	case chromeAnchorTopLeft:
		return margin, margin
	case chromeAnchorTopCenter:
		return (width - clusterW) / 2, margin
	case chromeAnchorLeftCenter:
		return margin, (height - clusterH) / 2
	case chromeAnchorRightCenter:
		return width - clusterW - margin, (height - clusterH) / 2
	case chromeAnchorBottomLeft:
		return margin, height - clusterH - margin
	case chromeAnchorBottomCenter:
		return (width - clusterW) / 2, height - clusterH - margin
	case chromeAnchorBottomRight:
		return width - clusterW - margin, height - clusterH - margin
	default:
		return width - clusterW - margin, margin
	}
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
	vector.FillRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), fill, false)
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
	case iconMedia:
		vector.StrokeRect(screen, left+1, top+2, right-left-2, bottom-top-5, 1.4, clr, false)
		vector.StrokeLine(screen, left+5, top+2, left+8, top-1, 1.4, clr, true)
		vector.StrokeLine(screen, right-5, top+2, right-8, top-1, 1.4, clr, true)
		vector.StrokeLine(screen, left+4, cy, right-4, cy, 1.4, clr, true)
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
	buttons := a.layoutChromeButtons(screen.Bounds().Dx(), screen.Bounds().Dy(), snap)
	a.chromeButtons = buttons
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
			vector.FillRect(screen, float32(bx), float32(by), float32(bw), 28, rgba(8, 12, 18, 220, alpha), false)
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
	left := fmt.Sprintf("RTC %s  HID %s  Video %s", rtcLabel(snap.RTCState), readyWord(snap.HIDReady), readyWord(snap.VideoReady))
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
	vector.FillRect(screen, 0, 0, float32(bounds.Dx()), float32(bounds.Dy()), color.RGBA{A: 170}, false)

	sections := settingsSections(snap)
	section := a.currentSection(sections)
	sidebarW := 156.0
	contentW := min(720, float64(bounds.Dx())-sidebarW-84)
	panelW := sidebarW + contentW + 44
	panelW = min(panelW, float64(bounds.Dx()-48))
	panelH := min(max(a.settingsPanelHeight(section, contentW), settingsSidebarHeight(len(sections))), float64(bounds.Dy()-56))

	panelX := (float64(bounds.Dx()) - panelW) / 2
	panelY := (float64(bounds.Dy()) - panelH) / 2

	a.settingsPanel = rect{x: panelX, y: panelY, w: panelW, h: panelH}
	vector.FillRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), color.RGBA{R: 13, G: 20, B: 30, A: 246}, false)
	vector.StrokeRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), 1, color.RGBA{R: 88, G: 102, B: 118, A: 180}, false)
	vector.FillRect(screen, float32(panelX), float32(panelY), float32(sidebarW), float32(panelH), color.RGBA{R: 18, G: 28, B: 40, A: 255}, false)
	drawText(screen, "Settings", panelX+18, panelY+16, 20, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	drawWrappedText(screen, fallbackLabel(snap.DeviceID, snap.Hostname, snap.BaseURL), panelX+18, panelY+40, sidebarW-36, 11, color.RGBA{R: 166, G: 178, B: 190, A: 255})

	closeBtn := chromeButton{
		id:      "settings_close",
		hint:    "Close settings",
		icon:    iconClose,
		enabled: true,
		rect:    rect{x: panelX + panelW - 40, y: panelY + 12, w: 26, h: 26},
	}
	a.settingsButtons = append(a.settingsButtons[:0], closeBtn)
	drawChromeButton(screen, closeBtn, 1)

	sideY := panelY + 72
	sideBtnH, sideGap, sideFontSize := settingsSidebarMetrics(panelH, len(sections))
	for _, section := range sections {
		btn := chromeButton{
			id:      "section:" + section.id.String(),
			label:   section.label,
			enabled: true,
			active:  a.settingsSection == section.id,
			rect:    rect{x: panelX + 10, y: sideY, w: sidebarW - 20, h: sideBtnH},
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
		vector.FillRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), fill, false)
		vector.StrokeRect(screen, float32(btn.rect.x), float32(btn.rect.y), float32(btn.rect.w), float32(btn.rect.h), 1, stroke, false)
		drawText(screen, section.label, btn.rect.x+10, btn.rect.y+(btn.rect.h-sideFontSize)/2-1, sideFontSize, textClr)
		sideY += sideBtnH + sideGap
	}

	contentX := panelX + sidebarW + 18
	contentY := panelY + 18
	contentW = panelW - sidebarW - 32
	contentDescH := wrappedTextHeight(section.description, contentW-48, 12)
	contentHeaderH := 28 + contentDescH + 18
	drawText(screen, section.label, contentX, contentY, 22, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	drawWrappedText(screen, section.description, contentX, contentY+28, contentW-48, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	vector.StrokeLine(screen, float32(contentX), float32(contentY+contentHeaderH), float32(contentX+contentW), float32(contentY+contentHeaderH), 1, color.RGBA{R: 42, G: 54, B: 68, A: 180}, false)

	switch section.id {
	case sectionGeneral:
		a.drawSettingsGeneral(screen, snap, contentX, contentY+contentHeaderH+18, contentW)
	case sectionMouse:
		a.drawSettingsMouse(screen, snap, contentX, contentY+contentHeaderH+18, contentW)
	case sectionKeyboard:
		a.drawSettingsKeyboard(screen, snap, contentX, contentY+contentHeaderH+18, contentW)
	case sectionVideo:
		a.drawSettingsVideo(screen, snap, contentX, contentY+contentHeaderH+18, contentW)
	case sectionHardware:
		a.drawSettingsHardware(screen, contentX, contentY+contentHeaderH+18, contentW)
	case sectionAccess:
		a.drawSettingsAccess(screen, contentX, contentY+contentHeaderH+18, contentW)
	case sectionAppearance:
		a.drawSettingsAppearance(screen, contentX, contentY+contentHeaderH+18, contentW)
	case sectionNetwork:
		a.drawSettingsNetwork(screen, contentX, contentY+contentHeaderH+18, contentW)
	case sectionAdvanced:
		a.drawSettingsAdvanced(screen, contentX, contentY+contentHeaderH+18, contentW)
	default:
		a.drawSettingsPlanned(screen, section, contentX, contentY+contentHeaderH+18, contentW)
	}
}

func (a *App) settingsPanelHeight(section settingsSectionDef, contentW float64) float64 {
	headerH := 18 + 28 + wrappedTextHeight(section.description, contentW-48, 12) + 18 + 18
	bodyH := a.settingsSectionBodyHeight(section.id, contentW)
	return headerH + bodyH + 18
}

func (a *App) settingsSectionBodyHeight(section settingsSection, w float64) float64 {
	switch section {
	case sectionKeyboard:
		return a.settingsKeyboardBodyHeight(w)
	case sectionGeneral, sectionHardware, sectionAccess:
		return a.settingsWideBodyHeight(section, w)
	case sectionMouse, sectionAdvanced:
		return a.settingsWideBodyHeight(section, w)
	case sectionVideo, sectionNetwork, sectionAppearance:
		return a.settingsWideBodyHeight(section, w)
	default:
		return a.settingsPlannedBodyHeight(section, w)
	}
}

func (a *App) settingsWideBodyHeight(section settingsSection, w float64) float64 {
	switch section {
	case sectionGeneral:
		leftW := (w - 14) * 0.58
		rightW := w - leftW - 14
		descH := wrappedTextHeight("Reconnect the native session, manage auto-updates, or force a device reboot.", rightW-32, 12)
		rightH := 48 + descH + 20 + 30 + 8 + 30 + 8 + 30 + 18
		return max(214, rightH)
	case sectionMouse:
		leftW := (w - 14) * 0.54
		rightW := w - leftW - 14
		descH := wrappedTextHeight("Throttle local wheel bursts before sending them to the device.", leftW-32, 12)
		leftH := 48 + descH + 20 + 30 + 8 + 30 + 18 + 68
		rightH := 244.0
		if a.jigglerEditorOpen {
			rightH = 560
		}
		a.mu.RLock()
		state := a.sectionData.Mouse
		a.mu.RUnlock()
		if state.Error != "" {
			base := 208.0
			if a.jigglerEditorOpen {
				base = 520
			}
			rightH = max(rightH, base+wrappedTextHeight(state.Error, rightW-32, 12)+24)
		}
		return max(leftH, rightH)
	case sectionVideo:
		leftW := (w - 14) * 0.48
		rightW := w - leftW - 14
		qualityState := a.settingsAction(settingsGroupVideoQuality)
		leftH := 174.0
		if qualityState.Pending || qualityState.Error != "" {
			leftH += wrappedTextHeight(fallbackLabel(qualityState.Error, "Applying…"), leftW-32, 12)
		}
		a.mu.RLock()
		edid := a.ctrl.Snapshot().EDID
		a.mu.RUnlock()
		if edid == "" {
			edid = "Unavailable on current target"
		}
		rightH := 48 + wrappedTextHeight(edid, rightW-32, 12) + 24
		return max(leftH, max(174, rightH))
	case sectionHardware:
		a.mu.RLock()
		state := a.sectionData.Hardware
		a.mu.RUnlock()
		leftW := (w - 14) * 0.48
		rightW := w - leftW - 14
		leftH := max(220, 82+wrappedTextHeight("Rotate the displayed feed to match the connected panel orientation.", leftW-32, 12)+68)
		rightH := max(336, 126+wrappedTextHeight(usbDevicesSummary(state.State.USBDevices), rightW-32, 12)+184)
		if state.Error != "" {
			errH := wrappedTextHeight(state.Error, w-32, 12) + 24
			return max(leftH, max(rightH, 312+errH))
		}
		return max(leftH, rightH)
	case sectionAccess:
		a.mu.RLock()
		state := a.sectionData.Access
		a.mu.RUnlock()
		leftW := (w - 14) * 0.5
		rightW := w - leftW - 14
		leftH := max(220, 156+wrappedTextHeight(fallbackLabel(state.State.Cloud.AppURL, "Unavailable"), leftW-32, 12)+24)
		rightH := max(220, 84+wrappedTextHeight("Use the target's currently exposed TLS mode. Native client transport follows whatever the device publishes.", rightW-32, 12)+66)
		if state.Error != "" {
			errH := wrappedTextHeight(state.Error, w-32, 12) + 24
			return max(leftH, max(rightH, 192+errH))
		}
		return max(leftH, rightH)
	case sectionNetwork:
		a.mu.RLock()
		state := a.sectionData.Network
		a.mu.RUnlock()
		if state.Error == "" {
			return 152
		}
		return max(152, 124+wrappedTextHeight(state.Error, w-32, 12)+24)
	case sectionAdvanced:
		a.mu.RLock()
		state := a.sectionData.Advanced
		a.mu.RUnlock()
		if state.Error == "" {
			return 220
		}
		return max(220, 194+wrappedTextHeight(state.Error, w-32, 12)+24)
	case sectionAppearance:
		return max(330, 280+wrappedTextHeight("Position chooses where the chrome sits on screen. Layout changes whether the control buttons run across or down.", w-32, 12)+24)
	default:
		return 220
	}
}

func (a *App) settingsPlannedBodyHeight(section settingsSection, w float64) float64 {
	defs := settingsSections(session.Snapshot{})
	var current settingsSectionDef
	for _, def := range defs {
		if def.id == section {
			current = def
			break
		}
	}
	bodyH := 46 + wrappedTextHeight(current.description, w-32, 12) + 24 + 24
	for _, item := range current.items {
		bodyH += wrappedTextHeight("• "+item, w-40, 12) + 10
	}
	bodyH += wrappedTextHeight("This section exists in the upstream product structure but is not currently exposed by this target or the desktop client.", w-32, 12) + 32
	return max(220, bodyH)
}

func settingsSidebarHeight(count int) float64 {
	if count <= 0 {
		return 320
	}
	return 72 + float64(count)*30 + float64(count-1)*6 + 18
}

func settingsSidebarMetrics(panelH float64, count int) (btnH, gap, fontSize float64) {
	btnH = 30
	gap = 6
	fontSize = 13
	if count <= 0 {
		return btnH, gap, fontSize
	}
	available := panelH - 72 - 18
	if available <= 0 {
		return 22, 2, 12
	}
	for _, candidateGap := range []float64{6, 4, 2} {
		candidateH := (available - float64(count-1)*candidateGap) / float64(count)
		if candidateH >= 30 {
			return 30, candidateGap, 13
		}
		if candidateH >= 24 {
			return candidateH, candidateGap, 12
		}
	}
	candidateH := (available - float64(count-1)*2) / float64(count)
	if candidateH < 20 {
		candidateH = 20
	}
	return candidateH, 2, 12
}

func (a *App) refreshSettingsSection(section settingsSection) {
	seq := a.markSettingsSectionLoading(section)
	go func() {
		_ = a.loadSettingsSection(section, seq)
	}()
}

func (a *App) refreshSettingsSectionSync(section settingsSection) error {
	return a.loadSettingsSection(section, a.nextSectionLoadSeq(section))
}

func (a *App) markSettingsSectionLoading(section settingsSection) uint64 {
	seq := a.nextSectionLoadSeq(section)
	a.mu.Lock()
	defer a.mu.Unlock()
	switch section {
	case sectionGeneral:
		a.sectionData.General.Loading = true
		a.sectionData.General.Error = ""
	case sectionMouse:
		a.sectionData.Mouse.Loading = true
		a.sectionData.Mouse.Error = ""
	case sectionAccess:
		a.sectionData.Access.Loading = true
		a.sectionData.Access.Error = ""
	case sectionHardware:
		a.sectionData.Hardware.Loading = true
		a.sectionData.Hardware.Error = ""
	case sectionNetwork:
		a.sectionData.Network.Loading = true
		a.sectionData.Network.Error = ""
	case sectionAdvanced:
		a.sectionData.Advanced.Loading = true
		a.sectionData.Advanced.Error = ""
	}
	return seq
}

func (a *App) loadSettingsSection(section settingsSection, seq uint64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var err error
	switch section {
	case sectionGeneral:
		state := generalState{Loading: false}
		if enabled, callErr := a.ctrl.GetAutoUpdateState(ctx); callErr == nil {
			state.AutoUpdate = &enabled
		}
		if state.AutoUpdate == nil {
			state.Error = "No general RPC state available on this target"
			err = errors.New(state.Error)
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.General = state
		}
		a.mu.Unlock()
	case sectionMouse:
		state := mouseState{Loading: false}
		if enabled, callErr := a.ctrl.GetJigglerState(ctx); callErr == nil {
			state.JigglerEnabled = &enabled
		}
		if cfg, callErr := a.ctrl.GetJigglerConfig(ctx); callErr == nil {
			state.JigglerConfig = &cfg
		}
		if state.JigglerEnabled == nil && state.JigglerConfig == nil {
			state.Error = "No mouse RPC state available on this target"
			err = errors.New(state.Error)
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.Mouse = state
		}
		a.mu.Unlock()
	case sectionAccess:
		state := accessState{Loading: false}
		if cloud, callErr := a.ctrl.GetCloudState(ctx); callErr == nil {
			state.State.Cloud = cloud
		}
		if tlsMode, callErr := a.ctrl.GetTLSState(ctx); callErr == nil {
			state.State.TLS = tlsMode
		}
		if state.State.Cloud.URL == "" && state.State.TLS == session.TLSModeUnknown {
			state.Error = "No access RPC state available on this target"
			err = errors.New(state.Error)
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.Access = state
		}
		a.mu.Unlock()
	case sectionHardware:
		state := hardwareState{Loading: false}
		if usbEnabled, callErr := a.ctrl.GetUSBEmulationState(ctx); callErr == nil {
			state.State.USBEmulation = &usbEnabled
		}
		if usbConfig, callErr := a.ctrl.GetUSBConfig(ctx); callErr == nil {
			state.State.USBConfig = usbConfig
		}
		if devices, callErr := a.ctrl.GetUSBDevices(ctx); callErr == nil {
			state.State.USBDevices = devices
			state.State.USBDeviceCount = usbDeviceCount(devices)
		}
		if rotation, callErr := a.ctrl.GetDisplayRotation(ctx); callErr == nil {
			state.State.DisplayRotation = rotation
		}
		if state.State.USBEmulation == nil &&
			state.State.USBConfig == (session.USBConfig{}) &&
			state.State.DisplayRotation == session.DisplayRotationUnknown {
			state.Error = "No hardware RPC state available on this target"
			err = errors.New(state.Error)
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.Hardware = state
		}
		a.mu.Unlock()
	case sectionNetwork:
		state := networkState{Loading: false}
		if settings, callErr := a.ctrl.GetNetworkSettings(ctx); callErr == nil {
			state.State.Hostname = settings.Hostname
			state.State.IP = settings.IP
		}
		if current, callErr := a.ctrl.GetNetworkState(ctx); callErr == nil {
			if state.State.Hostname == "" {
				state.State.Hostname = current.Hostname
			}
			if state.State.IP == "" {
				state.State.IP = current.IP
			}
			state.State.DHCP = current.DHCP
		}
		if state.State.Hostname == "" && state.State.IP == "" && state.State.DHCP == nil {
			state.Error = "No network RPC state available on this target"
			err = errors.New(state.Error)
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.Network = state
		}
		a.mu.Unlock()
	case sectionAdvanced:
		state := advancedState{Loading: false}
		if devMode, callErr := a.ctrl.GetDeveloperModeState(ctx); callErr == nil {
			state.State.DevMode = devMode
		}
		if usbEnabled, callErr := a.ctrl.GetUSBEmulationState(ctx); callErr == nil {
			state.State.USBEmulation = &usbEnabled
		}
		if version, callErr := a.ctrl.GetLocalVersion(ctx); callErr == nil {
			state.State.Version = version
		}
		if state.State.DevMode == nil && state.State.USBEmulation == nil && state.State.Version.AppVersion == "" {
			state.Error = "No advanced RPC state available on this target"
			err = errors.New(state.Error)
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.Advanced = state
		}
		a.mu.Unlock()
	}
	return err
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
	vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), color.RGBA{R: 18, G: 28, B: 40, A: 255}, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 1, color.RGBA{R: 54, G: 68, B: 84, A: 180}, false)
	descY := y + 36
	if title != "" {
		drawText(screen, title, x+16, y+14, 15, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	} else {
		descY = y + 14
	}
	if desc != "" {
		drawWrappedText(screen, desc, x+16, descY, w-32, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	}
	return rect{x: x, y: y, w: w, h: h}
}

func drawSettingsKeyValue(screen *ebiten.Image, label, value string, x, y, split float64) {
	drawText(screen, label, x, y, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(value, "Unavailable"), x+split, y, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
}

func drawSettingsSectionLabel(screen *ebiten.Image, label string, x, y float64) {
	drawText(screen, label, x, y, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}

func (a *App) drawSettingsAction(screen *ebiten.Image, id, label string, x, y, w float64, visual settingsActionVisual) {
	btn := chromeButton{id: id, label: label, enabled: visual.Enabled, active: visual.Active, rect: rect{x: x, y: y, w: w, h: 30}}
	a.settingsButtons = append(a.settingsButtons, btn)
	fill := color.RGBA{R: 30, G: 42, B: 58, A: 255}
	stroke := color.RGBA{R: 80, G: 96, B: 112, A: 180}
	textClr := color.RGBA{R: 228, G: 236, B: 244, A: 255}
	if visual.Active {
		fill = color.RGBA{R: 28, G: 66, B: 116, A: 255}
		stroke = color.RGBA{R: 134, G: 186, B: 248, A: 180}
	}
	if visual.Pending {
		fill = color.RGBA{R: 88, G: 70, B: 24, A: 255}
		stroke = color.RGBA{R: 234, G: 179, B: 8, A: 180}
	}
	if !visual.Enabled {
		fill = color.RGBA{R: 24, G: 30, B: 38, A: 255}
		stroke = color.RGBA{R: 60, G: 68, B: 76, A: 150}
		textClr = color.RGBA{R: 128, G: 136, B: 144, A: 255}
	}
	vector.FillRect(screen, float32(x), float32(y), float32(w), 30, fill, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), 30, 1, stroke, false)
	drawText(screen, label, x+12, y+8, 13, textClr)
}

func (a *App) drawSettingsActionStatus(screen *ebiten.Image, group settingsActionGroup, x, y, w float64) {
	state := a.settingsAction(group)
	switch {
	case state.Pending:
		drawWrappedText(screen, "Applying…", x, y, w, 12, color.RGBA{R: 245, G: 200, B: 96, A: 255})
	case state.Error != "":
		drawWrappedText(screen, state.Error, x, y, w, 12, color.RGBA{R: 220, G: 132, B: 132, A: 255})
	}
}

func (a *App) drawSettingsInput(screen *ebiten.Image, id string, x, y, w, h float64, value, placeholder string, focused bool) {
	border := color.RGBA{R: 84, G: 104, B: 122, A: 120}
	if focused {
		border = color.RGBA{R: 96, G: 165, B: 250, A: 180}
	}
	vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), color.RGBA{R: 8, G: 12, B: 18, A: 255}, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 1, border, false)
	a.settingsButtons = append(a.settingsButtons, chromeButton{id: id, enabled: true, rect: rect{x: x, y: y, w: w, h: h}})
	text := value
	textColor := color.RGBA{R: 236, G: 241, B: 245, A: 255}
	if text == "" {
		text = placeholder
		textColor = color.RGBA{R: 106, G: 120, B: 138, A: 255}
	}
	if focused && value != "" && time.Now().UnixNano()/500_000_000%2 == 0 {
		text += "|"
	}
	drawText(screen, trimTextToWidth(text, w-24, 13), x+12, y+10, 13, textColor)
}

func (a *App) drawSettingsGeneral(screen *ebiten.Image, snap session.Snapshot, x, y, w float64) {
	a.mu.RLock()
	state := a.sectionData.General
	a.mu.RUnlock()
	leftW := (w - 14) * 0.58
	rightX := x + leftW + 14
	rightW := w - leftW - 14
	cardH := a.settingsWideBodyHeight(sectionGeneral, w)
	a.drawSettingsCard(screen, x, y, leftW, cardH, "Device", "")
	drawSettingsKeyValue(screen, "Base URL", snap.BaseURL, x+16, y+48, 116)
	drawSettingsKeyValue(screen, "Phase", snap.Phase.String(), x+16, y+74, 116)
	drawSettingsKeyValue(screen, "Signaling", signalingLabel(snap.SignalingMode), x+16, y+100, 116)
	drawSettingsKeyValue(screen, "App Version", snap.AppVersion, x+16, y+132, 116)
	drawSettingsKeyValue(screen, "System Version", snap.SystemVersion, x+16, y+158, 116)
	updateLabel := "No updates reported"
	if snap.AppUpdateAvailable || snap.SystemUpdateAvailable {
		updateLabel = "Updates available"
	}
	drawSettingsKeyValue(screen, "Updates", updateLabel, x+16, y+184, 116)
	a.drawSettingsCard(screen, rightX, y, rightW, cardH, "Actions", "")
	drawWrappedText(screen, "Reconnect the native session, manage auto-updates, or force a device reboot.", rightX+16, y+48, rightW-32, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	a.drawSettingsAction(screen, "reconnect", reconnectLabel(snap.Phase), rightX+16, y+98, rightW-32, settingsActionVisual{Enabled: true})
	a.drawSettingsAction(screen, "reboot", "Reboot device", rightX+16, y+136, rightW-32, settingsActionVisual{Enabled: snap.Phase != session.PhaseConnecting})
	autoUpdate := a.settingsAction(settingsGroupAutoUpdate)
	drawSettingsSectionLabel(screen, "Auto updates", rightX+16, y+184)
	if state.Loading {
		drawText(screen, "Loading…", rightX+120, y+184, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	} else {
		drawSettingsKeyValue(screen, "State", boolPtrWord(state.AutoUpdate), rightX+16, y+206, 56)
		a.drawSettingsAction(screen, "auto_update_on", "Enabled", rightX+16, y+228, 92, settingsActionVisual{Enabled: state.AutoUpdate != nil && (!autoUpdate.Pending || autoUpdate.PendingChoice == "on"), Active: state.AutoUpdate != nil && *state.AutoUpdate, Pending: autoUpdate.Pending && autoUpdate.PendingChoice == "on"})
		a.drawSettingsAction(screen, "auto_update_off", "Disabled", rightX+120, y+228, 94, settingsActionVisual{Enabled: state.AutoUpdate != nil && (!autoUpdate.Pending || autoUpdate.PendingChoice == "off"), Active: state.AutoUpdate != nil && !*state.AutoUpdate, Pending: autoUpdate.Pending && autoUpdate.PendingChoice == "off"})
		a.drawSettingsActionStatus(screen, settingsGroupAutoUpdate, rightX+16, y+266, rightW-32)
	}
	if state.Error != "" {
		drawWrappedText(screen, state.Error, rightX+16, y+266, rightW-32, 12, color.RGBA{R: 220, G: 132, B: 132, A: 255})
	}
}

func (a *App) drawSettingsMouse(screen *ebiten.Image, snap session.Snapshot, x, y, w float64) {
	a.mu.RLock()
	state := a.sectionData.Mouse
	a.mu.RUnlock()
	leftW := (w - 14) * 0.54
	rightX := x + leftW + 14
	rightW := w - leftW - 14
	cardH := a.settingsWideBodyHeight(sectionMouse, w)
	a.drawSettingsCard(screen, x, y, leftW, cardH, "Pointer", "")
	drawSettingsSectionLabel(screen, "Remote mode", x+16, y+48)
	a.drawSettingsAction(screen, "mouse_absolute", "Absolute", x+16, y+66, 110, settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected, Active: !a.relative})
	a.drawSettingsAction(screen, "mouse_relative", "Relative", x+138, y+66, 110, settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected, Active: a.relative})
	drawSettingsSectionLabel(screen, "Local cursor", x+16, y+114)
	a.drawSettingsAction(screen, "mouse_hide_cursor", "Hide Host Cursor", x+16, y+132, 154, settingsActionVisual{Enabled: true, Active: a.hideCursor})
	drawSettingsSectionLabel(screen, "Wheel", x+16, y+180)
	drawWrappedText(screen, "Throttle local wheel bursts before sending them to the device.", x+16, y+198, leftW-32, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	a.drawSettingsAction(screen, "scroll_0", "Off", x+16, y+248, 64, settingsActionVisual{Enabled: true, Active: a.scrollThrottle == 0})
	a.drawSettingsAction(screen, "scroll_10", "Low", x+92, y+248, 64, settingsActionVisual{Enabled: true, Active: a.scrollThrottle == 10*time.Millisecond})
	a.drawSettingsAction(screen, "scroll_25", "Medium", x+168, y+248, 84, settingsActionVisual{Enabled: true, Active: a.scrollThrottle == 25*time.Millisecond})
	a.drawSettingsAction(screen, "scroll_50", "High", x+264, y+248, 72, settingsActionVisual{Enabled: true, Active: a.scrollThrottle == 50*time.Millisecond})
	a.drawSettingsAction(screen, "scroll_100", "Very High", x+348, y+248, 108, settingsActionVisual{Enabled: true, Active: a.scrollThrottle == 100*time.Millisecond})
	a.drawSettingsAction(screen, "scroll_invert", "Invert Scroll", x+16, y+286, 128, settingsActionVisual{Enabled: true, Active: a.invertScroll})
	a.drawSettingsCard(screen, rightX, y, rightW, cardH, "Jiggler", "")
	if state.Loading {
		drawText(screen, "Loading jiggler state…", rightX+16, y+48, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	} else {
		drawSettingsKeyValue(screen, "State", boolPtrWord(state.JigglerEnabled), rightX+16, y+48, 70)
		drawSettingsKeyValue(screen, "Preset", jigglerPresetLabel(state.JigglerEnabled, state.JigglerConfig), rightX+16, y+74, 70)
		drawWrappedText(screen, "Use native presets or open a compact custom editor for the device jiggler schedule.", rightX+16, y+106, rightW-32, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
		jiggler := a.settingsAction(settingsGroupJiggler)
		a.drawSettingsAction(screen, "jiggler_disabled", "Disabled", rightX+16, y+156, 88, settingsActionVisual{Enabled: state.JigglerEnabled != nil && (!jiggler.Pending || jiggler.PendingChoice == "disabled"), Active: state.JigglerEnabled != nil && !*state.JigglerEnabled, Pending: jiggler.Pending && jiggler.PendingChoice == "disabled"})
		a.drawSettingsAction(screen, "jiggler_frequent", "Frequent", rightX+116, y+156, 88, settingsActionVisual{Enabled: !jiggler.Pending || jiggler.PendingChoice == "frequent", Active: jigglerPresetLabel(state.JigglerEnabled, state.JigglerConfig) == "Frequent", Pending: jiggler.Pending && jiggler.PendingChoice == "frequent"})
		a.drawSettingsAction(screen, "jiggler_standard", "Standard", rightX+216, y+156, 88, settingsActionVisual{Enabled: !jiggler.Pending || jiggler.PendingChoice == "standard", Active: jigglerPresetLabel(state.JigglerEnabled, state.JigglerConfig) == "Standard", Pending: jiggler.Pending && jiggler.PendingChoice == "standard"})
		a.drawSettingsAction(screen, "jiggler_light", "Light", rightX+16, y+194, 72, settingsActionVisual{Enabled: !jiggler.Pending || jiggler.PendingChoice == "light", Active: jigglerPresetLabel(state.JigglerEnabled, state.JigglerConfig) == "Light", Pending: jiggler.Pending && jiggler.PendingChoice == "light"})
		a.drawSettingsAction(screen, "jiggler_custom", "Custom", rightX+100, y+194, 84, settingsActionVisual{Enabled: !jiggler.Pending, Active: a.jigglerEditorOpen || jigglerPresetLabel(state.JigglerEnabled, state.JigglerConfig) == "Custom"})
		if a.jigglerEditorOpen {
			drawSettingsSectionLabel(screen, "Custom config", rightX+16, y+236)
			drawSettingsKeyValue(screen, "Idle (s)", fmt.Sprintf("%d", a.jigglerEditorConfig.InactivityLimitSeconds), rightX+16, y+258, 70)
			a.drawSettingsAction(screen, "jiggler_custom_inactivity_minus", "-", rightX+148, y+246, 30, settingsActionVisual{Enabled: true})
			a.drawSettingsAction(screen, "jiggler_custom_inactivity_plus", "+", rightX+186, y+246, 30, settingsActionVisual{Enabled: true})
			drawSettingsKeyValue(screen, "Jitter", fmt.Sprintf("%d%%", a.jigglerEditorConfig.JitterPercentage), rightX+232, y+258, 56)
			a.drawSettingsAction(screen, "jiggler_custom_jitter_minus", "-", rightX+320, y+246, 30, settingsActionVisual{Enabled: true})
			a.drawSettingsAction(screen, "jiggler_custom_jitter_plus", "+", rightX+358, y+246, 30, settingsActionVisual{Enabled: true})
			drawSettingsSectionLabel(screen, "Cron", rightX+16, y+298)
			a.drawSettingsInput(screen, "jiggler_focus_cron", rightX+16, y+316, rightW-32, 34, a.jigglerEditorConfig.ScheduleCronTab, "0 * * * * *", a.settingsInputFocus == settingsInputJigglerCron)
			drawSettingsSectionLabel(screen, "Timezone", rightX+16, y+366)
			a.drawSettingsInput(screen, "jiggler_focus_timezone", rightX+16, y+384, rightW-32, 34, a.jigglerEditorConfig.Timezone, "UTC", a.settingsInputFocus == settingsInputJigglerTimezone)
			a.drawSettingsAction(screen, "jiggler_custom_save", "Save Custom", rightX+16, y+434, 116, settingsActionVisual{Enabled: !jiggler.Pending})
			a.drawSettingsAction(screen, "jiggler_custom_cancel", "Cancel", rightX+144, y+434, 86, settingsActionVisual{Enabled: !jiggler.Pending})
			a.drawSettingsActionStatus(screen, settingsGroupJiggler, rightX+16, y+474, rightW-32)
			if a.jigglerEditorError != "" {
				drawWrappedText(screen, a.jigglerEditorError, rightX+16, y+496, rightW-32, 12, color.RGBA{R: 220, G: 132, B: 132, A: 255})
			}
		} else {
			a.drawSettingsActionStatus(screen, settingsGroupJiggler, rightX+16, y+232, rightW-32)
		}
	}
	if state.Error != "" {
		errY := y + 232.0
		if a.jigglerEditorOpen {
			errY = y + 520
		}
		drawWrappedText(screen, state.Error, rightX+16, errY, rightW-32, 12, color.RGBA{R: 220, G: 132, B: 132, A: 255})
	}
}

func (a *App) drawSettingsKeyboard(screen *ebiten.Image, snap session.Snapshot, x, y, w float64) {
	cardDesc := "This layout affects paste and keyboard macros. Live typing is sent as physical HID keys."
	cardH := a.settingsKeyboardBodyHeight(w)
	a.drawSettingsCard(screen, x, y, w, cardH, "", cardDesc)
	descH := wrappedTextHeight(cardDesc, w-32, 12)
	bodyY := y + 18 + descH + 22
	drawText(screen, "Active layout", x+16, bodyY, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	layout := snap.KeyboardLayout
	if layout == "" {
		layout = "en-US"
	}
	layoutState := a.settingsAction(settingsGroupKeyboardLayout)
	drawText(screen, keyboardLayoutLabel(layout), x+118, bodyY, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	a.drawSettingsAction(screen, "toggle_pressed_keys", "Show Pressed Keys", x+w-174, bodyY-14, 158, settingsActionVisual{Enabled: true, Active: a.showPressedKeys})
	drawText(screen, "Layout presets", x+16, bodyY+40, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	options := input.SupportedKeyboardLayouts()
	rowX := x + 16
	rowY := bodyY + 64
	for i, option := range options {
		btnW := 94.0
		if len(option.Label) > 7 {
			btnW = 112
		}
		a.drawSettingsAction(screen, "layout:"+option.Code, option.Label, rowX, rowY, btnW, settingsActionVisual{
			Enabled: snap.Phase == session.PhaseConnected && (!layoutState.Pending || layoutState.PendingChoice == option.Code),
			Active:  layout == option.Code,
			Pending: layoutState.Pending && layoutState.PendingChoice == option.Code,
		})
		rowX += btnW + 10
		if (i+1)%4 == 0 {
			rowX = x + 16
			rowY += 38
		}
	}
	statusY := rowY + 4
	a.drawSettingsActionStatus(screen, settingsGroupKeyboardLayout, x+16, statusY, w-32)
	noteY := statusY
	if layoutState.Pending || layoutState.Error != "" {
		noteY += wrappedTextHeight(fallbackLabel(layoutState.Error, "Applying…"), w-32, 12) + 8
	}
	drawWrappedText(screen, "Make this match the remote OS only for pasted text and macros.", x+16, noteY, w-32, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}

func (a *App) settingsKeyboardBodyHeight(w float64) float64 {
	cardDesc := "This layout affects paste and keyboard macros. Live typing is sent as physical HID keys."
	descH := wrappedTextHeight(cardDesc, w-32, 12)
	noteH := wrappedTextHeight("Make this match the remote OS only for pasted text and macros.", w-32, 13)
	rows := (len(input.SupportedKeyboardLayouts()) + 3) / 4
	return 18 + descH + 22 + 18 + 40 + float64(rows)*38 + 18 + 16 + noteH + 16
}

func (a *App) drawSettingsVideo(screen *ebiten.Image, snap session.Snapshot, x, y, w float64) {
	leftW := (w - 14) * 0.48
	rightX := x + leftW + 14
	rightW := w - leftW - 14
	qualityState := a.settingsAction(settingsGroupVideoQuality)
	cardH := a.settingsWideBodyHeight(sectionVideo, w)
	a.drawSettingsCard(screen, x, y, leftW, cardH, "Stream", "")
	drawSettingsSectionLabel(screen, "Quality preset", x+16, y+48)
	a.drawSettingsAction(screen, "quality_preset_high", "High", x+16, y+68, 96, settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected && (!qualityState.Pending || qualityState.PendingChoice == "high"), Active: snap.Quality >= 0.95, Pending: qualityState.Pending && qualityState.PendingChoice == "high"})
	a.drawSettingsAction(screen, "quality_preset_medium", "Medium", x+124, y+68, 96, settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected && (!qualityState.Pending || qualityState.PendingChoice == "medium"), Active: snap.Quality >= 0.45 && snap.Quality < 0.95, Pending: qualityState.Pending && qualityState.PendingChoice == "medium"})
	a.drawSettingsAction(screen, "quality_preset_low", "Low", x+232, y+68, 96, settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected && (!qualityState.Pending || qualityState.PendingChoice == "low"), Active: snap.Quality < 0.45, Pending: qualityState.Pending && qualityState.PendingChoice == "low"})
	drawText(screen, fmt.Sprintf("Current factor %.2f", snap.Quality), x+16, y+120, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	a.drawSettingsActionStatus(screen, settingsGroupVideoQuality, x+16, y+144, leftW-32)
	a.drawSettingsCard(screen, rightX, y, rightW, cardH, "EDID", "")
	edid := snap.EDID
	if edid == "" {
		edid = "Unavailable on current target"
	}
	drawWrappedText(screen, edid, rightX+16, y+48, rightW-32, 12, color.RGBA{R: 236, G: 241, B: 245, A: 255})
}

func (a *App) drawSettingsHardware(screen *ebiten.Image, x, y, w float64) {
	a.mu.RLock()
	state := a.sectionData.Hardware
	a.mu.RUnlock()
	leftW := (w - 14) * 0.48
	rightX := x + leftW + 14
	rightW := w - leftW - 14
	cardH := a.settingsWideBodyHeight(sectionHardware, w)
	a.drawSettingsCard(screen, x, y, leftW, cardH, "Display", "")
	a.drawSettingsCard(screen, rightX, y, rightW, cardH, "USB", "")
	if state.Loading {
		drawText(screen, "Loading hardware state…", x+16, y+48, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		return
	}
	drawSettingsKeyValue(screen, "Rotation", string(state.State.DisplayRotation), x+16, y+50, 86)
	drawWrappedText(screen, "Rotate the JetKVM device display. This does not rotate the remote host video feed.", x+16, y+82, leftW-32, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	rotateState := a.settingsAction(settingsGroupDisplayRotate)
	a.drawSettingsAction(screen, "rotate_normal", "Normal", x+16, y+150, 88, settingsActionVisual{Enabled: state.State.DisplayRotation != session.DisplayRotationUnknown && (!rotateState.Pending || rotateState.PendingChoice == "270"), Active: state.State.DisplayRotation == session.DisplayRotationNormal, Pending: rotateState.Pending && rotateState.PendingChoice == "270"})
	a.drawSettingsAction(screen, "rotate_inverted", "Inverted", x+116, y+150, 98, settingsActionVisual{Enabled: state.State.DisplayRotation != session.DisplayRotationUnknown && (!rotateState.Pending || rotateState.PendingChoice == "90"), Active: state.State.DisplayRotation == session.DisplayRotationInverted, Pending: rotateState.Pending && rotateState.PendingChoice == "90"})
	a.drawSettingsActionStatus(screen, settingsGroupDisplayRotate, x+16, y+188, leftW-32)
	drawSettingsKeyValue(screen, "USB Emulation", boolPtrWord(state.State.USBEmulation), rightX+16, y+50, 112)
	drawSettingsKeyValue(screen, "USB Config", usbConfigLabel(state.State.USBConfig), rightX+16, y+76, 112)
	drawSettingsSectionLabel(screen, "Configured devices", rightX+16, y+108)
	drawWrappedText(screen, usbDevicesSummary(state.State.USBDevices), rightX+16, y+126, rightW-32, 12, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	if state.State.USBEmulation != nil {
		usbState := a.settingsAction(settingsGroupUSBEmulation)
		a.drawSettingsAction(screen, "usb_emulation_on", "USB On", rightX+16, y+150, 86, settingsActionVisual{Enabled: !usbState.Pending || usbState.PendingChoice == "on", Active: *state.State.USBEmulation, Pending: usbState.Pending && usbState.PendingChoice == "on"})
		a.drawSettingsAction(screen, "usb_emulation_off", "USB Off", rightX+114, y+150, 92, settingsActionVisual{Enabled: !usbState.Pending || usbState.PendingChoice == "off", Active: !*state.State.USBEmulation, Pending: usbState.Pending && usbState.PendingChoice == "off"})
		a.drawSettingsActionStatus(screen, settingsGroupUSBEmulation, rightX+16, y+188, rightW-32)
	}
	usbDevicesState := a.settingsAction(settingsGroupUSBDevices)
	drawSettingsSectionLabel(screen, "Preset", rightX+16, y+226)
	preset := usbDevicePresetLabel(state.State.USBDevices)
	a.drawSettingsAction(screen, "usb_devices_default", "Default", rightX+16, y+244, 86, settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "default", Active: preset == "Default", Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "default"})
	a.drawSettingsAction(screen, "usb_devices_keyboard_only", "Keyboard Only", rightX+114, y+244, 122, settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "keyboard_only", Active: preset == "Keyboard Only", Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "keyboard_only"})
	drawText(screen, "Custom", rightX+248, y+254, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	deviceToggles := []struct {
		id, label string
		active    bool
		x, y, w   float64
	}{
		{id: "usb_toggle_keyboard", label: "Keyboard", active: state.State.USBDevices.Keyboard, x: rightX + 16, y: y + 286, w: 94},
		{id: "usb_toggle_absolute_mouse", label: "Abs Mouse", active: state.State.USBDevices.AbsoluteMouse, x: rightX + 122, y: y + 286, w: 98},
		{id: "usb_toggle_relative_mouse", label: "Rel Mouse", active: state.State.USBDevices.RelativeMouse, x: rightX + 232, y: y + 286, w: 96},
		{id: "usb_toggle_mass_storage", label: "Mass Storage", active: state.State.USBDevices.MassStorage, x: rightX + 16, y: y + 324, w: 110},
		{id: "usb_toggle_serial_console", label: "Serial", active: state.State.USBDevices.SerialConsole, x: rightX + 138, y: y + 324, w: 74},
		{id: "usb_toggle_network", label: "Network", active: state.State.USBDevices.Network, x: rightX + 224, y: y + 324, w: 88},
	}
	for _, toggle := range deviceToggles {
		a.drawSettingsAction(screen, toggle.id, toggle.label, toggle.x, toggle.y, toggle.w, settingsActionVisual{
			Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom",
			Active:  toggle.active,
			Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom",
		})
	}
	a.drawSettingsActionStatus(screen, settingsGroupUSBDevices, rightX+16, y+366, rightW-32)
	if state.Error != "" {
		drawWrappedText(screen, state.Error, x+16, y+392, w-32, 12, color.RGBA{R: 220, G: 132, B: 132, A: 255})
	}
}

func (a *App) drawSettingsAccess(screen *ebiten.Image, x, y, w float64) {
	a.mu.RLock()
	state := a.sectionData.Access
	a.mu.RUnlock()
	leftW := (w - 14) * 0.5
	rightX := x + leftW + 14
	rightW := w - leftW - 14
	cardH := a.settingsWideBodyHeight(sectionAccess, w)
	a.drawSettingsCard(screen, x, y, leftW, cardH, "Cloud", "")
	a.drawSettingsCard(screen, rightX, y, rightW, cardH, "TLS", "")
	if state.Loading {
		drawText(screen, "Loading access state…", x+16, y+48, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		return
	}
	drawSettingsKeyValue(screen, "Connected", boolWord(state.State.Cloud.Connected), x+16, y+50, 96)
	drawSettingsSectionLabel(screen, "Cloud API", x+16, y+84)
	drawWrappedText(screen, fallbackLabel(state.State.Cloud.URL, "Unavailable"), x+16, y+102, leftW-32, 12, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawSettingsSectionLabel(screen, "Cloud App", x+16, y+138)
	drawWrappedText(screen, fallbackLabel(state.State.Cloud.AppURL, "Unavailable"), x+16, y+156, leftW-32, 12, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawSettingsKeyValue(screen, "Mode", string(state.State.TLS), rightX+16, y+50, 70)
	drawWrappedText(screen, "Use the target's currently exposed TLS mode. Native client transport follows whatever the device publishes.", rightX+16, y+84, rightW-32, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	tlsState := a.settingsAction(settingsGroupTLSMode)
	a.drawSettingsAction(screen, "tls_disabled", "Disabled", rightX+16, y+150, 92, settingsActionVisual{Enabled: state.State.TLS != session.TLSModeUnknown && (!tlsState.Pending || tlsState.PendingChoice == "disabled"), Active: state.State.TLS == session.TLSModeDisabled, Pending: tlsState.Pending && tlsState.PendingChoice == "disabled"})
	a.drawSettingsAction(screen, "tls_self_signed", "Self-Signed", rightX+120, y+150, 114, settingsActionVisual{Enabled: state.State.TLS != session.TLSModeUnknown && (!tlsState.Pending || tlsState.PendingChoice == "self-signed"), Active: state.State.TLS == session.TLSModeSelfSigned, Pending: tlsState.Pending && tlsState.PendingChoice == "self-signed"})
	a.drawSettingsActionStatus(screen, settingsGroupTLSMode, rightX+16, y+188, rightW-32)
	if state.Error != "" {
		drawWrappedText(screen, state.Error, x+16, y+192, w-32, 12, color.RGBA{R: 220, G: 132, B: 132, A: 255})
	}
}

func (a *App) drawSettingsNetwork(screen *ebiten.Image, x, y, w float64) {
	a.mu.RLock()
	state := a.sectionData.Network
	a.mu.RUnlock()
	cardH := a.settingsWideBodyHeight(sectionNetwork, w)
	a.drawSettingsCard(screen, x, y, w, cardH, "Current state", "")
	if state.Loading {
		drawText(screen, "Loading network state…", x+16, y+48, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		return
	}
	drawSettingsKeyValue(screen, "Hostname", state.State.Hostname, x+16, y+48, 96)
	drawSettingsKeyValue(screen, "IP", state.State.IP, x+16, y+74, 96)
	drawSettingsKeyValue(screen, "DHCP", boolPtrWord(state.State.DHCP), x+16, y+100, 96)
	if state.Error != "" {
		drawWrappedText(screen, state.Error, x+16, y+124, w-32, 12, color.RGBA{R: 220, G: 132, B: 132, A: 255})
	}
}

func (a *App) drawSettingsAdvanced(screen *ebiten.Image, x, y, w float64) {
	a.mu.RLock()
	state := a.sectionData.Advanced
	a.mu.RUnlock()
	cardH := a.settingsWideBodyHeight(sectionAdvanced, w)
	a.drawSettingsCard(screen, x, y, w, cardH, "Current state", "")
	if state.Loading {
		drawText(screen, "Loading advanced state…", x+16, y+48, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		return
	}
	drawSettingsKeyValue(screen, "Developer Mode", boolPtrWord(state.State.DevMode), x+16, y+48, 128)
	drawSettingsKeyValue(screen, "USB Emulation", boolPtrWord(state.State.USBEmulation), x+16, y+74, 128)
	drawSettingsKeyValue(screen, "App Version", state.State.Version.AppVersion, x+16, y+106, 128)
	drawSettingsKeyValue(screen, "System Version", state.State.Version.SystemVersion, x+16, y+132, 128)
	if state.State.DevMode != nil {
		devModeState := a.settingsAction(settingsGroupDeveloperMode)
		a.drawSettingsAction(screen, "developer_mode_on", "Developer Mode On", x+16, y+166, 156, settingsActionVisual{Enabled: !devModeState.Pending || devModeState.PendingChoice == "on", Active: *state.State.DevMode, Pending: devModeState.Pending && devModeState.PendingChoice == "on"})
		a.drawSettingsAction(screen, "developer_mode_off", "Developer Mode Off", x+184, y+166, 160, settingsActionVisual{Enabled: !devModeState.Pending || devModeState.PendingChoice == "off", Active: !*state.State.DevMode, Pending: devModeState.Pending && devModeState.PendingChoice == "off"})
		a.drawSettingsActionStatus(screen, settingsGroupDeveloperMode, x+16, y+204, w-32)
	}
	if state.Error != "" {
		drawWrappedText(screen, state.Error, x+16, y+204, w-32, 12, color.RGBA{R: 220, G: 132, B: 132, A: 255})
	}
}

func (a *App) drawSettingsAppearance(screen *ebiten.Image, x, y, w float64) {
	cardH := a.settingsWideBodyHeight(sectionAppearance, w)
	a.drawSettingsCard(screen, x, y, w, cardH, "Chrome", "")
	drawSettingsSectionLabel(screen, "Top bar", x+16, y+48)
	a.drawSettingsAction(screen, "pin_chrome_off", "Auto-hide", x+136, y+36, 96, settingsActionVisual{Enabled: true, Active: !a.prefs.PinChrome})
	a.drawSettingsAction(screen, "pin_chrome_on", "Pinned", x+244, y+36, 84, settingsActionVisual{Enabled: true, Active: a.prefs.PinChrome})
	drawSettingsSectionLabel(screen, "Position", x+16, y+90)
	positionOptions := []struct {
		id, label, value string
		x, y, w          float64
	}{
		{id: "chrome_anchor:top_left", label: "Top Left", value: "top_left", x: x + 136, y: y + 78, w: 96},
		{id: "chrome_anchor:top_center", label: "Top Center", value: "top_center", x: x + 244, y: y + 78, w: 108},
		{id: "chrome_anchor:top_right", label: "Top Right", value: "top_right", x: x + 364, y: y + 78, w: 100},
		{id: "chrome_anchor:left_center", label: "Left Center", value: "left_center", x: x + 136, y: y + 116, w: 108},
		{id: "chrome_anchor:right_center", label: "Right Center", value: "right_center", x: x + 256, y: y + 116, w: 118},
		{id: "chrome_anchor:bottom_left", label: "Bottom Left", value: "bottom_left", x: x + 386, y: y + 116, w: 108},
		{id: "chrome_anchor:bottom_center", label: "Bottom Center", value: "bottom_center", x: x + 136, y: y + 154, w: 126},
		{id: "chrome_anchor:bottom_right", label: "Bottom Right", value: "bottom_right", x: x + 274, y: y + 154, w: 118},
	}
	for _, option := range positionOptions {
		a.drawSettingsAction(screen, option.id, option.label, option.x, option.y, option.w, settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor.String() == option.value})
	}
	drawSettingsSectionLabel(screen, "Layout", x+16, y+206)
	a.drawSettingsAction(screen, "chrome_layout:horizontal", "Horizontal", x+136, y+194, 112, settingsActionVisual{Enabled: true, Active: a.prefs.ChromeLayout == chromeLayoutHorizontal})
	a.drawSettingsAction(screen, "chrome_layout:vertical", "Vertical", x+260, y+194, 96, settingsActionVisual{Enabled: true, Active: a.prefs.ChromeLayout == chromeLayoutVertical})
	drawSettingsSectionLabel(screen, "Window", x+16, y+248)
	a.drawSettingsAction(screen, "fullscreen", "Toggle Fullscreen", x+136, y+236, 160, settingsActionVisual{Enabled: true, Active: ebiten.IsFullscreen()})
	drawWrappedText(screen, "Position chooses where the chrome sits on screen. Layout changes whether the control buttons run across or down.", x+16, y+280, w-32, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}

func boolWord(v bool) string {
	if v {
		return "Enabled"
	}
	return "Disabled"
}

func boolPtrWord(v *bool) string {
	if v == nil {
		return "Unavailable"
	}
	return boolWord(*v)
}

func usbConfigLabel(cfg session.USBConfig) string {
	if cfg == (session.USBConfig{}) {
		return ""
	}
	product := fallbackLabel(cfg.Product, cfg.ProductID)
	vendor := fallbackLabel(cfg.Manufacturer, cfg.VendorID)
	return fmt.Sprintf("%s / %s", vendor, product)
}

func jigglerPresetLabel(enabled *bool, cfg *session.JigglerConfig) string {
	if enabled != nil && !*enabled {
		return "Disabled"
	}
	if cfg == nil {
		return "Unavailable"
	}
	switch {
	case cfg.InactivityLimitSeconds == 30 && cfg.JitterPercentage == 25 && cfg.ScheduleCronTab == "*/30 * * * * *":
		return "Frequent"
	case cfg.InactivityLimitSeconds == 60 && cfg.JitterPercentage == 25 && cfg.ScheduleCronTab == "0 * * * * *":
		return "Standard"
	case cfg.InactivityLimitSeconds == 300 && cfg.JitterPercentage == 25 && cfg.ScheduleCronTab == "0 */5 * * * *":
		return "Light"
	default:
		return "Custom"
	}
}

func usbDevicesSummary(devices session.USBDevices) string {
	return fmt.Sprintf("%d configured classes", usbDeviceCount(devices))
}

func usbDeviceCount(devices session.USBDevices) int {
	count := 0
	if devices.Keyboard {
		count++
	}
	if devices.AbsoluteMouse {
		count++
	}
	if devices.RelativeMouse {
		count++
	}
	if devices.MassStorage {
		count++
	}
	if devices.SerialConsole {
		count++
	}
	if devices.Network {
		count++
	}
	return count
}

func usbDevicePresetLabel(devices session.USBDevices) string {
	switch devices {
	case defaultUSBDevices():
		return "Default"
	case keyboardOnlyUSBDevices():
		return "Keyboard Only"
	default:
		return "Custom"
	}
}

func keyboardLayoutLabel(code string) string {
	for _, layout := range input.SupportedKeyboardLayouts() {
		if layout.Code == code {
			return layout.Label
		}
	}
	return fallbackLabel(code, "en-US")
}

func (a *App) drawSettingsPlanned(screen *ebiten.Image, section settingsSectionDef, x, y, w float64) {
	cardH := a.settingsPlannedBodyHeight(section.id, w)
	a.drawSettingsCard(screen, x, y, w, cardH, "Not exposed here", "")
	drawWrappedText(screen, section.description, x+16, y+46, w-32, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, "Current upstream surface", x+16, y+86, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	lineY := y + 110
	for _, item := range section.items {
		drawWrappedText(screen, "• "+item, x+24, lineY, w-40, 12, color.RGBA{R: 236, G: 241, B: 245, A: 255})
		lineY += 22
	}
	drawWrappedText(screen, "This section exists in the upstream product structure but is not currently exposed by this target or the desktop client.", x+16, y+176, w-32, 12, color.RGBA{R: 166, G: 178, B: 190, A: 255})
}
