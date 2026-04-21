package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
	"github.com/lkarlslund/jetkvm-desktop/pkg/discovery"
	"github.com/lkarlslund/jetkvm-desktop/pkg/hotkeys"
	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/logging"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
	"github.com/lkarlslund/jetkvm-desktop/pkg/ui"
	"github.com/lkarlslund/jetkvm-desktop/pkg/virtualmedia"
)

type Config struct {
	BaseURL                string
	Password               string
	RPCTimeout             time.Duration
	ExperimentalUSBNetwork bool
}

type App struct {
	cfg  Config
	ctrl *session.Controller
	ctx  context.Context

	mu                     sync.RWMutex
	lastImg                *ebiten.Image
	lastFrameAt            time.Time
	keyboard               *input.Keyboard
	hotkeys                hotkeys.Manager
	lastX                  int
	lastY                  int
	lastButtons            byte
	lastPhase              session.Phase
	lastTitle              string
	relative               bool
	renderRect             rect
	focused                bool
	lastWidth              int
	lastHeight             int
	resizeUntil            time.Time
	lastUIX                int
	lastUIY                int
	uiVisibleUntil         time.Time
	settingsOpen           bool
	pasteOpen              bool
	statsOpen              bool
	mediaOpen              bool
	serialConsoleOpen      bool
	settingsSection        settingsSection
	chromeButtons          []chromeButton
	settingsPanel          rect
	pastePanel             rect
	mediaPanel             rect
	serialConsolePanel     rect
	prefs                  Preferences
	systemTheme            Theme
	systemThemeCheckedAt   time.Time
	hideCursor             bool
	invertScroll           bool
	showPressedKeys        bool
	scrollThrottle         time.Duration
	pointerMoveThrottle    time.Duration
	lastPointerAt          time.Time
	lastWheelAt            time.Time
	suppressKeysUntilClear bool
	suppressMouseUntilUp   bool
	sectionData            sectionData
	pasteText              string
	pasteDelay             uint16
	pasteInvalid           string
	pasteError             string
	stats                  client.StatsSnapshot
	statsHistory           []statsPoint
	lastStatsPoll          time.Time
	launcherOpen           bool
	launcherMode           launcherMode
	launcherInput          string
	launcherPassword       string
	launcherError          string
	pendingTarget          string
	discovery              *discovery.Scanner
	discovered             []discovery.Device
	settingsActions        map[settingsActionGroup]settingsActionState
	sectionLoadSeq         map[settingsSection]uint64
	mediaView              mediaView
	mediaURL               string
	mediaMode              virtualmedia.Mode
	mediaURLFocused        bool
	mediaState             *virtualmedia.State
	mediaFiles             []mediaFileRow
	mediaSpace             mediaSpaceSnapshot
	mediaSelectedFile      string
	mediaLoading           bool
	mediaError             string
	mediaUploadPath        string
	mediaUploadFocused     bool
	mediaUploading         bool
	mediaUploadProgress    float64
	mediaUploadSent        int64
	mediaUploadTotal       int64
	mediaUploadSpeed       float64
	serialConsoleScroll    int
	mediaStorageLoaded     bool
	settingsInputFocus     settingsInputField
	textInput              ui.TextInputState
	jigglerEditorOpen      bool
	jigglerEditorConfig    session.JigglerConfig
	jigglerEditorError     string
	accessEditor           accessEditorState
	tlsEditor              tlsEditorState
	tlsEditorLoaded        bool
	tlsEditorDirty         bool
	videoCustomEDID        string
	videoCustomEDIDLoaded  bool
	videoCustomEDIDDirty   bool
	videoCustomEDIDMessage string
	videoCustomEDIDSuccess bool
	h265ConfirmOpen        bool
	atxConfirmAction       session.ATXPowerAction
	advancedSSHKey         string
	advancedSSHLoaded      bool
	advancedSSHDirty       bool
	networkEditor          networkEditorState
	networkEditorLoaded    bool
	networkEditorDirty     bool
	usbNetworkEditor       usbNetworkEditorState
	usbNetworkEditorLoaded bool
	usbNetworkEditorDirty  bool
	macroEditor            macroEditorState
	mqttEditor             mqttEditorState
	mqttEditorLoaded       bool
	mqttEditorDirty        bool
	mqttTestMessage        string
	mqttTestSuccess        bool
	updateActionMessage    string
	updateActionSuccess    bool
	factoryResetConfirm    bool
	factoryResetMessage    string
	factoryResetSuccess    bool
	hardwareConn           hardwareConnectionState
	launcherRuntime        ui.Runtime
	overlayRuntime         ui.Runtime
	settingsRuntime        ui.Runtime
	pasteRuntime           ui.Runtime
	mediaRuntime           ui.Runtime
	serialConsoleRuntime   ui.Runtime
	chromeRuntime          ui.Runtime
}

type hardwareConnectionState struct {
	USBDevices        session.USBDevices
	USBDevicesLoaded  bool
	USBDevicesLoading bool
	USBDevicesError   string
}

type statsPoint struct {
	At              time.Time
	BitrateKbps     float64
	JitterMs        float64
	RoundTripMs     float64
	FramesPerSecond float64
}

//go:generate go tool github.com/dmarkham/enumer -type=launcherMode,settingsActionGroup,mediaView -linecomment -json -text -output app_enums.go

type launcherMode uint8

const (
	launcherModeBrowse   launcherMode = iota // browse
	launcherModePassword                     // password
)

type settingsActionGroup uint8

const (
	settingsGroupKeyboardLayout  settingsActionGroup = iota // keyboard_layout
	settingsGroupVideoQuality    settingsActionGroup = iota // video_quality
	settingsGroupVideoCodec      settingsActionGroup = iota // video_codec
	settingsGroupVideoEDID       settingsActionGroup = iota // video_edid
	settingsGroupATXPower        settingsActionGroup = iota // atx_power
	settingsGroupActiveExtension settingsActionGroup = iota // active_extension
	settingsGroupDCPower         settingsActionGroup = iota // dc_power
	settingsGroupSerialSettings  settingsActionGroup = iota // serial_settings
	settingsGroupSerialCommand   settingsActionGroup = iota // serial_command
	settingsGroupTLSMode         settingsActionGroup = iota // tls_mode
	settingsGroupDisplayRotate   settingsActionGroup = iota // display_rotation
	settingsGroupBacklight       settingsActionGroup = iota // backlight
	settingsGroupVideoSleep      settingsActionGroup = iota // video_sleep
	settingsGroupUSBEmulation    settingsActionGroup = iota // usb_emulation
	settingsGroupUSBDevices      settingsActionGroup = iota // usb_devices
	settingsGroupAutoUpdate      settingsActionGroup = iota // auto_update
	settingsGroupUpdateStatus    settingsActionGroup = iota // update_status
	settingsGroupDeveloperMode   settingsActionGroup = iota // developer_mode
	settingsGroupDevChannel      settingsActionGroup = iota // dev_channel
	settingsGroupLoopbackOnly    settingsActionGroup = iota // loopback_only
	settingsGroupSSHKey          settingsActionGroup = iota // ssh_key
	settingsGroupNetworkSave     settingsActionGroup = iota // network_save
	settingsGroupMacrosSave      settingsActionGroup = iota // macros_save
	settingsGroupJiggler         settingsActionGroup = iota // jiggler
	settingsGroupLocalAuth       settingsActionGroup = iota // local_auth
	settingsGroupMQTTSave        settingsActionGroup = iota // mqtt_save
	settingsGroupMQTTTest        settingsActionGroup = iota // mqtt_test
	settingsGroupUpdateInstall   settingsActionGroup = iota // update_install
	settingsGroupFactoryReset    settingsActionGroup = iota // factory_reset
	settingsGroupNetworkRefresh  settingsActionGroup = iota // network_refresh
	settingsGroupNetworkRenew    settingsActionGroup = iota // network_renew_dhcp
	settingsGroupUSBNetworkSave  settingsActionGroup = iota // usb_network_save
)

type settingsActionState struct {
	Pending       bool
	PendingChoice string
	Error         string
	RequestSeq    uint64
}

type mediaView uint8

const (
	mediaViewHome    mediaView = iota // home
	mediaViewURL                      // url
	mediaViewStorage                  // storage
	mediaViewUpload                   // upload
)

type settingsInputField uint8

const (
	settingsInputNone settingsInputField = iota
	settingsInputJigglerCron
	settingsInputJigglerTimezone
	settingsInputAccessPassword
	settingsInputAccessConfirmPassword
	settingsInputAccessOldPassword
	settingsInputAccessNewPassword
	settingsInputAccessConfirmNewPassword
	settingsInputAccessDisablePassword
	settingsInputAdvancedSSH
	settingsInputNetworkHostname
	settingsInputNetworkDomain
	settingsInputNetworkHTTPProxy
	settingsInputNetworkIPv4Address
	settingsInputNetworkIPv4Netmask
	settingsInputNetworkIPv4Gateway
	settingsInputNetworkIPv4DNS
	settingsInputNetworkIPv6Prefix
	settingsInputNetworkIPv6Gateway
	settingsInputNetworkIPv6DNS
	settingsInputNetworkTimeSyncNTP
	settingsInputNetworkTimeSyncHTTP
	settingsInputUSBNetworkUplinkInterface
	settingsInputUSBNetworkSubnetCIDR
	settingsInputMacroName
	settingsInputMacroKeys
	settingsInputMacroModifiers
	settingsInputMacroDelay
	settingsInputMQTTBroker
	settingsInputMQTTPort
	settingsInputMQTTUsername
	settingsInputMQTTPassword
	settingsInputMQTTBaseTopic
	settingsInputMQTTDebounce
)

type mqttEditorState struct {
	Enabled           bool
	Broker            string
	Port              string
	Username          string
	Password          string
	BaseTopic         string
	UseTLS            bool
	TLSInsecure       bool
	EnableHADiscovery bool
	EnableActions     bool
	DebounceMs        string
}

type networkEditorState struct {
	DHCPClient         string
	Hostname           string
	Domain             string
	HTTPProxy          string
	IPv4Mode           string
	IPv4Address        string
	IPv4Netmask        string
	IPv4Gateway        string
	IPv4DNS            string
	IPv6Mode           string
	IPv6Prefix         string
	IPv6Gateway        string
	IPv6DNS            string
	MDNSMode           string
	TimeSyncMode       string
	TimeSyncNTPServers string
	TimeSyncHTTPURLs   string
}

type usbNetworkEditorState struct {
	Enabled         bool
	HostPreset      string
	Protocol        string
	SharingMode     string
	UplinkMode      string
	UplinkInterface string
	IPv4SubnetCIDR  string
	DHCPEnabled     bool
	DNSProxyEnabled bool
}

type accessEditorMode uint8

const (
	accessEditorModeNone accessEditorMode = iota
	accessEditorModeCreate
	accessEditorModeUpdate
	accessEditorModeDisable
)

type accessEditorState struct {
	Mode               accessEditorMode
	Password           string
	ConfirmPassword    string
	OldPassword        string
	NewPassword        string
	ConfirmNewPassword string
	DisablePassword    string
	Message            string
	Success            bool
}

type tlsEditorState struct {
	Certificate string
	PrivateKey  string
	Message     string
	Success     bool
}

type macroEditorMode uint8

const (
	macroEditorModeNone macroEditorMode = iota
	macroEditorModeCreate
	macroEditorModeEdit
)

type macroEditorStep struct {
	Keys      string
	Modifiers string
	Delay     string
}

type macroEditorState struct {
	Mode       macroEditorMode
	ExistingID string
	Name       string
	Steps      []macroEditorStep
	Selected   int
	Message    string
	Success    bool
}

type mediaFileRow struct {
	Filename  string
	Size      int64
	CreatedAt time.Time
}

type mediaSpaceSnapshot struct {
	BytesUsed int64
	BytesFree int64
}

func New(cfg Config) (*App, error) {
	prefs := loadPreferences()
	launcherOpen := strings.TrimSpace(cfg.BaseURL) == ""
	manager := hotkeys.NewManager()
	manager.SetEnabled(prefs.ExperimentalGlobalHotkeys)
	return &App{
		cfg:                 cfg,
		keyboard:            input.NewKeyboard(),
		hotkeys:             manager,
		lastPhase:           session.PhaseIdle,
		focused:             true,
		uiVisibleUntil:      time.Now().Add(3 * time.Second),
		settingsSection:     sectionGeneral,
		prefs:               prefs,
		hideCursor:          prefs.HideCursor,
		invertScroll:        prefs.InvertScroll,
		showPressedKeys:     prefs.ShowPressedKeys,
		scrollThrottle:      throttleDurationFromMs(prefs.ScrollThrottleMs),
		pointerMoveThrottle: throttleDurationFromMs(prefs.PointerMoveThrottleMs),
		pasteDelay:          100,
		launcherOpen:        launcherOpen,
		launcherMode:        launcherModeBrowse,
		discovery:           discovery.NewScanner(),
		settingsActions:     make(map[settingsActionGroup]settingsActionState),
		sectionLoadSeq:      make(map[settingsSection]uint64),
		mediaView:           mediaViewHome,
		mediaMode:           virtualmedia.ModeCDROM,
	}, nil
}

func (a *App) Start(ctx context.Context) {
	a.ctx = ctx
	if a.discovery != nil {
		a.discovery.Start(ctx)
	}
	if strings.TrimSpace(a.cfg.BaseURL) != "" {
		a.connectTo(a.cfg.BaseURL)
	}
}

func (a *App) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if a.serialConsoleOpen {
			a.closeSerialConsoleOverlay()
			a.revealUIFor(1200 * time.Millisecond)
			return nil
		}
		if a.pasteOpen {
			a.closePasteOverlay()
			a.revealUIFor(1200 * time.Millisecond)
			return nil
		}
		if a.mediaOpen {
			a.closeMediaOverlay()
			a.revealUIFor(1200 * time.Millisecond)
			return nil
		}
		if a.settingsOpen {
			a.closeSettingsOverlay()
			a.revealUIFor(1200 * time.Millisecond)
			return nil
		}
	}
	if a.launcherOpen {
		a.syncDiscovery()
		a.syncLauncherInput()
		a.syncUIPointer()
		a.updateTextSelectionDrag()
		return nil
	}
	if a.ctrl == nil {
		return nil
	}
	a.syncSessionState()
	a.syncWindowTitle()
	a.syncChromeVisibility()
	a.syncStats()
	a.syncSettingsInput()
	nowFocused := ebiten.IsFocused()
	if a.focused && !nowFocused {
		a.releaseAllKeys(true)
		if a.relative {
			a.relative = false
			a.applyCursorMode()
		}
	}
	a.focused = nowFocused
	a.syncUIPointer()
	a.updateTextSelectionDrag()

	a.syncPasteInput()
	a.syncMediaInput()
	a.syncSerialConsoleInput()
	a.syncVideoFrame()
	a.syncKeyboard()
	a.syncMouse()
	return nil
}

func (a *App) syncUIPointer() {
	x, y := ebiten.CursorPosition()
	point := ui.Point{X: float64(x), Y: float64(y)}
	pressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	justPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	justReleased := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)
	if !pressed && !justPressed && !justReleased {
		return
	}
	if a.launcherOpen {
		a.launcherRuntime.HandlePointer(point, pressed, justPressed, justReleased)
		return
	}
	if a.mediaOpen {
		if justPressed && !a.mediaPanel.contains(x, y) && shouldDismissOverlayOnOutsidePress("media") {
			a.closeMediaOverlay()
			return
		}
		if a.mediaRuntime.HandlePointer(point, pressed, justPressed, justReleased) {
			return
		}
	}
	if a.pasteOpen {
		if justPressed && !a.pastePanel.contains(x, y) && shouldDismissOverlayOnOutsidePress("paste") {
			a.closePasteOverlay()
			return
		}
		if a.pasteRuntime.HandlePointer(point, pressed, justPressed, justReleased) {
			return
		}
	}
	if a.settingsOpen {
		if a.settingsRuntime.HandlePointer(point, pressed, justPressed, justReleased) {
			return
		}
	}
	if a.serialConsoleOpen {
		if a.serialConsoleRuntime.HandlePointer(point, pressed, justPressed, justReleased) {
			return
		}
	}
	if a.overlayRuntime.HandlePointer(point, pressed, justPressed, justReleased) {
		return
	}
	a.chromeRuntime.HandlePointer(point, pressed, justPressed, justReleased)
}

func shouldDismissOverlayOnOutsidePress(kind string) bool {
	switch kind {
	case "media", "paste":
		return true
	case "settings":
		return false
	default:
		return false
	}
}

func (a *App) syncVideoFrame() {
	frame, at := a.ctrl.LatestFrameInfo()
	if frame == nil || !at.After(a.lastFrameAt) {
		return
	}
	img := ebiten.NewImageFromImage(frame)
	a.mu.Lock()
	a.lastImg = img
	a.lastFrameAt = at
	a.mu.Unlock()
}

func (a *App) Draw(screen *ebiten.Image) {
	if a.launcherOpen {
		a.drawLauncher(screen)
		return
	}
	if a.ctrl == nil {
		screen.Fill(a.currentTheme().Background)
		return
	}
	snap := a.ctrl.Snapshot()
	screen.Fill(a.currentTheme().Background)
	videoArea := screen.Bounds()
	a.mu.RLock()
	img := a.lastImg
	a.mu.RUnlock()
	if img != nil {
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		op := &ebiten.DrawImageOptions{}
		op.Filter = ebiten.FilterLinear
		scale := min(float64(videoArea.Dx())/float64(w), float64(videoArea.Dy())/float64(h))
		drawW := float64(w) * scale
		drawH := float64(h) * scale
		x := float64(videoArea.Min.X) + (float64(videoArea.Dx())-drawW)/2
		y := float64(videoArea.Min.Y) + (float64(videoArea.Dy())-drawH)/2
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(x, y)
		screen.DrawImage(img, op)
		a.renderRect = rect{
			x: x,
			y: y,
			w: drawW,
			h: drawH,
		}
	} else {
		a.renderRect = rect{}
	}
	a.drawTopBar(screen, snap)
	a.drawStatusFooter(screen, snap)
	a.drawPressedKeysOverlay(screen)
	a.drawOverlay(screen, snap, img != nil)
	a.drawStatsOverlay(screen)
	a.drawHint(screen)
	a.drawMediaOverlay(screen, snap)
	a.drawSettingsOverlay(screen, snap)
	a.drawPasteOverlay(screen, snap)
	a.drawSerialConsoleOverlay(screen, snap)
}

func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	if a.lastWidth != 0 && a.lastHeight != 0 && (a.lastWidth != outsideWidth || a.lastHeight != outsideHeight) {
		a.resizeUntil = time.Now().Add(200 * time.Millisecond)
	}
	a.lastWidth = outsideWidth
	a.lastHeight = outsideHeight
	return outsideWidth, outsideHeight
}

func (a *App) syncKeyboard() {
	if !a.focused || a.settingsOpen || a.pasteOpen || a.mediaOpen || a.serialConsoleOpen || a.ctrl.Snapshot().Phase != session.PhaseConnected {
		if a.hotkeys != nil {
			a.hotkeys.Reset()
		}
		return
	}
	now := time.Now()
	rawKeys := inpututil.AppendPressedKeys(nil)
	if a.suppressKeysUntilClear {
		if len(rawKeys) == 0 {
			a.suppressKeysUntilClear = false
		} else {
			a.releaseAllKeys(false)
			return
		}
	}
	keys := make([]input.Key, 0, len(rawKeys))
	for _, rawKey := range rawKeys {
		if key, ok := toInputKey(rawKey); ok {
			keys = append(keys, key)
		}
	}
	if a.handleExperimentalHotkeys(keys) {
		return
	}
	for _, evt := range a.keyboard.Update(keys, now) {
		_ = a.ctrl.SendKeypress(evt.HID, evt.Press)
	}
	if a.keyboard.KeepAlive(now) {
		_ = a.ctrl.SendKeypressKeepAlive()
	}
}

func (a *App) handleExperimentalHotkeys(keys []input.Key) bool {
	if a.hotkeys == nil {
		return false
	}
	result := a.hotkeys.Update(keys)
	if !result.Consumed {
		return false
	}
	if len(result.Actions) == 0 {
		return true
	}
	a.releaseAllKeys(true)
	a.suppressKeysUntilClear = true
	for _, action := range result.Actions {
		_ = a.ctrl.ExecuteRemoteHotkey(action)
	}
	return true
}

func (a *App) syncMouse() {
	log := logging.Subsystem("app")
	snapshot := a.ctrl.Snapshot()
	now := time.Now()
	x, y := ebiten.CursorPosition()
	buttons := currentMouseButtons(ebiten.IsMouseButtonPressed)
	if a.settingsOpen || a.pasteOpen || a.mediaOpen || a.serialConsoleOpen || snapshot.Phase != session.PhaseConnected {
		if buttons != a.lastButtons {
			log.Trace().
				Int("x", x).
				Int("y", y).
				Uint8("buttons", buttons).
				Bool("settings_open", a.settingsOpen).
				Bool("paste_open", a.pasteOpen).
				Bool("media_open", a.mediaOpen).
				Bool("serial_console_open", a.serialConsoleOpen).
				Str("phase", snapshot.Phase.String()).
				Msg("mouse input suppressed")
		}
		return
	}
	if a.suppressMouseUntilUp {
		a.lastX = x
		a.lastY = y
		if buttons != a.lastButtons {
			log.Trace().
				Int("x", x).
				Int("y", y).
				Uint8("buttons", buttons).
				Msg("mouse input suppressed until buttons released")
		}
		if buttons == 0 {
			log.Trace().Msg("mouse suppression cleared")
			a.suppressMouseUntilUp = false
			a.lastButtons = 0
		}
		return
	}
	if !a.relative && a.prefs.AbsoluteSideButtonsViaRel && buttons&sideMouseButtonMask != 0 {
		a.ensureConnectionUSBDevicesLoaded()
	}
	if a.relative {
		dx, dy, sendRelative := shouldSendRelativeMouse(a.lastX, a.lastY, x, y, a.lastButtons, buttons, a.pointerMoveThrottle, a.lastPointerAt, now)
		if sendRelative {
			if buttons != a.lastButtons {
				log.Trace().
					Int8("dx", dx).
					Int8("dy", dy).
					Uint8("buttons", buttons).
					Uint8("last_buttons", a.lastButtons).
					Msg("sending relative mouse report")
			}
			if err := a.ctrl.SendRelMouse(dx, dy, buttons); err != nil {
				log.Debug().
					Err(err).
					Int8("dx", dx).
					Int8("dy", dy).
					Uint8("buttons", buttons).
					Msg("failed to send relative mouse report")
			}
			a.lastPointerAt = now
			a.lastX = x
			a.lastY = y
			a.lastButtons = buttons
		}
	} else {
		if !a.renderRect.valid() {
			if buttons != 0 {
				log.Debug().
					Int("x", x).
					Int("y", y).
					Uint8("buttons", buttons).
					Msg("mouse input dropped because render rect is invalid")
			}
			return
		}
		if now.Before(a.resizeUntil) {
			a.lastX = x
			a.lastY = y
			a.lastButtons = buttons
			if buttons != 0 {
				log.Trace().
					Int("x", x).
					Int("y", y).
					Uint8("buttons", buttons).
					Time("resize_until", a.resizeUntil).
					Msg("mouse input delayed during resize")
			}
			return
		}
		if !a.renderRect.contains(x, y) && buttons == 0 && a.lastButtons == 0 {
			a.lastX = x
			a.lastY = y
			return
		}
		positionChanged := x != a.lastX || y != a.lastY
		buttonsChanged := buttons != a.lastButtons
		if positionChanged || buttonsChanged {
			if shouldThrottlePointerMovement(a.pointerMoveThrottle, a.lastPointerAt, now, positionChanged, buttonsChanged) {
				goto wheel
			}
			nx, ny := a.renderRect.toHID(x, y)
			absButtons := buttons
			routeSideButtons := a.shouldRouteSideButtonsToRelativeFor(buttons, a.lastButtons)
			if routeSideButtons {
				absButtons &^= sideMouseButtonMask
			}
			lastAbsButtons := a.lastButtons
			if routeSideButtons {
				lastAbsButtons &^= sideMouseButtonMask
			}
			sendAbsolute := x != a.lastX || y != a.lastY || absButtons != lastAbsButtons
			if sendAbsolute {
				if buttons != a.lastButtons || absButtons != lastAbsButtons {
					log.Trace().
						Int("x", x).
						Int("y", y).
						Int32("hid_x", nx).
						Int32("hid_y", ny).
						Uint8("buttons", absButtons).
						Uint8("last_buttons", lastAbsButtons).
						Msg("sending absolute mouse report")
				}
				if err := a.ctrl.SendAbsPointer(nx, ny, absButtons); err != nil {
					log.Debug().
						Err(err).
						Int32("hid_x", nx).
						Int32("hid_y", ny).
						Uint8("buttons", absButtons).
						Msg("failed to send absolute mouse report")
				}
			}
			if routeSideButtons {
				sideButtons := buttons & sideMouseButtonMask
				lastSideButtons := a.lastButtons & sideMouseButtonMask
				if sideButtons != lastSideButtons {
					log.Trace().
						Uint8("buttons", sideButtons).
						Uint8("last_buttons", lastSideButtons).
						Msg("routing side buttons via relative mouse")
					if err := a.ctrl.SendRelMouse(0, 0, sideButtons); err != nil {
						log.Debug().
							Err(err).
							Uint8("buttons", sideButtons).
							Uint8("last_buttons", lastSideButtons).
							Msg("failed to route side buttons via relative mouse")
					}
				}
			}
			a.lastPointerAt = now
			a.lastX = x
			a.lastY = y
			a.lastButtons = buttons
		}
	}
wheel:
	wheelX, wheelY := ebiten.Wheel()
	if (wheelX != 0 || wheelY != 0) && (a.scrollThrottle == 0 || now.Sub(a.lastWheelAt) >= a.scrollThrottle) {
		reportY := normalizeWheelDeltaY(wheelY, a.invertScroll)
		reportX := normalizeWheelDeltaX(wheelX, a.invertScroll)
		if reportY != 0 || reportX != 0 {
			a.runAsync(func() {
				if err := a.ctrl.SendWheel(reportY, reportX); err != nil {
					log.Debug().Err(err).Int8("wheel_y", reportY).Int8("wheel_x", reportX).Msg("failed to send mouse wheel report")
				}
			})
		}
		a.lastWheelAt = now
	}
}

const (
	defaultPointerMoveThrottleMs = 8
	maxPointerMoveThrottleMs     = 32
	maxScrollThrottleMs          = 100
)

func shouldThrottlePointerMovement(throttle time.Duration, lastSentAt, now time.Time, movementChanged, buttonsChanged bool) bool {
	if throttle <= 0 || !movementChanged || buttonsChanged || lastSentAt.IsZero() {
		return false
	}
	return now.Sub(lastSentAt) < throttle
}

func shouldSendRelativeMouse(lastX, lastY, x, y int, lastButtons, buttons byte, throttle time.Duration, lastSentAt, now time.Time) (dx, dy int8, send bool) {
	dx = int8(clamp(float64(x-lastX), -127, 127))
	dy = int8(clamp(float64(y-lastY), -127, 127))
	movementChanged := dx != 0 || dy != 0
	buttonsChanged := buttons != lastButtons
	if !movementChanged && !buttonsChanged {
		return 0, 0, false
	}
	if shouldThrottlePointerMovement(throttle, lastSentAt, now, movementChanged, buttonsChanged) {
		return dx, dy, false
	}
	return dx, dy, true
}

const (
	mouseButtonLeftMask    byte = 1 << 0
	mouseButtonRightMask   byte = 1 << 1
	mouseButtonMiddleMask  byte = 1 << 2
	mouseButtonBackMask    byte = 1 << 3
	mouseButtonForwardMask byte = 1 << 4
	sideMouseButtonMask    byte = mouseButtonBackMask | mouseButtonForwardMask
)

type mouseButtonBinding struct {
	button ebiten.MouseButton
	mask   byte
}

var supportedMouseButtons = [...]mouseButtonBinding{
	{button: ebiten.MouseButtonLeft, mask: mouseButtonLeftMask},
	{button: ebiten.MouseButtonRight, mask: mouseButtonRightMask},
	{button: ebiten.MouseButtonMiddle, mask: mouseButtonMiddleMask},
	{button: ebiten.MouseButton3, mask: mouseButtonBackMask},
	{button: ebiten.MouseButton4, mask: mouseButtonForwardMask},
}

func currentMouseButtons(isPressed func(ebiten.MouseButton) bool) byte {
	buttons := byte(0)
	for _, binding := range supportedMouseButtons {
		if isPressed(binding.button) {
			buttons |= binding.mask
		}
	}
	return buttons
}

func (a *App) shouldRouteSideButtonsToRelative() bool {
	if !a.prefs.AbsoluteSideButtonsViaRel || a.relative {
		return false
	}
	a.mu.RLock()
	devices := a.hardwareConn.USBDevices
	devicesLoaded := a.hardwareConn.USBDevicesLoaded
	a.mu.RUnlock()
	return devicesLoaded && devices.AbsoluteMouse && devices.RelativeMouse
}

func (a *App) shouldRouteSideButtonsToRelativeFor(buttons, lastButtons byte) bool {
	return a.shouldRouteSideButtonsToRelative() && (buttons|lastButtons)&sideMouseButtonMask != 0
}

func (a *App) connectionUSBDevicesSnapshot() (session.USBDevices, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.hardwareConn.USBDevices, a.hardwareConn.USBDevicesLoaded
}

func (a *App) setConnectionUSBDevices(devices session.USBDevices) {
	a.mu.Lock()
	a.hardwareConn.USBDevices = devices
	a.hardwareConn.USBDevicesLoaded = true
	a.hardwareConn.USBDevicesLoading = false
	a.hardwareConn.USBDevicesError = ""
	a.mu.Unlock()
}

func (a *App) failConnectionUSBDevicesLoad(err error) {
	a.mu.Lock()
	a.hardwareConn.USBDevicesLoading = false
	a.hardwareConn.USBDevicesError = err.Error()
	a.mu.Unlock()
}

func (a *App) resetConnectionHardwareState() {
	a.mu.Lock()
	a.hardwareConn = hardwareConnectionState{}
	a.mu.Unlock()
}

func (a *App) ensureConnectionUSBDevicesLoaded() {
	if a.ctrl == nil || a.ctrl.Snapshot().Phase != session.PhaseConnected {
		return
	}

	a.mu.Lock()
	if a.hardwareConn.USBDevicesLoaded || a.hardwareConn.USBDevicesLoading {
		a.mu.Unlock()
		return
	}
	a.hardwareConn.USBDevicesLoading = true
	a.mu.Unlock()

	a.runAsync(func() {
		log := logging.Subsystem("app")
		timeout := a.cfg.RPCTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		devices, err := a.ctrl.GetUSBDevices(ctx)
		if err != nil {
			log.Debug().Err(err).Msg("failed to load connection-scoped USB device capabilities")
			a.failConnectionUSBDevicesLoad(err)
			return
		}
		a.setConnectionUSBDevices(devices)
	})
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func normalizeWheelAxis(value float64) int8 {
	if value == 0 {
		return 0
	}
	magnitude := math.Abs(value)
	switch {
	case magnitude < 1:
		value = math.Copysign(1, value)
	default:
		value = math.Round(value)
	}
	return int8(clamp(value, -127, 127))
}

func normalizeWheelDeltaY(value float64, invert bool) int8 {
	delta := normalizeWheelAxis(value)
	if invert {
		return -delta
	}
	return delta
}

func normalizeWheelDeltaX(value float64, invert bool) int8 {
	return normalizeWheelAxis(value)
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func toInputKey(key ebiten.Key) (input.Key, bool) {
	switch key {
	case ebiten.KeyA:
		return input.KeyA, true
	case ebiten.KeyB:
		return input.KeyB, true
	case ebiten.KeyC:
		return input.KeyC, true
	case ebiten.KeyD:
		return input.KeyD, true
	case ebiten.KeyE:
		return input.KeyE, true
	case ebiten.KeyF:
		return input.KeyF, true
	case ebiten.KeyG:
		return input.KeyG, true
	case ebiten.KeyH:
		return input.KeyH, true
	case ebiten.KeyI:
		return input.KeyI, true
	case ebiten.KeyJ:
		return input.KeyJ, true
	case ebiten.KeyK:
		return input.KeyK, true
	case ebiten.KeyL:
		return input.KeyL, true
	case ebiten.KeyM:
		return input.KeyM, true
	case ebiten.KeyN:
		return input.KeyN, true
	case ebiten.KeyO:
		return input.KeyO, true
	case ebiten.KeyP:
		return input.KeyP, true
	case ebiten.KeyQ:
		return input.KeyQ, true
	case ebiten.KeyR:
		return input.KeyR, true
	case ebiten.KeyS:
		return input.KeyS, true
	case ebiten.KeyT:
		return input.KeyT, true
	case ebiten.KeyU:
		return input.KeyU, true
	case ebiten.KeyV:
		return input.KeyV, true
	case ebiten.KeyW:
		return input.KeyW, true
	case ebiten.KeyX:
		return input.KeyX, true
	case ebiten.KeyY:
		return input.KeyY, true
	case ebiten.KeyZ:
		return input.KeyZ, true
	case ebiten.Key1:
		return input.Key1, true
	case ebiten.Key2:
		return input.Key2, true
	case ebiten.Key3:
		return input.Key3, true
	case ebiten.Key4:
		return input.Key4, true
	case ebiten.Key5:
		return input.Key5, true
	case ebiten.Key6:
		return input.Key6, true
	case ebiten.Key7:
		return input.Key7, true
	case ebiten.Key8:
		return input.Key8, true
	case ebiten.Key9:
		return input.Key9, true
	case ebiten.Key0:
		return input.Key0, true
	case ebiten.KeyEnter:
		return input.KeyEnter, true
	case ebiten.KeyEscape:
		return input.KeyEscape, true
	case ebiten.KeyBackspace:
		return input.KeyBackspace, true
	case ebiten.KeyTab:
		return input.KeyTab, true
	case ebiten.KeySpace:
		return input.KeySpace, true
	case ebiten.KeyMinus:
		return input.KeyMinus, true
	case ebiten.KeyEqual:
		return input.KeyEqual, true
	case ebiten.KeyLeftBracket:
		return input.KeyLeftBracket, true
	case ebiten.KeyRightBracket:
		return input.KeyRightBracket, true
	case ebiten.KeyBackslash:
		return input.KeyBackslash, true
	case ebiten.KeyIntlBackslash:
		return input.KeyIntlBackslash, true
	case ebiten.KeySemicolon:
		return input.KeySemicolon, true
	case ebiten.KeyApostrophe:
		return input.KeyApostrophe, true
	case ebiten.KeyGraveAccent:
		return input.KeyGraveAccent, true
	case ebiten.KeyComma:
		return input.KeyComma, true
	case ebiten.KeyPeriod:
		return input.KeyPeriod, true
	case ebiten.KeySlash:
		return input.KeySlash, true
	case ebiten.KeyCapsLock:
		return input.KeyCapsLock, true
	case ebiten.KeyF1:
		return input.KeyF1, true
	case ebiten.KeyF2:
		return input.KeyF2, true
	case ebiten.KeyF3:
		return input.KeyF3, true
	case ebiten.KeyF4:
		return input.KeyF4, true
	case ebiten.KeyF5:
		return input.KeyF5, true
	case ebiten.KeyF6:
		return input.KeyF6, true
	case ebiten.KeyF7:
		return input.KeyF7, true
	case ebiten.KeyF8:
		return input.KeyF8, true
	case ebiten.KeyF9:
		return input.KeyF9, true
	case ebiten.KeyF10:
		return input.KeyF10, true
	case ebiten.KeyF11:
		return input.KeyF11, true
	case ebiten.KeyF12:
		return input.KeyF12, true
	case ebiten.KeyF13:
		return input.KeyF13, true
	case ebiten.KeyF14:
		return input.KeyF14, true
	case ebiten.KeyF15:
		return input.KeyF15, true
	case ebiten.KeyF16:
		return input.KeyF16, true
	case ebiten.KeyF17:
		return input.KeyF17, true
	case ebiten.KeyF18:
		return input.KeyF18, true
	case ebiten.KeyF19:
		return input.KeyF19, true
	case ebiten.KeyF20:
		return input.KeyF20, true
	case ebiten.KeyF21:
		return input.KeyF21, true
	case ebiten.KeyF22:
		return input.KeyF22, true
	case ebiten.KeyF23:
		return input.KeyF23, true
	case ebiten.KeyF24:
		return input.KeyF24, true
	case ebiten.KeyPrintScreen:
		return input.KeyPrintScreen, true
	case ebiten.KeyScrollLock:
		return input.KeyScrollLock, true
	case ebiten.KeyPause:
		return input.KeyPause, true
	case ebiten.KeyContextMenu:
		return input.KeyContextMenu, true
	case ebiten.KeyInsert:
		return input.KeyInsert, true
	case ebiten.KeyHome:
		return input.KeyHome, true
	case ebiten.KeyPageUp:
		return input.KeyPageUp, true
	case ebiten.KeyDelete:
		return input.KeyDelete, true
	case ebiten.KeyEnd:
		return input.KeyEnd, true
	case ebiten.KeyPageDown:
		return input.KeyPageDown, true
	case ebiten.KeyRight:
		return input.KeyRight, true
	case ebiten.KeyLeft:
		return input.KeyLeft, true
	case ebiten.KeyDown:
		return input.KeyDown, true
	case ebiten.KeyUp:
		return input.KeyUp, true
	case ebiten.KeyNumLock:
		return input.KeyNumLock, true
	case ebiten.KeyNumpadDivide:
		return input.KeyNumpadDivide, true
	case ebiten.KeyNumpadMultiply:
		return input.KeyNumpadMultiply, true
	case ebiten.KeyNumpadSubtract:
		return input.KeyNumpadSubtract, true
	case ebiten.KeyNumpadAdd:
		return input.KeyNumpadAdd, true
	case ebiten.KeyNumpadEnter:
		return input.KeyNumpadEnter, true
	case ebiten.KeyNumpad1:
		return input.KeyNumpad1, true
	case ebiten.KeyNumpad2:
		return input.KeyNumpad2, true
	case ebiten.KeyNumpad3:
		return input.KeyNumpad3, true
	case ebiten.KeyNumpad4:
		return input.KeyNumpad4, true
	case ebiten.KeyNumpad5:
		return input.KeyNumpad5, true
	case ebiten.KeyNumpad6:
		return input.KeyNumpad6, true
	case ebiten.KeyNumpad7:
		return input.KeyNumpad7, true
	case ebiten.KeyNumpad8:
		return input.KeyNumpad8, true
	case ebiten.KeyNumpad9:
		return input.KeyNumpad9, true
	case ebiten.KeyNumpad0:
		return input.KeyNumpad0, true
	case ebiten.KeyNumpadDecimal:
		return input.KeyNumpadDecimal, true
	case ebiten.KeyNumpadEqual:
		return input.KeyNumpadEqual, true
	case ebiten.KeyControlLeft:
		return input.KeyControlLeft, true
	case ebiten.KeyShiftLeft:
		return input.KeyShiftLeft, true
	case ebiten.KeyAltLeft:
		return input.KeyAltLeft, true
	case ebiten.KeyMetaLeft:
		return input.KeyMetaLeft, true
	case ebiten.KeyControlRight:
		return input.KeyControlRight, true
	case ebiten.KeyShiftRight:
		return input.KeyShiftRight, true
	case ebiten.KeyAltRight:
		return input.KeyAltRight, true
	case ebiten.KeyMetaRight:
		return input.KeyMetaRight, true
	default:
		return input.KeyUnknown, false
	}
}

func (a *App) runAsync(fn func()) {
	go fn()
}

func (a *App) settingsAction(group settingsActionGroup) settingsActionState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settingsActions[group]
}

func (a *App) settingsActionPending(group settingsActionGroup) bool {
	return a.settingsAction(group).Pending
}

func (a *App) beginSettingsAction(group settingsActionGroup, choice string) uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.settingsActions[group]
	state.Pending = true
	state.PendingChoice = choice
	state.Error = ""
	state.RequestSeq++
	a.settingsActions[group] = state
	return state.RequestSeq
}

func (a *App) finishSettingsAction(group settingsActionGroup, seq uint64, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.settingsActions[group]
	if state.RequestSeq != seq {
		return
	}
	state.Pending = false
	state.PendingChoice = ""
	if err != nil {
		state.Error = err.Error()
	} else {
		state.Error = ""
	}
	a.settingsActions[group] = state
}

func (a *App) withSettingsAction(group settingsActionGroup, choice string, fn func() error) {
	seq := a.beginSettingsAction(group, choice)
	go func() {
		a.finishSettingsAction(group, seq, fn())
	}()
}

func (a *App) nextSectionLoadSeq(section settingsSection) uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sectionLoadSeq[section]++
	return a.sectionLoadSeq[section]
}

func (a *App) setMouseRelative(relative bool) {
	a.relative = relative
	a.applyCursorMode()
	a.lastX, a.lastY = ebiten.CursorPosition()
	a.revealUIFor(1200 * time.Millisecond)
}

func (a *App) applyCursorMode() {
	switch {
	case a.settingsOpen:
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
	case a.pasteOpen:
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
	case a.mediaOpen:
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
	case a.serialConsoleOpen:
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
	case a.relative:
		ebiten.SetCursorMode(ebiten.CursorModeCaptured)
	case a.hideCursor:
		ebiten.SetCursorMode(ebiten.CursorModeHidden)
	default:
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
	}
}

func (a *App) savePreferences() {
	a.prefs.HideCursor = a.hideCursor
	a.prefs.InvertScroll = a.invertScroll
	a.prefs.ShowPressedKeys = a.showPressedKeys
	a.prefs.ScrollThrottle = scrollThrottlePref(a.scrollThrottle)
	a.prefs.ScrollThrottleMs = int(a.scrollThrottle / time.Millisecond)
	a.prefs.PointerMoveThrottleMs = int(a.pointerMoveThrottle / time.Millisecond)
	if a.hotkeys != nil {
		a.hotkeys.SetEnabled(a.prefs.ExperimentalGlobalHotkeys)
	}
	_ = savePreferences(a.prefs)
}

func (a *App) syncMQTTEditorLocked(settings session.MQTTSettings) {
	if a.mqttEditorDirty {
		return
	}
	port := settings.Port
	if port == 0 {
		port = 1883
	}
	baseTopic := settings.BaseTopic
	if baseTopic == "" {
		baseTopic = "jetkvm"
	}
	a.mqttEditor = mqttEditorState{
		Enabled:           settings.Enabled,
		Broker:            settings.Broker,
		Port:              strconv.Itoa(port),
		Username:          settings.Username,
		Password:          settings.Password,
		BaseTopic:         baseTopic,
		UseTLS:            settings.UseTLS,
		TLSInsecure:       settings.TLSInsecure,
		EnableHADiscovery: settings.EnableHADiscovery,
		EnableActions:     settings.EnableActions,
		DebounceMs:        strconv.Itoa(maxInt(settings.DebounceMs, 0)),
	}
	a.mqttEditorLoaded = true
}

func (a *App) currentSettingsTextValue() *string {
	switch a.settingsInputFocus {
	case settingsInputJigglerCron:
		return &a.jigglerEditorConfig.ScheduleCronTab
	case settingsInputJigglerTimezone:
		return &a.jigglerEditorConfig.Timezone
	case settingsInputAccessPassword:
		return &a.accessEditor.Password
	case settingsInputAccessConfirmPassword:
		return &a.accessEditor.ConfirmPassword
	case settingsInputAccessOldPassword:
		return &a.accessEditor.OldPassword
	case settingsInputAccessNewPassword:
		return &a.accessEditor.NewPassword
	case settingsInputAccessConfirmNewPassword:
		return &a.accessEditor.ConfirmNewPassword
	case settingsInputAccessDisablePassword:
		return &a.accessEditor.DisablePassword
	case settingsInputAdvancedSSH:
		return &a.advancedSSHKey
	case settingsInputNetworkHostname:
		return &a.networkEditor.Hostname
	case settingsInputNetworkDomain:
		return &a.networkEditor.Domain
	case settingsInputNetworkHTTPProxy:
		return &a.networkEditor.HTTPProxy
	case settingsInputNetworkIPv4Address:
		return &a.networkEditor.IPv4Address
	case settingsInputNetworkIPv4Netmask:
		return &a.networkEditor.IPv4Netmask
	case settingsInputNetworkIPv4Gateway:
		return &a.networkEditor.IPv4Gateway
	case settingsInputNetworkIPv4DNS:
		return &a.networkEditor.IPv4DNS
	case settingsInputNetworkIPv6Prefix:
		return &a.networkEditor.IPv6Prefix
	case settingsInputNetworkIPv6Gateway:
		return &a.networkEditor.IPv6Gateway
	case settingsInputNetworkIPv6DNS:
		return &a.networkEditor.IPv6DNS
	case settingsInputNetworkTimeSyncNTP:
		return &a.networkEditor.TimeSyncNTPServers
	case settingsInputNetworkTimeSyncHTTP:
		return &a.networkEditor.TimeSyncHTTPURLs
	case settingsInputUSBNetworkUplinkInterface:
		return &a.usbNetworkEditor.UplinkInterface
	case settingsInputUSBNetworkSubnetCIDR:
		return &a.usbNetworkEditor.IPv4SubnetCIDR
	case settingsInputMacroName:
		return &a.macroEditor.Name
	case settingsInputMacroKeys:
		if step := a.selectedMacroEditorStep(); step != nil {
			return &step.Keys
		}
	case settingsInputMacroModifiers:
		if step := a.selectedMacroEditorStep(); step != nil {
			return &step.Modifiers
		}
	case settingsInputMacroDelay:
		if step := a.selectedMacroEditorStep(); step != nil {
			return &step.Delay
		}
	case settingsInputMQTTBroker:
		return &a.mqttEditor.Broker
	case settingsInputMQTTPort:
		return &a.mqttEditor.Port
	case settingsInputMQTTUsername:
		return &a.mqttEditor.Username
	case settingsInputMQTTPassword:
		return &a.mqttEditor.Password
	case settingsInputMQTTBaseTopic:
		return &a.mqttEditor.BaseTopic
	case settingsInputMQTTDebounce:
		return &a.mqttEditor.DebounceMs
	default:
		return nil
	}
	return nil
}

func (a *App) selectedMacroEditorStep() *macroEditorStep {
	if a.macroEditor.Selected < 0 || a.macroEditor.Selected >= len(a.macroEditor.Steps) {
		return nil
	}
	return &a.macroEditor.Steps[a.macroEditor.Selected]
}

func (a *App) mqttSettingsFromEditor() (session.MQTTSettings, error) {
	broker := strings.TrimSpace(a.mqttEditor.Broker)
	port, err := strconv.Atoi(strings.TrimSpace(a.mqttEditor.Port))
	if err != nil || port < 1 || port > 65535 {
		return session.MQTTSettings{}, errors.New("port must be between 1 and 65535")
	}
	debounce, err := strconv.Atoi(strings.TrimSpace(a.mqttEditor.DebounceMs))
	if err != nil || debounce < 0 {
		return session.MQTTSettings{}, errors.New("debounce must be zero or greater")
	}
	baseTopic := strings.TrimSpace(a.mqttEditor.BaseTopic)
	if baseTopic == "" {
		baseTopic = "jetkvm"
	}
	if a.mqttEditor.Enabled && broker == "" {
		return session.MQTTSettings{}, errors.New("broker address is required when MQTT is enabled")
	}
	return session.MQTTSettings{
		Enabled:           a.mqttEditor.Enabled,
		Broker:            broker,
		Port:              port,
		Username:          strings.TrimSpace(a.mqttEditor.Username),
		Password:          a.mqttEditor.Password,
		BaseTopic:         baseTopic,
		UseTLS:            a.mqttEditor.UseTLS,
		TLSInsecure:       a.mqttEditor.TLSInsecure,
		EnableHADiscovery: a.mqttEditor.EnableHADiscovery,
		EnableActions:     a.mqttEditor.EnableActions,
		DebounceMs:        debounce,
	}, nil
}

func (a *App) networkSettingsFromEditor() (session.NetworkSettings, error) {
	settings := session.NetworkSettings{
		DHCPClient:   strings.TrimSpace(a.networkEditor.DHCPClient),
		Hostname:     strings.TrimSpace(a.networkEditor.Hostname),
		Domain:       strings.TrimSpace(a.networkEditor.Domain),
		HTTPProxy:    strings.TrimSpace(a.networkEditor.HTTPProxy),
		IPv4Mode:     strings.TrimSpace(a.networkEditor.IPv4Mode),
		IPv6Mode:     strings.TrimSpace(a.networkEditor.IPv6Mode),
		MDNSMode:     strings.TrimSpace(a.networkEditor.MDNSMode),
		TimeSyncMode: strings.TrimSpace(a.networkEditor.TimeSyncMode),
	}
	if settings.IPv4Mode == "" {
		settings.IPv4Mode = "dhcp"
	}
	if settings.IPv6Mode == "" {
		settings.IPv6Mode = "slaac"
	}
	if settings.MDNSMode == "" {
		settings.MDNSMode = "auto"
	}
	if settings.TimeSyncMode == "" {
		settings.TimeSyncMode = "ntp_only"
	}
	if settings.IPv4Mode == "static" {
		address := strings.TrimSpace(a.networkEditor.IPv4Address)
		gateway := strings.TrimSpace(a.networkEditor.IPv4Gateway)
		if address == "" {
			return session.NetworkSettings{}, errors.New("IPv4 address is required in static IPv4 mode")
		}
		if net.ParseIP(address) == nil {
			return session.NetworkSettings{}, errors.New("IPv4 address must be valid")
		}
		if gateway != "" && net.ParseIP(gateway) == nil {
			return session.NetworkSettings{}, errors.New("IPv4 gateway must be valid")
		}
		settings.IPv4Static = &session.IPv4StaticConfig{
			Address: address,
			Netmask: strings.TrimSpace(a.networkEditor.IPv4Netmask),
			Gateway: gateway,
			DNS:     splitCSV(a.networkEditor.IPv4DNS),
		}
	}
	if settings.IPv6Mode == "static" {
		prefix := strings.TrimSpace(a.networkEditor.IPv6Prefix)
		if prefix == "" {
			return session.NetworkSettings{}, errors.New("IPv6 prefix is required in static IPv6 mode")
		}
		settings.IPv6Static = &session.IPv6StaticConfig{
			Prefix:  prefix,
			Gateway: strings.TrimSpace(a.networkEditor.IPv6Gateway),
			DNS:     splitCSV(a.networkEditor.IPv6DNS),
		}
	}
	if settings.TimeSyncMode == "custom" {
		settings.TimeSyncNTPServers = splitCSV(a.networkEditor.TimeSyncNTPServers)
		settings.TimeSyncHTTPUrls = splitCSV(a.networkEditor.TimeSyncHTTPURLs)
	}
	return settings, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (a *App) usbNetworkConfigFromEditor() (session.USBNetworkConfig, error) {
	cfg := session.USBNetworkConfig{
		Enabled:         a.usbNetworkEditor.Enabled,
		HostPreset:      strings.TrimSpace(a.usbNetworkEditor.HostPreset),
		Protocol:        strings.TrimSpace(a.usbNetworkEditor.Protocol),
		SharingMode:     strings.TrimSpace(a.usbNetworkEditor.SharingMode),
		UplinkMode:      strings.TrimSpace(a.usbNetworkEditor.UplinkMode),
		UplinkInterface: strings.TrimSpace(a.usbNetworkEditor.UplinkInterface),
		IPv4SubnetCIDR:  strings.TrimSpace(a.usbNetworkEditor.IPv4SubnetCIDR),
		DHCPEnabled:     a.usbNetworkEditor.DHCPEnabled,
		DNSProxyEnabled: a.usbNetworkEditor.DNSProxyEnabled,
	}
	if cfg.HostPreset == "" {
		cfg.HostPreset = "auto"
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "ncm"
	}
	if cfg.SharingMode == "" {
		cfg.SharingMode = "nat"
	}
	if cfg.UplinkMode == "" {
		cfg.UplinkMode = "auto"
	}
	if cfg.IPv4SubnetCIDR == "" {
		cfg.IPv4SubnetCIDR = "10.55.0.0/24"
	}
	if cfg.UplinkMode == "manual" && cfg.UplinkInterface == "" {
		return session.USBNetworkConfig{}, errors.New("uplink interface is required in manual uplink mode")
	}
	return cfg, nil
}

const (
	minLocalPasswordLength = 8
	maxLocalPasswordLength = 72
)

func validateLocalPassword(value string) error {
	switch {
	case value == "":
		return errors.New("password is required")
	case len(value) < minLocalPasswordLength:
		return fmt.Errorf("password must be at least %d characters", minLocalPasswordLength)
	case len(value) > maxLocalPasswordLength:
		return fmt.Errorf("password must be at most %d characters", maxLocalPasswordLength)
	default:
		return nil
	}
}

func (a *App) clearAccessEditor(message string, success bool) {
	a.accessEditor = accessEditorState{
		Mode:    accessEditorModeNone,
		Message: message,
		Success: success,
	}
	if a.settingsInputFocus >= settingsInputAccessPassword && a.settingsInputFocus <= settingsInputAccessDisablePassword {
		a.settingsInputFocus = settingsInputNone
	}
}

func (a *App) syncAdvancedSSHKeyLocked(sshKey string) {
	if a.advancedSSHDirty {
		return
	}
	a.advancedSSHKey = sshKey
	a.advancedSSHLoaded = true
}

func (a *App) syncTLSEditorLocked(state session.TLSState) {
	if a.tlsEditorDirty {
		return
	}
	a.tlsEditor = tlsEditorState{
		Certificate: state.Certificate,
		PrivateKey:  state.PrivateKey,
	}
	a.tlsEditorLoaded = true
}

func (a *App) syncCustomEDIDLocked(edid string) {
	if a.videoCustomEDIDDirty {
		return
	}
	a.videoCustomEDID = edid
	a.videoCustomEDIDLoaded = true
}

func (a *App) syncNetworkEditorLocked(settings session.NetworkSettings) {
	if a.networkEditorDirty {
		return
	}
	var ipv4Address, ipv4Netmask, ipv4Gateway, ipv4DNS string
	if settings.IPv4Static != nil {
		ipv4Address = settings.IPv4Static.Address
		ipv4Netmask = settings.IPv4Static.Netmask
		ipv4Gateway = settings.IPv4Static.Gateway
		ipv4DNS = strings.Join(settings.IPv4Static.DNS, ", ")
	}
	var ipv6Prefix, ipv6Gateway, ipv6DNS string
	if settings.IPv6Static != nil {
		ipv6Prefix = settings.IPv6Static.Prefix
		ipv6Gateway = settings.IPv6Static.Gateway
		ipv6DNS = strings.Join(settings.IPv6Static.DNS, ", ")
	}
	a.networkEditor = networkEditorState{
		DHCPClient:         settings.DHCPClient,
		Hostname:           settings.Hostname,
		Domain:             settings.Domain,
		HTTPProxy:          settings.HTTPProxy,
		IPv4Mode:           settings.IPv4Mode,
		IPv4Address:        ipv4Address,
		IPv4Netmask:        ipv4Netmask,
		IPv4Gateway:        ipv4Gateway,
		IPv4DNS:            ipv4DNS,
		IPv6Mode:           settings.IPv6Mode,
		IPv6Prefix:         ipv6Prefix,
		IPv6Gateway:        ipv6Gateway,
		IPv6DNS:            ipv6DNS,
		MDNSMode:           settings.MDNSMode,
		TimeSyncMode:       settings.TimeSyncMode,
		TimeSyncNTPServers: strings.Join(settings.TimeSyncNTPServers, ", "),
		TimeSyncHTTPURLs:   strings.Join(settings.TimeSyncHTTPUrls, ", "),
	}
	a.networkEditorLoaded = true
}

func (a *App) syncUSBNetworkEditorLocked(cfg session.USBNetworkConfig) {
	if a.usbNetworkEditorDirty {
		return
	}
	a.usbNetworkEditor = usbNetworkEditorState{
		Enabled:         cfg.Enabled,
		HostPreset:      cfg.HostPreset,
		Protocol:        cfg.Protocol,
		SharingMode:     cfg.SharingMode,
		UplinkMode:      cfg.UplinkMode,
		UplinkInterface: cfg.UplinkInterface,
		IPv4SubnetCIDR:  cfg.IPv4SubnetCIDR,
		DHCPEnabled:     cfg.DHCPEnabled,
		DNSProxyEnabled: cfg.DNSProxyEnabled,
	}
	a.usbNetworkEditorLoaded = true
}

func (a *App) setAccessEditorMode(mode accessEditorMode) {
	a.accessEditor = accessEditorState{Mode: mode}
	switch mode {
	case accessEditorModeCreate:
		a.settingsInputFocus = settingsInputAccessPassword
	case accessEditorModeUpdate:
		a.settingsInputFocus = settingsInputAccessOldPassword
	case accessEditorModeDisable:
		a.settingsInputFocus = settingsInputAccessDisablePassword
	default:
		a.settingsInputFocus = settingsInputNone
	}
}

func (a *App) setMacroEditorMode(mode macroEditorMode, macro *session.KeyboardMacro) {
	switch mode {
	case macroEditorModeCreate:
		a.macroEditor = macroEditorState{
			Mode: mode,
			Steps: []macroEditorStep{
				{Delay: "50"},
			},
			Selected: 0,
		}
		a.settingsInputFocus = settingsInputMacroName
	case macroEditorModeEdit:
		if macro == nil {
			return
		}
		steps := make([]macroEditorStep, 0, len(macro.Steps))
		for _, step := range macro.Steps {
			steps = append(steps, macroEditorStep{
				Keys:      strings.Join(step.Keys, ","),
				Modifiers: strings.Join(step.Modifiers, ","),
				Delay:     strconv.Itoa(maxInt(step.Delay, 0)),
			})
		}
		if len(steps) == 0 {
			steps = []macroEditorStep{{Delay: "50"}}
		}
		a.macroEditor = macroEditorState{
			Mode:       mode,
			ExistingID: macro.ID,
			Name:       macro.Name,
			Steps:      steps,
			Selected:   0,
		}
		a.settingsInputFocus = settingsInputMacroName
	default:
		a.clearMacroEditor("", false)
	}
}

func (a *App) clearMacroEditor(message string, success bool) {
	a.macroEditor = macroEditorState{
		Mode:    macroEditorModeNone,
		Message: message,
		Success: success,
	}
	switch a.settingsInputFocus {
	case settingsInputMacroName, settingsInputMacroKeys, settingsInputMacroModifiers, settingsInputMacroDelay:
		a.settingsInputFocus = settingsInputNone
	}
}

func normalizeMacroSortOrders(macros []session.KeyboardMacro) []session.KeyboardMacro {
	out := make([]session.KeyboardMacro, len(macros))
	copy(out, macros)
	for i := range out {
		out[i].SortOrder = i + 1
	}
	return out
}

func generateMacroID() string {
	return fmt.Sprintf("macro-%d", time.Now().UnixNano())
}

func parseMacroTokenList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		out = append(out, token)
	}
	return out
}

func (a *App) macroFromEditor() (session.KeyboardMacro, error) {
	name := strings.TrimSpace(a.macroEditor.Name)
	if name == "" {
		return session.KeyboardMacro{}, errors.New("macro name is required")
	}
	if len(a.macroEditor.Steps) == 0 {
		return session.KeyboardMacro{}, errors.New("at least one step is required")
	}
	steps := make([]session.KeyboardMacroStep, 0, len(a.macroEditor.Steps))
	for i, step := range a.macroEditor.Steps {
		delay, err := strconv.Atoi(strings.TrimSpace(step.Delay))
		if err != nil || delay < 0 {
			return session.KeyboardMacro{}, fmt.Errorf("step %d delay must be zero or greater", i+1)
		}
		keys := parseMacroTokenList(step.Keys)
		modifiers := parseMacroTokenList(step.Modifiers)
		if len(keys) == 0 && len(modifiers) == 0 {
			return session.KeyboardMacro{}, fmt.Errorf("step %d must include keys or modifiers", i+1)
		}
		steps = append(steps, session.KeyboardMacroStep{
			Keys:      keys,
			Modifiers: modifiers,
			Delay:     delay,
		})
	}
	id := a.macroEditor.ExistingID
	if id == "" {
		id = generateMacroID()
	}
	return session.KeyboardMacro{
		ID:    id,
		Name:  name,
		Steps: steps,
	}, nil
}

func (a *App) currentMacros() []session.KeyboardMacro {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]session.KeyboardMacro, len(a.sectionData.Macros.Macros))
	copy(out, a.sectionData.Macros.Macros)
	return out
}

func (a *App) findMacroByID(id string) (session.KeyboardMacro, bool) {
	for _, macro := range a.currentMacros() {
		if macro.ID == id {
			return macro, true
		}
	}
	return session.KeyboardMacro{}, false
}

func (a *App) invokeLocalAuthSubmit() {
	if a.settingsActionPending(settingsGroupLocalAuth) {
		return
	}
	switch a.accessEditor.Mode {
	case accessEditorModeCreate:
		password := a.accessEditor.Password
		if err := validateLocalPassword(password); err != nil {
			seq := a.beginSettingsAction(settingsGroupLocalAuth, "create")
			a.finishSettingsAction(settingsGroupLocalAuth, seq, err)
			return
		}
		if password != a.accessEditor.ConfirmPassword {
			seq := a.beginSettingsAction(settingsGroupLocalAuth, "create")
			a.finishSettingsAction(settingsGroupLocalAuth, seq, errors.New("passwords do not match"))
			return
		}
		a.withSettingsAction(settingsGroupLocalAuth, "create", func() error {
			if err := a.ctrl.CreateLocalPassword(password); err != nil {
				return err
			}
			a.cfg.Password = password
			a.ctrl.SetPassword(password)
			a.clearAccessEditor("Password protection enabled", true)
			return a.refreshSettingsSectionSync(sectionAccess)
		})
	case accessEditorModeUpdate:
		if strings.TrimSpace(a.accessEditor.OldPassword) == "" {
			seq := a.beginSettingsAction(settingsGroupLocalAuth, "update")
			a.finishSettingsAction(settingsGroupLocalAuth, seq, errors.New("current password is required"))
			return
		}
		newPassword := a.accessEditor.NewPassword
		if err := validateLocalPassword(newPassword); err != nil {
			seq := a.beginSettingsAction(settingsGroupLocalAuth, "update")
			a.finishSettingsAction(settingsGroupLocalAuth, seq, err)
			return
		}
		if newPassword != a.accessEditor.ConfirmNewPassword {
			seq := a.beginSettingsAction(settingsGroupLocalAuth, "update")
			a.finishSettingsAction(settingsGroupLocalAuth, seq, errors.New("new passwords do not match"))
			return
		}
		oldPassword := a.accessEditor.OldPassword
		a.withSettingsAction(settingsGroupLocalAuth, "update", func() error {
			if err := a.ctrl.UpdateLocalPassword(oldPassword, newPassword); err != nil {
				return err
			}
			a.cfg.Password = newPassword
			a.ctrl.SetPassword(newPassword)
			a.clearAccessEditor("Password updated", true)
			return a.refreshSettingsSectionSync(sectionAccess)
		})
	case accessEditorModeDisable:
		password := a.accessEditor.DisablePassword
		if strings.TrimSpace(password) == "" {
			seq := a.beginSettingsAction(settingsGroupLocalAuth, "disable")
			a.finishSettingsAction(settingsGroupLocalAuth, seq, errors.New("current password is required"))
			return
		}
		a.withSettingsAction(settingsGroupLocalAuth, "disable", func() error {
			if err := a.ctrl.DeleteLocalPassword(password); err != nil {
				return err
			}
			a.cfg.Password = ""
			a.ctrl.SetPassword("")
			a.clearAccessEditor("Password protection disabled", true)
			return a.refreshSettingsSectionSync(sectionAccess)
		})
	}
}

func (a *App) syncSettingsInput() {
	if !a.settingsOpen {
		return
	}
	if a.syncTextInputBinding() == nil {
		return
	}
	a.syncFocusedTextInput()
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		switch a.settingsSection {
		case sectionMouse:
			if !a.jigglerEditorOpen {
				return
			}
			if a.settingsInputFocus == settingsInputJigglerCron {
				a.settingsInputFocus = settingsInputJigglerTimezone
			} else {
				a.settingsInputFocus = settingsInputJigglerCron
			}
		case sectionMQTT:
			switch a.settingsInputFocus {
			case settingsInputMQTTBroker:
				a.settingsInputFocus = settingsInputMQTTPort
			case settingsInputMQTTPort:
				a.settingsInputFocus = settingsInputMQTTUsername
			case settingsInputMQTTUsername:
				a.settingsInputFocus = settingsInputMQTTPassword
			case settingsInputMQTTPassword:
				a.settingsInputFocus = settingsInputMQTTBaseTopic
			case settingsInputMQTTBaseTopic:
				a.settingsInputFocus = settingsInputMQTTDebounce
			default:
				a.settingsInputFocus = settingsInputMQTTBroker
			}
		case sectionAccess:
			switch a.accessEditor.Mode {
			case accessEditorModeCreate:
				if a.settingsInputFocus == settingsInputAccessPassword {
					a.settingsInputFocus = settingsInputAccessConfirmPassword
				} else {
					a.settingsInputFocus = settingsInputAccessPassword
				}
			case accessEditorModeUpdate:
				switch a.settingsInputFocus {
				case settingsInputAccessOldPassword:
					a.settingsInputFocus = settingsInputAccessNewPassword
				case settingsInputAccessNewPassword:
					a.settingsInputFocus = settingsInputAccessConfirmNewPassword
				default:
					a.settingsInputFocus = settingsInputAccessOldPassword
				}
			case accessEditorModeDisable:
				a.settingsInputFocus = settingsInputAccessDisablePassword
			}
		case sectionAdvanced:
			a.settingsInputFocus = settingsInputAdvancedSSH
		case sectionHardware:
			if a.settingsInputFocus == settingsInputUSBNetworkUplinkInterface {
				a.settingsInputFocus = settingsInputUSBNetworkSubnetCIDR
			} else {
				a.settingsInputFocus = settingsInputUSBNetworkUplinkInterface
			}
		case sectionNetwork:
			switch a.settingsInputFocus {
			case settingsInputNetworkHostname:
				a.settingsInputFocus = settingsInputNetworkDomain
			case settingsInputNetworkDomain:
				a.settingsInputFocus = settingsInputNetworkHTTPProxy
			case settingsInputNetworkHTTPProxy:
				a.settingsInputFocus = settingsInputNetworkIPv4Address
			case settingsInputNetworkIPv4Address:
				a.settingsInputFocus = settingsInputNetworkIPv4Netmask
			case settingsInputNetworkIPv4Netmask:
				a.settingsInputFocus = settingsInputNetworkIPv4Gateway
			case settingsInputNetworkIPv4Gateway:
				a.settingsInputFocus = settingsInputNetworkIPv4DNS
			case settingsInputNetworkIPv4DNS:
				a.settingsInputFocus = settingsInputNetworkIPv6Prefix
			case settingsInputNetworkIPv6Prefix:
				a.settingsInputFocus = settingsInputNetworkIPv6Gateway
			case settingsInputNetworkIPv6Gateway:
				a.settingsInputFocus = settingsInputNetworkIPv6DNS
			case settingsInputNetworkIPv6DNS:
				a.settingsInputFocus = settingsInputNetworkTimeSyncNTP
			case settingsInputNetworkTimeSyncNTP:
				a.settingsInputFocus = settingsInputNetworkTimeSyncHTTP
			default:
				a.settingsInputFocus = settingsInputNetworkHostname
			}
		case sectionMacros:
			switch a.settingsInputFocus {
			case settingsInputMacroName:
				a.settingsInputFocus = settingsInputMacroKeys
			case settingsInputMacroKeys:
				a.settingsInputFocus = settingsInputMacroModifiers
			case settingsInputMacroModifiers:
				a.settingsInputFocus = settingsInputMacroDelay
			default:
				a.settingsInputFocus = settingsInputMacroName
			}
		}
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		switch a.settingsSection {
		case sectionMouse:
			a.invokeAction("jiggler_custom_save")
		case sectionAccess:
			a.invokeAction("access_submit")
		case sectionAdvanced:
			a.invokeAction("advanced_save_ssh")
		case sectionNetwork:
			a.invokeAction("network_save")
		case sectionMacros:
			a.invokeAction("macro_save")
		case sectionMQTT:
			a.invokeAction("mqtt_save_settings")
		}
		return
	}
}

func (a *App) invokeAction(id string) {
	switch id {
	case "paste_load_clipboard":
		a.loadClipboardText()
	case "paste_send":
		go a.submitPaste()
	case "paste_cancel":
		a.runAsync(func() {
			_ = a.ctrl.CancelPaste()
		})
		a.closePasteOverlay()
	case "settings_close":
		a.closeSettingsOverlay()
	case "media_close":
		a.closeMediaOverlay()
	default:
		if a.invokeMediaAction(id) {
			return
		}
	}
	switch id {
	case "tls_disabled":
		if a.settingsActionPending(settingsGroupTLSMode) {
			return
		}
		a.withSettingsAction(settingsGroupTLSMode, "disabled", func() error {
			if err := a.ctrl.SetTLSMode(session.TLSModeDisabled); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionAccess)
		})
	case "tls_self_signed":
		if a.settingsActionPending(settingsGroupTLSMode) {
			return
		}
		a.withSettingsAction(settingsGroupTLSMode, "self-signed", func() error {
			if err := a.ctrl.SetTLSMode(session.TLSModeSelfSigned); err != nil {
				return err
			}
			a.tlsEditorDirty = false
			return a.refreshSettingsSectionSync(sectionAccess)
		})
	case "tls_custom_load_certificate":
		text, err := readClipboardText()
		if err != nil {
			a.tlsEditor.Message = err.Error()
			a.tlsEditor.Success = false
			return
		}
		a.tlsEditor.Certificate = text
		a.tlsEditorDirty = true
		a.tlsEditor.Message = "Certificate loaded from clipboard"
		a.tlsEditor.Success = true
	case "tls_custom_load_key":
		text, err := readClipboardText()
		if err != nil {
			a.tlsEditor.Message = err.Error()
			a.tlsEditor.Success = false
			return
		}
		a.tlsEditor.PrivateKey = text
		a.tlsEditorDirty = true
		a.tlsEditor.Message = "Private key loaded from clipboard"
		a.tlsEditor.Success = true
	case "tls_custom_clear_certificate":
		a.tlsEditor.Certificate = ""
		a.tlsEditorDirty = true
		a.tlsEditor.Message = ""
		a.tlsEditor.Success = false
	case "tls_custom_clear_key":
		a.tlsEditor.PrivateKey = ""
		a.tlsEditorDirty = true
		a.tlsEditor.Message = ""
		a.tlsEditor.Success = false
	case "tls_custom":
		a.invokeCustomTLS()
	case "tls_custom_save":
		a.invokeCustomTLS()
	case "video_edid:jetkvm_default":
		a.invokeEDIDAction("jetkvm_default", videoEDIDPresetJetKVMDefault)
	case "video_edid:acer_b246wl":
		a.invokeEDIDAction("acer_b246wl", videoEDIDPresetAcerB246WL)
	case "video_edid:asus_pa248qv":
		a.invokeEDIDAction("asus_pa248qv", videoEDIDPresetASUSPA248QV)
	case "video_edid:dell_d2721h":
		a.invokeEDIDAction("dell_d2721h", videoEDIDPresetDellD2721H)
	case "video_edid:dell_idrac":
		a.invokeEDIDAction("dell_idrac", videoEDIDPresetDellIDRAC)
	case "video_edid_load_custom":
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
	case "video_edid_clear_custom":
		a.videoCustomEDID = ""
		a.videoCustomEDIDDirty = true
		a.videoCustomEDIDMessage = ""
		a.videoCustomEDIDSuccess = false
	case "video_edid_apply_custom":
		a.invokeCustomEDID()
	case "access_enable_password":
		a.setAccessEditorMode(accessEditorModeCreate)
	case "access_change_password":
		a.setAccessEditorMode(accessEditorModeUpdate)
	case "access_disable_password":
		a.setAccessEditorMode(accessEditorModeDisable)
	case "access_cancel_editor":
		a.clearAccessEditor("", false)
	case "access_focus_password":
		a.settingsInputFocus = settingsInputAccessPassword
	case "access_focus_confirm_password":
		a.settingsInputFocus = settingsInputAccessConfirmPassword
	case "access_focus_old_password":
		a.settingsInputFocus = settingsInputAccessOldPassword
	case "access_focus_new_password":
		a.settingsInputFocus = settingsInputAccessNewPassword
	case "access_focus_confirm_new_password":
		a.settingsInputFocus = settingsInputAccessConfirmNewPassword
	case "access_focus_disable_password":
		a.settingsInputFocus = settingsInputAccessDisablePassword
	case "access_submit":
		a.invokeLocalAuthSubmit()
	case "advanced_focus_ssh":
		a.settingsInputFocus = settingsInputAdvancedSSH
	case "advanced_save_ssh":
		a.invokeSaveSSHKey()
	case "usb_network_focus_uplink_interface":
		a.settingsInputFocus = settingsInputUSBNetworkUplinkInterface
	case "usb_network_focus_subnet":
		a.settingsInputFocus = settingsInputUSBNetworkSubnetCIDR
	case "network_focus_hostname":
		a.settingsInputFocus = settingsInputNetworkHostname
	case "network_focus_domain":
		a.settingsInputFocus = settingsInputNetworkDomain
	case "network_focus_http_proxy":
		a.settingsInputFocus = settingsInputNetworkHTTPProxy
	case "network_focus_ipv4_address":
		a.settingsInputFocus = settingsInputNetworkIPv4Address
	case "network_focus_ipv4_netmask":
		a.settingsInputFocus = settingsInputNetworkIPv4Netmask
	case "network_focus_ipv4_gateway":
		a.settingsInputFocus = settingsInputNetworkIPv4Gateway
	case "network_focus_ipv4_dns":
		a.settingsInputFocus = settingsInputNetworkIPv4DNS
	case "network_focus_ipv6_prefix":
		a.settingsInputFocus = settingsInputNetworkIPv6Prefix
	case "network_focus_ipv6_gateway":
		a.settingsInputFocus = settingsInputNetworkIPv6Gateway
	case "network_focus_ipv6_dns":
		a.settingsInputFocus = settingsInputNetworkIPv6DNS
	case "network_focus_time_sync_ntp":
		a.settingsInputFocus = settingsInputNetworkTimeSyncNTP
	case "network_focus_time_sync_http":
		a.settingsInputFocus = settingsInputNetworkTimeSyncHTTP
	case "network_save":
		a.invokeNetworkSave()
	case "network_renew_dhcp":
		a.invokeRenewDHCPLease()
	case "network_ipv4_mode:disabled":
		a.networkEditor.IPv4Mode = "disabled"
		a.networkEditorDirty = true
	case "network_ipv4_mode:dhcp":
		a.networkEditor.IPv4Mode = "dhcp"
		a.networkEditorDirty = true
	case "network_ipv4_mode:static":
		a.networkEditor.IPv4Mode = "static"
		a.networkEditorDirty = true
	case "network_ipv6_mode:disabled":
		a.networkEditor.IPv6Mode = "disabled"
		a.networkEditorDirty = true
	case "network_ipv6_mode:slaac":
		a.networkEditor.IPv6Mode = "slaac"
		a.networkEditorDirty = true
	case "network_ipv6_mode:static":
		a.networkEditor.IPv6Mode = "static"
		a.networkEditorDirty = true
	case "network_mdns_mode:auto":
		a.networkEditor.MDNSMode = "auto"
		a.networkEditorDirty = true
	case "network_mdns_mode:disabled":
		a.networkEditor.MDNSMode = "disabled"
		a.networkEditorDirty = true
	case "network_mdns_mode:ipv4_only":
		a.networkEditor.MDNSMode = "ipv4_only"
		a.networkEditorDirty = true
	case "network_mdns_mode:ipv6_only":
		a.networkEditor.MDNSMode = "ipv6_only"
		a.networkEditorDirty = true
	case "network_time_sync_mode:ntp_only":
		a.networkEditor.TimeSyncMode = "ntp_only"
		a.networkEditorDirty = true
	case "network_time_sync_mode:ntp_and_http":
		a.networkEditor.TimeSyncMode = "ntp_and_http"
		a.networkEditorDirty = true
	case "network_time_sync_mode:http_only":
		a.networkEditor.TimeSyncMode = "http_only"
		a.networkEditorDirty = true
	case "network_time_sync_mode:custom":
		a.networkEditor.TimeSyncMode = "custom"
		a.networkEditorDirty = true
	case "usb_network_enabled_toggle":
		a.usbNetworkEditor.Enabled = !a.usbNetworkEditor.Enabled
		a.usbNetworkEditorDirty = true
	case "usb_network_dhcp_toggle":
		a.usbNetworkEditor.DHCPEnabled = !a.usbNetworkEditor.DHCPEnabled
		a.usbNetworkEditorDirty = true
	case "usb_network_dns_proxy_toggle":
		a.usbNetworkEditor.DNSProxyEnabled = !a.usbNetworkEditor.DNSProxyEnabled
		a.usbNetworkEditorDirty = true
	case "usb_network_host_preset:auto":
		a.usbNetworkEditor.HostPreset = "auto"
		a.usbNetworkEditorDirty = true
	case "usb_network_host_preset:linux":
		a.usbNetworkEditor.HostPreset = "linux"
		a.usbNetworkEditorDirty = true
	case "usb_network_host_preset:macos":
		a.usbNetworkEditor.HostPreset = "macos"
		a.usbNetworkEditorDirty = true
	case "usb_network_host_preset:windows":
		a.usbNetworkEditor.HostPreset = "windows"
		a.usbNetworkEditorDirty = true
	case "usb_network_host_preset:custom":
		a.usbNetworkEditor.HostPreset = "custom"
		a.usbNetworkEditorDirty = true
	case "usb_network_protocol:ecm":
		a.usbNetworkEditor.Protocol = "ecm"
		a.usbNetworkEditorDirty = true
	case "usb_network_protocol:ncm":
		a.usbNetworkEditor.Protocol = "ncm"
		a.usbNetworkEditorDirty = true
	case "usb_network_protocol:rndis":
		a.usbNetworkEditor.Protocol = "rndis"
		a.usbNetworkEditorDirty = true
	case "usb_network_sharing_mode:nat":
		a.usbNetworkEditor.SharingMode = "nat"
		a.usbNetworkEditorDirty = true
	case "usb_network_sharing_mode:bridge":
		a.usbNetworkEditor.SharingMode = "bridge"
		a.usbNetworkEditorDirty = true
	case "usb_network_uplink_mode:auto":
		a.usbNetworkEditor.UplinkMode = "auto"
		a.usbNetworkEditorDirty = true
	case "usb_network_uplink_mode:manual":
		a.usbNetworkEditor.UplinkMode = "manual"
		a.usbNetworkEditorDirty = true
	case "usb_network_save":
		a.invokeUSBNetworkSave()
	case "rotate_normal":
		if a.settingsActionPending(settingsGroupDisplayRotate) {
			return
		}
		a.withSettingsAction(settingsGroupDisplayRotate, "270", func() error {
			if err := a.ctrl.SetDisplayRotation(session.DisplayRotationNormal); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionHardware)
		})
	case "rotate_inverted":
		if a.settingsActionPending(settingsGroupDisplayRotate) {
			return
		}
		a.withSettingsAction(settingsGroupDisplayRotate, "90", func() error {
			if err := a.ctrl.SetDisplayRotation(session.DisplayRotationInverted); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionHardware)
		})
	case "backlight_brightness:0":
		a.invokeBacklightBrightnessAction("0", 0)
	case "backlight_brightness:10":
		a.invokeBacklightBrightnessAction("10", 10)
	case "backlight_brightness:35":
		a.invokeBacklightBrightnessAction("35", 35)
	case "backlight_brightness:64":
		a.invokeBacklightBrightnessAction("64", 64)
	case "backlight_dim:0":
		a.invokeBacklightDimAfterAction("0", 0)
	case "backlight_dim:60":
		a.invokeBacklightDimAfterAction("60", 60)
	case "backlight_dim:300":
		a.invokeBacklightDimAfterAction("300", 300)
	case "backlight_dim:600":
		a.invokeBacklightDimAfterAction("600", 600)
	case "backlight_dim:1800":
		a.invokeBacklightDimAfterAction("1800", 1800)
	case "backlight_dim:3600":
		a.invokeBacklightDimAfterAction("3600", 3600)
	case "backlight_off:0":
		a.invokeBacklightOffAfterAction("0", 0)
	case "backlight_off:300":
		a.invokeBacklightOffAfterAction("300", 300)
	case "backlight_off:600":
		a.invokeBacklightOffAfterAction("600", 600)
	case "backlight_off:1800":
		a.invokeBacklightOffAfterAction("1800", 1800)
	case "backlight_off:3600":
		a.invokeBacklightOffAfterAction("3600", 3600)
	case "hardware_hdmi_sleep_toggle":
		a.invokeVideoSleepToggle()
	case "usb_emulation_toggle":
		if a.settingsActionPending(settingsGroupUSBEmulation) {
			return
		}
		a.mu.RLock()
		usbEnabled := a.sectionData.Hardware.State.USBEmulation
		a.mu.RUnlock()
		if usbEnabled == nil {
			return
		}
		next := !*usbEnabled
		choice := "off"
		if next {
			choice = "on"
		}
		a.withSettingsAction(settingsGroupUSBEmulation, choice, func() error {
			if err := a.ctrl.SetUSBEmulation(next); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionHardware)
		})
	case "auto_update_on":
		if a.settingsActionPending(settingsGroupAutoUpdate) {
			return
		}
		a.withSettingsAction(settingsGroupAutoUpdate, "on", func() error {
			if err := a.ctrl.SetAutoUpdateState(true); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionGeneral)
		})
	case "auto_update_off":
		if a.settingsActionPending(settingsGroupAutoUpdate) {
			return
		}
		a.withSettingsAction(settingsGroupAutoUpdate, "off", func() error {
			if err := a.ctrl.SetAutoUpdateState(false); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionGeneral)
		})
	case "developer_mode_toggle":
		if a.settingsActionPending(settingsGroupDeveloperMode) {
			return
		}
		a.mu.RLock()
		devMode := a.sectionData.Advanced.State.DevMode
		a.mu.RUnlock()
		if devMode == nil {
			return
		}
		next := !*devMode
		choice := "off"
		if next {
			choice = "on"
		}
		a.withSettingsAction(settingsGroupDeveloperMode, choice, func() error {
			if err := a.ctrl.SetDeveloperModeState(next); err != nil {
				return err
			}
			return a.refreshSettingsSectionSync(sectionAdvanced)
		})
	case "dev_channel_toggle":
		a.invokeDevChannelToggle()
	case "loopback_only_toggle":
		a.invokeLoopbackOnlyToggle()
	case "factory_reset":
		a.factoryResetConfirm = true
		a.factoryResetMessage = ""
	case "factory_reset_cancel":
		a.factoryResetConfirm = false
	case "factory_reset_confirm":
		a.invokeFactoryReset()
	case "network_refresh":
		a.invokeRefreshNetworkServices()
	case "jiggler_disabled":
		a.invokeJigglerPresetAction("disabled", false, session.JigglerConfig{})
	case "jiggler_frequent":
		a.invokeJigglerPresetAction("frequent", true, session.JigglerConfig{
			InactivityLimitSeconds: 30,
			JitterPercentage:       25,
			ScheduleCronTab:        "*/30 * * * * *",
		})
	case "jiggler_standard":
		a.invokeJigglerPresetAction("standard", true, session.JigglerConfig{
			InactivityLimitSeconds: 60,
			JitterPercentage:       25,
			ScheduleCronTab:        "0 * * * * *",
		})
	case "jiggler_light":
		a.invokeJigglerPresetAction("light", true, session.JigglerConfig{
			InactivityLimitSeconds: 300,
			JitterPercentage:       25,
			ScheduleCronTab:        "0 */5 * * * *",
		})
	case "jiggler_custom":
		a.openJigglerEditor()
	case "jiggler_custom_cancel":
		a.closeJigglerEditor()
	case "jiggler_focus_cron":
		a.settingsInputFocus = settingsInputJigglerCron
	case "jiggler_focus_timezone":
		a.settingsInputFocus = settingsInputJigglerTimezone
	case "jiggler_custom_inactivity_minus":
		a.jigglerEditorConfig.InactivityLimitSeconds = maxInt(5, a.jigglerEditorConfig.InactivityLimitSeconds-5)
		a.jigglerEditorError = ""
	case "jiggler_custom_inactivity_plus":
		a.jigglerEditorConfig.InactivityLimitSeconds = minInt(3600, a.jigglerEditorConfig.InactivityLimitSeconds+5)
		a.jigglerEditorError = ""
	case "jiggler_custom_jitter_minus":
		a.jigglerEditorConfig.JitterPercentage = maxInt(0, a.jigglerEditorConfig.JitterPercentage-5)
		a.jigglerEditorError = ""
	case "jiggler_custom_jitter_plus":
		a.jigglerEditorConfig.JitterPercentage = minInt(100, a.jigglerEditorConfig.JitterPercentage+5)
		a.jigglerEditorError = ""
	case "jiggler_custom_save":
		a.invokeJigglerCustomSave()
	case "usb_devices_default":
		a.invokeUSBDevicesAction("default", defaultUSBDevices())
	case "usb_devices_keyboard_only":
		a.invokeUSBDevicesAction("keyboard_only", keyboardOnlyUSBDevices())
	case "usb_toggle_keyboard":
		a.toggleUSBDevice("keyboard")
	case "usb_toggle_absolute_mouse":
		a.toggleUSBDevice("absolute_mouse")
	case "usb_toggle_relative_mouse":
		a.toggleUSBDevice("relative_mouse")
	case "usb_toggle_mass_storage":
		a.toggleUSBDevice("mass_storage")
	case "usb_toggle_serial_console":
		a.toggleUSBDevice("serial_console")
	case "usb_toggle_network":
		a.toggleUSBDevice("network")
	case "mqtt_focus_broker":
		a.settingsInputFocus = settingsInputMQTTBroker
	case "mqtt_focus_port":
		a.settingsInputFocus = settingsInputMQTTPort
	case "mqtt_focus_username":
		a.settingsInputFocus = settingsInputMQTTUsername
	case "mqtt_focus_password":
		a.settingsInputFocus = settingsInputMQTTPassword
	case "mqtt_focus_base_topic":
		a.settingsInputFocus = settingsInputMQTTBaseTopic
	case "mqtt_focus_debounce":
		a.settingsInputFocus = settingsInputMQTTDebounce
	case "mqtt_enabled_toggle":
		a.mqttEditor.Enabled = !a.mqttEditor.Enabled
		a.mqttEditorDirty = true
		a.mqttTestMessage = ""
	case "mqtt_use_tls_toggle":
		a.mqttEditor.UseTLS = !a.mqttEditor.UseTLS
		a.mqttEditorDirty = true
		a.mqttTestMessage = ""
	case "mqtt_tls_insecure_toggle":
		a.mqttEditor.TLSInsecure = !a.mqttEditor.TLSInsecure
		a.mqttEditorDirty = true
		a.mqttTestMessage = ""
	case "mqtt_ha_discovery_toggle":
		a.mqttEditor.EnableHADiscovery = !a.mqttEditor.EnableHADiscovery
		a.mqttEditorDirty = true
		a.mqttTestMessage = ""
	case "mqtt_actions_toggle":
		a.mqttEditor.EnableActions = !a.mqttEditor.EnableActions
		a.mqttEditorDirty = true
		a.mqttTestMessage = ""
	case "mqtt_save_settings":
		if a.settingsActionPending(settingsGroupMQTTSave) || a.settingsActionPending(settingsGroupMQTTTest) {
			return
		}
		settings, err := a.mqttSettingsFromEditor()
		if err != nil {
			a.finishSettingsAction(settingsGroupMQTTSave, a.beginSettingsAction(settingsGroupMQTTSave, "save"), err)
			return
		}
		a.mqttTestMessage = ""
		a.withSettingsAction(settingsGroupMQTTSave, "save", func() error {
			if err := a.ctrl.SetMQTTSettings(settings); err != nil {
				return err
			}
			a.mqttEditorDirty = false
			return a.refreshSettingsSectionSync(sectionMQTT)
		})
	case "mqtt_test_connection":
		if a.settingsActionPending(settingsGroupMQTTSave) || a.settingsActionPending(settingsGroupMQTTTest) {
			return
		}
		settings, err := a.mqttSettingsFromEditor()
		if err != nil {
			a.finishSettingsAction(settingsGroupMQTTTest, a.beginSettingsAction(settingsGroupMQTTTest, "test"), err)
			return
		}
		a.mqttTestMessage = ""
		a.withSettingsAction(settingsGroupMQTTTest, "test", func() error {
			result, err := a.ctrl.TestMQTTConnection(settings)
			if err != nil {
				return err
			}
			a.mqttTestSuccess = result.Success
			if result.Success {
				a.mqttTestMessage = "Connection test succeeded"
			} else {
				a.mqttTestMessage = fallbackLabel(result.Error, "Connection test failed")
			}
			return nil
		})
	case "macro_create":
		a.setMacroEditorMode(macroEditorModeCreate, nil)
	case "macro_focus_name":
		a.settingsInputFocus = settingsInputMacroName
	case "macro_focus_keys":
		a.settingsInputFocus = settingsInputMacroKeys
	case "macro_focus_modifiers":
		a.settingsInputFocus = settingsInputMacroModifiers
	case "macro_focus_delay":
		a.settingsInputFocus = settingsInputMacroDelay
	case "macro_editor_cancel":
		a.clearMacroEditor("", false)
	case "macro_step_prev":
		if a.macroEditor.Selected > 0 {
			a.macroEditor.Selected--
		}
	case "macro_step_next":
		if a.macroEditor.Selected+1 < len(a.macroEditor.Steps) {
			a.macroEditor.Selected++
		}
	case "macro_step_add":
		if len(a.macroEditor.Steps) >= 10 {
			a.macroEditor.Message = "Maximum of 10 steps reached"
			a.macroEditor.Success = false
			return
		}
		a.macroEditor.Steps = append(a.macroEditor.Steps, macroEditorStep{Delay: "50"})
		a.macroEditor.Selected = len(a.macroEditor.Steps) - 1
		a.settingsInputFocus = settingsInputMacroKeys
		a.macroEditor.Message = ""
	case "macro_step_remove":
		if len(a.macroEditor.Steps) <= 1 || a.macroEditor.Selected < 0 || a.macroEditor.Selected >= len(a.macroEditor.Steps) {
			return
		}
		a.macroEditor.Steps = append(a.macroEditor.Steps[:a.macroEditor.Selected], a.macroEditor.Steps[a.macroEditor.Selected+1:]...)
		if a.macroEditor.Selected >= len(a.macroEditor.Steps) {
			a.macroEditor.Selected = len(a.macroEditor.Steps) - 1
		}
		a.macroEditor.Message = ""
	case "macro_step_up":
		if a.macroEditor.Selected <= 0 || a.macroEditor.Selected >= len(a.macroEditor.Steps) {
			return
		}
		i := a.macroEditor.Selected
		a.macroEditor.Steps[i-1], a.macroEditor.Steps[i] = a.macroEditor.Steps[i], a.macroEditor.Steps[i-1]
		a.macroEditor.Selected--
	case "macro_step_down":
		if a.macroEditor.Selected < 0 || a.macroEditor.Selected+1 >= len(a.macroEditor.Steps) {
			return
		}
		i := a.macroEditor.Selected
		a.macroEditor.Steps[i], a.macroEditor.Steps[i+1] = a.macroEditor.Steps[i+1], a.macroEditor.Steps[i]
		a.macroEditor.Selected++
	case "macro_save":
		a.invokeSaveMacro()
	case "layout:en-US":
		a.invokeKeyboardLayoutAction("en-US")
	default:
		if code, ok := strings.CutPrefix(id, "layout:"); ok {
			a.invokeKeyboardLayoutAction(code)
			return
		}
		if strings.HasPrefix(id, "discover:") {
			a.connectFromLauncher(strings.TrimPrefix(id, "discover:"))
			return
		}
		if macroID, ok := strings.CutPrefix(id, "macro_edit:"); ok {
			a.invokeEditMacro(macroID)
			return
		}
		if macroID, ok := strings.CutPrefix(id, "macro_duplicate:"); ok {
			a.invokeDuplicateMacro(macroID)
			return
		}
		if macroID, ok := strings.CutPrefix(id, "macro_delete:"); ok {
			a.invokeDeleteMacro(macroID)
			return
		}
		if macroID, ok := strings.CutPrefix(id, "macro_move_up:"); ok {
			a.invokeMoveMacro(macroID, -1)
			return
		}
		if macroID, ok := strings.CutPrefix(id, "macro_move_down:"); ok {
			a.invokeMoveMacro(macroID, 1)
			return
		}
		if len(id) > 8 && id[:8] == "section:" {
			section, ok := parseSettingsSection(id[8:])
			if !ok {
				return
			}
			a.settingsSection = section
			if section != sectionMouse {
				a.closeJigglerEditor()
			}
			if section != sectionAccess {
				switch a.settingsInputFocus {
				case settingsInputAccessPassword, settingsInputAccessConfirmPassword, settingsInputAccessOldPassword, settingsInputAccessNewPassword, settingsInputAccessConfirmNewPassword, settingsInputAccessDisablePassword:
					a.settingsInputFocus = settingsInputNone
				}
				if a.accessEditor.Mode != accessEditorModeNone {
					a.clearAccessEditor("", false)
				}
			}
			if section != sectionAdvanced && a.settingsInputFocus == settingsInputAdvancedSSH {
				a.settingsInputFocus = settingsInputNone
			}
			if section != sectionHardware {
				switch a.settingsInputFocus {
				case settingsInputUSBNetworkUplinkInterface, settingsInputUSBNetworkSubnetCIDR:
					a.settingsInputFocus = settingsInputNone
				}
			}
			if section != sectionNetwork {
				switch a.settingsInputFocus {
				case settingsInputNetworkHostname, settingsInputNetworkDomain, settingsInputNetworkHTTPProxy,
					settingsInputNetworkIPv4Address, settingsInputNetworkIPv4Netmask, settingsInputNetworkIPv4Gateway,
					settingsInputNetworkIPv4DNS, settingsInputNetworkIPv6Prefix, settingsInputNetworkIPv6Gateway,
					settingsInputNetworkIPv6DNS, settingsInputNetworkTimeSyncNTP, settingsInputNetworkTimeSyncHTTP:
					a.settingsInputFocus = settingsInputNone
				}
			}
			if section != sectionMacros {
				switch a.settingsInputFocus {
				case settingsInputMacroName, settingsInputMacroKeys, settingsInputMacroModifiers, settingsInputMacroDelay:
					a.settingsInputFocus = settingsInputNone
				}
				if a.macroEditor.Mode != macroEditorModeNone {
					a.clearMacroEditor("", false)
				}
			}
			if section != sectionMQTT {
				switch a.settingsInputFocus {
				case settingsInputMQTTBroker, settingsInputMQTTPort, settingsInputMQTTUsername, settingsInputMQTTPassword, settingsInputMQTTBaseTopic, settingsInputMQTTDebounce:
					a.settingsInputFocus = settingsInputNone
				}
			}
			a.refreshSettingsSection(a.settingsSection)
		}
	}
}

func (a *App) invokeKeyboardLayoutAction(layout string) {
	if a.settingsActionPending(settingsGroupKeyboardLayout) {
		return
	}
	a.withSettingsAction(settingsGroupKeyboardLayout, layout, func() error {
		return a.ctrl.SetKeyboardLayout(layout)
	})
}

func (a *App) invokeJigglerPresetAction(choice string, enabled bool, cfg session.JigglerConfig) {
	if a.settingsActionPending(settingsGroupJiggler) {
		return
	}
	a.withSettingsAction(settingsGroupJiggler, choice, func() error {
		if enabled {
			if err := a.ctrl.SetJigglerConfig(cfg); err != nil {
				return err
			}
		}
		if err := a.ctrl.SetJigglerState(enabled); err != nil {
			return err
		}
		a.closeJigglerEditor()
		return a.refreshSettingsSectionSync(sectionMouse)
	})
}

func (a *App) invokeVideoCodecAction(choice string, codec session.VideoCodec) {
	if a.settingsActionPending(settingsGroupVideoCodec) {
		return
	}
	a.withSettingsAction(settingsGroupVideoCodec, choice, func() error {
		if err := a.ctrl.SetVideoCodec(codec); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionVideo)
	})
}

func (a *App) openH265CodecConfirm() {
	if a.settingsActionPending(settingsGroupVideoCodec) || a.sectionData.Video.State.Codec == session.VideoCodecH265 {
		return
	}
	a.h265ConfirmOpen = true
}

func (a *App) confirmH265CodecAction() {
	a.h265ConfirmOpen = false
	a.invokeVideoCodecAction("h265", session.VideoCodecH265)
}

func (a *App) invokeATXShortPress() {
	a.invokeATXAction(session.ATXPowerActionShortPress)
}

func (a *App) openATXResetConfirm() {
	if a.settingsActionPending(settingsGroupATXPower) {
		return
	}
	a.atxConfirmAction = session.ATXPowerActionReset
}

func (a *App) openATXLongPressConfirm() {
	if a.settingsActionPending(settingsGroupATXPower) {
		return
	}
	a.atxConfirmAction = session.ATXPowerActionLongPress
}

func (a *App) confirmATXAction() {
	action := a.atxConfirmAction
	a.atxConfirmAction = ""
	if action == "" {
		return
	}
	a.invokeATXAction(action)
}

func (a *App) cancelATXAction() {
	a.atxConfirmAction = ""
}

func (a *App) invokeATXAction(action session.ATXPowerAction) {
	if action == "" || a.settingsActionPending(settingsGroupATXPower) {
		return
	}
	a.withSettingsAction(settingsGroupATXPower, string(action), func() error {
		if err := a.ctrl.SetATXPowerAction(action); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionATX)
	})
}

func (a *App) invokeActiveExtensionAction(extensionID string) {
	if a.settingsActionPending(settingsGroupActiveExtension) {
		return
	}
	a.withSettingsAction(settingsGroupActiveExtension, extensionID, func() error {
		if err := a.ctrl.SetActiveExtension(extensionID); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionATX)
	})
}

func (a *App) invokeDCPowerAction(enabled bool) {
	if a.settingsActionPending(settingsGroupDCPower) {
		return
	}
	choice := "off"
	if enabled {
		choice = "on"
	}
	a.withSettingsAction(settingsGroupDCPower, choice, func() error {
		if err := a.ctrl.SetDCPowerState(enabled); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionATX)
	})
}

func (a *App) invokeDCRestoreStateAction(state int) {
	if a.settingsActionPending(settingsGroupDCPower) {
		return
	}
	a.withSettingsAction(settingsGroupDCPower, fmt.Sprintf("restore:%d", state), func() error {
		if err := a.ctrl.SetDCRestoreState(state); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionATX)
	})
}

func (a *App) updateSerialSettings(choice string, mutate func(*session.SerialSettings)) {
	if a.settingsActionPending(settingsGroupSerialSettings) {
		return
	}
	a.mu.RLock()
	settings := a.sectionData.ATX.SerialSettings
	a.mu.RUnlock()
	if settings == nil {
		return
	}
	next := *settings
	next.Buttons = append([]session.QuickButton(nil), settings.Buttons...)
	mutate(&next)
	a.withSettingsAction(settingsGroupSerialSettings, choice, func() error {
		if err := a.ctrl.SetSerialSettings(next); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionATX)
	})
}

func (a *App) invokeSerialQuickCommand(button session.QuickButton) {
	if a.settingsActionPending(settingsGroupSerialCommand) {
		return
	}
	command := button.Command + button.Terminator.Value
	a.withSettingsAction(settingsGroupSerialCommand, button.ID, func() error {
		if err := a.ctrl.SendCustomCommand(command); err != nil {
			return err
		}
		a.mu.RLock()
		history := append([]string(nil), a.sectionData.ATX.SerialHistory...)
		a.mu.RUnlock()
		trimmed := strings.TrimSpace(button.Command)
		if trimmed != "" {
			history = append([]string{trimmed}, history...)
			if len(history) > 20 {
				history = history[:20]
			}
			if err := a.ctrl.SetSerialCommandHistory(history); err == nil {
				a.mu.Lock()
				a.sectionData.ATX.SerialHistory = history
				a.mu.Unlock()
			}
		}
		return nil
	})
}

func (a *App) invokeEDIDAction(choice, edid string) {
	if a.settingsActionPending(settingsGroupVideoEDID) {
		return
	}
	a.withSettingsAction(settingsGroupVideoEDID, choice, func() error {
		if err := a.ctrl.SetEDID(edid); err != nil {
			return err
		}
		a.videoCustomEDIDDirty = false
		a.videoCustomEDIDMessage = ""
		a.videoCustomEDIDSuccess = false
		return a.refreshSettingsSectionSync(sectionVideo)
	})
}

func (a *App) invokeCustomEDID() {
	if a.settingsActionPending(settingsGroupVideoEDID) {
		return
	}
	edid := strings.TrimSpace(a.videoCustomEDID)
	if edid == "" {
		a.finishSettingsAction(settingsGroupVideoEDID, a.beginSettingsAction(settingsGroupVideoEDID, "custom"), errors.New("custom EDID is empty"))
		return
	}
	a.withSettingsAction(settingsGroupVideoEDID, "custom", func() error {
		if err := a.ctrl.SetEDID(edid); err != nil {
			return err
		}
		a.videoCustomEDIDDirty = false
		a.videoCustomEDIDMessage = "Custom EDID applied"
		a.videoCustomEDIDSuccess = true
		return a.refreshSettingsSectionSync(sectionVideo)
	})
}

func (a *App) invokeBacklightBrightnessAction(choice string, brightness int) {
	a.updateBacklightSettings(choice, func(settings *session.BacklightSettings) {
		settings.MaxBrightness = brightness
	})
}

func (a *App) invokeBacklightDimAfterAction(choice string, dimAfter int) {
	a.updateBacklightSettings(choice, func(settings *session.BacklightSettings) {
		settings.DimAfter = dimAfter
		if settings.OffAfter != 0 && settings.DimAfter > settings.OffAfter {
			settings.DimAfter = 0
		}
	})
}

func (a *App) invokeBacklightOffAfterAction(choice string, offAfter int) {
	a.updateBacklightSettings(choice, func(settings *session.BacklightSettings) {
		settings.OffAfter = offAfter
		if settings.OffAfter != 0 && settings.DimAfter > settings.OffAfter {
			settings.DimAfter = 0
		}
	})
}

func (a *App) updateBacklightSettings(choice string, mutate func(*session.BacklightSettings)) {
	if a.settingsActionPending(settingsGroupBacklight) {
		return
	}
	a.mu.RLock()
	settings := a.sectionData.Hardware.State.Backlight
	a.mu.RUnlock()
	mutate(&settings)
	a.withSettingsAction(settingsGroupBacklight, choice, func() error {
		if err := a.ctrl.SetBacklightSettings(settings); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionHardware)
	})
}

func (a *App) invokeVideoSleepToggle() {
	if a.settingsActionPending(settingsGroupVideoSleep) {
		return
	}
	a.mu.RLock()
	mode := a.sectionData.Hardware.State.VideoSleepMode
	a.mu.RUnlock()
	if mode == nil {
		return
	}
	duration := 90
	choice := "on"
	if mode.Duration >= 0 {
		duration = -1
		choice = "off"
	}
	a.withSettingsAction(settingsGroupVideoSleep, choice, func() error {
		if err := a.ctrl.SetVideoSleepMode(duration); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionHardware)
	})
}

func (a *App) invokeDevChannelToggle() {
	if a.settingsActionPending(settingsGroupDevChannel) {
		return
	}
	a.mu.RLock()
	devChannel := a.sectionData.Advanced.State.DevChannel
	a.mu.RUnlock()
	if devChannel == nil {
		return
	}
	next := !*devChannel
	choice := "off"
	if next {
		choice = "on"
	}
	a.withSettingsAction(settingsGroupDevChannel, choice, func() error {
		if err := a.ctrl.SetDevChannelState(next); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionAdvanced)
	})
}

func (a *App) invokeLoopbackOnlyToggle() {
	if a.settingsActionPending(settingsGroupLoopbackOnly) {
		return
	}
	a.mu.RLock()
	loopback := a.sectionData.Advanced.State.LoopbackOnly
	a.mu.RUnlock()
	if loopback == nil {
		return
	}
	next := !*loopback
	choice := "off"
	if next {
		choice = "on"
	}
	a.withSettingsAction(settingsGroupLoopbackOnly, choice, func() error {
		if err := a.ctrl.SetLocalLoopbackOnly(next); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionAdvanced)
	})
}

func (a *App) invokeSaveSSHKey() {
	if a.settingsActionPending(settingsGroupSSHKey) {
		return
	}
	sshKey := strings.TrimSpace(a.advancedSSHKey)
	a.withSettingsAction(settingsGroupSSHKey, "save", func() error {
		if err := a.ctrl.SetSSHKeyState(sshKey); err != nil {
			return err
		}
		a.advancedSSHDirty = false
		a.mu.Lock()
		a.sectionData.Advanced.State.SSHKey = sshKey
		a.mu.Unlock()
		return nil
	})
}

func (a *App) invokeInstallUpdates() {
	if a.settingsActionPending(settingsGroupUpdateInstall) {
		return
	}
	a.withSettingsAction(settingsGroupUpdateInstall, "install", func() error {
		if err := a.ctrl.TryUpdate(); err != nil {
			return err
		}
		a.updateActionMessage = "Update install requested"
		a.updateActionSuccess = true
		return nil
	})
}

func (a *App) invokeCustomTLS() {
	if a.settingsActionPending(settingsGroupTLSMode) {
		return
	}
	a.withSettingsAction(settingsGroupTLSMode, "custom", func() error {
		if err := a.ctrl.SetTLSState(session.TLSState{
			Mode:        session.TLSModeCustom,
			Certificate: strings.TrimSpace(a.tlsEditor.Certificate),
			PrivateKey:  strings.TrimSpace(a.tlsEditor.PrivateKey),
		}); err != nil {
			return err
		}
		a.tlsEditorDirty = false
		a.tlsEditor.Message = "Custom TLS settings updated"
		a.tlsEditor.Success = true
		return a.refreshSettingsSectionSync(sectionAccess)
	})
}

func (a *App) invokeNetworkSave() {
	if a.settingsActionPending(settingsGroupNetworkSave) {
		return
	}
	settings, err := a.networkSettingsFromEditor()
	if err != nil {
		a.finishSettingsAction(settingsGroupNetworkSave, a.beginSettingsAction(settingsGroupNetworkSave, "save"), err)
		return
	}
	a.withSettingsAction(settingsGroupNetworkSave, "save", func() error {
		if err := a.ctrl.SetNetworkSettings(settings); err != nil {
			return err
		}
		a.networkEditorDirty = false
		return a.refreshSettingsSectionSync(sectionNetwork)
	})
}

func (a *App) invokeRefreshNetworkServices() {
	if a.settingsActionPending(settingsGroupNetworkRefresh) {
		return
	}
	a.withSettingsAction(settingsGroupNetworkRefresh, "refresh", func() error {
		return a.refreshSettingsSectionSync(sectionNetwork)
	})
}

func (a *App) invokeRenewDHCPLease() {
	if a.settingsActionPending(settingsGroupNetworkRenew) {
		return
	}
	a.withSettingsAction(settingsGroupNetworkRenew, "renew", func() error {
		if err := a.ctrl.RenewDHCPLease(); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionNetwork)
	})
}

func (a *App) invokeUSBNetworkSave() {
	if !a.cfg.ExperimentalUSBNetwork {
		return
	}
	if a.settingsActionPending(settingsGroupUSBNetworkSave) {
		return
	}
	cfg, err := a.usbNetworkConfigFromEditor()
	if err != nil {
		a.finishSettingsAction(settingsGroupUSBNetworkSave, a.beginSettingsAction(settingsGroupUSBNetworkSave, "save"), err)
		return
	}
	a.withSettingsAction(settingsGroupUSBNetworkSave, "save", func() error {
		if err := a.ctrl.SetUSBNetworkConfig(cfg); err != nil {
			return err
		}
		a.usbNetworkEditorDirty = false
		return a.refreshSettingsSectionSync(sectionHardware)
	})
}

func (a *App) invokeFactoryReset() {
	if a.settingsActionPending(settingsGroupFactoryReset) {
		return
	}
	a.withSettingsAction(settingsGroupFactoryReset, "confirm", func() error {
		if err := a.ctrl.FactoryReset(); err != nil {
			return err
		}
		a.factoryResetConfirm = false
		a.factoryResetMessage = "Factory reset requested"
		a.factoryResetSuccess = true
		return nil
	})
}

func (a *App) invokeEditMacro(id string) {
	macro, ok := a.findMacroByID(id)
	if !ok {
		return
	}
	a.setMacroEditorMode(macroEditorModeEdit, &macro)
}

func (a *App) invokeDuplicateMacro(id string) {
	if a.settingsActionPending(settingsGroupMacrosSave) {
		return
	}
	macro, ok := a.findMacroByID(id)
	if !ok {
		return
	}
	macros := a.currentMacros()
	copyMacro := macro
	copyMacro.ID = generateMacroID()
	copyMacro.Name = strings.TrimSpace(copyMacro.Name + " (copy)")
	macros = append(macros, copyMacro)
	macros = normalizeMacroSortOrders(macros)
	a.withSettingsAction(settingsGroupMacrosSave, "duplicate", func() error {
		if err := a.ctrl.SetKeyboardMacros(macros); err != nil {
			return err
		}
		a.clearMacroEditor("Macro duplicated", true)
		return a.refreshSettingsSectionSync(sectionMacros)
	})
}

func (a *App) invokeDeleteMacro(id string) {
	if a.settingsActionPending(settingsGroupMacrosSave) {
		return
	}
	macros := a.currentMacros()
	filtered := make([]session.KeyboardMacro, 0, len(macros))
	for _, macro := range macros {
		if macro.ID != id {
			filtered = append(filtered, macro)
		}
	}
	filtered = normalizeMacroSortOrders(filtered)
	a.withSettingsAction(settingsGroupMacrosSave, "delete", func() error {
		if err := a.ctrl.SetKeyboardMacros(filtered); err != nil {
			return err
		}
		if a.macroEditor.ExistingID == id {
			a.clearMacroEditor("Macro deleted", true)
		}
		return a.refreshSettingsSectionSync(sectionMacros)
	})
}

func (a *App) invokeMoveMacro(id string, delta int) {
	if a.settingsActionPending(settingsGroupMacrosSave) {
		return
	}
	macros := a.currentMacros()
	index := -1
	for i, macro := range macros {
		if macro.ID == id {
			index = i
			break
		}
	}
	if index == -1 {
		return
	}
	target := index + delta
	if target < 0 || target >= len(macros) {
		return
	}
	macros[index], macros[target] = macros[target], macros[index]
	macros = normalizeMacroSortOrders(macros)
	a.withSettingsAction(settingsGroupMacrosSave, "reorder", func() error {
		if err := a.ctrl.SetKeyboardMacros(macros); err != nil {
			return err
		}
		return a.refreshSettingsSectionSync(sectionMacros)
	})
}

func (a *App) invokeSaveMacro() {
	if a.settingsActionPending(settingsGroupMacrosSave) {
		return
	}
	macro, err := a.macroFromEditor()
	if err != nil {
		a.finishSettingsAction(settingsGroupMacrosSave, a.beginSettingsAction(settingsGroupMacrosSave, "save"), err)
		return
	}
	macros := a.currentMacros()
	switch a.macroEditor.Mode {
	case macroEditorModeCreate:
		macros = append(macros, macro)
	case macroEditorModeEdit:
		replaced := false
		for i := range macros {
			if macros[i].ID == macro.ID {
				macros[i].Name = macro.Name
				macros[i].Steps = macro.Steps
				replaced = true
				break
			}
		}
		if !replaced {
			macros = append(macros, macro)
		}
	default:
		return
	}
	macros = normalizeMacroSortOrders(macros)
	message := "Macro saved"
	if a.macroEditor.Mode == macroEditorModeCreate {
		message = "Macro created"
	}
	a.withSettingsAction(settingsGroupMacrosSave, "save", func() error {
		if err := a.ctrl.SetKeyboardMacros(macros); err != nil {
			return err
		}
		a.clearMacroEditor(message, true)
		return a.refreshSettingsSectionSync(sectionMacros)
	})
}

func (a *App) invokeJigglerCustomSave() {
	if a.settingsActionPending(settingsGroupJiggler) {
		return
	}
	if err := validateJigglerConfig(a.jigglerEditorConfig); err != nil {
		a.jigglerEditorError = err.Error()
		return
	}
	cfg := a.jigglerEditorConfig
	a.withSettingsAction(settingsGroupJiggler, "custom", func() error {
		if err := a.ctrl.SetJigglerConfig(cfg); err != nil {
			return err
		}
		if err := a.ctrl.SetJigglerState(true); err != nil {
			return err
		}
		a.closeJigglerEditor()
		return a.refreshSettingsSectionSync(sectionMouse)
	})
}

func (a *App) invokeUSBDevicesAction(choice string, devices session.USBDevices) {
	if a.settingsActionPending(settingsGroupUSBDevices) {
		return
	}
	a.withSettingsAction(settingsGroupUSBDevices, choice, func() error {
		if err := a.ctrl.SetUSBDevices(devices); err != nil {
			return err
		}
		a.setConnectionUSBDevices(devices)
		return a.refreshSettingsSectionSync(sectionHardware)
	})
}

func (a *App) toggleUSBDevice(kind string) {
	devices, loaded := a.connectionUSBDevicesSnapshot()
	if !loaded {
		a.mu.RLock()
		devices = a.sectionData.Hardware.State.USBDevices
		a.mu.RUnlock()
	}
	switch kind {
	case "keyboard":
		devices.Keyboard = !devices.Keyboard
	case "absolute_mouse":
		devices.AbsoluteMouse = !devices.AbsoluteMouse
	case "relative_mouse":
		devices.RelativeMouse = !devices.RelativeMouse
	case "mass_storage":
		devices.MassStorage = !devices.MassStorage
	case "serial_console":
		devices.SerialConsole = !devices.SerialConsole
	case "network":
		devices.Network = !devices.Network
	default:
		return
	}
	a.invokeUSBDevicesAction("custom", devices)
}

func (a *App) openJigglerEditor() {
	if a.jigglerEditorOpen {
		return
	}
	a.mu.RLock()
	state := a.sectionData.Mouse
	a.mu.RUnlock()
	cfg := state.JigglerConfig
	if cfg == nil {
		defaultCfg := standardJigglerConfig()
		cfg = &defaultCfg
	}
	a.jigglerEditorConfig = *cfg
	if a.jigglerEditorConfig.InactivityLimitSeconds == 0 {
		a.jigglerEditorConfig = standardJigglerConfig()
	}
	a.jigglerEditorOpen = true
	a.jigglerEditorError = ""
	a.settingsInputFocus = settingsInputJigglerCron
}

func (a *App) closeJigglerEditor() {
	a.jigglerEditorOpen = false
	a.jigglerEditorError = ""
	a.settingsInputFocus = settingsInputNone
}

func defaultUSBDevices() session.USBDevices {
	return session.USBDevices{
		Keyboard:      true,
		AbsoluteMouse: true,
		RelativeMouse: true,
		MassStorage:   true,
		SerialConsole: false,
		Network:       false,
	}
}

func keyboardOnlyUSBDevices() session.USBDevices {
	return session.USBDevices{
		Keyboard:      true,
		AbsoluteMouse: false,
		RelativeMouse: false,
		MassStorage:   false,
		SerialConsole: false,
		Network:       false,
	}
}

func standardJigglerConfig() session.JigglerConfig {
	return session.JigglerConfig{
		InactivityLimitSeconds: 60,
		JitterPercentage:       25,
		ScheduleCronTab:        "0 * * * * *",
	}
}

func validateJigglerConfig(cfg session.JigglerConfig) error {
	switch {
	case cfg.InactivityLimitSeconds <= 0:
		return errors.New("inactivity limit must be greater than zero")
	case cfg.JitterPercentage < 0:
		return errors.New("jitter percentage cannot be negative")
	case strings.TrimSpace(cfg.ScheduleCronTab) == "":
		return errors.New("cron schedule is required")
	default:
		return nil
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (a *App) syncChromeVisibility() {
	if a.ctrl == nil {
		return
	}
	snap := a.ctrl.Snapshot()
	hotZone := a.chromeRevealZone(a.lastWidth, a.lastHeight, snap)
	x, y := ebiten.CursorPosition()
	if x != a.lastUIX || y != a.lastUIY {
		if hotZone.contains(x, y) || a.settingsOpen || a.pasteOpen || a.mediaOpen || a.serialConsoleOpen {
			a.revealUIFor(1600 * time.Millisecond)
		}
		a.lastUIX = x
		a.lastUIY = y
	}
	if a.settingsOpen || a.pasteOpen || a.mediaOpen || a.serialConsoleOpen {
		a.applyCursorMode()
		a.revealUIFor(500 * time.Millisecond)
	}
}

func (a *App) syncSessionState() {
	if a.ctrl == nil {
		return
	}
	snap := a.ctrl.Snapshot()
	phase := snap.Phase
	if phase == session.PhaseAuthFailed && a.lastPhase != session.PhaseAuthFailed {
		errMsg := ""
		if a.launcherMode == launcherModePassword {
			errMsg = authPromptError(snap.LastError)
		}
		a.showPasswordPrompt(a.cfg.BaseURL, errMsg)
		a.settingsOpen = false
		a.pasteOpen = false
		a.statsOpen = false
		a.mediaOpen = false
		a.serialConsoleOpen = false
		a.relative = false
		a.applyCursorMode()
	}
	if phase == a.lastPhase {
		return
	}
	if a.lastPhase == session.PhaseConnected && phase != session.PhaseConnected {
		a.resetConnectionHardwareState()
		if a.pasteOpen {
			a.pasteOpen = false
		}
		if a.mediaOpen {
			a.mediaOpen = false
		}
		if a.serialConsoleOpen {
			a.serialConsoleOpen = false
		}
		a.releaseAllKeys(false)
		a.releasePointerState()
		a.lastButtons = 0
		if a.relative {
			a.relative = false
			a.applyCursorMode()
		}
	}
	if phase == session.PhaseConnected && a.lastPhase != session.PhaseConnected {
		a.resetConnectionHardwareState()
		a.maybeExpandBrowseWindow()
		a.launcherOpen = false
		a.launcherMode = launcherModeBrowse
		a.launcherError = ""
		a.launcherPassword = ""
		a.lastX, a.lastY = ebiten.CursorPosition()
		a.lastButtons = 0
		a.revealUIFor(2 * time.Second)
		if a.prefs.AbsoluteSideButtonsViaRel {
			a.ensureConnectionUSBDevicesLoaded()
		}
	}
	a.lastPhase = phase
}

func (a *App) syncWindowTitle() {
	if a.launcherOpen {
		title := "jetkvm-desktop"
		if title != a.lastTitle {
			ebiten.SetWindowTitle(title)
			a.lastTitle = title
		}
		return
	}
	if a.ctrl == nil {
		return
	}
	snap := a.ctrl.Snapshot()
	title := "jetkvm-desktop"
	if snap.DeviceID != "" {
		title = snap.DeviceID
	} else if snap.Hostname != "" {
		title = snap.Hostname
	}
	title = fmt.Sprintf("%s [%s]", title, snap.Phase.String())
	if title == a.lastTitle {
		return
	}
	ebiten.SetWindowTitle(title)
	a.lastTitle = title
}

func (a *App) releaseAllKeys(send bool) {
	if a.hotkeys != nil {
		a.hotkeys.Reset()
	}
	if send {
		for _, evt := range a.keyboard.ReleaseAll() {
			_ = a.ctrl.SendKeypress(evt.HID, evt.Press)
		}
		return
	}
	_ = a.keyboard.ReleaseAll()
}

func (a *App) armOverlayDismissSuppression() {
	a.releaseAllKeys(false)
	a.suppressKeysUntilClear = true
	a.suppressMouseUntilUp = true
	a.lastButtons = 0
	a.lastX, a.lastY = ebiten.CursorPosition()
}

func (a *App) closePasteOverlay() {
	if !a.pasteOpen {
		return
	}
	a.pasteOpen = false
	a.armOverlayDismissSuppression()
	a.applyCursorMode()
}

func (a *App) closeSettingsOverlay() {
	if !a.settingsOpen {
		return
	}
	a.settingsOpen = false
	a.h265ConfirmOpen = false
	a.atxConfirmAction = ""
	a.settingsInputFocus = settingsInputNone
	a.closeJigglerEditor()
	a.clearAccessEditor("", false)
	a.armOverlayDismissSuppression()
	a.applyCursorMode()
}

func (a *App) closeMediaOverlay() {
	if !a.mediaOpen || a.mediaUploading {
		return
	}
	a.mediaOpen = false
	a.mediaURLFocused = false
	a.mediaUploadFocused = false
	a.armOverlayDismissSuppression()
	a.applyCursorMode()
}

func (a *App) closeSerialConsoleOverlay() {
	if !a.serialConsoleOpen {
		return
	}
	a.serialConsoleOpen = false
	a.armOverlayDismissSuppression()
	a.applyCursorMode()
}

func (a *App) releasePointerState() {
	if a.lastButtons == 0 {
		return
	}
	if a.relative {
		_ = a.ctrl.SendRelMouse(0, 0, 0)
		return
	}
	if a.renderRect.valid() {
		nx, ny := a.renderRect.toHID(a.lastX, a.lastY)
		_ = a.ctrl.SendAbsPointer(nx, ny, 0)
	}
}

func (a *App) drawOverlay(screen *ebiten.Image, snap session.Snapshot, hasVideo bool) {
	a.overlayRuntime.BeginFrame()
	title := ""
	detail := ""
	switch snap.Phase {
	case session.PhaseConnecting:
		title = "Connecting"
		detail = "Opening auth, WebRTC, and HID channels"
	case session.PhaseReconnecting:
		title = "Reconnecting"
		detail = snap.Status
	case session.PhaseAuthFailed:
		title = "Authentication Failed"
		detail = "Check the password and local auth mode"
	case session.PhaseOtherSession:
		title = "Session Replaced"
		detail = "Another client took over the device"
	case session.PhaseRebooting:
		title = "Rebooting"
		detail = "Waiting for the device to come back"
	case session.PhaseDisconnected:
		title = "Disconnected"
		detail = "The session dropped before it became ready"
	case session.PhaseFatal:
		title = "Fatal Error"
		detail = snap.LastError
	case session.PhaseConnected:
		if !hasVideo || !snap.VideoReady {
			title = "Loading Video"
			detail = "Waiting for the first decoded frame"
		}
	}
	if title == "" {
		return
	}
	if detail == "" && snap.LastError != "" && snap.Phase != session.PhaseConnected {
		detail = snap.LastError
	}
	a.drawUIRoot(screen, &a.overlayRuntime, func(chromeButton) {}, overlayBannerRootElement{
		title:      title,
		detail:     detail,
		takeover:   snap.Phase == session.PhaseOtherSession,
		withButton: snap.Phase == session.PhaseOtherSession,
		width:      min(420, float64(screen.Bounds().Dx()-52)),
		onClick: func() {
			a.releaseAllKeys(true)
			a.ctrl.ReconnectNow()
			a.revealUIFor(2 * time.Second)
		},
	})
}

func (a *App) drawPressedKeysOverlay(screen *ebiten.Image) {
	if !a.showPressedKeys || a.settingsOpen || a.mediaOpen || a.serialConsoleOpen {
		return
	}
	pressed := a.keyboard.Pressed()
	if len(pressed) == 0 {
		return
	}
	line := "Keys: "
	for i, key := range pressed {
		if i > 0 {
			line += "  "
		}
		line += key.String()
	}
	w, _ := ui.MeasureText(line, 12)
	x := 14.0
	y := float64(screen.Bounds().Dy()) - 58
	a.drawUIRoot(screen, nil, func(chromeButton) {}, pressedKeysOverlayElement{
		text: line,
		x:    x,
		y:    y,
		w:    w + 20,
	})
}

type overlayBannerElement struct {
	title      string
	detail     string
	takeover   bool
	withButton bool
	onClick    func()
}

func (e overlayBannerElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e overlayBannerElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: e.title, Size: 22, Color: ctx.Theme.Title}),
	}
	if e.detail != "" {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 10}),
			ui.Fixed(ui.Label{Text: e.detail, Size: 14, Color: ctx.Theme.Muted}),
		)
	}
	if e.withButton {
		children = append(children,
			ui.Fixed(ui.Spacer{H: 12}),
			ui.Fixed(ui.Button{Label: "Take Back Control", Enabled: true, Active: true, OnClick: e.onClick}),
		)
	}
	ui.Column{Children: children}.Draw(ctx, bounds)
}

type overlayBannerRootElement struct {
	title      string
	detail     string
	takeover   bool
	withButton bool
	width      float64
	onClick    func()
}

func (overlayBannerRootElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e overlayBannerRootElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	x := bounds.W - e.width - 26
	if x < 12 {
		x = 12
	}
	ui.Positioned{
		X: x,
		Y: 84,
		W: e.width,
		H: 96,
		Child: ui.Panel{
			Fill:   ctx.Theme.ModalFill,
			Stroke: ctx.Theme.ModalStroke,
			Insets: ui.Insets{Top: 20, Right: 16, Bottom: 12, Left: 16},
			Child: overlayBannerElement{
				title:      e.title,
				detail:     e.detail,
				takeover:   e.takeover,
				withButton: e.withButton,
				onClick:    e.onClick,
			},
		},
	}.Draw(ctx, bounds)
}

type pressedKeysOverlayElement struct {
	text string
	x    float64
	y    float64
	w    float64
}

func (pressedKeysOverlayElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e pressedKeysOverlayElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Positioned{
		X: e.x,
		Y: e.y,
		W: e.w,
		H: 28,
		Child: ui.Panel{
			Fill:   ctx.Theme.ModalFill,
			Stroke: ctx.Theme.ModalStroke,
			Insets: ui.SymmetricInsets(10, 8),
			Child:  ui.Label{Text: e.text, Size: 12, Color: ctx.Theme.Body},
		},
	}.Draw(ctx, bounds)
}

func rtcLabel(state webrtc.PeerConnectionState) string {
	return state.String()
}

func signalingLabel(mode client.SignalingMode) string {
	if mode == client.SignalingModeUnknown {
		return "pending"
	}
	return mode.String()
}

func trimForFooter(value string) string {
	if len(value) <= 42 {
		return value
	}
	return value[:39] + "..."
}

type rect struct {
	x float64
	y float64
	w float64
	h float64
}

func (r rect) valid() bool {
	return r.w > 0 && r.h > 0
}

func (r rect) contains(cursorX, cursorY int) bool {
	return float64(cursorX) >= r.x && float64(cursorX) <= r.x+r.w &&
		float64(cursorY) >= r.y && float64(cursorY) <= r.y+r.h
}

func (r rect) toHID(cursorX, cursorY int) (int32, int32) {
	if !r.valid() {
		return 0, 0
	}
	relX := clamp((float64(cursorX)-r.x)/r.w, 0, 1)
	relY := clamp((float64(cursorY)-r.y)/r.h, 0, 1)
	return int32(relX * 32767.0), int32(relY * 32767.0)
}

func reconnectLabel(phase session.Phase) string {
	switch phase {
	case session.PhaseConnected:
		return "Reconnect"
	case session.PhaseConnecting, session.PhaseReconnecting:
		return "Retry"
	default:
		return "Connect"
	}
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("host or URL is required")
	}
	if !strings.Contains(raw, "://") {
		if !isValidConnectHost(raw) {
			return "", errors.New("enter a valid hostname or IP address")
		}
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid target")
	}
	host := parsed.Hostname()
	if host == "" || !isValidConnectHost(host) {
		return "", errors.New("enter a valid hostname or IP address")
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func isValidConnectHost(raw string) bool {
	if raw == "" {
		return false
	}
	if strings.ContainsAny(raw, "/?#") {
		return false
	}
	if ip := net.ParseIP(raw); ip != nil {
		return true
	}
	return isValidHostname(raw)
}

func isValidHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		host = strings.Trim(host, ".")
	}
	labels := strings.Split(host, ".")
	if len(labels) == 0 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func (a *App) connectFromLauncher(target string) {
	baseURL, err := normalizeBaseURL(target)
	if err != nil {
		a.launcherError = err.Error()
		return
	}
	a.launcherError = ""
	a.pendingTarget = baseURL
	a.launcherInput = baseURL
	a.launcherOpen = false
	a.connectTo(baseURL)
}

func (a *App) showPasswordPrompt(target, errMsg string) {
	a.pendingTarget = target
	a.launcherOpen = true
	a.launcherMode = launcherModePassword
	a.launcherError = errMsg
}

func authPromptError(lastError string) string {
	lastError = strings.TrimSpace(lastError)
	if lastError == "" {
		return "Authentication failed"
	}
	return lastError
}

func (a *App) effectivePassword() string {
	if a.launcherPassword != "" {
		return a.launcherPassword
	}
	return a.cfg.Password
}

func (a *App) connectTo(target string) {
	baseURL, err := normalizeBaseURL(target)
	if err != nil {
		a.launcherError = err.Error()
		a.launcherOpen = true
		a.launcherMode = launcherModeBrowse
		return
	}
	if a.ctrl != nil {
		a.ctrl.Stop()
	}
	password := a.effectivePassword()
	a.cfg.BaseURL = baseURL
	a.cfg.Password = password
	a.lastImg = nil
	a.lastFrameAt = time.Time{}
	a.lastPhase = session.PhaseIdle
	a.resetConnectionHardwareState()
	a.stats = client.StatsSnapshot{}
	a.statsHistory = nil
	a.ctrl = session.New(session.Config{
		BaseURL:    baseURL,
		Password:   password,
		RPCTimeout: a.cfg.RPCTimeout,
		Reconnect:  true,
	})
	if a.ctx != nil {
		a.ctrl.Start(a.ctx)
	}
}
