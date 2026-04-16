package app

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/logging"
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
	onClick func()
}

type settingsSection uint8

const (
	sectionGeneral    settingsSection = iota // general
	sectionMouse                             // mouse
	sectionKeyboard                          // keyboard
	sectionVideo                             // video
	sectionHardware                          // hardware
	sectionATX                               // atx
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
	Video    videoState
	Access   accessState
	ATX      atxState
	Hardware hardwareState
	Network  networkState
	Macros   macrosState
	MQTT     mqttState
	Advanced advancedState
}

type generalState struct {
	Loading    bool
	Error      string
	AutoUpdate *bool
	Update     session.UpdateStatus
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

type atxState struct {
	Loading         bool
	Error           string
	ActiveExtension string
	State           *session.ATXState
}

type videoState struct {
	Loading bool
	Error   string
	State   session.VideoState
}

type hardwareState struct {
	Loading               bool
	Error                 string
	State                 session.HardwareState
	USBConfigLoaded       bool
	USBDevicesLoaded      bool
	DisplayRotationLoaded bool
	BacklightLoaded       bool
	VideoSleepLoaded      bool
}

type networkState struct {
	Loading        bool
	Error          string
	Settings       session.NetworkSettings
	State          session.NetworkState
	PublicIPs      []session.PublicIP
	PublicIPError  string
	Tailscale      *session.TailscaleStatus
	TailscaleError string
}

type macrosState struct {
	Loading bool
	Error   string
	Macros  []session.KeyboardMacro
}

type mqttState struct {
	Loading  bool
	Error    string
	Settings session.MQTTSettings
	Status   session.MQTTStatus
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

const (
	settingsScrollThrottleSliderID  = "scroll_throttle_slider"
	settingsPointerThrottleSliderID = "pointer_throttle_slider"
)

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
			id:          sectionATX,
			label:       "ATX",
			description: "Host power board controls and LED status",
			available:   true,
			items: []string{
				"Power, reset, and long-press controls for the host machine",
				"Power LED and HDD LED status",
				"Available when the ATX extension is active on the JetKVM",
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
			available:   true,
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
			available:   true,
			items: []string{
				"Broker connection and TLS options",
				"Base topic and Home Assistant discovery",
				"Connection test and save",
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

const (
	videoEDIDPresetJetKVMDefault = "00ffffffffffff0028b4010001eeffc0302301038047287856ee91a3544c99260f5054000000d1c081c0318001010101010101010101023a801871382d40582c4500c48e2100001e011d007251d01e206e285500c48e2100001e000000fd00174c0f5111000a202020202020000000fc004a65744b564d2076310a202020011d020322d1431004012309070783010000e200cfe40d100401e305000065030c001000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000cf"
	videoEDIDPresetAcerB246WL    = "00FFFFFFFFFFFF00047265058A3F6101101E0104A53420783FC125A8554EA0260D5054BFEF80714F8140818081C081008B009500B300283C80A070B023403020360006442100001A000000FD00304C575716010A202020202020000000FC0042323436574C0A202020202020000000FF0054384E4545303033383532320A01F802031CF14F90020304050607011112131415161F2309070783010000011D8018711C1620582C250006442100009E011D007251D01E206E28550006442100001E8C0AD08A20E02D10103E9600064421000018C344806E70B028401720A80406442100001E00000000000000000000000000000000000000000000000000000096"
	videoEDIDPresetASUSPA248QV   = "00FFFFFFFFFFFF0006B3872401010101021F010380342078EA6DB5A7564EA0250D5054BF6F00714F8180814081C0A9409500B300D1C0283C80A070B023403020360006442100001A000000FD00314B1E5F19000A202020202020000000FC00504132343851560A2020202020000000FF004D314C4D51533035323135370A014D02032AF14B900504030201111213141F230907078301000065030C001000681A00000101314BE6E2006A023A801871382D40582C450006442100001ECD5F80B072B0374088D0360006442100001C011D007251D01E206E28550006442100001E8C0AD08A20E02D10103E960006442100001800000000000000000000000000DC"
	videoEDIDPresetDellD2721H    = "00FFFFFFFFFFFF0010AC132045393639201E0103803C22782ACD25A3574B9F270D5054A54B00714F8180A9C0D1C00101010101010101023A801871382D40582C450056502100001E000000FF00335335475132330A2020202020000000FC0044454C4C204432373231480A20000000FD00384C1E5311000A202020202020018102031AB14F90050403020716010611121513141F65030C001000023A801871382D40582C450056502100001E011D8018711C1620582C250056502100009E011D007251D01E206E28550056502100001E8C0AD08A20E02D10103E960056502100001800000000000000000000000000000000000000000000000000000000004F"
	videoEDIDPresetDellIDRAC     = "00FFFFFFFFFFFF0010AC0100020000000111010380221BFF0A00000000000000000000ADCE0781800101010101010101010101010101000000FF0030303030303030303030303030000000FF0030303030303030303030303030000000FD00384C1F530B000A000000000000000000FC0044454C4C2049445241430A2020000A"
)

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
		defs = append(defs, chromeButton{id: "reconnect", hint: reconnectLabel(snap.Phase), icon: iconReconnect, enabled: true, onClick: func() {
			if a.ctrl == nil {
				return
			}
			a.releaseAllKeys(true)
			a.ctrl.ReconnectNow()
		}})
	}
	if snap.Phase == session.PhaseConnected {
		defs = append(defs, chromeButton{id: "paste", hint: "Paste text", icon: iconPaste, enabled: true, active: a.pasteOpen, onClick: func() {
			if a.pasteOpen {
				a.closePasteOverlay()
			} else {
				a.pasteOpen = true
				a.loadClipboardText()
				a.settingsOpen = false
				a.mediaOpen = false
				a.applyCursorMode()
			}
		}})
		defs = append(defs, chromeButton{id: "media", hint: "Virtual media", icon: iconMedia, enabled: true, active: a.mediaOpen, onClick: func() {
			if a.mediaOpen {
				a.closeMediaOverlay()
			} else {
				a.openMediaOverlay()
			}
		}})
	}
	defs = append(defs,
		chromeButton{id: "stats", hint: "Connection stats", icon: iconStats, enabled: true, active: a.statsOpen, onClick: func() { a.statsOpen = !a.statsOpen }},
		chromeButton{id: "fullscreen", hint: "Toggle fullscreen", icon: iconFullscreen, enabled: true, active: ebiten.IsFullscreen(), onClick: func() { ebiten.SetFullscreen(!ebiten.IsFullscreen()) }},
		chromeButton{id: "settings", hint: "Settings", icon: iconSettings, enabled: true, active: a.settingsOpen, onClick: func() {
			if a.settingsOpen {
				a.closeSettingsOverlay()
			} else {
				a.settingsOpen = true
				a.pasteOpen = false
				a.mediaOpen = false
				a.refreshSettingsSection(a.settingsSection)
				a.applyCursorMode()
			}
			a.revealUIFor(1200 * time.Millisecond)
		}},
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
	if a.prefs.HideHeaderBar {
		return
	}
	buttons := a.layoutChromeButtons(screen.Bounds().Dx(), screen.Bounds().Dy(), snap)
	a.chromeButtons = buttons
	a.drawUIRoot(screen, &a.chromeRuntime, func(chromeButton) {}, chromeButtonsElement{
		buttons: buttons,
		alpha:   alpha,
	})
}

func (a *App) drawHint(screen *ebiten.Image) {
	alpha := a.uiAlpha()
	if alpha <= 0 {
		return
	}
	if a.prefs.HideHeaderBar {
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
			a.drawUIRoot(screen, nil, func(chromeButton) {}, chromeTooltipElement{
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
	if a.prefs.HideStatusBar {
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
	a.drawUIRoot(screen, nil, func(chromeButton) {}, footerStatusElement{
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
		a.settingsRuntime.BeginFrame()
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
	a.drawUIRoot(screen, &a.settingsRuntime, func(chromeButton) {}, settingsOverlayRootElement{
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
		ui.Backdrop{Color: ctx.Theme.Backdrop},
		ui.Positioned{
			X: e.outerRect.X,
			Y: e.outerRect.Y,
			W: e.outerRect.W,
			H: e.outerRect.H,
			Child: ui.Panel{
				Fill:   ctx.Theme.ModalFill,
				Stroke: ctx.Theme.ModalStroke,
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
		if ctx.Runtime != nil {
			actionID := btn.id
			onClick := btn.onClick
			ctx.Runtime.Register(ui.Control{
				ID:      actionID,
				Rect:    ui.Rect{X: btn.rect.x, Y: btn.rect.y, W: btn.rect.w, H: btn.rect.h},
				Enabled: btn.enabled,
				OnClick: func(ui.PointerEvent) {
					if onClick != nil {
						onClick()
					} else if ctx.OnAction != nil {
						ctx.OnAction(actionID)
					}
				},
			})
		}
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
	children := []ui.Element{
		ui.Row{
			Children: []ui.Child{
				ui.Fixed(ui.Constrained{
					MinW: e.sidebarW,
					MaxW: e.sidebarW,
					Child: ui.Panel{
						Fill:   ctx.Theme.SectionFill,
						Stroke: ctx.Theme.SectionStroke,
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
				Child:      ui.Button{Label: "X", Enabled: true, OnClick: func() { e.app.closeSettingsOverlay() }},
			},
		},
	}
	if e.app.h265ConfirmOpen {
		children = append(children, settingsH265ConfirmElement{app: e.app})
	}
	if e.app.atxConfirmAction != "" {
		children = append(children, settingsATXConfirmElement{app: e.app})
	}
	if e.app.accessEditor.Mode != accessEditorModeNone {
		children = append(children, settingsAccessEditorModalElement{app: e.app})
	}
	ui.Stack{Children: children}.Draw(ctx, bounds)
}

type settingsH265ConfirmElement struct {
	app *App
}

func (settingsH265ConfirmElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e settingsH265ConfirmElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	panelW := min(520, bounds.W-56)
	ui.Stack{Children: []ui.Element{
		ui.Backdrop{Color: color.RGBA{A: 80}},
		ui.Align{
			Horizontal: ui.AlignCenter,
			Vertical:   ui.AlignCenter,
			Child: ui.Constrained{
				MaxW: panelW,
				Child: ui.Panel{
					Fill:   ctx.Theme.ModalFill,
					Stroke: ctx.Theme.ModalStroke,
					Insets: ui.UniformInsets(22),
					Child: ui.Column{
						Children: []ui.Child{
							ui.Fixed(ui.Label{Text: "H265 Is Not Supported", Size: 20, Color: ctx.Theme.Title}),
							ui.Fixed(ui.Spacer{H: 12}),
							ui.Fixed(ui.Paragraph{
								Text:  "This desktop client cannot decode H265/HEVC video yet. If you continue, the JetKVM may switch to H265 and video may stop until you change it back.",
								Size:  13,
								Color: ctx.Theme.Body,
							}),
							ui.Fixed(ui.Spacer{H: 16}),
							ui.Fixed(ui.Wrap{Children: []ui.Element{
								settingsActionButton("Switch To H265", settingsActionVisual{Enabled: !e.app.settingsActionPending(settingsGroupVideoCodec)}, 132, e.app.confirmH265CodecAction),
								settingsActionButton("Cancel", settingsActionVisual{Enabled: !e.app.settingsActionPending(settingsGroupVideoCodec)}, 84, func() { e.app.h265ConfirmOpen = false }),
							}, Spacing: 12, LineSpacing: 8}),
						},
					},
				},
			},
		},
	}}.Draw(ctx, bounds)
}

type settingsAccessEditorModalElement struct {
	app *App
}

func (settingsAccessEditorModalElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e settingsAccessEditorModalElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	title, body := e.app.settingsAccessEditorContent(e.app.settingsActionPending(settingsGroupLocalAuth))
	if body == nil {
		return
	}
	if ctx.Runtime != nil {
		ctx.Runtime.Register(ui.Control{
			ID:      "access_modal_backdrop",
			Rect:    bounds,
			Enabled: true,
			OnClick: func(ui.PointerEvent) {},
		})
	}
	panelW := min(520, bounds.W-56)
	ui.Stack{Children: []ui.Element{
		ui.Backdrop{Color: color.RGBA{A: 80}},
		ui.Align{
			Horizontal: ui.AlignCenter,
			Vertical:   ui.AlignCenter,
			Child: ui.Constrained{
				MaxW: panelW,
				Child: ui.Panel{
					Fill:   ctx.Theme.ModalFill,
					Stroke: ctx.Theme.ModalStroke,
					Insets: ui.UniformInsets(22),
					Child: ui.Column{
						Children: []ui.Child{
							ui.Fixed(ui.Label{Text: title, Size: 20, Color: ctx.Theme.Title}),
							ui.Fixed(ui.Spacer{H: 14}),
							ui.Fixed(body),
						},
					},
				},
			},
		},
	}}.Draw(ctx, bounds)
}

type settingsATXConfirmElement struct {
	app *App
}

func (settingsATXConfirmElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e settingsATXConfirmElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	if e.app.atxConfirmAction == "" {
		return
	}
	if ctx.Runtime != nil {
		ctx.Runtime.Register(ui.Control{
			ID:      "atx_modal_backdrop",
			Rect:    bounds,
			Enabled: true,
			OnClick: func(ui.PointerEvent) {},
		})
	}
	title := "Confirm ATX Action"
	message := "This action will be sent to the attached host machine through the ATX board."
	confirmLabel := "Continue"
	switch e.app.atxConfirmAction {
	case session.ATXPowerActionReset:
		title = "Reset Host Machine"
		message = "This sends a reset-button press to the attached host machine. JetKVM itself will stay online."
		confirmLabel = "Reset Host"
	case session.ATXPowerActionLongPress:
		title = "Long Press Power Button"
		message = "This sends a long power-button press to the attached host machine to force it off. JetKVM itself will stay online."
		confirmLabel = "Force Off Host"
	}
	panelW := min(520, bounds.W-56)
	ui.Stack{Children: []ui.Element{
		ui.Backdrop{Color: color.RGBA{A: 80}},
		ui.Align{
			Horizontal: ui.AlignCenter,
			Vertical:   ui.AlignCenter,
			Child: ui.Constrained{
				MaxW: panelW,
				Child: ui.Panel{
					Fill:   ctx.Theme.ModalFill,
					Stroke: ctx.Theme.ModalStroke,
					Insets: ui.UniformInsets(22),
					Child: ui.Column{
						Children: []ui.Child{
							ui.Fixed(ui.Label{Text: title, Size: 20, Color: ctx.Theme.Title}),
							ui.Fixed(ui.Spacer{H: 12}),
							ui.Fixed(ui.Paragraph{Text: message, Size: 13, Color: ctx.Theme.Body}),
							ui.Fixed(ui.Spacer{H: 16}),
							ui.Fixed(ui.Wrap{Children: []ui.Element{
								settingsActionButton(confirmLabel, settingsActionVisual{Enabled: !e.app.settingsActionPending(settingsGroupATXPower)}, 132, e.app.confirmATXAction),
								settingsActionButton("Cancel", settingsActionVisual{Enabled: !e.app.settingsActionPending(settingsGroupATXPower)}, 84, e.app.cancelATXAction),
							}, Spacing: 12, LineSpacing: 8}),
						},
					},
				},
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
		ui.Fixed(ui.Label{Text: "Settings", Size: 20, Color: ctx.Theme.Title}),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{
			Text:  fallbackLabel(e.snap.DeviceID, e.snap.Hostname, e.snap.BaseURL),
			Size:  11,
			Color: ctx.Theme.Muted,
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

type settingsHeaderBlock struct {
	title       string
	description string
}

func (e settingsHeaderBlock) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return ui.Column{Children: []ui.Child{
		ui.Fixed(ui.Label{Text: e.title, Size: 22, Color: ctx.Theme.Title}),
		ui.Fixed(ui.Spacer{H: 6}),
		ui.Fixed(ui.Constrained{MaxW: 640, Child: ui.Paragraph{Text: e.description, Size: 12, Color: ctx.Theme.Muted}}),
	}}.Measure(ctx, constraints)
}

func (e settingsHeaderBlock) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Column{Children: []ui.Child{
		ui.Fixed(ui.Label{Text: e.title, Size: 22, Color: ctx.Theme.Title}),
		ui.Fixed(ui.Spacer{H: 6}),
		ui.Fixed(ui.Constrained{MaxW: 640, Child: ui.Paragraph{Text: e.description, Size: 12, Color: ctx.Theme.Muted}}),
	}}.Draw(ctx, bounds)
}

func settingsHeaderElement(title, description string) ui.Element {
	return settingsHeaderBlock{title: title, description: description}
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
	case sectionATX:
		a.sectionData.ATX.Loading = true
		a.sectionData.ATX.Error = ""
	case sectionVideo:
		a.sectionData.Video.Loading = true
		a.sectionData.Video.Error = ""
	case sectionHardware:
		a.sectionData.Hardware.Loading = true
		a.sectionData.Hardware.Error = ""
	case sectionNetwork:
		a.sectionData.Network.Loading = true
		a.sectionData.Network.Error = ""
	case sectionMacros:
		a.sectionData.Macros.Loading = true
		a.sectionData.Macros.Error = ""
	case sectionMQTT:
		a.sectionData.MQTT.Loading = true
		a.sectionData.MQTT.Error = ""
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
		if update, callErr := a.ctrl.GetUpdateStatus(ctx); callErr == nil {
			state.Update = update
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
	case sectionVideo:
		state := videoState{Loading: false}
		if codec, callErr := a.ctrl.GetVideoCodec(ctx); callErr == nil {
			state.State.Codec = codec
		}
		if edid, callErr := a.ctrl.GetEDID(ctx); callErr == nil {
			state.State.EDID = edid
		}
		if state.State.Codec == session.VideoCodecUnknown && state.State.EDID == "" {
			state.Error = "No video RPC state available on this target"
			err = errors.New(state.Error)
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.Video = state
			a.syncCustomEDIDLocked(state.State.EDID)
		}
		a.mu.Unlock()
	case sectionAccess:
		state := accessState{Loading: false}
		if authMode, loopbackOnly, callErr := a.ctrl.GetLocalAccessState(ctx); callErr == nil {
			state.State.LocalAuthMode = authMode
			state.State.LoopbackOnly = loopbackOnly
		}
		if cloud, callErr := a.ctrl.GetCloudState(ctx); callErr == nil {
			state.State.Cloud = cloud
		}
		if tlsState, callErr := a.ctrl.GetTLSState(ctx); callErr == nil {
			state.State.TLS = tlsState
		}
		if state.State.LocalAuthMode == session.LocalAuthModeUnknown && state.State.Cloud.URL == "" && state.State.TLS.Mode == session.TLSModeUnknown {
			state.Error = "No access RPC state available on this target"
			err = errors.New(state.Error)
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.Access = state
			a.syncTLSEditorLocked(state.State.TLS)
		}
		a.mu.Unlock()
	case sectionATX:
		state := atxState{Loading: false}
		loaded := false
		if activeExtension, callErr := a.ctrl.GetActiveExtension(ctx); callErr == nil {
			state.ActiveExtension = activeExtension
			loaded = true
		}
		if state.ActiveExtension == "atx-power" {
			if atxCurrent, callErr := a.ctrl.GetATXState(ctx); callErr == nil {
				state.State = atxCurrent
			}
		}
		if !loaded && state.State == nil {
			state.Error = "No ATX RPC state available on this target"
			err = errors.New(state.Error)
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.ATX = state
		}
		a.mu.Unlock()
	case sectionHardware:
		state := hardwareState{Loading: false}
		log := logging.Subsystem("app")
		if usbEnabled, callErr := a.ctrl.GetUSBEmulationState(ctx); callErr == nil {
			state.State.USBEmulation = &usbEnabled
		} else {
			log.Debug().Err(callErr).Str("section", string(section)).Msg("failed to load USB emulation state")
		}
		if usbConfig, callErr := a.ctrl.GetUSBConfig(ctx); callErr == nil {
			state.State.USBConfig = usbConfig
			state.USBConfigLoaded = true
		} else {
			log.Debug().Err(callErr).Str("section", string(section)).Msg("failed to load USB config")
		}
		if devices, loaded := a.connectionUSBDevicesSnapshot(); loaded {
			state.State.USBDevices = devices
			state.State.USBDeviceCount = usbDeviceCount(devices)
			state.USBDevicesLoaded = true
		} else if devices, callErr := a.ctrl.GetUSBDevices(ctx); callErr == nil {
			state.State.USBDevices = devices
			state.State.USBDeviceCount = usbDeviceCount(devices)
			state.USBDevicesLoaded = true
			a.setConnectionUSBDevices(devices)
		} else {
			log.Debug().Err(callErr).Str("section", string(section)).Msg("failed to load USB device set")
		}
		if a.cfg.ExperimentalUSBNetwork {
			if usbNetwork, callErr := a.ctrl.GetUSBNetworkConfig(ctx); callErr == nil {
				state.State.USBNetwork = &usbNetwork
				a.mu.Lock()
				if a.sectionLoadSeq[section] == seq {
					a.syncUSBNetworkEditorLocked(usbNetwork)
				}
				a.mu.Unlock()
			} else {
				log.Debug().Err(callErr).Str("section", string(section)).Msg("failed to load USB network config")
			}
		}
		if rotation, callErr := a.ctrl.GetDisplayRotation(ctx); callErr == nil {
			state.State.DisplayRotation = rotation
			state.DisplayRotationLoaded = true
		} else {
			log.Debug().Err(callErr).Str("section", string(section)).Msg("failed to load display rotation")
		}
		if backlight, callErr := a.ctrl.GetBacklightSettings(ctx); callErr == nil {
			state.State.Backlight = backlight
			state.BacklightLoaded = true
		} else {
			log.Debug().Err(callErr).Str("section", string(section)).Msg("failed to load backlight settings")
		}
		if sleepMode, callErr := a.ctrl.GetVideoSleepMode(ctx); callErr == nil {
			state.State.VideoSleepMode = sleepMode
			state.VideoSleepLoaded = true
		} else {
			log.Debug().Err(callErr).Str("section", string(section)).Msg("failed to load HDMI sleep mode")
		}
		if state.State.USBEmulation == nil &&
			!state.USBConfigLoaded &&
			!state.USBDevicesLoaded &&
			!state.DisplayRotationLoaded &&
			!state.BacklightLoaded &&
			!state.VideoSleepLoaded {
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
			a.mu.Lock()
			if a.sectionLoadSeq[section] == seq {
				a.syncNetworkEditorLocked(settings)
			}
			a.mu.Unlock()
			state.Settings = settings
		}
		if current, callErr := a.ctrl.GetNetworkState(ctx); callErr == nil {
			state.State = current
		}
		if addresses, callErr := a.ctrl.GetPublicIPAddresses(ctx, false); callErr == nil {
			state.PublicIPs = addresses
		} else {
			state.PublicIPError = callErr.Error()
		}
		if tailscale, callErr := a.ctrl.GetTailscaleStatus(ctx); callErr == nil {
			state.Tailscale = tailscale
		} else {
			state.TailscaleError = callErr.Error()
		}
		if state.Settings.Hostname == "" && state.State.Hostname == "" && state.State.InterfaceName == "" {
			state.Error = "No network RPC state available on this target"
			err = errors.New(state.Error)
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.Network = state
		}
		a.mu.Unlock()
	case sectionMacros:
		state := macrosState{Loading: false}
		if macros, callErr := a.ctrl.GetKeyboardMacros(ctx); callErr == nil {
			state.Macros = macros
		}
		if state.Macros == nil {
			state.Macros = []session.KeyboardMacro{}
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.Macros = state
		}
		a.mu.Unlock()
	case sectionMQTT:
		state := mqttState{Loading: false}
		if settings, callErr := a.ctrl.GetMQTTSettings(ctx); callErr == nil {
			state.Settings = settings
		}
		if status, callErr := a.ctrl.GetMQTTStatus(ctx); callErr == nil {
			state.Status = status
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.MQTT = state
			a.syncMQTTEditorLocked(state.Settings)
		}
		a.mu.Unlock()
	case sectionAdvanced:
		state := advancedState{Loading: false}
		if devMode, callErr := a.ctrl.GetDeveloperModeState(ctx); callErr == nil {
			state.State.DevMode = devMode
		}
		if devChannel, callErr := a.ctrl.GetDevChannelState(ctx); callErr == nil {
			state.State.DevChannel = devChannel
		}
		if loopbackOnly, callErr := a.ctrl.GetLocalLoopbackOnly(ctx); callErr == nil {
			state.State.LoopbackOnly = loopbackOnly
		}
		if usbEnabled, callErr := a.ctrl.GetUSBEmulationState(ctx); callErr == nil {
			state.State.USBEmulation = &usbEnabled
		}
		if sshKey, callErr := a.ctrl.GetSSHKeyState(ctx); callErr == nil {
			state.State.SSHKey = sshKey
		}
		if version, callErr := a.ctrl.GetLocalVersion(ctx); callErr == nil {
			state.State.Version = version
		}
		if state.State.DevMode == nil && state.State.DevChannel == nil && state.State.LoopbackOnly == nil && state.State.USBEmulation == nil && state.State.Version.AppVersion == "" && state.State.SSHKey == "" {
			state.Error = "No advanced RPC state available on this target"
			err = errors.New(state.Error)
		}
		a.mu.Lock()
		if a.sectionLoadSeq[section] == seq {
			a.sectionData.Advanced = state
			a.syncAdvancedSSHKeyLocked(state.State.SSHKey)
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
	ctx := a.newUIContext(nil, nil, func(chromeButton) {})
	size := body.Measure(ctx, ui.Constraints{MaxW: width})
	return size.H
}

type settingsCardBlock struct {
	title string
	body  ui.Element
}

func (e settingsCardBlock) bodyElement(ctx *ui.Context) ui.Element {
	children := make([]ui.Child, 0, 3)
	if e.title != "" {
		children = append(children,
			ui.Fixed(ui.Label{Text: e.title, Size: 15, Color: ctx.Theme.Title}),
			ui.Fixed(ui.Spacer{H: 12}),
		)
	}
	if e.body != nil {
		children = append(children, ui.Fixed(e.body))
	}
	return ui.Panel{
		Fill:   ctx.Theme.PanelFill,
		Stroke: ctx.Theme.PanelStroke,
		Insets: ui.UniformInsets(16),
		Child:  ui.Column{Children: children},
	}
}

func (e settingsCardBlock) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return e.bodyElement(ctx).Measure(ctx, constraints)
}

func (e settingsCardBlock) Draw(ctx *ui.Context, bounds ui.Rect) {
	e.bodyElement(ctx).Draw(ctx, bounds)
}

func settingsCardElement(title string, body ui.Element) ui.Element {
	return settingsCardBlock{title: title, body: body}
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

func settingsActionButton(label string, visual settingsActionVisual, width float64, onClick func()) ui.Element {
	return ui.Button{
		Label:   label,
		Enabled: visual.Enabled,
		Active:  visual.Active,
		Pending: visual.Pending,
		Width:   width,
		OnClick: onClick,
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

func settingsToggleControl(visual settingsActionVisual, onClick func()) ui.Element {
	return ui.Toggle{
		Enabled: visual.Enabled,
		Active:  visual.Active,
		Pending: visual.Pending,
		OnClick: onClick,
	}
}

type settingsToggleRow struct {
	id      string
	label   string
	visual  settingsActionVisual
	onClick func()
}

func (e settingsToggleRow) row(ctx *ui.Context, interactive bool) ui.Element {
	active := e.visual.Active
	if interactive && ctx.Runtime != nil && e.id != "" {
		active = ctx.Runtime.ToggleValue(e.id, e.visual.Active)
	}
	toggle := func() ui.Element {
		if !interactive {
			return ui.Toggle{
				Enabled: e.visual.Enabled,
				Active:  active,
				Pending: e.visual.Pending,
			}
		}
		if e.onClick != nil {
			return settingsToggleControl(settingsActionVisual{
				Enabled: e.visual.Enabled,
				Active:  active,
				Pending: e.visual.Pending,
			}, e.onClick)
		}
		return settingsToggleElement(e.id, settingsActionVisual{
			Enabled: e.visual.Enabled,
			Active:  active,
			Pending: e.visual.Pending,
		})
	}()
	return ui.Row{
		AlignY: ui.AlignCenter,
		Children: []ui.Child{
			ui.Fixed(toggle),
			ui.Fixed(ui.Spacer{W: 12}),
			ui.Flex(ui.Label{Text: e.label, Size: 13, Color: ctx.Theme.Body}, 1),
		},
	}
}

func (e settingsToggleRow) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return e.row(ctx, false).Measure(ctx, constraints)
}

func (e settingsToggleRow) Draw(ctx *ui.Context, bounds ui.Rect) {
	if ctx.Runtime != nil {
		var onClick func()
		switch {
		case e.onClick != nil:
			onClick = e.onClick
		case e.id != "" && ctx.OnAction != nil:
			onClick = func() { ctx.OnAction(e.id) }
		}
		if onClick != nil {
			ctx.Runtime.Register(ui.Control{
				ID:      e.id,
				Rect:    bounds,
				Enabled: e.visual.Enabled,
				OnClick: func(ui.PointerEvent) {
					if e.id != "" {
						ctx.Runtime.SetToggleValue(e.id, !ctx.Runtime.ToggleValue(e.id, e.visual.Active))
					}
					onClick()
				},
			})
		}
	}
	e.row(ctx, true).Draw(ctx, bounds)
}

func settingsToggleRowElement(id, label string, visual settingsActionVisual) ui.Element {
	return settingsToggleRow{id: id, label: label, visual: visual}
}

func settingsToggleRowControl(label string, visual settingsActionVisual, onClick func()) ui.Element {
	return settingsToggleRow{label: label, visual: visual, onClick: onClick}
}

type settingsSliderRow struct {
	id       string
	label    string
	value    string
	min      float64
	max      float64
	step     float64
	current  float64
	enabled  bool
	onChange func(float64)
	onCommit func(float64)
}

func (e settingsSliderRow) content(ctx *ui.Context) ui.Element {
	valueColor := ctx.Theme.Muted
	if e.enabled {
		valueColor = ctx.Theme.AccentText
	}
	return ui.Column{
		Spacing: 8,
		Children: []ui.Child{
			ui.Fixed(ui.Row{
				AlignY: ui.AlignCenter,
				Children: []ui.Child{
					ui.Flex(ui.Label{Text: e.label, Size: 13, Color: ctx.Theme.Body}, 1),
					ui.Fixed(ui.Label{Text: e.value, Size: 12, Color: valueColor}),
				},
			}),
			ui.Fixed(ui.Slider{
				ID:       e.id,
				Value:    e.current,
				Min:      e.min,
				Max:      e.max,
				Step:     e.step,
				Enabled:  e.enabled,
				OnChange: e.onChange,
				OnCommit: e.onCommit,
			}),
		},
	}
}

func (e settingsSliderRow) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return e.content(ctx).Measure(ctx, constraints)
}

func (e settingsSliderRow) Draw(ctx *ui.Context, bounds ui.Rect) {
	e.content(ctx).Draw(ctx, bounds)
}

func settingsSliderRowElement(id, label, value string, current, minValue, maxValue, step float64, enabled bool, onChange, onCommit func(float64)) ui.Element {
	return settingsSliderRow{
		id:       id,
		label:    label,
		value:    value,
		min:      minValue,
		max:      maxValue,
		step:     step,
		current:  current,
		enabled:  enabled,
		onChange: onChange,
		onCommit: onCommit,
	}
}

func settingsStatusElement(text string, clr color.Color) ui.Element {
	if text == "" {
		return nil
	}
	return ui.Paragraph{Text: text, Size: 12, Color: clr}
}

type settingsSectionLabel struct {
	label string
}

func (e settingsSectionLabel) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return ui.Label{Text: e.label, Size: 12, Color: ctx.Theme.Muted}.Measure(ctx, constraints)
}

func (e settingsSectionLabel) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Label{Text: e.label, Size: 12, Color: ctx.Theme.Muted}.Draw(ctx, bounds)
}

func settingsSectionLabelElement(label string) ui.Element {
	return settingsSectionLabel{label: label}
}

func settingsKeyValueElement(label, value string, split float64) ui.Element {
	return ui.KeyValue{
		Label:      label,
		Value:      fallbackLabel(value, "Unavailable"),
		LabelWidth: split - 12,
	}
}

func throttleLabel(value time.Duration) string {
	if value <= 0 {
		return "Off"
	}
	return fmt.Sprintf("%d ms", value/time.Millisecond)
}

func (a *App) settingsMouseBody(snap session.Snapshot) ui.Element {
	a.mu.RLock()
	state := a.sectionData.Mouse
	a.mu.RUnlock()
	jiggler := a.settingsAction(settingsGroupJiggler)
	scrollThrottleMs := float64(a.scrollThrottle / time.Millisecond)
	pointerThrottleMs := float64(a.pointerMoveThrottle / time.Millisecond)
	updateScrollThrottle := func(value float64) {
		a.scrollThrottle = throttleDurationFromMs(int(value))
	}
	updatePointerThrottle := func(value float64) {
		a.pointerMoveThrottle = throttleDurationFromMs(int(value))
	}
	saveThrottlePrefs := func(float64) {
		a.savePreferences()
	}

	leftChildren := []ui.Child{
		ui.Fixed(settingsSectionLabelElement("Remote mode")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{
			Children: []ui.Element{
				settingsActionButton("Absolute", settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected, Active: !a.relative}, 110, func() { a.setMouseRelative(false) }),
				settingsActionButton("Relative", settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected, Active: a.relative}, 110, func() { a.setMouseRelative(true) }),
			},
			Spacing:     12,
			LineSpacing: 8,
		}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsSectionLabelElement("Local cursor")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsToggleRowControl("Hide Host Cursor", settingsActionVisual{Enabled: true, Active: a.hideCursor}, func() {
			a.hideCursor = !a.hideCursor
			a.applyCursorMode()
			a.savePreferences()
		})),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsSectionLabelElement("Wheel")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{
			Text:  "Throttle local wheel bursts before sending them to the device.",
			Size:  12,
			Color: a.currentTheme().Muted,
		}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(settingsSliderRowElement(settingsScrollThrottleSliderID, "Wheel Throttle", throttleLabel(a.scrollThrottle), scrollThrottleMs, 0, maxScrollThrottleMs, 5, true, updateScrollThrottle, saveThrottlePrefs)),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsSectionLabelElement("Pointer")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{
			Text:  "Coalesce movement reports before sending them to the device. Button changes still flush the latest position immediately.",
			Size:  12,
			Color: a.currentTheme().Muted,
		}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(settingsSliderRowElement(settingsPointerThrottleSliderID, "Movement Throttle", throttleLabel(a.pointerMoveThrottle), pointerThrottleMs, 0, maxPointerMoveThrottleMs, 1, true, updatePointerThrottle, saveThrottlePrefs)),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsToggleRowControl("Invert Scroll", settingsActionVisual{Enabled: true, Active: a.invertScroll}, func() {
			a.invertScroll = !a.invertScroll
			a.savePreferences()
		})),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsSectionLabelElement("Compatibility")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{
			Text:  "Reroute Back and Forward side-button presses through the relative mouse gadget while staying in absolute mode. Use this when the host ignores side buttons from the absolute pointer device.",
			Size:  12,
			Color: a.currentTheme().Muted,
		}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(settingsToggleRowControl("Reroute Side Buttons in Absolute Mode", settingsActionVisual{Enabled: true, Active: a.prefs.AbsoluteSideButtonsViaRel}, func() {
			a.prefs.AbsoluteSideButtonsViaRel = !a.prefs.AbsoluteSideButtonsViaRel
			a.savePreferences()
		})),
	}

	rightChildren := []ui.Child{}
	if state.Loading {
		rightChildren = append(rightChildren,
			ui.Fixed(ui.Label{Text: "Loading jiggler state…", Size: 13, Color: a.currentTheme().Body}),
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
				Color: a.currentTheme().Muted,
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
				ui.Fixed(a.decorateTextField(ui.TextField{
					ID:          "jiggler_focus_cron",
					Value:       a.jigglerEditorConfig.ScheduleCronTab,
					Placeholder: "0 * * * * *",
					Focused:     a.settingsInputFocus == settingsInputJigglerCron,
					Enabled:     true,
				})),
				ui.Fixed(ui.Spacer{H: 16}),
				ui.Fixed(settingsSectionLabelElement("Timezone")),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{
					ID:          "jiggler_focus_timezone",
					Value:       a.jigglerEditorConfig.Timezone,
					Placeholder: "UTC",
					Focused:     a.settingsInputFocus == settingsInputJigglerTimezone,
					Enabled:     true,
				})),
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
		statusColor := a.currentTheme().WarningStroke
		switch {
		case jiggler.Pending:
			statusText = "Applying…"
		case jiggler.Error != "":
			statusText = jiggler.Error
			statusColor = a.currentTheme().Error
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
				ui.Fixed(settingsStatusElement(a.jigglerEditorError, a.currentTheme().Error)),
			)
		}
	}
	if state.Error != "" {
		rightChildren = append(rightChildren,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(settingsStatusElement(state.Error, a.currentTheme().Error)),
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
	case sectionATX:
		return a.settingsATXBody(snap)
	case sectionAccess:
		return a.settingsAccessBody()
	case sectionAppearance:
		return a.settingsAppearanceBody()
	case sectionMacros:
		return a.settingsMacrosBody()
	case sectionNetwork:
		return a.settingsNetworkBody()
	case sectionMQTT:
		return a.settingsMQTTBody()
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
	deviceCard := settingsCardElement("Device", ui.Column{Children: []ui.Child{
		ui.Fixed(settingsKeyValueElement("Base URL", snap.BaseURL, 116)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("Phase", snap.Phase.String(), 116)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("Signaling", signalingLabel(snap.SignalingMode), 116)),
	}})
	updateChildren := []ui.Child{
		ui.Fixed(ui.Row{
			AlignY: ui.AlignStart,
			Children: []ui.Child{
				ui.Flex(ui.Column{Children: []ui.Child{
					ui.Fixed(settingsSectionLabelElement("Current version")),
					ui.Fixed(ui.Spacer{H: 8}),
					ui.Fixed(settingsKeyValueElement("App", fallbackLabel(state.Update.Local.AppVersion, snap.AppVersion), 72)),
					ui.Fixed(ui.Spacer{H: 10}),
					ui.Fixed(settingsKeyValueElement("System", fallbackLabel(state.Update.Local.SystemVersion, snap.SystemVersion), 72)),
				}}, 1),
				ui.Fixed(ui.Spacer{W: 20}),
				ui.Flex(ui.Column{Children: []ui.Child{
					ui.Fixed(settingsSectionLabelElement("Detected latest")),
					ui.Fixed(ui.Spacer{H: 8}),
					ui.Fixed(settingsKeyValueElement("App", fallbackLabel(state.Update.Remote.AppVersion, "Unavailable"), 72)),
					ui.Fixed(ui.Spacer{H: 10}),
					ui.Fixed(settingsKeyValueElement("System", fallbackLabel(state.Update.Remote.SystemVersion, "Unavailable"), 72)),
				}}, 1),
			},
		}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsActionButton("Check for updates", settingsActionVisual{
			Enabled: !a.settingsActionPending(settingsGroupUpdateStatus),
		}, 152, func() {
			if a.settingsActionPending(settingsGroupUpdateStatus) {
				return
			}
			a.withSettingsAction(settingsGroupUpdateStatus, "refresh", func() error {
				a.updateActionMessage = ""
				return a.refreshSettingsSectionSync(sectionGeneral)
			})
		})),
	}
	if snap.AppUpdateAvailable || snap.SystemUpdateAvailable {
		updateChildren = append(updateChildren,
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(settingsActionButton("Install updates", settingsActionVisual{
				Enabled: !a.settingsActionPending(settingsGroupUpdateInstall),
			}, 132, a.invokeInstallUpdates)),
		)
	}
	autoUpdate := a.settingsAction(settingsGroupAutoUpdate)
	updateChildren = append(updateChildren, ui.Fixed(ui.Spacer{H: 18}), ui.Fixed(settingsSectionLabelElement("Auto updates")), ui.Fixed(ui.Spacer{H: 8}))
	if state.Loading {
		updateChildren = append(updateChildren, ui.Fixed(ui.Label{Text: "Loading…", Size: 12, Color: a.currentTheme().Muted}))
	} else {
		updateChildren = append(updateChildren,
			ui.Fixed(ui.Row{AlignY: ui.AlignCenter, Children: []ui.Child{
				ui.Fixed(settingsToggleControl(settingsActionVisual{
					Enabled: state.AutoUpdate != nil && !autoUpdate.Pending,
					Active:  state.AutoUpdate != nil && *state.AutoUpdate,
					Pending: autoUpdate.Pending,
				}, func() {
					if state.AutoUpdate == nil || autoUpdate.Pending {
						return
					}
					next := !*state.AutoUpdate
					a.withSettingsAction(settingsGroupAutoUpdate, strconv.FormatBool(next), func() error {
						if err := a.ctrl.SetAutoUpdateState(next); err != nil {
							return err
						}
						return a.refreshSettingsSectionSync(sectionGeneral)
					})
				})),
				ui.Fixed(ui.Spacer{W: 12}),
				ui.Fixed(ui.Label{Text: "Enabled", Size: 13, Color: a.currentTheme().Body}),
			}}),
		)
		switch {
		case autoUpdate.Pending:
			updateChildren = append(updateChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", a.currentTheme().WarningStroke)))
		case autoUpdate.Error != "":
			updateChildren = append(updateChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(autoUpdate.Error, a.currentTheme().Error)))
		}
	}
	updateState := a.settingsAction(settingsGroupUpdateStatus)
	switch {
	case updateState.Pending:
		updateChildren = append(updateChildren,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(ui.Row{
				AlignY: ui.AlignCenter,
				Children: []ui.Child{
					ui.Fixed(ui.Spinner{Size: 14, Color: a.currentTheme().AccentText}),
					ui.Fixed(ui.Spacer{W: 10}),
					ui.Fixed(ui.Label{Text: "Checking latest versions…", Size: 12, Color: a.currentTheme().AccentText}),
				},
			}),
		)
	case updateState.Error != "":
		updateChildren = append(updateChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(updateState.Error, a.currentTheme().Error)))
	}
	installState := a.settingsAction(settingsGroupUpdateInstall)
	switch {
	case installState.Pending:
		updateChildren = append(updateChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Starting update…", a.currentTheme().WarningStroke)))
	case installState.Error != "":
		updateChildren = append(updateChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(installState.Error, a.currentTheme().Error)))
	case a.updateActionMessage != "":
		msgColor := a.currentTheme().WarningStroke
		if a.updateActionSuccess {
			msgColor = color.RGBA{R: 134, G: 239, B: 172, A: 255}
		}
		updateChildren = append(updateChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(a.updateActionMessage, msgColor)))
	}
	updatesCard := settingsCardElement("Updates", ui.Column{Children: updateChildren})
	actionChildren := []ui.Child{
		ui.Fixed(ui.Paragraph{Text: "Reconnect the native session or force a device reboot.", Size: 12, Color: a.currentTheme().Muted}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsActionButton(reconnectLabel(snap.Phase), settingsActionVisual{Enabled: true}, 0, func() {
			if a.ctrl == nil {
				return
			}
			a.releaseAllKeys(true)
			a.ctrl.ReconnectNow()
		})),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsActionButton("Reboot device", settingsActionVisual{Enabled: snap.Phase != session.PhaseConnecting}, 0, func() {
			a.runAsync(func() {
				_ = a.ctrl.Reboot()
			})
		})),
	}
	if state.Error != "" {
		actionChildren = append(actionChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, a.currentTheme().Error)))
	}
	actionsCard := settingsCardElement("Actions", ui.Column{Children: actionChildren})
	return ui.Column{
		Children: []ui.Child{
			ui.Fixed(settingsTwoPane(deviceCard, 58, actionsCard, 42)),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(updatesCard),
		},
	}
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
		option := option
		buttons = append(buttons, settingsActionButton(option.Label, settingsActionVisual{
			Enabled: snap.Phase == session.PhaseConnected && (!layoutState.Pending || layoutState.PendingChoice == option.Code),
			Active:  layout == option.Code,
			Pending: layoutState.Pending && layoutState.PendingChoice == option.Code,
		}, btnW, func() {
			if a.settingsActionPending(settingsGroupKeyboardLayout) {
				return
			}
			a.withSettingsAction(settingsGroupKeyboardLayout, option.Code, func() error {
				return a.ctrl.SetKeyboardLayout(option.Code)
			})
		}))
	}
	children := []ui.Child{
		ui.Fixed(ui.Paragraph{Text: "This layout affects paste and keyboard macros. Live typing is sent as physical HID keys.", Size: 12, Color: a.currentTheme().Muted}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(ui.Row{Children: []ui.Child{
			ui.Fixed(settingsKeyValueElement("Active layout", keyboardLayoutLabel(layout), 118)),
			ui.Flex(ui.Spacer{}, 1),
		}, Spacing: 12}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsToggleRowControl("Show Pressed Keys", settingsActionVisual{Enabled: true, Active: a.showPressedKeys}, func() {
			a.showPressedKeys = !a.showPressedKeys
			a.savePreferences()
		})),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsSectionLabelElement("Layout presets")),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(ui.Wrap{Children: buttons, Spacing: 10, LineSpacing: 8}),
	}
	switch {
	case layoutState.Pending:
		children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", a.currentTheme().WarningStroke)))
	case layoutState.Error != "":
		children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(layoutState.Error, a.currentTheme().Error)))
	}
	children = append(children,
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Paragraph{Text: "Make this match the remote OS only for pasted text and macros.", Size: 13, Color: a.currentTheme().Muted}),
	)
	return settingsCardElement("", ui.Column{Children: children})
}

func (a *App) settingsVideoBody(snap session.Snapshot) ui.Element {
	a.mu.RLock()
	state := a.sectionData.Video
	a.mu.RUnlock()
	qualityState := a.settingsAction(settingsGroupVideoQuality)
	streamChildren := []ui.Child{
		ui.Fixed(settingsSectionLabelElement("Quality preset")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionButton("High", settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected && (!qualityState.Pending || qualityState.PendingChoice == "high"), Active: snap.Quality >= 0.95, Pending: qualityState.Pending && qualityState.PendingChoice == "high"}, 96, func() {
				if a.settingsActionPending(settingsGroupVideoQuality) {
					return
				}
				a.withSettingsAction(settingsGroupVideoQuality, "high", func() error { return a.ctrl.SetQuality(1.0) })
			}),
			settingsActionButton("Medium", settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected && (!qualityState.Pending || qualityState.PendingChoice == "medium"), Active: snap.Quality >= 0.45 && snap.Quality < 0.95, Pending: qualityState.Pending && qualityState.PendingChoice == "medium"}, 96, func() {
				if a.settingsActionPending(settingsGroupVideoQuality) {
					return
				}
				a.withSettingsAction(settingsGroupVideoQuality, "medium", func() error { return a.ctrl.SetQuality(0.5) })
			}),
			settingsActionButton("Low", settingsActionVisual{Enabled: snap.Phase == session.PhaseConnected && (!qualityState.Pending || qualityState.PendingChoice == "low"), Active: snap.Quality < 0.45, Pending: qualityState.Pending && qualityState.PendingChoice == "low"}, 96, func() {
				if a.settingsActionPending(settingsGroupVideoQuality) {
					return
				}
				a.withSettingsAction(settingsGroupVideoQuality, "low", func() error { return a.ctrl.SetQuality(0.1) })
			}),
		}, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Label{Text: fmt.Sprintf("Current factor %.2f", snap.Quality), Size: 13, Color: a.currentTheme().Body}),
	}
	codecState := a.settingsAction(settingsGroupVideoCodec)
	streamChildren = append(streamChildren,
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsSectionLabelElement("Codec preference")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionButton("Auto", settingsActionVisual{Enabled: !codecState.Pending || codecState.PendingChoice == "auto", Active: state.State.Codec == session.VideoCodecAuto, Pending: codecState.Pending && codecState.PendingChoice == "auto"}, 72, func() {
				a.h265ConfirmOpen = false
				a.invokeVideoCodecAction("auto", session.VideoCodecAuto)
			}),
			settingsActionButton("H265", settingsActionVisual{Enabled: !codecState.Pending || codecState.PendingChoice == "h265", Active: state.State.Codec == session.VideoCodecH265, Pending: codecState.Pending && codecState.PendingChoice == "h265"}, 72, a.openH265CodecConfirm),
			settingsActionButton("H264", settingsActionVisual{Enabled: !codecState.Pending || codecState.PendingChoice == "h264", Active: state.State.Codec == session.VideoCodecH264, Pending: codecState.Pending && codecState.PendingChoice == "h264"}, 72, func() {
				a.h265ConfirmOpen = false
				a.invokeVideoCodecAction("h264", session.VideoCodecH264)
			}),
		}, Spacing: 12, LineSpacing: 8}),
	)
	switch {
	case qualityState.Pending:
		streamChildren = append(streamChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", a.currentTheme().WarningStroke)))
	case qualityState.Error != "":
		streamChildren = append(streamChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(qualityState.Error, a.currentTheme().Error)))
	}
	switch {
	case codecState.Pending:
		streamChildren = append(streamChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Updating codec…", a.currentTheme().WarningStroke)))
	case codecState.Error != "":
		streamChildren = append(streamChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(codecState.Error, a.currentTheme().Error)))
	}
	edid := state.State.EDID
	if edid == "" {
		edid = snap.EDID
	}
	if edid == "" {
		edid = "Unavailable on current target"
	}
	edidState := a.settingsAction(settingsGroupVideoEDID)
	edidChildren := []ui.Child{
		ui.Fixed(ui.Paragraph{Text: edid, Size: 12, Color: a.currentTheme().Body}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Presets")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionElement("video_edid:jetkvm_default", "JetKVM", settingsActionVisual{Enabled: !edidState.Pending || edidState.PendingChoice == "jetkvm_default", Active: edid == videoEDIDPresetJetKVMDefault, Pending: edidState.Pending && edidState.PendingChoice == "jetkvm_default"}, 84),
			settingsActionElement("video_edid:acer_b246wl", "Acer", settingsActionVisual{Enabled: !edidState.Pending || edidState.PendingChoice == "acer_b246wl", Active: edid == videoEDIDPresetAcerB246WL, Pending: edidState.Pending && edidState.PendingChoice == "acer_b246wl"}, 72),
			settingsActionElement("video_edid:asus_pa248qv", "ASUS", settingsActionVisual{Enabled: !edidState.Pending || edidState.PendingChoice == "asus_pa248qv", Active: edid == videoEDIDPresetASUSPA248QV, Pending: edidState.Pending && edidState.PendingChoice == "asus_pa248qv"}, 72),
			settingsActionElement("video_edid:dell_d2721h", "Dell", settingsActionVisual{Enabled: !edidState.Pending || edidState.PendingChoice == "dell_d2721h", Active: edid == videoEDIDPresetDellD2721H, Pending: edidState.Pending && edidState.PendingChoice == "dell_d2721h"}, 72),
			settingsActionElement("video_edid:dell_idrac", "iDRAC", settingsActionVisual{Enabled: !edidState.Pending || edidState.PendingChoice == "dell_idrac", Active: edid == videoEDIDPresetDellIDRAC, Pending: edidState.Pending && edidState.PendingChoice == "dell_idrac"}, 72),
		}, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 16}),
		ui.Fixed(settingsSectionLabelElement("Custom EDID")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{Text: pemSummary(a.videoCustomEDID), Size: 12, Color: a.currentTheme().Body}),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionButton("Load from Clipboard", settingsActionVisual{Enabled: !edidState.Pending}, 146, func() {
				text, err := readClipboardText()
				if err != nil {
					a.videoCustomEDIDMessage = err.Error()
					a.videoCustomEDIDSuccess = false
					return
				}
				a.videoCustomEDID = strings.TrimSpace(text)
				a.videoCustomEDIDDirty = true
				a.videoCustomEDIDMessage = "Custom EDID loaded from clipboard"
				a.videoCustomEDIDSuccess = true
			}),
			settingsActionButton("Clear", settingsActionVisual{Enabled: !edidState.Pending}, 70, func() {
				a.videoCustomEDID = ""
				a.videoCustomEDIDDirty = true
				a.videoCustomEDIDMessage = ""
				a.videoCustomEDIDSuccess = false
			}),
			settingsActionButton("Apply Custom", settingsActionVisual{Enabled: !edidState.Pending && strings.TrimSpace(a.videoCustomEDID) != "", Active: !isKnownEDIDPreset(edid) && strings.TrimSpace(edid) != "" && edid == strings.TrimSpace(a.videoCustomEDID), Pending: edidState.Pending && edidState.PendingChoice == "custom"}, 116, a.invokeCustomEDID),
		}, Spacing: 12, LineSpacing: 8}),
	}
	switch {
	case edidState.Pending:
		edidChildren = append(edidChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying EDID…", a.currentTheme().WarningStroke)))
	case edidState.Error != "":
		edidChildren = append(edidChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(edidState.Error, a.currentTheme().Error)))
	}
	if a.videoCustomEDIDMessage != "" {
		msgColor := a.currentTheme().WarningStroke
		if a.videoCustomEDIDSuccess {
			msgColor = color.RGBA{R: 134, G: 239, B: 172, A: 255}
		}
		edidChildren = append(edidChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(a.videoCustomEDIDMessage, msgColor)))
	}
	return settingsTwoPane(
		settingsCardElement("Stream", ui.Column{Children: streamChildren}),
		48,
		settingsCardElement("EDID", ui.Column{Children: edidChildren}),
		52,
	)
}

func (a *App) settingsHardwareBody() ui.Element {
	a.mu.RLock()
	state := a.sectionData.Hardware
	a.mu.RUnlock()
	if state.Loading {
		return settingsTwoPane(
			settingsCardElement("Display", ui.Label{Text: "Loading hardware state…", Size: 13, Color: a.currentTheme().Body}),
			48,
			settingsCardElement("USB", ui.Spacer{}),
			52,
		)
	}

	rotateState := a.settingsAction(settingsGroupDisplayRotate)
	backlightState := a.settingsAction(settingsGroupBacklight)
	videoSleepPending := a.settingsActionPending(settingsGroupVideoSleep)
	usbState := a.settingsAction(settingsGroupUSBEmulation)
	usbDevicesState := a.settingsAction(settingsGroupUSBDevices)

	displayChildren := []ui.Child{}
	if state.DisplayRotationLoaded {
		displayChildren = append(displayChildren,
			ui.Fixed(settingsKeyValueElement("Rotation", string(state.State.DisplayRotation), 86)),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(ui.Paragraph{Text: "Rotate the JetKVM device display. This does not rotate the remote host video feed.", Size: 12, Color: a.currentTheme().Muted}),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("rotate_normal", "Normal", settingsActionVisual{Enabled: !rotateState.Pending || rotateState.PendingChoice == "270", Active: state.State.DisplayRotation == session.DisplayRotationNormal, Pending: rotateState.Pending && rotateState.PendingChoice == "270"}, 88),
				settingsActionElement("rotate_inverted", "Inverted", settingsActionVisual{Enabled: !rotateState.Pending || rotateState.PendingChoice == "90", Active: state.State.DisplayRotation == session.DisplayRotationInverted, Pending: rotateState.Pending && rotateState.PendingChoice == "90"}, 98),
			}, Spacing: 12, LineSpacing: 8}),
		)
	} else {
		displayChildren = append(displayChildren,
			ui.Fixed(ui.Paragraph{Text: "Display rotation controls are unavailable on this target.", Size: 12, Color: a.currentTheme().Muted}),
		)
	}

	if state.BacklightLoaded {
		displayChildren = append(displayChildren,
			ui.Fixed(ui.Spacer{H: 18}),
			ui.Fixed(settingsSectionLabelElement("Brightness")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("backlight_brightness:0", "Off", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "0", Active: state.State.Backlight.MaxBrightness == 0, Pending: backlightState.Pending && backlightState.PendingChoice == "0"}, 64),
				settingsActionElement("backlight_brightness:10", "Low", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "10", Active: state.State.Backlight.MaxBrightness == 10, Pending: backlightState.Pending && backlightState.PendingChoice == "10"}, 64),
				settingsActionElement("backlight_brightness:35", "Medium", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "35", Active: state.State.Backlight.MaxBrightness == 35, Pending: backlightState.Pending && backlightState.PendingChoice == "35"}, 84),
				settingsActionElement("backlight_brightness:64", "High", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "64", Active: state.State.Backlight.MaxBrightness == 64, Pending: backlightState.Pending && backlightState.PendingChoice == "64"}, 72),
			}, Spacing: 12, LineSpacing: 8}),
			ui.Fixed(ui.Spacer{H: 18}),
			ui.Fixed(settingsSectionLabelElement("Dim display after")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("backlight_dim:0", "Never", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "0", Active: state.State.Backlight.DimAfter == 0, Pending: backlightState.Pending && backlightState.PendingChoice == "0"}, 76),
				settingsActionElement("backlight_dim:60", "1m", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "60", Active: state.State.Backlight.DimAfter == 60, Pending: backlightState.Pending && backlightState.PendingChoice == "60"}, 56),
				settingsActionElement("backlight_dim:300", "5m", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "300", Active: state.State.Backlight.DimAfter == 300, Pending: backlightState.Pending && backlightState.PendingChoice == "300"}, 56),
				settingsActionElement("backlight_dim:600", "10m", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "600", Active: state.State.Backlight.DimAfter == 600, Pending: backlightState.Pending && backlightState.PendingChoice == "600"}, 64),
				settingsActionElement("backlight_dim:1800", "30m", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "1800", Active: state.State.Backlight.DimAfter == 1800, Pending: backlightState.Pending && backlightState.PendingChoice == "1800"}, 64),
				settingsActionElement("backlight_dim:3600", "1h", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "3600", Active: state.State.Backlight.DimAfter == 3600, Pending: backlightState.Pending && backlightState.PendingChoice == "3600"}, 56),
			}, Spacing: 12, LineSpacing: 8}),
			ui.Fixed(ui.Spacer{H: 18}),
			ui.Fixed(settingsSectionLabelElement("Turn display off after")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("backlight_off:0", "Never", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "0", Active: state.State.Backlight.OffAfter == 0, Pending: backlightState.Pending && backlightState.PendingChoice == "0"}, 76),
				settingsActionElement("backlight_off:300", "5m", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "300", Active: state.State.Backlight.OffAfter == 300, Pending: backlightState.Pending && backlightState.PendingChoice == "300"}, 56),
				settingsActionElement("backlight_off:600", "10m", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "600", Active: state.State.Backlight.OffAfter == 600, Pending: backlightState.Pending && backlightState.PendingChoice == "600"}, 64),
				settingsActionElement("backlight_off:1800", "30m", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "1800", Active: state.State.Backlight.OffAfter == 1800, Pending: backlightState.Pending && backlightState.PendingChoice == "1800"}, 64),
				settingsActionElement("backlight_off:3600", "1h", settingsActionVisual{Enabled: !backlightState.Pending || backlightState.PendingChoice == "3600", Active: state.State.Backlight.OffAfter == 3600, Pending: backlightState.Pending && backlightState.PendingChoice == "3600"}, 56),
			}, Spacing: 12, LineSpacing: 8}),
		)
	}

	if state.VideoSleepLoaded {
		displayChildren = append(displayChildren,
			ui.Fixed(ui.Spacer{H: 18}),
			ui.Fixed(settingsToggleRowElement("hardware_hdmi_sleep_toggle", "HDMI Sleep Power Saving", settingsActionVisual{Enabled: !videoSleepPending, Active: state.State.VideoSleepMode != nil && state.State.VideoSleepMode.Duration >= 0, Pending: videoSleepPending})),
		)
	}

	switch {
	case rotateState.Pending:
		displayChildren = append(displayChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", a.currentTheme().WarningStroke)))
	case rotateState.Error != "":
		displayChildren = append(displayChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(rotateState.Error, a.currentTheme().Error)))
	}
	switch {
	case backlightState.Pending:
		displayChildren = append(displayChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Updating display settings…", a.currentTheme().WarningStroke)))
	case backlightState.Error != "":
		displayChildren = append(displayChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(backlightState.Error, a.currentTheme().Error)))
	}

	preset := usbDevicePresetLabel(state.State.USBDevices)
	usbChildren := []ui.Child{}
	if state.State.USBEmulation != nil {
		usbChildren = append(usbChildren,
			ui.Fixed(settingsToggleRowElement("usb_emulation_toggle", "Enable USB Emulation", settingsActionVisual{Enabled: !usbState.Pending, Active: *state.State.USBEmulation, Pending: usbState.Pending})),
		)
	}
	if state.USBConfigLoaded {
		if len(usbChildren) > 0 {
			usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 14}))
		}
		usbChildren = append(usbChildren,
			ui.Fixed(settingsKeyValueElement("Advertises as", usbConfigLabel(state.State.USBConfig), 112)),
		)
	}
	if state.USBDevicesLoaded {
		if len(usbChildren) > 0 {
			usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 14}))
		}
		usbChildren = append(usbChildren,
			ui.Fixed(settingsKeyValueElement("USB Profile", preset, 112)),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Paragraph{Text: usbDevicesSummary(state.State.USBDevices), Size: 12, Color: a.currentTheme().Body}),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Quick profiles")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("usb_devices_default", "Keyboard + Mouse + Storage", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "default", Active: preset == "Keyboard + Mouse + Storage", Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "default"}, 214),
				settingsActionElement("usb_devices_keyboard_only", "Keyboard Only", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "keyboard_only", Active: preset == "Keyboard Only", Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "keyboard_only"}, 122),
			}, Spacing: 12, LineSpacing: 8}),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsSectionLabelElement("Custom capabilities")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Column{Children: []ui.Child{
				ui.Fixed(settingsToggleRowElement("usb_toggle_keyboard", "Keyboard Input", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.Keyboard, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"})),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(settingsToggleRowElement("usb_toggle_absolute_mouse", "Absolute Mouse", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.AbsoluteMouse, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"})),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(settingsToggleRowElement("usb_toggle_relative_mouse", "Relative Mouse", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.RelativeMouse, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"})),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(settingsToggleRowElement("usb_toggle_mass_storage", "Virtual Media", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.MassStorage, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"})),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(settingsToggleRowElement("usb_toggle_serial_console", "Serial Console", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.SerialConsole, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"})),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(settingsToggleRowElement("usb_toggle_network", "USB Network Adapter", settingsActionVisual{Enabled: !usbDevicesState.Pending || usbDevicesState.PendingChoice == "custom", Active: state.State.USBDevices.Network, Pending: usbDevicesState.Pending && usbDevicesState.PendingChoice == "custom"})),
			}}),
		)
	} else if len(usbChildren) == 0 {
		usbChildren = append(usbChildren,
			ui.Fixed(ui.Paragraph{Text: "USB gadget controls are unavailable on this target.", Size: 12, Color: a.currentTheme().Muted}),
		)
	}
	switch {
	case usbState.Pending:
		usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", a.currentTheme().WarningStroke)))
	case usbState.Error != "":
		usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(usbState.Error, a.currentTheme().Error)))
	}
	switch {
	case usbDevicesState.Pending:
		usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", a.currentTheme().WarningStroke)))
	case usbDevicesState.Error != "":
		usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(usbDevicesState.Error, a.currentTheme().Error)))
	}
	if state.Error != "" {
		usbChildren = append(usbChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, a.currentTheme().Error)))
	}

	children := []ui.Child{ui.Fixed(settingsTwoPane(settingsCardElement("Display", ui.Column{Children: displayChildren}), 48, settingsCardElement("USB", ui.Column{Children: usbChildren}), 52))}
	if a.cfg.ExperimentalUSBNetwork && (state.State.USBNetwork != nil || a.usbNetworkEditorLoaded) {
		usbNetworkState := a.settingsAction(settingsGroupUSBNetworkSave)
		protocolEditable := a.usbNetworkEditor.HostPreset == "custom"
		usbNetworkChildren := []ui.Child{
			ui.Fixed(settingsToggleRowElement("usb_network_enabled_toggle", "Enable USB Network Gadget", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.Enabled, Pending: usbNetworkState.Pending})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Host preset")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("usb_network_host_preset:auto", "Auto", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.HostPreset == "auto"}, 72),
				settingsActionElement("usb_network_host_preset:linux", "Linux", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.HostPreset == "linux"}, 72),
				settingsActionElement("usb_network_host_preset:macos", "macOS", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.HostPreset == "macos"}, 80),
				settingsActionElement("usb_network_host_preset:windows", "Windows", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.HostPreset == "windows"}, 92),
				settingsActionElement("usb_network_host_preset:custom", "Custom", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.HostPreset == "custom"}, 82),
			}, Spacing: 12, LineSpacing: 8}),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Protocol")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("usb_network_protocol:ecm", "ECM", settingsActionVisual{Enabled: !usbNetworkState.Pending && protocolEditable, Active: a.usbNetworkEditor.Protocol == "ecm"}, 68),
				settingsActionElement("usb_network_protocol:ncm", "NCM", settingsActionVisual{Enabled: !usbNetworkState.Pending && protocolEditable, Active: a.usbNetworkEditor.Protocol == "ncm"}, 68),
				settingsActionElement("usb_network_protocol:rndis", "RNDIS", settingsActionVisual{Enabled: !usbNetworkState.Pending && protocolEditable, Active: a.usbNetworkEditor.Protocol == "rndis"}, 78),
			}, Spacing: 12, LineSpacing: 8}),
		}
		if !protocolEditable {
			usbNetworkChildren = append(usbNetworkChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(ui.Paragraph{Text: "Protocol follows the selected host preset. Switch to Custom to edit it directly.", Size: 12, Color: a.currentTheme().Muted}))
		}
		usbNetworkChildren = append(usbNetworkChildren,
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Sharing mode")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("usb_network_sharing_mode:nat", "NAT", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.SharingMode == "nat"}, 72),
				settingsActionElement("usb_network_sharing_mode:bridge", "Bridge", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.SharingMode == "bridge"}, 84),
			}, Spacing: 12, LineSpacing: 8}),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Uplink")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("usb_network_uplink_mode:auto", "Auto", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.UplinkMode == "auto"}, 72),
				settingsActionElement("usb_network_uplink_mode:manual", "Manual", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.UplinkMode == "manual"}, 82),
			}, Spacing: 12, LineSpacing: 8}),
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(settingsSectionLabelElement("Uplink interface")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "usb_network_focus_uplink_interface", Value: a.usbNetworkEditor.UplinkInterface, Placeholder: "eth0", Focused: a.settingsInputFocus == settingsInputUSBNetworkUplinkInterface, Enabled: !usbNetworkState.Pending && a.usbNetworkEditor.UplinkMode == "manual"})),
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(settingsSectionLabelElement("IPv4 subnet CIDR")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "usb_network_focus_subnet", Value: a.usbNetworkEditor.IPv4SubnetCIDR, Placeholder: "10.55.0.0/24", Focused: a.settingsInputFocus == settingsInputUSBNetworkSubnetCIDR, Enabled: !usbNetworkState.Pending})),
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(settingsToggleRowElement("usb_network_dhcp_toggle", "Enable DHCP", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.DHCPEnabled})),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(settingsToggleRowElement("usb_network_dns_proxy_toggle", "Enable DNS Proxy", settingsActionVisual{Enabled: !usbNetworkState.Pending, Active: a.usbNetworkEditor.DNSProxyEnabled})),
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(settingsActionElement("usb_network_save", "Save USB Network", settingsActionVisual{Enabled: !usbNetworkState.Pending}, 0)),
		)
		switch {
		case usbNetworkState.Pending:
			usbNetworkChildren = append(usbNetworkChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Saving…", a.currentTheme().WarningStroke)))
		case usbNetworkState.Error != "":
			usbNetworkChildren = append(usbNetworkChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(usbNetworkState.Error, a.currentTheme().Error)))
		}
		children = append(children, ui.Fixed(ui.Spacer{H: 14}), ui.Fixed(settingsCardElement("USB Network", ui.Column{Children: usbNetworkChildren})))
	}
	return ui.Column{Children: children}
}

func (a *App) settingsATXBody(snap session.Snapshot) ui.Element {
	a.mu.RLock()
	state := a.sectionData.ATX
	a.mu.RUnlock()
	actionState := a.settingsAction(settingsGroupATXPower)
	activeExtension := state.ActiveExtension
	if activeExtension == "" {
		activeExtension = snap.ActiveExtension
	}
	atxState := state.State
	if atxState == nil {
		atxState = snap.ATXState
	}
	if state.Loading {
		return settingsCardElement("ATX Power", ui.Label{Text: "Loading ATX state…", Size: 13, Color: a.currentTheme().Body})
	}

	statusChildren := []ui.Child{
		ui.Fixed(ui.Paragraph{Text: "These actions target the attached host machine through the JetKVM ATX power board. They do not reboot or power off the JetKVM itself.", Size: 12, Color: a.currentTheme().Muted}),
	}
	if activeExtension != "atx-power" {
		statusChildren = append(statusChildren,
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(settingsStatusElement("ATX power control is not active on this device. Enable the ATX extension in the JetKVM web UI to use these controls here.", a.currentTheme().Muted)),
		)
		if state.Error != "" {
			statusChildren = append(statusChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, a.currentTheme().Error)))
		}
		return settingsCardElement("ATX Power", ui.Column{Children: statusChildren})
	}

	ledWord := func(on bool) string {
		if on {
			return "On"
		}
		return "Off"
	}
	if atxState != nil {
		statusChildren = append(statusChildren,
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(ui.Row{
				Children: []ui.Child{
					ui.Flex(settingsKeyValueElement("Power LED", ledWord(atxState.Power), 86), 1),
					ui.Fixed(ui.Spacer{W: 12}),
					ui.Flex(settingsKeyValueElement("HDD LED", ledWord(atxState.HDD), 86), 1),
				},
				Spacing: 0,
			}),
		)
	} else {
		statusChildren = append(statusChildren,
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(ui.Label{Text: "ATX LED state unavailable", Size: 12, Color: a.currentTheme().Muted}),
		)
	}

	statusChildren = append(statusChildren,
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(settingsSectionLabelElement("Actions")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionButton("Power", settingsActionVisual{Enabled: !actionState.Pending, Pending: actionState.Pending && actionState.PendingChoice == string(session.ATXPowerActionShortPress)}, 84, a.invokeATXShortPress),
			settingsActionButton("Reset", settingsActionVisual{Enabled: !actionState.Pending, Pending: actionState.Pending && actionState.PendingChoice == string(session.ATXPowerActionReset)}, 84, a.openATXResetConfirm),
			settingsActionButton("Long Press", settingsActionVisual{Enabled: !actionState.Pending, Pending: actionState.Pending && actionState.PendingChoice == string(session.ATXPowerActionLongPress)}, 112, a.openATXLongPressConfirm),
		}, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(ui.Paragraph{Text: "Long Press mirrors the web UI host-power action. Hold for 3s to force off.", Size: 12, Color: a.currentTheme().Muted}),
	)
	switch {
	case actionState.Pending:
		statusChildren = append(statusChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Sending ATX action…", a.currentTheme().WarningStroke)))
	case actionState.Error != "":
		statusChildren = append(statusChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(actionState.Error, a.currentTheme().Error)))
	}
	if state.Error != "" {
		statusChildren = append(statusChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, a.currentTheme().Error)))
	}
	return settingsCardElement("ATX Power", ui.Column{Children: statusChildren})
}

func (a *App) settingsAccessBody() ui.Element {
	a.mu.RLock()
	state := a.sectionData.Access
	a.mu.RUnlock()
	localAuthState := a.settingsAction(settingsGroupLocalAuth)
	if state.Loading {
		return settingsTwoPane(
			settingsCardElement("Local Access", ui.Label{Text: "Loading access state…", Size: 13, Color: a.currentTheme().Body}),
			50,
			settingsCardElement("Remote Access", ui.Spacer{}),
			50,
		)
	}

	localChildren := []ui.Child{
		ui.Fixed(settingsKeyValueElement("Authentication", localAuthModeLabel(state.State.LocalAuthMode), 112)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("Loopback Only", boolWord(state.State.LoopbackOnly), 112)),
		ui.Fixed(ui.Spacer{H: 14}),
	}
	switch state.State.LocalAuthMode {
	case session.LocalAuthModePassword:
		localChildren = append(localChildren,
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("access_change_password", "Change Password", settingsActionVisual{Enabled: !localAuthState.Pending}, 136),
				settingsActionElement("access_disable_password", "Disable Password", settingsActionVisual{Enabled: !localAuthState.Pending}, 138),
			}, Spacing: 12, LineSpacing: 8}),
		)
	case session.LocalAuthModeNoPassword:
		localChildren = append(localChildren,
			ui.Fixed(settingsActionElement("access_enable_password", "Enable Password", settingsActionVisual{Enabled: !localAuthState.Pending}, 134)),
		)
	}
	switch {
	case localAuthState.Pending:
		localChildren = append(localChildren,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(settingsStatusElement("Saving…", a.currentTheme().WarningStroke)),
		)
	case localAuthState.Error != "":
		localChildren = append(localChildren,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(settingsStatusElement(localAuthState.Error, a.currentTheme().Error)),
		)
	}
	if a.accessEditor.Message != "" {
		msgColor := a.currentTheme().WarningStroke
		if a.accessEditor.Success {
			msgColor = color.RGBA{R: 134, G: 239, B: 172, A: 255}
		}
		localChildren = append(localChildren,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(settingsStatusElement(a.accessEditor.Message, msgColor)),
		)
	}

	leftChildren := []ui.Child{
		ui.Fixed(settingsCardElement("Local Access", ui.Column{Children: localChildren})),
	}
	tlsState := a.settingsAction(settingsGroupTLSMode)
	tlsChildren := []ui.Child{
		ui.Fixed(settingsKeyValueElement("Mode", string(state.State.TLS.Mode), 70)),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Paragraph{Text: "Use the target's currently exposed TLS mode. Native client transport follows whatever the device publishes.", Size: 12, Color: a.currentTheme().Muted}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionElement("tls_disabled", "Disabled", settingsActionVisual{Enabled: state.State.TLS.Mode != session.TLSModeUnknown && (!tlsState.Pending || tlsState.PendingChoice == "disabled"), Active: state.State.TLS.Mode == session.TLSModeDisabled, Pending: tlsState.Pending && tlsState.PendingChoice == "disabled"}, 92),
			settingsActionElement("tls_self_signed", "Self-Signed", settingsActionVisual{Enabled: state.State.TLS.Mode != session.TLSModeUnknown && (!tlsState.Pending || tlsState.PendingChoice == "self-signed"), Active: state.State.TLS.Mode == session.TLSModeSelfSigned, Pending: tlsState.Pending && tlsState.PendingChoice == "self-signed"}, 114),
			settingsActionElement("tls_custom", "Custom", settingsActionVisual{Enabled: state.State.TLS.Mode != session.TLSModeUnknown && (!tlsState.Pending || tlsState.PendingChoice == "custom"), Active: state.State.TLS.Mode == session.TLSModeCustom, Pending: tlsState.Pending && tlsState.PendingChoice == "custom"}, 82),
		}, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 16}),
		ui.Fixed(settingsSectionLabelElement("Certificate")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{Text: pemSummary(a.tlsEditor.Certificate), Size: 12, Color: a.currentTheme().Body}),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionElement("tls_custom_load_certificate", "Load from Clipboard", settingsActionVisual{Enabled: !tlsState.Pending}, 146),
			settingsActionElement("tls_custom_clear_certificate", "Clear", settingsActionVisual{Enabled: !tlsState.Pending}, 70),
		}, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 16}),
		ui.Fixed(settingsSectionLabelElement("Private Key")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{Text: pemSummary(a.tlsEditor.PrivateKey), Size: 12, Color: a.currentTheme().Body}),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionElement("tls_custom_load_key", "Load from Clipboard", settingsActionVisual{Enabled: !tlsState.Pending}, 146),
			settingsActionElement("tls_custom_clear_key", "Clear", settingsActionVisual{Enabled: !tlsState.Pending}, 70),
		}, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 16}),
		ui.Fixed(ui.Paragraph{Text: "Load the full PEM certificate chain and matching private key from the system clipboard, then apply Custom TLS.", Size: 12, Color: a.currentTheme().Muted}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(settingsActionElement("tls_custom_save", "Apply Custom TLS", settingsActionVisual{Enabled: !tlsState.Pending}, 142)),
	}
	switch {
	case tlsState.Pending:
		tlsChildren = append(tlsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", a.currentTheme().WarningStroke)))
	case tlsState.Error != "":
		tlsChildren = append(tlsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(tlsState.Error, a.currentTheme().Error)))
	}
	if a.tlsEditor.Message != "" {
		msgColor := a.currentTheme().WarningStroke
		if a.tlsEditor.Success {
			msgColor = color.RGBA{R: 134, G: 239, B: 172, A: 255}
		}
		tlsChildren = append(tlsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(a.tlsEditor.Message, msgColor)))
	}
	if state.Error != "" {
		tlsChildren = append(tlsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, a.currentTheme().Error)))
	}

	cloudChildren := []ui.Child{
		ui.Fixed(settingsKeyValueElement("Connected", boolWord(state.State.Cloud.Connected), 96)),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Cloud API")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{Text: fallbackLabel(state.State.Cloud.URL, "Unavailable"), Size: 12, Color: a.currentTheme().Body}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Cloud App")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Paragraph{Text: fallbackLabel(state.State.Cloud.AppURL, "Unavailable"), Size: 12, Color: a.currentTheme().Body}),
	}

	rightChildren := []ui.Child{
		ui.Fixed(settingsCardElement("TLS", ui.Column{Children: tlsChildren})),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsCardElement("Cloud", ui.Column{Children: cloudChildren})),
	}

	return settingsTwoPane(
		ui.Column{Children: leftChildren},
		50,
		ui.Column{Children: rightChildren},
		50,
	)
}

func localAuthModeLabel(mode session.LocalAuthMode) string {
	switch mode {
	case session.LocalAuthModePassword:
		return "Password"
	case session.LocalAuthModeNoPassword:
		return "No Password"
	default:
		return "Unavailable"
	}
}

func obscuredText(value string) string {
	if value == "" {
		return ""
	}
	return strings.Repeat("*", len([]rune(value)))
}

func pemSummary(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Not loaded"
	}
	lines := strings.Count(value, "\n") + 1
	return fmt.Sprintf("%d bytes across %d lines", len(value), lines)
}

func isKnownEDIDPreset(value string) bool {
	switch value {
	case videoEDIDPresetJetKVMDefault, videoEDIDPresetAcerB246WL, videoEDIDPresetASUSPA248QV, videoEDIDPresetDellD2721H, videoEDIDPresetDellIDRAC:
		return true
	default:
		return false
	}
}

func (a *App) settingsAccessEditorContent(pending bool) (string, ui.Element) {
	switch a.accessEditor.Mode {
	case accessEditorModeCreate:
		children := []ui.Child{
			ui.Fixed(settingsSectionLabelElement("New Password")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "access_focus_password", Value: a.accessEditor.Password, DisplayValue: obscuredText(a.accessEditor.Password), Placeholder: "Minimum 8 characters", Focused: a.settingsInputFocus == settingsInputAccessPassword, Enabled: !pending})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Confirm Password")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "access_focus_confirm_password", Value: a.accessEditor.ConfirmPassword, DisplayValue: obscuredText(a.accessEditor.ConfirmPassword), Placeholder: "Repeat password", Focused: a.settingsInputFocus == settingsInputAccessConfirmPassword, Enabled: !pending})),
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("access_submit", "Save Password", settingsActionVisual{Enabled: !pending}, 124),
				settingsActionElement("access_cancel_editor", "Cancel", settingsActionVisual{Enabled: !pending}, 90),
			}, Spacing: 12, LineSpacing: 8}),
		}
		return "Enable Password", ui.Column{Children: children}
	case accessEditorModeUpdate:
		children := []ui.Child{
			ui.Fixed(settingsSectionLabelElement("Current Password")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "access_focus_old_password", Value: a.accessEditor.OldPassword, DisplayValue: obscuredText(a.accessEditor.OldPassword), Placeholder: "Current password", Focused: a.settingsInputFocus == settingsInputAccessOldPassword, Enabled: !pending})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("New Password")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "access_focus_new_password", Value: a.accessEditor.NewPassword, DisplayValue: obscuredText(a.accessEditor.NewPassword), Placeholder: "Minimum 8 characters", Focused: a.settingsInputFocus == settingsInputAccessNewPassword, Enabled: !pending})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Confirm New Password")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "access_focus_confirm_new_password", Value: a.accessEditor.ConfirmNewPassword, DisplayValue: obscuredText(a.accessEditor.ConfirmNewPassword), Placeholder: "Repeat new password", Focused: a.settingsInputFocus == settingsInputAccessConfirmNewPassword, Enabled: !pending})),
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("access_submit", "Update Password", settingsActionVisual{Enabled: !pending}, 132),
				settingsActionElement("access_cancel_editor", "Cancel", settingsActionVisual{Enabled: !pending}, 90),
			}, Spacing: 12, LineSpacing: 8}),
		}
		return "Change Password", ui.Column{Children: children}
	case accessEditorModeDisable:
		children := []ui.Child{
			ui.Fixed(ui.Paragraph{Text: "Confirm the current password to disable local password protection.", Size: 12, Color: a.currentTheme().Muted}),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Current Password")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "access_focus_disable_password", Value: a.accessEditor.DisablePassword, DisplayValue: obscuredText(a.accessEditor.DisablePassword), Placeholder: "Current password", Focused: a.settingsInputFocus == settingsInputAccessDisablePassword, Enabled: !pending})),
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("access_submit", "Disable Password", settingsActionVisual{Enabled: !pending}, 134),
				settingsActionElement("access_cancel_editor", "Cancel", settingsActionVisual{Enabled: !pending}, 90),
			}, Spacing: 12, LineSpacing: 8}),
		}
		return "Disable Password", ui.Column{Children: children}
	default:
		return "", nil
	}
}

func (a *App) settingsNetworkBody() ui.Element {
	a.mu.RLock()
	state := a.sectionData.Network
	a.mu.RUnlock()
	saveState := a.settingsAction(settingsGroupNetworkSave)
	renewState := a.settingsAction(settingsGroupNetworkRenew)
	refreshState := a.settingsAction(settingsGroupNetworkRefresh)
	editorChildren := []ui.Child{}
	if state.Loading && !a.networkEditorLoaded {
		editorChildren = append(editorChildren, ui.Fixed(ui.Label{Text: "Loading network settings…", Size: 13, Color: a.currentTheme().Body}))
	} else {
		editorChildren = append(editorChildren,
			ui.Fixed(settingsSectionLabelElement("Hostname")), ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_hostname", Value: a.networkEditor.Hostname, Placeholder: fallbackLabel(state.Settings.Hostname, state.State.Hostname), Focused: a.settingsInputFocus == settingsInputNetworkHostname, Enabled: !saveState.Pending})),
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(settingsSectionLabelElement("Domain")), ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_domain", Value: a.networkEditor.Domain, Placeholder: fallbackLabel(state.Settings.Domain, dhcpLeaseDomain(state.State.DHCPLease)), Focused: a.settingsInputFocus == settingsInputNetworkDomain, Enabled: !saveState.Pending})),
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(settingsSectionLabelElement("HTTP Proxy")), ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_http_proxy", Value: a.networkEditor.HTTPProxy, Placeholder: state.Settings.HTTPProxy, Focused: a.settingsInputFocus == settingsInputNetworkHTTPProxy, Enabled: !saveState.Pending})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("IPv4 Mode")), ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("network_ipv4_mode:dhcp", "DHCP", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.IPv4Mode == "dhcp"}, 74),
				settingsActionElement("network_ipv4_mode:static", "Static", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.IPv4Mode == "static"}, 78),
				settingsActionElement("network_ipv4_mode:disabled", "Disabled", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.IPv4Mode == "disabled"}, 92),
			}, Spacing: 12, LineSpacing: 8}),
		)
		if a.networkEditor.IPv4Mode == "static" {
			editorChildren = append(editorChildren,
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(settingsSectionLabelElement("IPv4 Address")), ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_ipv4_address", Value: a.networkEditor.IPv4Address, Placeholder: networkIPv4StaticAddress(state.Settings), Focused: a.settingsInputFocus == settingsInputNetworkIPv4Address, Enabled: !saveState.Pending})),
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(settingsSectionLabelElement("IPv4 Netmask")), ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_ipv4_netmask", Value: a.networkEditor.IPv4Netmask, Placeholder: networkIPv4StaticNetmask(state.Settings), Focused: a.settingsInputFocus == settingsInputNetworkIPv4Netmask, Enabled: !saveState.Pending})),
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(settingsSectionLabelElement("IPv4 Gateway")), ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_ipv4_gateway", Value: a.networkEditor.IPv4Gateway, Placeholder: networkIPv4StaticGateway(state.Settings), Focused: a.settingsInputFocus == settingsInputNetworkIPv4Gateway, Enabled: !saveState.Pending})),
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(settingsSectionLabelElement("IPv4 DNS Servers")), ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_ipv4_dns", Value: a.networkEditor.IPv4DNS, Placeholder: strings.Join(networkIPv4StaticDNS(state.Settings), ", "), Focused: a.settingsInputFocus == settingsInputNetworkIPv4DNS, Enabled: !saveState.Pending})),
			)
		}
		editorChildren = append(editorChildren,
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("IPv6 Mode")), ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("network_ipv6_mode:slaac", "SLAAC", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.IPv6Mode == "slaac"}, 76),
				settingsActionElement("network_ipv6_mode:static", "Static", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.IPv6Mode == "static"}, 78),
				settingsActionElement("network_ipv6_mode:disabled", "Disabled", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.IPv6Mode == "disabled"}, 92),
			}, Spacing: 12, LineSpacing: 8}),
		)
		if a.networkEditor.IPv6Mode == "static" {
			editorChildren = append(editorChildren,
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(settingsSectionLabelElement("IPv6 Prefix")), ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_ipv6_prefix", Value: a.networkEditor.IPv6Prefix, Placeholder: networkIPv6StaticPrefix(state.Settings), Focused: a.settingsInputFocus == settingsInputNetworkIPv6Prefix, Enabled: !saveState.Pending})),
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(settingsSectionLabelElement("IPv6 Gateway")), ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_ipv6_gateway", Value: a.networkEditor.IPv6Gateway, Placeholder: networkIPv6StaticGateway(state.Settings), Focused: a.settingsInputFocus == settingsInputNetworkIPv6Gateway, Enabled: !saveState.Pending})),
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(settingsSectionLabelElement("IPv6 DNS Servers")), ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_ipv6_dns", Value: a.networkEditor.IPv6DNS, Placeholder: strings.Join(networkIPv6StaticDNS(state.Settings), ", "), Focused: a.settingsInputFocus == settingsInputNetworkIPv6DNS, Enabled: !saveState.Pending})),
			)
		}
		editorChildren = append(editorChildren,
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("mDNS")), ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("network_mdns_mode:auto", "Auto", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.MDNSMode == "auto"}, 72),
				settingsActionElement("network_mdns_mode:disabled", "Disabled", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.MDNSMode == "disabled"}, 92),
				settingsActionElement("network_mdns_mode:ipv4_only", "IPv4 Only", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.MDNSMode == "ipv4_only"}, 100),
				settingsActionElement("network_mdns_mode:ipv6_only", "IPv6 Only", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.MDNSMode == "ipv6_only"}, 100),
			}, Spacing: 12, LineSpacing: 8}),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Time Sync")), ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("network_time_sync_mode:ntp_only", "NTP Only", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.TimeSyncMode == "ntp_only"}, 94),
				settingsActionElement("network_time_sync_mode:ntp_and_http", "NTP + HTTP", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.TimeSyncMode == "ntp_and_http"}, 106),
				settingsActionElement("network_time_sync_mode:http_only", "HTTP Only", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.TimeSyncMode == "http_only"}, 102),
				settingsActionElement("network_time_sync_mode:custom", "Custom", settingsActionVisual{Enabled: !saveState.Pending, Active: a.networkEditor.TimeSyncMode == "custom"}, 82),
			}, Spacing: 12, LineSpacing: 8}),
		)
		if a.networkEditor.TimeSyncMode == "custom" {
			editorChildren = append(editorChildren,
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(settingsSectionLabelElement("NTP Servers")), ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_time_sync_ntp", Value: a.networkEditor.TimeSyncNTPServers, Placeholder: strings.Join(state.Settings.TimeSyncNTPServers, ", "), Focused: a.settingsInputFocus == settingsInputNetworkTimeSyncNTP, Enabled: !saveState.Pending})),
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(settingsSectionLabelElement("HTTP Time URLs")), ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "network_focus_time_sync_http", Value: a.networkEditor.TimeSyncHTTPURLs, Placeholder: strings.Join(state.Settings.TimeSyncHTTPUrls, ", "), Focused: a.settingsInputFocus == settingsInputNetworkTimeSyncHTTP, Enabled: !saveState.Pending})),
			)
		}
		editorChildren = append(editorChildren,
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("network_save", "Save Settings", settingsActionVisual{Enabled: !saveState.Pending}, 140),
				settingsActionElement("network_renew_dhcp", "Renew DHCP Lease", settingsActionVisual{Enabled: !renewState.Pending}, 152),
			}, Spacing: 12, LineSpacing: 8}),
		)
	}
	switch {
	case saveState.Pending:
		editorChildren = append(editorChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Saving…", a.currentTheme().WarningStroke)))
	case saveState.Error != "":
		editorChildren = append(editorChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(saveState.Error, a.currentTheme().Error)))
	}
	switch {
	case renewState.Pending:
		editorChildren = append(editorChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Renewing DHCP lease…", a.currentTheme().WarningStroke)))
	case renewState.Error != "":
		editorChildren = append(editorChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(renewState.Error, a.currentTheme().Error)))
	}
	if state.Error != "" {
		editorChildren = append(editorChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, a.currentTheme().Error)))
	}

	stateChildren := []ui.Child{
		ui.Fixed(settingsKeyValueElement("Interface", fallbackLabel(state.State.InterfaceName, "Unavailable"), 96)),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsKeyValueElement("MAC", fallbackLabel(state.State.MACAddress, "Unavailable"), 96)),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsKeyValueElement("Hostname", fallbackLabel(state.State.Hostname, state.Settings.Hostname, "Unavailable"), 96)),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsKeyValueElement("IPv4", fallbackLabel(state.State.IPv4, "Unavailable"), 96)),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsKeyValueElement("IPv4 Addr", fallbackLabel(strings.Join(state.State.IPv4Addresses, ", "), "Unavailable"), 96)),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsKeyValueElement("IPv6", fallbackLabel(state.State.IPv6, "Unavailable"), 96)),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsKeyValueElement("IPv6 Addr", fallbackLabel(ipv6AddressListLabel(state.State.IPv6Addresses), "Unavailable"), 96)),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsKeyValueElement("IPv6 LL", fallbackLabel(state.State.IPv6LinkLocal, "Unavailable"), 96)),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsKeyValueElement("IPv6 GW", fallbackLabel(state.State.IPv6Gateway, "Unavailable"), 96)),
	}

	leaseChildren := []ui.Child{}
	if state.State.DHCPLease == nil {
		leaseChildren = append(leaseChildren, ui.Fixed(ui.Paragraph{Text: "No DHCP lease is available for the current interface.", Size: 12, Color: a.currentTheme().Muted}))
	} else {
		leaseChildren = append(leaseChildren,
			ui.Fixed(settingsKeyValueElement("Address", fallbackLabel(state.State.DHCPLease.IP, "Unavailable"), 104)),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(settingsKeyValueElement("Netmask", fallbackLabel(state.State.DHCPLease.Netmask, "Unavailable"), 104)),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(settingsKeyValueElement("Routers", fallbackLabel(strings.Join(state.State.DHCPLease.Routers, ", "), "Unavailable"), 104)),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(settingsKeyValueElement("DNS", fallbackLabel(strings.Join(state.State.DHCPLease.DNSServers, ", "), "Unavailable"), 104)),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(settingsKeyValueElement("Domain", fallbackLabel(state.State.DHCPLease.Domain, "Unavailable"), 104)),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(settingsKeyValueElement("Lease Expiry", leaseExpiryLabel(state.State.DHCPLease.LeaseExpiry), 104)),
		)
	}

	serviceChildren := []ui.Child{
		ui.Fixed(settingsActionElement("network_refresh", "Refresh", settingsActionVisual{Enabled: !refreshState.Pending}, 0)),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Public IP")),
	}
	switch {
	case refreshState.Pending:
		serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(settingsStatusElement("Refreshing…", a.currentTheme().WarningStroke)))
	case refreshState.Error != "":
		serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(settingsStatusElement(refreshState.Error, a.currentTheme().Error)))
	case state.PublicIPError != "":
		serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(settingsStatusElement(state.PublicIPError, a.currentTheme().Error)))
	case len(state.PublicIPs) == 0:
		serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(ui.Paragraph{Text: "No public IP information available.", Size: 12, Color: a.currentTheme().Muted}))
	default:
		for _, address := range state.PublicIPs {
			serviceChildren = append(serviceChildren,
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(settingsKeyValueElement(address.IPAddress, address.LastUpdated.Local().Format("2006-01-02 15:04"), 136)),
			)
		}
	}

	serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 18}), ui.Fixed(settingsSectionLabelElement("Tailscale")))
	switch {
	case state.TailscaleError != "":
		serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(settingsStatusElement(state.TailscaleError, a.currentTheme().Error)))
	case state.Tailscale == nil:
		serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(ui.Paragraph{Text: "Tailscale state is unavailable.", Size: 12, Color: a.currentTheme().Muted}))
	case !state.Tailscale.Installed:
		serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(ui.Paragraph{Text: "Tailscale is not installed on this device.", Size: 12, Color: a.currentTheme().Muted}))
	default:
		serviceChildren = append(serviceChildren,
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(settingsKeyValueElement("Status", tailscaleStatusLabel(state.Tailscale), 96)),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(settingsKeyValueElement("Control URL", fallbackLabel(state.Tailscale.ControlURL, "Unavailable"), 96)),
		)
		if state.Tailscale.Self != nil {
			if state.Tailscale.Self.HostName != "" {
				serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(settingsKeyValueElement("Hostname", state.Tailscale.Self.HostName, 96)))
			}
			if state.Tailscale.Self.DNSName != "" {
				serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(settingsKeyValueElement("DNS Name", strings.TrimSuffix(state.Tailscale.Self.DNSName, "."), 96)))
			}
			if len(state.Tailscale.Self.TailscaleIPs) > 0 {
				serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(settingsKeyValueElement("IPs", strings.Join(state.Tailscale.Self.TailscaleIPs, ", "), 96)))
			}
		}
		if state.Tailscale.AuthURL != "" {
			serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(settingsKeyValueElement("Login URL", state.Tailscale.AuthURL, 96)))
		}
		if len(state.Tailscale.Health) > 0 {
			serviceChildren = append(serviceChildren, ui.Fixed(ui.Spacer{H: 8}), ui.Fixed(ui.Paragraph{Text: strings.Join(state.Tailscale.Health, " | "), Size: 12, Color: a.currentTheme().Muted}))
		}
	}

	return ui.Column{Children: []ui.Child{
		ui.Fixed(settingsTwoPane(settingsCardElement("Editable Settings", ui.Column{Children: editorChildren}), 54, settingsCardElement("Current State", ui.Column{Children: stateChildren}), 46)),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsTwoPane(settingsCardElement("DHCP Lease", ui.Column{Children: leaseChildren}), 46, settingsCardElement("Public Reachability", ui.Column{Children: serviceChildren}), 54)),
	}}
}

func ipv6AddressListLabel(addresses []session.IPv6Address) string {
	items := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address.Prefix != "" {
			items = append(items, address.Address+"/"+address.Prefix)
			continue
		}
		items = append(items, address.Address)
	}
	return strings.Join(items, ", ")
}

func leaseExpiryLabel(value time.Time) string {
	if value.IsZero() {
		return "Unavailable"
	}
	return value.Local().Format("2006-01-02 15:04")
}

func dhcpLeaseDomain(lease *session.DHCPLease) string {
	if lease == nil {
		return ""
	}
	return lease.Domain
}

func networkIPv4StaticAddress(settings session.NetworkSettings) string {
	if settings.IPv4Static == nil {
		return ""
	}
	return settings.IPv4Static.Address
}

func networkIPv4StaticNetmask(settings session.NetworkSettings) string {
	if settings.IPv4Static == nil {
		return ""
	}
	return settings.IPv4Static.Netmask
}

func networkIPv4StaticGateway(settings session.NetworkSettings) string {
	if settings.IPv4Static == nil {
		return ""
	}
	return settings.IPv4Static.Gateway
}

func networkIPv4StaticDNS(settings session.NetworkSettings) []string {
	if settings.IPv4Static == nil {
		return nil
	}
	return settings.IPv4Static.DNS
}

func networkIPv6StaticPrefix(settings session.NetworkSettings) string {
	if settings.IPv6Static == nil {
		return ""
	}
	return settings.IPv6Static.Prefix
}

func networkIPv6StaticGateway(settings session.NetworkSettings) string {
	if settings.IPv6Static == nil {
		return ""
	}
	return settings.IPv6Static.Gateway
}

func networkIPv6StaticDNS(settings session.NetworkSettings) []string {
	if settings.IPv6Static == nil {
		return nil
	}
	return settings.IPv6Static.DNS
}

func tailscaleStatusLabel(status *session.TailscaleStatus) string {
	if status == nil {
		return "Unavailable"
	}
	if status.Running {
		return "Connected"
	}
	if status.BackendState != "" {
		return status.BackendState
	}
	if status.Installed {
		return "Stopped"
	}
	return "Not installed"
}

func macroStepSummary(step session.KeyboardMacroStep) string {
	parts := make([]string, 0, len(step.Modifiers)+len(step.Keys))
	parts = append(parts, step.Modifiers...)
	parts = append(parts, step.Keys...)
	label := strings.Join(parts, " + ")
	if label == "" {
		label = "Delay only"
	}
	if step.Delay > 0 {
		label = fmt.Sprintf("%s (%dms)", label, step.Delay)
	}
	return label
}

func (a *App) settingsMacrosBody() ui.Element {
	a.mu.RLock()
	state := a.sectionData.Macros
	a.mu.RUnlock()
	macroState := a.settingsAction(settingsGroupMacrosSave)

	listChildren := []ui.Child{
		ui.Fixed(settingsActionElement("macro_create", "Add Macro", settingsActionVisual{Enabled: !macroState.Pending}, 0)),
	}
	if state.Loading {
		listChildren = append(listChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(ui.Label{Text: "Loading macros…", Size: 13, Color: a.currentTheme().Body}))
	} else if len(state.Macros) == 0 {
		listChildren = append(listChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(ui.Paragraph{Text: "No keyboard macros are saved on this device yet.", Size: 12, Color: a.currentTheme().Muted}))
	} else {
		for index, macro := range state.Macros {
			if index > 0 {
				listChildren = append(listChildren, ui.Fixed(ui.Spacer{H: 12}))
			}
			summary := "No steps"
			if len(macro.Steps) > 0 {
				summary = macroStepSummary(macro.Steps[0])
				if len(macro.Steps) > 1 {
					summary = fmt.Sprintf("%s + %d more", summary, len(macro.Steps)-1)
				}
			}
			listChildren = append(listChildren, ui.Fixed(settingsCardElement(macro.Name, ui.Column{Children: []ui.Child{
				ui.Fixed(settingsKeyValueElement("Summary", summary, 76)),
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(ui.Wrap{Children: []ui.Element{
					settingsActionElement("macro_move_up:"+macro.ID, "Up", settingsActionVisual{Enabled: !macroState.Pending && index > 0}, 54),
					settingsActionElement("macro_move_down:"+macro.ID, "Down", settingsActionVisual{Enabled: !macroState.Pending && index+1 < len(state.Macros)}, 64),
					settingsActionElement("macro_edit:"+macro.ID, "Edit", settingsActionVisual{Enabled: !macroState.Pending}, 60),
					settingsActionElement("macro_duplicate:"+macro.ID, "Duplicate", settingsActionVisual{Enabled: !macroState.Pending}, 92),
					settingsActionElement("macro_delete:"+macro.ID, "Delete", settingsActionVisual{Enabled: !macroState.Pending}, 72),
				}, Spacing: 10, LineSpacing: 8}),
			}})))
		}
	}
	switch {
	case macroState.Pending:
		listChildren = append(listChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Saving macros…", a.currentTheme().WarningStroke)))
	case macroState.Error != "":
		listChildren = append(listChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(macroState.Error, a.currentTheme().Error)))
	}
	if state.Error != "" {
		listChildren = append(listChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, a.currentTheme().Error)))
	}

	editorChildren := []ui.Child{}
	if a.macroEditor.Mode == macroEditorModeNone {
		editorChildren = append(editorChildren, ui.Fixed(ui.Paragraph{Text: "Select Edit on an existing macro or Add Macro to create a new one. Modifiers and keys use comma-separated HID token names.", Size: 12, Color: a.currentTheme().Muted}))
	} else {
		selected := a.macroEditor.Selected + 1
		total := len(a.macroEditor.Steps)
		step := a.selectedMacroEditorStep()
		editorChildren = append(editorChildren,
			ui.Fixed(settingsSectionLabelElement("Macro Name")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "macro_focus_name", Value: a.macroEditor.Name, Placeholder: "Wake Sequence", Focused: a.settingsInputFocus == settingsInputMacroName, Enabled: !macroState.Pending})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsKeyValueElement("Editing Step", fmt.Sprintf("%d of %d", selected, total), 96)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("macro_step_prev", "Previous", settingsActionVisual{Enabled: !macroState.Pending && a.macroEditor.Selected > 0}, 84),
				settingsActionElement("macro_step_next", "Next", settingsActionVisual{Enabled: !macroState.Pending && a.macroEditor.Selected+1 < len(a.macroEditor.Steps)}, 64),
				settingsActionElement("macro_step_add", "Add Step", settingsActionVisual{Enabled: !macroState.Pending && len(a.macroEditor.Steps) < 10}, 84),
				settingsActionElement("macro_step_remove", "Remove Step", settingsActionVisual{Enabled: !macroState.Pending && len(a.macroEditor.Steps) > 1}, 102),
				settingsActionElement("macro_step_up", "Move Up", settingsActionVisual{Enabled: !macroState.Pending && a.macroEditor.Selected > 0}, 84),
				settingsActionElement("macro_step_down", "Move Down", settingsActionVisual{Enabled: !macroState.Pending && a.macroEditor.Selected+1 < len(a.macroEditor.Steps)}, 96),
			}, Spacing: 10, LineSpacing: 8}),
		)
		if step != nil {
			editorChildren = append(editorChildren,
				ui.Fixed(ui.Spacer{H: 14}),
				ui.Fixed(settingsSectionLabelElement("Modifiers")),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "macro_focus_modifiers", Value: step.Modifiers, Placeholder: "ControlLeft,ShiftLeft", Focused: a.settingsInputFocus == settingsInputMacroModifiers, Enabled: !macroState.Pending})),
				ui.Fixed(ui.Spacer{H: 14}),
				ui.Fixed(settingsSectionLabelElement("Keys")),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "macro_focus_keys", Value: step.Keys, Placeholder: "KeyR", Focused: a.settingsInputFocus == settingsInputMacroKeys, Enabled: !macroState.Pending})),
				ui.Fixed(ui.Spacer{H: 14}),
				ui.Fixed(settingsSectionLabelElement("Delay (ms)")),
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(a.decorateTextField(ui.TextField{ID: "macro_focus_delay", Value: step.Delay, Placeholder: "50", Focused: a.settingsInputFocus == settingsInputMacroDelay, Enabled: !macroState.Pending})),
			)
		}
		editorChildren = append(editorChildren,
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(ui.Paragraph{Text: "Use comma-separated HID names for modifiers and keys. Example: modifiers `ControlLeft` and keys `KeyR`.", Size: 12, Color: a.currentTheme().Muted}),
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("macro_save", "Save Macro", settingsActionVisual{Enabled: !macroState.Pending}, 96),
				settingsActionElement("macro_editor_cancel", "Cancel", settingsActionVisual{Enabled: !macroState.Pending}, 72),
			}, Spacing: 10, LineSpacing: 8}),
		)
	}
	if a.macroEditor.Message != "" {
		msgColor := a.currentTheme().WarningStroke
		if a.macroEditor.Success {
			msgColor = color.RGBA{R: 134, G: 239, B: 172, A: 255}
		}
		editorChildren = append(editorChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(a.macroEditor.Message, msgColor)))
	}

	return settingsTwoPane(
		settingsCardElement("Library", ui.Column{Children: listChildren}),
		52,
		settingsCardElement("Editor", ui.Column{Children: editorChildren}),
		48,
	)
}

func (a *App) settingsMQTTBody() ui.Element {
	a.mu.RLock()
	state := a.sectionData.MQTT
	a.mu.RUnlock()
	saveState := a.settingsAction(settingsGroupMQTTSave)
	testState := a.settingsAction(settingsGroupMQTTTest)

	settingsChildren := []ui.Child{}
	if state.Loading && !a.mqttEditorLoaded {
		settingsChildren = append(settingsChildren, ui.Fixed(ui.Label{Text: "Loading MQTT settings…", Size: 13, Color: a.currentTheme().Body}))
	} else {
		settingsChildren = append(settingsChildren,
			ui.Fixed(settingsToggleRowElement("mqtt_enabled_toggle", "Enable MQTT", settingsActionVisual{Enabled: !saveState.Pending && !testState.Pending, Active: a.mqttEditor.Enabled})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Broker")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "mqtt_focus_broker", Value: a.mqttEditor.Broker, Placeholder: "mqtt.local", Focused: a.settingsInputFocus == settingsInputMQTTBroker, Enabled: !saveState.Pending && !testState.Pending})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Port")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "mqtt_focus_port", Value: a.mqttEditor.Port, Placeholder: "1883", Focused: a.settingsInputFocus == settingsInputMQTTPort, Enabled: !saveState.Pending && !testState.Pending})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Username")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "mqtt_focus_username", Value: a.mqttEditor.Username, Placeholder: "optional", Focused: a.settingsInputFocus == settingsInputMQTTUsername, Enabled: !saveState.Pending && !testState.Pending})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Password")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "mqtt_focus_password", Value: a.mqttEditor.Password, Placeholder: "optional", Focused: a.settingsInputFocus == settingsInputMQTTPassword, Enabled: !saveState.Pending && !testState.Pending})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Base Topic")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "mqtt_focus_base_topic", Value: a.mqttEditor.BaseTopic, Placeholder: "jetkvm", Focused: a.settingsInputFocus == settingsInputMQTTBaseTopic, Enabled: !saveState.Pending && !testState.Pending})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsToggleRowElement("mqtt_use_tls_toggle", "Use TLS", settingsActionVisual{Enabled: !saveState.Pending && !testState.Pending, Active: a.mqttEditor.UseTLS})),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsToggleRowElement("mqtt_tls_insecure_toggle", "Allow Insecure TLS", settingsActionVisual{Enabled: !saveState.Pending && !testState.Pending, Active: a.mqttEditor.TLSInsecure})),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsToggleRowElement("mqtt_ha_discovery_toggle", "Home Assistant Discovery", settingsActionVisual{Enabled: !saveState.Pending && !testState.Pending, Active: a.mqttEditor.EnableHADiscovery})),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsToggleRowElement("mqtt_actions_toggle", "Enable MQTT Actions", settingsActionVisual{Enabled: !saveState.Pending && !testState.Pending, Active: a.mqttEditor.EnableActions})),
			ui.Fixed(ui.Spacer{H: 14}),
			ui.Fixed(settingsSectionLabelElement("Debounce (ms)")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{ID: "mqtt_focus_debounce", Value: a.mqttEditor.DebounceMs, Placeholder: "0", Focused: a.settingsInputFocus == settingsInputMQTTDebounce, Enabled: !saveState.Pending && !testState.Pending})),
			ui.Fixed(ui.Spacer{H: 16}),
			ui.Fixed(ui.Wrap{Children: []ui.Element{
				settingsActionElement("mqtt_test_connection", "Test Connection", settingsActionVisual{Enabled: !saveState.Pending && !testState.Pending}, 128),
				settingsActionElement("mqtt_save_settings", "Save Settings", settingsActionVisual{Enabled: !saveState.Pending && !testState.Pending}, 116),
			}, Spacing: 12, LineSpacing: 8}),
		)
		switch {
		case saveState.Pending:
			settingsChildren = append(settingsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Saving…", a.currentTheme().WarningStroke)))
		case saveState.Error != "":
			settingsChildren = append(settingsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(saveState.Error, a.currentTheme().Error)))
		case testState.Pending:
			settingsChildren = append(settingsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Testing…", a.currentTheme().WarningStroke)))
		case testState.Error != "":
			settingsChildren = append(settingsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(testState.Error, a.currentTheme().Error)))
		}
		if a.mqttTestMessage != "" {
			testColor := a.currentTheme().WarningStroke
			if a.mqttTestSuccess {
				testColor = color.RGBA{R: 134, G: 239, B: 172, A: 255}
			}
			settingsChildren = append(settingsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(a.mqttTestMessage, testColor)))
		}
	}
	if state.Error != "" {
		settingsChildren = append(settingsChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, a.currentTheme().Error)))
	}

	statusChildren := []ui.Child{
		ui.Fixed(settingsKeyValueElement("Connected", boolWord(state.Status.Connected), 92)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("Broker", fallbackLabel(state.Settings.Broker, a.mqttEditor.Broker), 92)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("Base Topic", fallbackLabel(state.Settings.BaseTopic, a.mqttEditor.BaseTopic), 92)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("TLS", boolWord(state.Settings.UseTLS), 92)),
		ui.Fixed(ui.Spacer{H: 10}),
		ui.Fixed(settingsKeyValueElement("Actions", boolWord(state.Settings.EnableActions), 92)),
	}
	if state.Status.Error != "" {
		statusChildren = append(statusChildren, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Status.Error, a.currentTheme().Error)))
	}

	return settingsTwoPane(
		settingsCardElement("Configuration", ui.Column{Children: settingsChildren}),
		58,
		settingsCardElement("Status", ui.Column{Children: statusChildren}),
		42,
	)
}

func (a *App) settingsAdvancedBody() ui.Element {
	a.mu.RLock()
	state := a.sectionData.Advanced
	a.mu.RUnlock()
	children := []ui.Child{}
	if state.Loading {
		children = append(children, ui.Fixed(ui.Label{Text: "Loading advanced state…", Size: 13, Color: a.currentTheme().Body}))
	} else {
		children = append(children,
			ui.Fixed(settingsKeyValueElement("Developer Mode", boolPtrWord(state.State.DevMode), 128)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsKeyValueElement("Dev Channel", boolPtrWord(state.State.DevChannel), 128)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsKeyValueElement("Loopback Only", boolPtrWord(state.State.LoopbackOnly), 128)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsKeyValueElement("USB Emulation", boolPtrWord(state.State.USBEmulation), 128)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsKeyValueElement("App Version", state.State.Version.AppVersion, 128)),
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(settingsKeyValueElement("System Version", state.State.Version.SystemVersion, 128)),
		)
		if state.State.DevChannel != nil {
			devChannelState := a.settingsAction(settingsGroupDevChannel)
			children = append(children,
				ui.Fixed(ui.Spacer{H: 14}),
				ui.Fixed(settingsToggleRowElement("dev_channel_toggle", "Use Development Channel", settingsActionVisual{
					Enabled: !devChannelState.Pending,
					Active:  *state.State.DevChannel,
					Pending: devChannelState.Pending,
				})),
			)
			switch {
			case devChannelState.Pending:
				children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", a.currentTheme().WarningStroke)))
			case devChannelState.Error != "":
				children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(devChannelState.Error, a.currentTheme().Error)))
			}
		}
		if state.State.LoopbackOnly != nil {
			loopbackState := a.settingsAction(settingsGroupLoopbackOnly)
			children = append(children,
				ui.Fixed(ui.Spacer{H: 14}),
				ui.Fixed(settingsToggleRowElement("loopback_only_toggle", "Loopback Only", settingsActionVisual{
					Enabled: !loopbackState.Pending,
					Active:  *state.State.LoopbackOnly,
					Pending: loopbackState.Pending,
				})),
			)
			switch {
			case loopbackState.Pending:
				children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", a.currentTheme().WarningStroke)))
			case loopbackState.Error != "":
				children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(loopbackState.Error, a.currentTheme().Error)))
			}
		}
		if state.State.DevMode != nil {
			devModeState := a.settingsAction(settingsGroupDeveloperMode)
			children = append(children,
				ui.Fixed(ui.Spacer{H: 14}),
				ui.Fixed(settingsToggleRowElement("developer_mode_toggle", "Developer Mode", settingsActionVisual{
					Enabled: !devModeState.Pending,
					Active:  *state.State.DevMode,
					Pending: devModeState.Pending,
				})),
			)
			switch {
			case devModeState.Pending:
				children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Applying…", a.currentTheme().WarningStroke)))
			case devModeState.Error != "":
				children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(devModeState.Error, a.currentTheme().Error)))
			}
		}
		sshState := a.settingsAction(settingsGroupSSHKey)
		children = append(children,
			ui.Fixed(ui.Spacer{H: 18}),
			ui.Fixed(settingsSectionLabelElement("SSH Authorized Key")),
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(a.decorateTextField(ui.TextField{
				ID:          "advanced_focus_ssh",
				Value:       a.advancedSSHKey,
				Placeholder: "ssh-ed25519 AAAA...",
				Focused:     a.settingsInputFocus == settingsInputAdvancedSSH,
				Enabled:     !sshState.Pending,
			})),
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(settingsActionElement("advanced_save_ssh", "Save SSH Key", settingsActionVisual{Enabled: !sshState.Pending}, 128)),
		)
		switch {
		case sshState.Pending:
			children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Saving…", a.currentTheme().WarningStroke)))
		case sshState.Error != "":
			children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(sshState.Error, a.currentTheme().Error)))
		}
		children = append(children, ui.Fixed(ui.Spacer{H: 18}), ui.Fixed(settingsSectionLabelElement("Reset Device State")))
		if a.factoryResetConfirm {
			children = append(children,
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(ui.Paragraph{Text: "Factory reset removes stored configuration and restarts the device. This cannot be undone.", Size: 12, Color: a.currentTheme().Error}),
				ui.Fixed(ui.Spacer{H: 12}),
				ui.Fixed(ui.Wrap{Children: []ui.Element{
					settingsActionElement("factory_reset_confirm", "Confirm Reset", settingsActionVisual{Enabled: !a.settingsActionPending(settingsGroupFactoryReset)}, 128),
					settingsActionElement("factory_reset_cancel", "Cancel", settingsActionVisual{Enabled: !a.settingsActionPending(settingsGroupFactoryReset)}, 76),
				}, Spacing: 10, LineSpacing: 8}),
			)
		} else {
			children = append(children,
				ui.Fixed(ui.Spacer{H: 8}),
				ui.Fixed(settingsActionElement("factory_reset", "Factory Reset", settingsActionVisual{Enabled: !a.settingsActionPending(settingsGroupFactoryReset)}, 128)),
			)
		}
		factoryResetState := a.settingsAction(settingsGroupFactoryReset)
		switch {
		case factoryResetState.Pending:
			children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement("Resetting…", a.currentTheme().WarningStroke)))
		case factoryResetState.Error != "":
			children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(factoryResetState.Error, a.currentTheme().Error)))
		case a.factoryResetMessage != "":
			msgColor := a.currentTheme().WarningStroke
			if a.factoryResetSuccess {
				msgColor = color.RGBA{R: 134, G: 239, B: 172, A: 255}
			}
			children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(a.factoryResetMessage, msgColor)))
		}
	}
	if state.Error != "" {
		children = append(children, ui.Fixed(ui.Spacer{H: 12}), ui.Fixed(settingsStatusElement(state.Error, a.currentTheme().Error)))
	}
	return settingsCardElement("Current state", ui.Column{Children: children})
}

func (a *App) settingsAppearanceBody() ui.Element {
	themeButtons := []ui.Element{
		settingsActionButton("System", settingsActionVisual{Enabled: true, Active: a.prefs.Theme == themeSystem}, 92, func() {
			a.prefs.Theme = themeSystem
			a.refreshSystemTheme()
			a.savePreferences()
		}),
		settingsActionButton("Dark", settingsActionVisual{Enabled: true, Active: a.prefs.Theme == themeDark}, 84, func() {
			a.prefs.Theme = themeDark
			a.savePreferences()
		}),
		settingsActionButton("Light", settingsActionVisual{Enabled: true, Active: a.prefs.Theme == themeLight}, 84, func() {
			a.prefs.Theme = themeLight
			a.savePreferences()
		}),
	}
	positionButtons := []ui.Element{
		settingsActionButton("Top Left", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorTopLeft}, 96, func() { a.prefs.ChromeAnchor = chromeAnchorTopLeft; a.savePreferences() }),
		settingsActionButton("Top Center", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorTopCenter}, 108, func() { a.prefs.ChromeAnchor = chromeAnchorTopCenter; a.savePreferences() }),
		settingsActionButton("Top Right", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorTopRight}, 100, func() { a.prefs.ChromeAnchor = chromeAnchorTopRight; a.savePreferences() }),
		settingsActionButton("Left Center", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorLeftCenter}, 108, func() { a.prefs.ChromeAnchor = chromeAnchorLeftCenter; a.savePreferences() }),
		settingsActionButton("Right Center", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorRightCenter}, 118, func() { a.prefs.ChromeAnchor = chromeAnchorRightCenter; a.savePreferences() }),
		settingsActionButton("Bottom Left", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorBottomLeft}, 108, func() { a.prefs.ChromeAnchor = chromeAnchorBottomLeft; a.savePreferences() }),
		settingsActionButton("Bottom Center", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorBottomCenter}, 126, func() { a.prefs.ChromeAnchor = chromeAnchorBottomCenter; a.savePreferences() }),
		settingsActionButton("Bottom Right", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeAnchor == chromeAnchorBottomRight}, 118, func() { a.prefs.ChromeAnchor = chromeAnchorBottomRight; a.savePreferences() }),
	}
	return settingsCardElement("Appearance", ui.Column{Children: []ui.Child{
		ui.Fixed(settingsSectionLabelElement("Theme")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: themeButtons, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Icon bar")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsToggleRowControl("Pin Icon Bar", settingsActionVisual{Enabled: true, Active: a.prefs.PinChrome}, func() { a.prefs.PinChrome = !a.prefs.PinChrome; a.savePreferences() })),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsToggleRowControl("Hide Button Hints", settingsActionVisual{Enabled: true, Active: a.prefs.HideHeaderBar}, func() { a.prefs.HideHeaderBar = !a.prefs.HideHeaderBar; a.savePreferences() })),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsToggleRowControl("Hide Footer Status", settingsActionVisual{Enabled: true, Active: a.prefs.HideStatusBar}, func() { a.prefs.HideStatusBar = !a.prefs.HideStatusBar; a.savePreferences() })),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Position")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: positionButtons, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Layout")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(ui.Wrap{Children: []ui.Element{
			settingsActionButton("Horizontal", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeLayout == chromeLayoutHorizontal}, 112, func() { a.prefs.ChromeLayout = chromeLayoutHorizontal; a.savePreferences() }),
			settingsActionButton("Vertical", settingsActionVisual{Enabled: true, Active: a.prefs.ChromeLayout == chromeLayoutVertical}, 96, func() { a.prefs.ChromeLayout = chromeLayoutVertical; a.savePreferences() }),
		}, Spacing: 12, LineSpacing: 8}),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Window")),
		ui.Fixed(ui.Spacer{H: 8}),
		ui.Fixed(settingsActionButton("Toggle Fullscreen", settingsActionVisual{Enabled: true, Active: ebiten.IsFullscreen()}, 160, func() { ebiten.SetFullscreen(!ebiten.IsFullscreen()) })),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsStatusElement("Position chooses where the icon bar sits on screen. Layout changes whether the controls run across or down. Button hints and footer status are desktop-only UI helpers.", a.currentTheme().Muted)),
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
		ui.Fixed(settingsStatusElement(current.description, a.currentTheme().Muted)),
		ui.Fixed(ui.Spacer{H: 14}),
		ui.Fixed(settingsSectionLabelElement("Current upstream surface")),
	}
	for _, item := range current.items {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 8}),
			ui.Fixed(ui.Paragraph{Text: "• " + item, Size: 12, Color: a.currentTheme().Body}),
		)
	}
	children = append(children,
		ui.Fixed(ui.Spacer{H: 16}),
		ui.Fixed(ui.Paragraph{Text: "This section exists in the upstream product structure but is not currently exposed by this target or the desktop client.", Size: 12, Color: a.currentTheme().Muted}),
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
	labels := make([]string, 0, 6)
	if devices.Keyboard {
		labels = append(labels, "keyboard input")
	}
	if devices.AbsoluteMouse {
		labels = append(labels, "absolute mouse")
	}
	if devices.RelativeMouse {
		labels = append(labels, "relative mouse")
	}
	if devices.MassStorage {
		labels = append(labels, "virtual media")
	}
	if devices.SerialConsole {
		labels = append(labels, "serial console")
	}
	if devices.Network {
		labels = append(labels, "USB network adapter")
	}
	if len(labels) == 0 {
		return "No USB gadget capabilities enabled"
	}
	return strings.Join(labels, ", ")
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
		return "Keyboard + Mouse + Storage"
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
