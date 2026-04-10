package app

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
	"github.com/lkarlslund/jetkvm-desktop/pkg/ui"
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

func uiIcon(kind iconKind) ui.IconKind {
	switch kind {
	case iconReconnect:
		return ui.IconReconnect
	case iconMouse:
		return ui.IconMouse
	case iconPaste:
		return ui.IconPaste
	case iconMedia:
		return ui.IconMedia
	case iconStats:
		return ui.IconStats
	case iconMinus:
		return ui.IconMinus
	case iconPlus:
		return ui.IconPlus
	case iconPower:
		return ui.IconPower
	case iconSettings:
		return ui.IconSettings
	case iconFullscreen:
		return ui.IconFullscreen
	default:
		return ui.IconClose
	}
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

func (a *App) drawTopBar(screen *ebiten.Image, snap session.Snapshot) {
	alpha := a.uiAlpha()
	if alpha <= 0 {
		return
	}
	buttons := a.layoutChromeButtons(screen.Bounds().Dx(), screen.Bounds().Dy(), snap)
	a.chromeButtons = buttons
	a.drawUIRoot(screen, func(chromeButton) {}, chromeButtonsElement{
		buttons: buttons,
		alpha:   alpha,
	})
}

func (a *App) drawHint(screen *ebiten.Image) {
	alpha := a.uiAlpha()
	if alpha <= 0 {
		return
	}
	x, y := ebiten.CursorPosition()
	for _, btn := range a.chromeButtons {
		if btn.rect.contains(x, y) {
			w, _ := ui.MeasureText(btn.hint, 13)
			bx := btn.rect.x + (btn.rect.w-w)/2 - 10
			if bx < 12 {
				bx = 12
			}
			bw := w + 20
			by := btn.rect.y + btn.rect.h + 8
			a.drawUIRoot(screen, func(chromeButton) {}, chromeTooltipElement{
				text:  btn.hint,
				alpha: alpha,
				x:     bx,
				y:     by,
				w:     bw,
			})
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
	right := ""
	rightColor := color.Color(nil)
	if snap.LastError != "" && snap.Phase != session.PhaseConnected {
		right = trimForFooter(snap.LastError)
		rightColor = rgba(228, 142, 142, 255, max(alpha, 0.75))
	}
	a.drawUIRoot(screen, func(chromeButton) {}, footerStatusElement{
		left:       left,
		right:      right,
		leftColor:  clr,
		rightColor: rightColor,
		y:          y,
	})
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
	sections := settingsSections(snap)
	section := a.currentSection(sections)
	sidebarW := 156.0
	contentW := min(720, float64(bounds.Dx())-sidebarW-84)
	panelW := sidebarW + contentW + 44
	panelW = min(panelW, float64(bounds.Dx()-48))
	panelH := min(max(a.settingsPanelHeight(section, contentW), settingsSidebarHeight(len(sections))), float64(bounds.Dy()-56))

	panelX := (float64(bounds.Dx()) - panelW) / 2
	panelY := (float64(bounds.Dy()) - panelH) / 2
	outerRect := ui.Rect{X: panelX, Y: panelY, W: panelW, H: panelH}
	innerRect := outerRect.Inset(ui.UniformInsets(1))

	a.settingsPanel = rect{x: panelX, y: panelY, w: panelW, h: panelH}
	a.settingsButtons = a.settingsButtons[:0]
	a.drawUIRoot(screen, func(btn chromeButton) {
		a.settingsButtons = append(a.settingsButtons, btn)
	}, settingsOverlayRootElement{
		outerRect: outerRect,
		child: settingsOverlayElement{
			app:      a,
			snap:     snap,
			sections: sections,
			section:  section,
			sidebarW: sidebarW,
			panelH:   innerRect.H,
		},
	})
}

type settingsSidebarElement struct {
	app      *App
	snap     session.Snapshot
	sections []settingsSectionDef
	panelH   float64
}

type settingsOverlayElement struct {
	app      *App
	snap     session.Snapshot
	sections []settingsSectionDef
	section  settingsSectionDef
	sidebarW float64
	panelH   float64
}

type settingsOverlayRootElement struct {
	outerRect ui.Rect
	child     ui.Element
}

func (settingsOverlayRootElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e settingsOverlayRootElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Stack{Children: []ui.Element{
		ui.Backdrop{Color: color.RGBA{A: 170}},
		ui.Positioned{
			X: e.outerRect.X,
			Y: e.outerRect.Y,
			W: e.outerRect.W,
			H: e.outerRect.H,
			Child: ui.Panel{
				Fill:   color.RGBA{R: 13, G: 20, B: 30, A: 246},
				Stroke: color.RGBA{R: 88, G: 102, B: 118, A: 180},
				Child:  e.child,
			},
		},
	}}.Draw(ctx, bounds)
}

type chromeButtonsElement struct {
	buttons []chromeButton
	alpha   float64
}

func (chromeButtonsElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e chromeButtonsElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	children := make([]ui.Element, 0, len(e.buttons))
	for _, btn := range e.buttons {
		btn := btn
		children = append(children, ui.Positioned{
			X: btn.rect.x,
			Y: btn.rect.y,
			W: btn.rect.w,
			H: btn.rect.h,
			Child: ui.IconButton{
				Kind:    uiIcon(btn.icon),
				Active:  btn.active,
				Enabled: btn.enabled,
				Alpha:   e.alpha,
			},
		})
	}
	ui.Stack{Children: children}.Draw(ctx, bounds)
}

type chromeTooltipElement struct {
	text  string
	alpha float64
	x     float64
	y     float64
	w     float64
}

func (chromeTooltipElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e chromeTooltipElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Positioned{
		X:     e.x,
		Y:     e.y,
		W:     e.w,
		H:     28,
		Child: ui.Tooltip{Text: e.text, Alpha: e.alpha},
	}.Draw(ctx, bounds)
}

type footerStatusElement struct {
	left       string
	right      string
	leftColor  color.Color
	rightColor color.Color
	y          float64
}

func (footerStatusElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e footerStatusElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Positioned{
		X: 0,
		Y: e.y,
		W: bounds.W,
		H: 18,
		Child: ui.FooterStatus{
			Left:       e.left,
			Right:      e.right,
			Size:       12,
			LeftColor:  e.leftColor,
			RightColor: e.rightColor,
			Insets:     ui.Insets{Right: 14, Left: 14},
		},
	}.Draw(ctx, bounds)
}

func (e settingsOverlayElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e settingsOverlayElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Stack{Children: []ui.Element{
		ui.Row{
			Children: []ui.Child{
				ui.Fixed(ui.Constrained{
					MinW: e.sidebarW,
					MaxW: e.sidebarW,
					Child: ui.Panel{
						Fill:   color.RGBA{R: 18, G: 28, B: 40, A: 255},
						Insets: ui.Insets{Top: 16, Right: 10, Bottom: 18, Left: 10},
						Child: settingsSidebarElement{
							app:      e.app,
							snap:     e.snap,
							sections: e.sections,
							panelH:   bounds.H,
						},
					},
				}),
				ui.Fixed(ui.Spacer{W: 18}),
				ui.Flex(ui.Inset{
					Insets: ui.Insets{Top: 18, Right: 14, Bottom: 18},
					Child: ui.Column{
						Children: []ui.Child{
							ui.Fixed(settingsHeaderElement(e.section.label, e.section.description)),
							ui.Fixed(ui.Spacer{H: 18}),
							ui.Fixed(settingsSectionBodyHost{
								app:     e.app,
								section: e.section.id,
								snap:    e.snap,
							}),
						},
					},
				}, 1),
			},
			Spacing: 0,
		},
		ui.Inset{
			Insets: ui.Insets{Top: 11, Right: 13},
			Child: ui.Align{
				Horizontal: ui.AlignEnd,
				Vertical:   ui.AlignStart,
				Child:      ui.Button{ID: "settings_close", Label: "X", Enabled: true},
			},
		},
	}}.Draw(ctx, bounds)
}

type settingsSectionBodyHost struct {
	app     *App
	section settingsSection
	snap    session.Snapshot
}

func (e settingsSectionBodyHost) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	body := e.app.settingsSectionBody(e.section, e.snap)
	if body == nil {
		return constraints.Clamp(ui.Size{})
	}
	return body.Measure(ctx, constraints)
}

func (e settingsSectionBodyHost) Draw(ctx *ui.Context, bounds ui.Rect) {
	body := e.app.settingsSectionBody(e.section, e.snap)
	if body == nil {
		return
	}
	body.Draw(ctx, bounds)
}

func (e settingsSidebarElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e settingsSidebarElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	sideBtnH, sideGap, _ := settingsSidebarMetrics(e.panelH, len(e.sections))
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: "Settings", Size: 20, Color: color.RGBA{R: 240, G: 244, B: 248, A: 255}}),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{
			Text:  fallbackLabel(e.snap.DeviceID, e.snap.Hostname, e.snap.BaseURL),
			Size:  11,
			Color: color.RGBA{R: 166, G: 178, B: 190, A: 255},
		}),
		ui.Fixed(ui.Spacer{H: 18}),
	}
	for i, section := range e.sections {
		if i > 0 {
			children = append(children, ui.Fixed(ui.Spacer{H: sideGap}))
		}
		children = append(children, ui.Fixed(settingsSidebarButtonElement{
			app:     e.app,
			section: section,
			height:  sideBtnH,
		}))
	}
	ui.Column{Children: children}.Draw(ctx, bounds)
}

type settingsSidebarButtonElement struct {
	app     *App
	section settingsSectionDef
	height  float64
}

func (e settingsSidebarButtonElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: e.height})
}

func (e settingsSidebarButtonElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Button{
		ID:      "section:" + e.section.id.String(),
		Label:   e.section.label,
		Enabled: true,
		Active:  e.app.settingsSection == e.section.id,
	}.Draw(ctx, bounds)
}

func settingsHeaderElement(title, description string) ui.Element {
	return ui.Column{
		Children: []ui.Child{
			ui.Fixed(ui.Label{Text: title, Size: 22, Color: color.RGBA{R: 240, G: 244, B: 248, A: 255}}),
			ui.Fixed(ui.Spacer{H: 6}),
			ui.Fixed(ui.Constrained{
				MaxW:  640,
				Child: ui.Paragraph{Text: description, Size: 12, Color: color.RGBA{R: 166, G: 178, B: 190, A: 255}},
			}),
		},
	}
}

func (a *App) settingsPanelHeight(section settingsSectionDef, contentW float64) float64 {
	headerH := 18 + 28 + ui.WrappedTextHeight(section.description, contentW-48, 12) + 18 + 18
	bodyH := a.settingsSectionBodyHeight(section.id, contentW)
	return headerH + bodyH + 18
}

func (a *App) settingsSectionBodyHeight(section settingsSection, w float64) float64 {
	return a.measureSettingsBody(a.settingsSectionBody(section, a.ctrl.Snapshot()), w)
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

func (a *App) measureSettingsBody(body ui.Element, width float64) float64 {
	if body == nil {
		return 0
	}
	ctx := a.newUIContext(nil, func(chromeButton) {})
	size := body.Measure(ctx, ui.Constraints{MaxW: width})
	return size.H
}

func settingsCardElement(title string, body ui.Element) ui.Element {
	children := make([]ui.Child, 0, 3)
	if title != "" {
		children = append(children,
			ui.Fixed(ui.Label{Text: title, Size: 15, Color: color.RGBA{R: 240, G: 244, B: 248, A: 255}}),
			ui.Fixed(ui.Spacer{H: 12}),
		)
	}
	if body != nil {
		children = append(children, ui.Fixed(body))
	}
	return ui.Panel{
		Fill:   color.RGBA{R: 18, G: 28, B: 40, A: 255},
		Stroke: color.RGBA{R: 54, G: 68, B: 84, A: 180},
		Insets: ui.UniformInsets(16),
		Child: ui.Column{
			Children: children,
			Spacing:  0,
		},
	}
}

func settingsActionElement(id, label string, visual settingsActionVisual, width float64) ui.Element {
	return ui.Button{
		ID:      id,
		Label:   label,
		Enabled: visual.Enabled,
		Active:  visual.Active,
		Pending: visual.Pending,
		Width:   width,
	}
}

func settingsToggleElement(id string, visual settingsActionVisual) ui.Element {
	return ui.Toggle{
		ID:      id,
		Enabled: visual.Enabled,
		Active:  visual.Active,
		Pending: visual.Pending,
	}
}

func settingsStatusElement(text string, clr color.Color) ui.Element {
	if text == "" {
		return nil
	}
	return ui.Paragraph{Text: text, Size: 12, Color: clr}
}

func settingsSectionLabelElement(label string) ui.Element {
	return ui.Label{Text: label, Size: 12, Color: color.RGBA{R: 166, G: 178, B: 190, A: 255}}
}

func settingsKeyValueElement(label, value string, split float64) ui.Element {
	return ui.KeyValue{
		Label:      label,
		Value:      fallbackLabel(value, "Unavailable"),
		LabelWidth: split - 12,
	}
}

func (a *App) settingsMouseBody(snap session.Snapshot) ui.Element {
	a.mu.RLock()
	state := a.sectionData.Mouse
	a.mu.RUnlock()
	jiggler := a.settingsAction(settingsGroupJiggler)

	leftChildren := []ui.Child{
		ui.Fixed(settingsSectionLabelElement("Remote mode")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{
			Children: []ui.Element{
				settingsActionElement("mouse_absolute", "Absolute", settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected, Active: !a.relative}, 110),
				settingsActionElement("mouse_relative", "Relative", settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected, Active: a.relative}, 110),
			},
			Spacing:     12,
			LineSpacing: 8,
		}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsSectionLabelElement("Local cursor")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsActionElement("mouse_hide_cursor", "Hide Host Cursor", settingsActionVisual{Enabled: true, Active: a.hideCursor}, 154)),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsSectionLabelElement("Wheel")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{
			Text:  "Throttle local wheel bursts before sending them to the device.",
			Size:  12,
			Color: color.RGBA{R: 166, G: 178, B: 190, A: 255},
		}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Wrap{
			Children: []ui.Element{
				settingsActionElement("scroll_0", "Off", settingsActionVisual{Enabled: true, Active: a.scrollThrottle == 0}, 64),
				settingsActionElement("scroll_10", "Low", settingsActionVisual{Enabled: true, Active: a.scrollThrottle == 10*time.Millisecond}, 64),
				settingsActionElement("scroll_25", "Medium", settingsActionVisual{Enabled: true, Active: a.scrollThrottle == 25*time.Millisecond}, 84),
				settingsActionElement("scroll_50", "High", settingsActionVisual{Enabled: true, Active: a.scrollThrottle == 50*time.Millisecond}, 72),
				settingsActionElement("scroll_100", "Very High", settingsActionVisual{Enabled: true, Active: a.scrollThrottle == 100*time.Millisecond}, 108),
				settingsActionElement("scroll_invert", "Invert Scroll", settingsActionVisual{Enabled: true, Active: a.invertScroll}, 128),
			},
			Spacing:     12,
			LineSpacing: 8,
		}),
	}

	rightChildren := []ui.Child{}
	if state.Loading {
		rightChildren = append(rightChildren,
			ui.Fixed(ui.Label{Text: "Loading jiggler state…", Size: 13, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}}),
		)
	} else {
		rightChildren = append(rightChildren,
			ui.Fixed(settingsKeyValueElement("State", boolPtrWord(state.JigglerEnabled), 70)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsKeyValueElement("Preset", jigglerPresetLabel(state.JigglerEnabled, state.JigglerConfig), 70)),
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(ui.Paragraph{
				Text:  "Use native presets or open a compact custom editor for the device jiggler schedule.",
				Size:  12,
				Color: color.RGBA{R: 166, G: 178, B: 190, A: 255},
			}),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(ui.Wrap{
				Children: []ui.Element{
					settingsActionElement("jiggler_disabled", "Disabled", settingsActionVisual{Enabled: state.JigglerEnabled != nil && (!jiggler.Pending || jiggler.PendingChoice == "disabled"), Active: state.JigglerEnabled != nil && !*state.JigglerEnabled, Pending: jiggler.Pending && jiggler.PendingChoice == "disabled"}, 88),
					settingsActionElement("jiggler_frequent", "Frequent", settingsActionVisual{Enabled: !jiggler.Pending || jiggler.PendingChoice == "frequent", Active: jigglerPresetLabel(state.JigglerEnabled, state.JigglerConfig) == "Frequent", Pending: jiggler.Pending && jiggler.PendingChoice == "frequent"}, 88),
					settingsActionElement("jiggler_standard", "Standard", settingsActionVisual{Enabled: !jiggler.Pending || jiggler.PendingChoice == "standard", Active: jigglerPresetLabel(state.JigglerEnabled, state.JigglerConfig) == "Standard", Pending: jiggler.Pending && jiggler.PendingChoice == "standard"}, 88),
					settingsActionElement("jiggler_light", "Light", settingsActionVisual{Enabled: !jiggler.Pending || jiggler.PendingChoice == "light", Active: jigglerPresetLabel(state.JigglerEnabled, state.JigglerConfig) == "Light", Pending: jiggler.Pending && jiggler.PendingChoice == "light"}, 72),
					settingsActionElement("jiggler_custom", "Custom", settingsActionVisual{Enabled: !jiggler.Pending, Active: a.jigglerEditorOpen || jigglerPresetLabel(state.JigglerEnabled, state.JigglerConfig) == "Custom"}, 84),
				},
				Spacing:     12,
				LineSpacing: 8,
			}),
		)
		if a.jigglerEditorOpen {
			rightChildren = append(rightChildren,
				ui.Fixed(ui.Spacer{H: 18}),
				ui.Fixed(settingsSectionLabelElement("Custom config")),
				ui.Fixed(ui.Spacer{H: 10}),
				ui.Fixed(ui.Wrap{
					Children: []ui.Element{
						settingsKeyValueElement("Idle (s)", fmt.Sprintf("%d", a.jigglerEditorConfig.InactivityLimitSeconds), 70),
						settingsActionElement("jiggler_custom_inactivity_minus", "-", settingsActionVisual{Enabled: true}, 30),
						settingsActionElement("jiggler_custom_inactivity_plus", "+", settingsActionVisual{Enabled: true}, 30),
						settingsKeyValueElement("Jitter", fmt.Sprintf("%d%%", a.jigglerEditorConfig.JitterPercentage), 56),
						settingsActionElement("jiggler_custom_jitter_minus", "-", settingsActionVisual{Enabled: true}, 30),
						settingsActionElement("jiggler_custom_jitter_plus", "+", settingsActionVisual{Enabled: true}, 30),
					},
					Spacing:     8,
					LineSpacing: 8,
				}),
				ui.Fixed(ui.Spacer{H: 16}),
				ui.Fixed(settingsSectionLabelElement("Cron")),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(ui.TextField{
					ID:          "jiggler_focus_cron",
					Value:       a.jigglerEditorConfig.ScheduleCronTab,
					Placeholder: "0 * * * * *",
					Focused:     a.settingsInputFocus == settingsInputJigglerCron,
					Enabled:     true,
				}),
				ui.Fixed(ui.Spacer{H: 16}),
				ui.Fixed(settingsSectionLabelElement("Timezone")),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(ui.TextField{
					ID:          "jiggler_focus_timezone",
					Value:       a.jigglerEditorConfig.Timezone,
					Placeholder: "UTC",
					Focused:     a.settingsInputFocus == settingsInputJigglerTimezone,
					Enabled:     true,
				}),
				ui.Fixed(ui.Spacer{H: 16}),
				ui.Fixed(ui.Wrap{
					Children: []ui.Element{
						settingsActionElement("jiggler_custom_save", "Save Custom", settingsActionVisual{Enabled: !jiggler.Pending}, 116),
						settingsActionElement("jiggler_custom_cancel", "Cancel", settingsActionVisual{Enabled: !jiggler.Pending}, 86),
					},
					Spacing:     12,
					LineSpacing: 8,
				}),
			)
		}
		statusText := ""
		statusColor := color.RGBA{R: 245, G: 200, B: 96, A: 255}
		switch {
		case jiggler.Pending:
			statusText = "Applying…"
		case jiggler.Error != "":
			statusText = jiggler.Error
			statusColor = color.RGBA{R: 220, G: 132, B: 132, A: 255}
		}
		if statusText != "" {
			rightChildren = append(rightChildren,
				ui.Fixed(ui.Spacer{H: 16}),
				ui.Fixed(settingsStatusElement(statusText, statusColor)),
			)
		}
		if a.jigglerEditorError != "" {
			rightChildren = append(rightChildren,
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(settingsStatusElement(a.jigglerEditorError, color.RGBA{R: 220, G: 132, B: 132, A: 255})),
			)
		}
	}
	if state.Error != "" {
		rightChildren = append(rightChildren,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(settingsStatusElement(state.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})),
		)
	}

	return ui.Row{
		Children: []ui.Child{
			ui.Flex(settingsCardElement("Pointer", ui.Column{Children: leftChildren}), 54),
			ui.Flex(settingsCardElement("Jiggler", ui.Column{Children: rightChildren}), 46),
		},
		Spacing: 14,
	}
}

func settingsTwoPane(left ui.Element, leftFlex float64, right ui.Element, rightFlex float64) ui.Element {
	return ui.Row{
		Children: []ui.Child{
			ui.Flex(left, leftFlex),
			ui.Flex(right, rightFlex),
		},
		Spacing: 14,
	}
}

func (a *App) settingsSectionBody(section settingsSection, snap session.Snapshot) ui.Element {
	switch section {
	case sectionGeneral:
		return a.settingsGeneralBody(snap)
	case sectionMouse:
		return a.settingsMouseBody(snap)
	case sectionKeyboard:
		return a.settingsKeyboardBody(snap)
	case sectionVideo:
		return a.settingsVideoBody(snap)
	case sectionHardware:
		return a.settingsHardwareBody()
	case sectionAccess:
		return a.settingsAccessBody()
	case sectionAppearance:
		return a.settingsAppearanceBody()
	case sectionNetwork:
		return a.settingsNetworkBody()
	case sectionAdvanced:
		return a.settingsAdvancedBody()
	default:
		return a.settingsPlannedBody(section)
	}
}

func (a *App) settingsGeneralBody(snap session.Snapshot) ui.Element {
	a.mu.RLock()
	state := a.sectionData.General
	a.mu.RUnlock()
	updateLabel := "No updates reported"
	if snap.AppUpdateAvailable || snap.SystemUpdateAvailable {
		updateLabel = "Updates available"
	}
	deviceCard := settingsCardElement("Device", ui.Column{Children: []ui.Child{
		ui.Fixed(settingsKeyValueElement("Base URL", snap.BaseURL, 116)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("Phase", snap.Phase.String(), 116)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("Signaling", signalingLabel(snap.SignalingMode), 116)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("App Version", snap.AppVersion, 116)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("System Version", snap.SystemVersion, 116)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("Updates", updateLabel, 116)),
	}})
	autoUpdate := a.settingsAction(settingsGroupAutoUpdate)
	actionChildren := []ui.Child{
		ui.Fixed(ui.Paragraph{Text: "Reconnect the native session, manage auto-updates, or force a device reboot.", Size: 12, Color: color.RGBA{R: 166, G: 178, B: 190, A: 255}}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsActionElement("reconnect", reconnectLabel(snap.Phase), settingsActionVisual{Enabled: true}, 0)),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsActionElement("reboot", "Reboot device", settingsActionVisual{Enabled: snap.Phase != session.PhaseConnecting}, 0)),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsSectionLabelElement("Auto updates")),
		ui.Fixed(ui.Spacer{H: 8}),
	}
	if state.Loading {
		actionChildren = append(actionChildren, ui.Fixed(ui.Label{Text: "Loading…", Size: 12, Color: color.RGBA{R: 166, G: 178, B: 190, A: 255}}))
	} else {
		actionChildren = append(actionChildren,
			ui.Fixed(ui.Row{AlignY: ui.AlignCenter, Children: []ui.Child{
				ui.Fixed(settingsToggleElement("auto_update_toggle", settingsActionVisual{
					Enabled: state.AutoUpdate != nil && !autoUpdate.Pending,
					Active:  state.AutoUpdate != nil && *state.AutoUpdate,
					Pending: autoUpdate.Pending,
				})),
				ui.Fixed(ui.Spacer{W: 12}),
				ui.Fixed(ui.Label{Text: "Enabled", Size: 13, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}}),
			}}),
		)
		switch {
		case autoUpdate.Pending:
			actionChildren = append(actionChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", color.RGBA{R: 245, G: 200, B: 96, A: 255})))
		case autoUpdate.Error != "":
			actionChildren = append(actionChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(autoUpdate.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
		}
	}
	if state.Error != "" {
		actionChildren = append(actionChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
	}
	actionsCard := settingsCardElement("Actions", ui.Column{Children: actionChildren})
	return settingsTwoPane(deviceCard, 58, actionsCard, 42)
}

func (a *App) settingsKeyboardBody(snap session.Snapshot) ui.Element {
	layout := snap.KeyboardLayout
	if layout == "" {
		layout = "en-US"
	}
	layoutState := a.settingsAction(settingsGroupKeyboardLayout)
	options := input.SupportedKeyboardLayouts()
	buttons := make([]ui.Element, 0, len(options))
	for _, option := range options {
		btnW := 94.0
		if len(option.Label) > 7 {
			btnW = 112
		}
		buttons = append(buttons, settingsActionElement("layout:"+option.Code, option.Label, settingsActionVisual{
			Enabled: snap.Phase == session.PhaseConnected && (!layoutState.Pending || layoutState.PendingChoice == option.Code),
			Active:  layout == option.Code,
			Pending: layoutState.Pending && layoutState.PendingChoice == option.Code,
		}, btnW))
	}
	children := []ui.Child{
		ui.Fixed(ui.Paragraph{Text: "This layout affects paste and keyboard macros. Live typing is sent as physical HID keys.", Size: 12, Color: color.RGBA{R: 166, G: 178, B: 190, A: 255}}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(ui.Row{Children: []ui.Child{
			ui.Fixed(settingsKeyValueElement("Active layout", keyboardLayoutLabel(layout), 118)),
			ui.Flex(ui.Spacer{}, 1),
			ui.Fixed(settingsActionElement("toggle_pressed_keys", "Show Pressed Keys", settingsActionVisual{Enabled: true, Active: a.showPressedKeys}, 158)),
		}, Spacing: 12}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsSectionLabelElement("Layout presets")),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(ui.Wrap{Children: buttons, Spacing: 10, LineSpacing: 8}),
	}
	switch {
	case layoutState.Pending:
		children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", color.RGBA{R: 245, G: 200, B: 96, A: 255})))
	case layoutState.Error != "":
		children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(layoutState.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
	}
	children = append(children,
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Paragraph{Text: "Make this match the remote OS only for pasted text and macros.", Size: 13, Color: color.RGBA{R: 166, G: 178, B: 190, A: 255}}),
	)
	return settingsCardElement("", ui.Column{Children: children})
}

func (a *App) settingsVideoBody(snap session.Snapshot) ui.Element {
	qualityState := a.settingsAction(settingsGroupVideoQuality)
	streamChildren := []ui.Child{
		ui.Fixed(settingsSectionLabelElement("Quality preset")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionElement("quality_preset_high", "High", settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected && (!qualityState.Pending || qualityState.PendingChoice == "high"), Active: snap.Quality >= 0.95, Pending: qualityState.Pending && qualityState.PendingChoice == "high"}, 96),
			settingsActionElement("quality_preset_medium", "Medium", settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected && (!qualityState.Pending || qualityState.PendingChoice == "medium"), Active: snap.Quality >= 0.45 && snap.Quality < 0.95, Pending: qualityState.Pending && qualityState.PendingChoice == "medium"}, 96),
			settingsActionElement("quality_preset_low", "Low", settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected && (!qualityState.Pending || qualityState.PendingChoice == "low"), Active: snap.Quality < 0.45, Pending: qualityState.Pending && qualityState.PendingChoice == "low"}, 96),
		}, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Label{Text: fmt.Sprintf("Current factor %.2f", snap.Quality), Size: 13, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}}),
	}
	switch {
	case qualityState.Pending:
		streamChildren = append(streamChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", color.RGBA{R: 245, G: 200, B: 96, A: 255})))
	case qualityState.Error != "":
		streamChildren = append(streamChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(qualityState.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
	}
	edid := snap.EDID
	if edid == "" {
		edid = "Unavailable on current target"
	}
	return settingsTwoPane(
		settingsCardElement("Stream", ui.Column{Children: streamChildren}),
		48,
		settingsCardElement("EDID", ui.Column{Children: []ui.Child{ui.Fixed(ui.Paragraph{Text: edid, Size: 12, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}})}}),
		52,
	)
}

func (a *App) settingsHardwareBody() ui.Element {
	a.mu.RLock()
	state := a.sectionData.Hardware
	a.mu.RUnlock()
	if state.Loading {
		return settingsTwoPane(
			settingsCardElement("Display", ui.Label{Text: "Loading hardware state…", Size: 13, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}}),
			48,
			settingsCardElement("USB", ui.Spacer{}),
			52,
		)
	}
	rotateState := a.settingsAction(settingsGroupDisplayRotate)
	displayChildren := []ui.Child{
		ui.Fixed(settingsKeyValueElement("Rotation", string(state.State.DisplayRotation), 86)),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Paragraph{Text: "Rotate the JetKVM device display. This does not rotate the remote host video feed.", Size: 12, Color: color.RGBA{R: 166, G: 178, B: 190, A: 255}}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionElement("rotate_normal", "Normal", settingsActionVisual{Enabled: state.State.DisplayRotation != session.DisplayRotationUnknown && (!rotateState.Pending || rotateState.PendingChoice == "270"), Active: state.State.DisplayRotation == session.DisplayRotationNormal, Pending: rotateState.Pending && rotateState.PendingChoice == "270"}, 88),
			settingsActionElement("rotate_inverted", "Inverted", settingsActionVisual{Enabled: state.State.DisplayRotation != session.DisplayRotationUnknown && (!rotateState.Pending || rotateState.PendingChoice == "90"), Active: state.State.DisplayRotation == session.DisplayRotationInverted, Pending: rotateState.Pending && rotateState.PendingChoice == "90"}, 98),
		}, Spacing: 12, LineSpacing: 8}),
	}
	switch {
	case rotateState.Pending:
		displayChildren = append(displayChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", color.RGBA{R: 245, G: 200, B: 96, A: 255})))
	case rotateState.Error != "":
		displayChildren = append(displayChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(rotateState.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
	}
	usbState := a.settingsAction(settingsGroupUSBEmulation)
	usbDevicesState := a.settingsAction(settingsGroupUSBDevices)
	preset := usbDevicePresetLabel(state.State.USBDevices)
	usbChildren := []ui.Child{
		ui.Fixed(settingsKeyValueElement("USB Emulation", boolPtrWord(state.State.USBEmulation), 112)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("USB Config", usbConfigLabel(state.State.USBConfig), 112)),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Configured devices")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{Text: usbDevicesSummary(state.State.USBDevices), Size: 12, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}}),
	}
	if state.State.USBEmulation != nil {
		usbChildren = append(usbChildren,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("usb_emulation_on", "USB On", settingsActionVisual{Enabled: !usbState.Pending || usbState.PendingChoice == "on", Active: *state.State.USBEmulation, Pending: usbState.Pending && usbState.PendingChoice == "on"}, 86),
				settingsActionElement("usb_emulation_off", "USB Off", settingsActionVisual{Enabled: !usbState.Pending || usbState.PendingChoice == "off", Active: !*state.State.USBEmulation, Pending: usbState.Pending && usbState.PendingChoice == "off"}, 92),
			}, Spacing: 12, LineSpacing: 8}),
		)
		switch {
		case usbState.Pending:
			usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", color.RGBA{R: 245, G: 200, B: 96, A: 255})))
		case usbState.Error != "":
			usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(usbState.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
		}
	}
	usbChildren = append(usbChildren,
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Preset")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionElement("usb_devices_default", "Default", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "default", Active: preset == "Default", Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "default"}, 86),
			settingsActionElement("usb_devices_keyboard_only", "Keyboard Only", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "keyboard_only", Active: preset == "Keyboard Only", Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "keyboard_only"}, 122),
		}, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsSectionLabelElement("Custom")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionElement("usb_toggle_keyboard", "Keyboard", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.Keyboard, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"}, 94),
			settingsActionElement("usb_toggle_absolute_mouse", "Abs Mouse", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.AbsoluteMouse, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"}, 98),
			settingsActionElement("usb_toggle_relative_mouse", "Rel Mouse", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.RelativeMouse, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"}, 96),
			settingsActionElement("usb_toggle_mass_storage", "Mass Storage", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.MassStorage, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"}, 110),
			settingsActionElement("usb_toggle_serial_console", "Serial", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.SerialConsole, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"}, 74),
			settingsActionElement("usb_toggle_network", "Network", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.Network, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"}, 88),
		}, Spacing: 12, LineSpacing: 8}),
	)
	switch {
	case usbDevicesState.Pending:
		usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", color.RGBA{R: 245, G: 200, B: 96, A: 255})))
	case usbDevicesState.Error != "":
		usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(usbDevicesState.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
	}
	if state.Error != "" {
		usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
	}
	return settingsTwoPane(settingsCardElement("Display", ui.Column{Children: displayChildren}), 48, settingsCardElement("USB", ui.Column{Children: usbChildren}), 52)
}

func (a *App) settingsAccessBody() ui.Element {
	a.mu.RLock()
	state := a.sectionData.Access
	a.mu.RUnlock()
	if state.Loading {
		return settingsTwoPane(
			settingsCardElement("Cloud", ui.Label{Text: "Loading access state…", Size: 13, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}}),
			50,
			settingsCardElement("TLS", ui.Spacer{}),
			50,
		)
	}
	cloudChildren := []ui.Child{
		ui.Fixed(settingsKeyValueElement("Connected", boolWord(state.State.Cloud.Connected), 96)),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Cloud API")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{Text: fallbackLabel(state.State.Cloud.URL, "Unavailable"), Size: 12, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Cloud App")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{Text: fallbackLabel(state.State.Cloud.AppURL, "Unavailable"), Size: 12, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}}),
	}
	tlsState := a.settingsAction(settingsGroupTLSMode)
	tlsChildren := []ui.Child{
		ui.Fixed(settingsKeyValueElement("Mode", string(state.State.TLS), 70)),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Paragraph{Text: "Use the target's currently exposed TLS mode. Native client transport follows whatever the device publishes.", Size: 12, Color: color.RGBA{R: 166, G: 178, B: 190, A: 255}}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionElement("tls_disabled", "Disabled", settingsActionVisual{Enabled: state.State.TLS != session.TLSModeUnknown && (!tlsState.Pending || tlsState.PendingChoice == "disabled"), Active: state.State.TLS == session.TLSModeDisabled, Pending: tlsState.Pending && tlsState.PendingChoice == "disabled"}, 92),
			settingsActionElement("tls_self_signed", "Self-Signed", settingsActionVisual{Enabled: state.State.TLS != session.TLSModeUnknown && (!tlsState.Pending || tlsState.PendingChoice == "self-signed"), Active: state.State.TLS == session.TLSModeSelfSigned, Pending: tlsState.Pending && tlsState.PendingChoice == "self-signed"}, 114),
		}, Spacing: 12, LineSpacing: 8}),
	}
	switch {
	case tlsState.Pending:
		tlsChildren = append(tlsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", color.RGBA{R: 245, G: 200, B: 96, A: 255})))
	case tlsState.Error != "":
		tlsChildren = append(tlsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(tlsState.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
	}
	if state.Error != "" {
		tlsChildren = append(tlsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
	}
	return settingsTwoPane(settingsCardElement("Cloud", ui.Column{Children: cloudChildren}), 50, settingsCardElement("TLS", ui.Column{Children: tlsChildren}), 50)
}

func (a *App) settingsNetworkBody() ui.Element {
	a.mu.RLock()
	state := a.sectionData.Network
	a.mu.RUnlock()
	children := []ui.Child{}
	if state.Loading {
		children = append(children, ui.Fixed(ui.Label{Text: "Loading network state…", Size: 13, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}}))
	} else {
		children = append(children,
			ui.Fixed(settingsKeyValueElement("Hostname", state.State.Hostname, 96)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsKeyValueElement("IP", state.State.IP, 96)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsKeyValueElement("DHCP", boolPtrWord(state.State.DHCP), 96)),
		)
	}
	if state.Error != "" {
		children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
	}
	return settingsCardElement("Current state", ui.Column{Children: children})
}

func (a *App) settingsAdvancedBody() ui.Element {
	a.mu.RLock()
	state := a.sectionData.Advanced
	a.mu.RUnlock()
	children := []ui.Child{}
	if state.Loading {
		children = append(children, ui.Fixed(ui.Label{Text: "Loading advanced state…", Size: 13, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}}))
	} else {
		children = append(children,
			ui.Fixed(settingsKeyValueElement("Developer Mode", boolPtrWord(state.State.DevMode), 128)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsKeyValueElement("USB Emulation", boolPtrWord(state.State.USBEmulation), 128)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsKeyValueElement("App Version", state.State.Version.AppVersion, 128)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsKeyValueElement("System Version", state.State.Version.SystemVersion, 128)),
		)
		if state.State.DevMode != nil {
			devModeState := a.settingsAction(settingsGroupDeveloperMode)
			children = append(children,
				ui.Fixed(ui.Spacer{H: 14}),
				ui.Fixed(ui.Wrap{Children: []ui.Element{
					settingsActionElement("developer_mode_on", "Developer Mode On", settingsActionVisual{Enabled: !devModeState.Pending || devModeState.PendingChoice == "on", Active: *state.State.DevMode, Pending: devModeState.Pending && devModeState.PendingChoice == "on"}, 156),
					settingsActionElement("developer_mode_off", "Developer Mode Off", settingsActionVisual{Enabled: !devModeState.Pending || devModeState.PendingChoice == "off", Active: !*state.State.DevMode, Pending: devModeState.Pending && devModeState.PendingChoice == "off"}, 160),
				}, Spacing: 12, LineSpacing: 8}),
			)
			switch {
			case devModeState.Pending:
				children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", color.RGBA{R: 245, G: 200, B: 96, A: 255})))
			case devModeState.Error != "":
				children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(devModeState.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
			}
		}
	}
	if state.Error != "" {
		children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, color.RGBA{R: 220, G: 132, B: 132, A: 255})))
	}
	return settingsCardElement("Current state", ui.Column{Children: children})
}

func (a *App) settingsAppearanceBody() ui.Element {
	positionButtons := []ui.Element{
		settingsActionElement("chrome_anchor:top_left", "Top Left", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorTopLeft}, 96),
		settingsActionElement("chrome_anchor:top_center", "Top Center", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorTopCenter}, 108),
		settingsActionElement("chrome_anchor:top_right", "Top Right", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorTopRight}, 100),
		settingsActionElement("chrome_anchor:left_center", "Left Center", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorLeftCenter}, 108),
		settingsActionElement("chrome_anchor:right_center", "Right Center", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorRightCenter}, 118),
		settingsActionElement("chrome_anchor:bottom_left", "Bottom Left", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorBottomLeft}, 108),
		settingsActionElement("chrome_anchor:bottom_center", "Bottom Center", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorBottomCenter}, 126),
		settingsActionElement("chrome_anchor:bottom_right", "Bottom Right", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorBottomRight}, 118),
	}
	return settingsCardElement("Chrome", ui.Column{Children: []ui.Child{
		ui.Fixed(settingsSectionLabelElement("Top bar")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionElement("pin_chrome_off", "Auto-hide", settingsActionVisual{Enabled: true, Active: !a.prefs.PinChrome}, 96),
			settingsActionElement("pin_chrome_on", "Pinned", settingsActionVisual{Enabled: true, Active: a.prefs.PinChrome}, 84),
		}, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Position")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: positionButtons, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Layout")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionElement("chrome_layout:horizontal", "Horizontal", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeLayout == chromeLayoutHorizontal}, 112),
			settingsActionElement("chrome_layout:vertical", "Vertical", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeLayout == chromeLayoutVertical}, 96),
		}, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Window")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsActionElement("fullscreen", "Toggle Fullscreen", settingsActionVisual{Enabled: true, Active: ebiten.IsFullscreen()}, 160)),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Paragraph{Text: "Position chooses where the chrome sits on screen. Layout changes whether the control buttons run across or down.", Size: 12, Color: color.RGBA{R: 166, G: 178, B: 190, A: 255}}),
	}})
}

func (a *App) settingsPlannedBody(section settingsSection) ui.Element {
	defs := settingsSections(session.Snapshot{})
	var current settingsSectionDef
	for _, def := range defs {
		if def.id == section {
			current = def
			break
		}
	}
	children := []ui.Child{
		ui.Fixed(ui.Paragraph{Text: current.description, Size: 12, Color: color.RGBA{R: 166, G: 178, B: 190, A: 255}}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Current upstream surface")),
	}
	for _, item := range current.items {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Paragraph{Text: "• " + item, Size: 12, Color: color.RGBA{R: 236, G: 241, B: 245, A: 255}}),
		)
	}
	children = append(children,
		ui.Fixed(ui.Spacer{H: 16}),
		ui.Fixed(ui.Paragraph{Text: "This section exists in the upstream product structure but is not currently exposed by this target or the desktop client.", Size: 12, Color: color.RGBA{R: 166, G: 178, B: 190, A: 255}}),
	)
	return settingsCardElement("Not exposed here", ui.Column{Children: children})
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
