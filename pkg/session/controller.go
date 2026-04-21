package session

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
	"github.com/lkarlslund/jetkvm-desktop/pkg/hotkeys"
	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/logging"
	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/auth"
	"github.com/lkarlslund/jetkvm-desktop/pkg/virtualmedia"
)

//go:generate go tool github.com/dmarkham/enumer -type=Phase -linecomment -text -output controller_enums.go

type Phase uint8

const (
	PhaseIdle         Phase = iota // idle
	PhaseConnecting                // connecting
	PhaseConnected                 // connected
	PhaseReconnecting              // reconnecting
	PhaseDisconnected              // disconnected
	PhaseAuthFailed                // auth_failed
	PhaseOtherSession              // other_session
	PhaseRebooting                 // rebooting
	PhaseFatal                     // fatal_error
)

type Config struct {
	BaseURL         string
	Password        string
	RPCTimeout      time.Duration
	MutationTimeout time.Duration
	Reconnect       bool
	ReconnectBase   time.Duration
	ReconnectMax    time.Duration
}

type Snapshot struct {
	Phase                  Phase
	Status                 string
	BaseURL                string
	DeviceID               string
	Hostname               string
	ActiveExtension        string
	ATXState               *ATXState
	SerialSettings         *SerialSettings
	SerialConsoleReady     bool
	SerialConsoleBuffer    string
	SerialConsoleError     string
	SerialConsoleTruncated bool
	Quality                float64
	KeyboardLayout         string
	EDID                   string
	AppVersion             string
	SystemVersion          string
	AppUpdateAvailable     bool
	SystemUpdateAvailable  bool
	HIDReady               bool
	VideoReady             bool
	LastError              string
	RTCState               webrtc.PeerConnectionState
	SignalingMode          client.SignalingMode
	PasteInProgress        bool
}

type Controller struct {
	cfg Config

	mu        sync.RWMutex
	snapshot  Snapshot
	serialLog serialScrollback
	current   *client.Client
	runParent context.Context
	cancelRun context.CancelFunc
	running   bool
}

const (
	serialScrollbackMaxLines = 5000
	serialScrollbackMaxBytes = 1 << 20
)

type serialScrollback struct {
	chunks    []string
	lineCount int
	byteCount int
	truncated bool
}

func (s *serialScrollback) Reset() {
	s.chunks = nil
	s.lineCount = 0
	s.byteCount = 0
	s.truncated = false
}

func (s *serialScrollback) Append(text string) (string, bool) {
	if text == "" {
		return s.String(), s.truncated
	}
	s.chunks = append(s.chunks, text)
	s.byteCount += len(text)
	s.lineCount += serialChunkLines(text)
	for s.byteCount > serialScrollbackMaxBytes || s.lineCount > serialScrollbackMaxLines {
		if len(s.chunks) == 0 {
			break
		}
		dropped := s.chunks[0]
		s.chunks = s.chunks[1:]
		s.byteCount -= len(dropped)
		s.lineCount -= serialChunkLines(dropped)
		s.truncated = true
	}
	return s.String(), s.truncated
}

func (s *serialScrollback) String() string {
	if len(s.chunks) == 0 {
		return ""
	}
	return strings.Join(s.chunks, "")
}

func serialChunkLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func defaultSerialSettings() SerialSettings {
	return SerialSettings{
		BaudRate: 9600,
		DataBits: 8,
		Parity:   "none",
		StopBits: "1",
		Terminator: Terminator{
			Label: "LF (\\n)",
			Value: "\n",
		},
		HideSerialSettings: false,
		EnableEcho:         false,
		NormalizeMode:      "names",
		NormalizeLineEnd:   "keep",
		TabRender:          "",
		PreserveANSI:       true,
		ShowNLTag:          true,
		Buttons:            []QuickButton{},
	}
}

func New(cfg Config) *Controller {
	if cfg.RPCTimeout == 0 {
		cfg.RPCTimeout = 5 * time.Second
	}
	if cfg.MutationTimeout == 0 {
		cfg.MutationTimeout = 20 * time.Second
	}
	if cfg.ReconnectBase == 0 {
		cfg.ReconnectBase = 500 * time.Millisecond
	}
	if cfg.ReconnectMax == 0 {
		cfg.ReconnectMax = 10 * time.Second
	}
	return &Controller{
		cfg: cfg,
		snapshot: Snapshot{
			Phase:   PhaseIdle,
			Status:  "idle",
			BaseURL: cfg.BaseURL,
		},
	}
}

func (c *Controller) Start(parent context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancelRun != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	c.runParent = ctx
	c.cancelRun = cancel
	c.running = true
	go c.run(ctx)
}

func (c *Controller) Stop() {
	c.mu.Lock()
	if c.cancelRun != nil {
		c.cancelRun()
		c.cancelRun = nil
	}
	c.running = false
	c.runParent = nil
	if c.current != nil {
		_ = c.current.Close()
		c.current = nil
	}
	c.mu.Unlock()
}

func (c *Controller) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshot
}

func (c *Controller) LatestFrame() image.Image {
	c.mu.RLock()
	current := c.current
	c.mu.RUnlock()
	if current == nil {
		return nil
	}
	return current.LatestFrame()
}

func (c *Controller) LatestFrameInfo() (image.Image, time.Time) {
	c.mu.RLock()
	current := c.current
	c.mu.RUnlock()
	if current == nil {
		return nil, time.Time{}
	}
	return current.LatestFrameInfo()
}

func (c *Controller) ReconnectNow() {
	c.mu.Lock()
	current := c.current
	parent := c.runParent
	shouldStart := !c.running && c.cancelRun != nil && parent != nil
	if shouldStart {
		c.running = true
	}
	c.mu.Unlock()
	if current != nil {
		_ = current.Close()
	}
	if shouldStart {
		go c.run(parent)
	}
}

func (c *Controller) SetPassword(password string) {
	c.mu.Lock()
	c.cfg.Password = password
	c.mu.Unlock()
}

func (c *Controller) Reboot() error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.Reboot(ctx)
}

func (c *Controller) TryUpdate() error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.TryUpdate(ctx)
}

func (c *Controller) FactoryReset() error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.FactoryReset(ctx)
}

func (c *Controller) SetQuality(value float64) error {
	if err := c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetStreamQualityFactor(ctx, value)
	}, func(ctx context.Context) (bool, error) {
		current := c.clientIfConnected()
		if current == nil {
			return false, errors.New("client not connected")
		}
		quality, err := current.GetStreamQualityFactor(ctx)
		if err != nil {
			return false, err
		}
		return quality == value, nil
	}); err != nil {
		return err
	}
	c.setState(func(s *Snapshot) {
		s.Quality = value
	})
	return nil
}

func (c *Controller) SetKeyboardLayout(layout string) error {
	layout = normalizeKeyboardLayoutCode(layout)
	if layout == "" {
		return errors.New("keyboard layout is required")
	}
	if err := c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetKeyboardLayout(ctx, layout)
	}, func(ctx context.Context) (bool, error) {
		current := c.clientIfConnected()
		if current == nil {
			return false, errors.New("client not connected")
		}
		currentLayout, err := current.GetKeyboardLayout(ctx)
		if err != nil {
			return false, err
		}
		return normalizeKeyboardLayoutCode(currentLayout) == layout, nil
	}); err != nil {
		return err
	}
	c.setState(func(s *Snapshot) {
		s.KeyboardLayout = layout
	})
	return nil
}

func (c *Controller) SetTLSMode(mode TLSMode) error {
	if mode == "" {
		return errors.New("tls mode is required")
	}
	return c.SetTLSState(TLSState{Mode: mode})
}

func (c *Controller) SetDisplayRotation(rotation DisplayRotation) error {
	if rotation == "" {
		return errors.New("display rotation is required")
	}
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetDisplayRotation(ctx, string(rotation))
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetDisplayRotation(ctx)
		if err != nil {
			return false, err
		}
		return current == rotation, nil
	})
}

func (c *Controller) SetUSBEmulation(enabled bool) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetUSBEmulationState(ctx, enabled)
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetUSBEmulationState(ctx)
		if err != nil {
			return false, err
		}
		return current == enabled, nil
	})
}

func (c *Controller) SetATXPowerAction(action ATXPowerAction) error {
	if action == "" {
		return errors.New("ATX power action is required")
	}
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SetATXPowerAction(withTimeout(context.Background(), c.cfg.MutationTimeout), string(action))
}

func (c *Controller) SetActiveExtension(extensionID string) error {
	if err := c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetActiveExtension(ctx, extensionID)
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetActiveExtension(ctx)
		if err != nil {
			return false, err
		}
		return current == extensionID, nil
	}); err != nil {
		return err
	}

	c.setState(func(s *Snapshot) {
		s.ActiveExtension = extensionID
		s.SerialConsoleError = ""
	})
	if extensionID != "serial-console" {
		c.setState(func(s *Snapshot) {
			s.SerialSettings = nil
		})
		return nil
	}

	settings, err := c.GetSerialSettings(withTimeout(context.Background(), c.cfg.RPCTimeout))
	if err != nil {
		defaults := defaultSerialSettings()
		c.setState(func(s *Snapshot) {
			s.SerialSettings = &defaults
			s.SerialConsoleError = err.Error()
		})
		return nil
	}
	c.setState(func(s *Snapshot) {
		s.SerialSettings = settings
	})
	return nil
}

func (c *Controller) SetDCPowerState(enabled bool) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SetDCPowerState(withTimeout(context.Background(), c.cfg.MutationTimeout), enabled)
}

func (c *Controller) SetDCRestoreState(state int) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SetDCRestoreState(withTimeout(context.Background(), c.cfg.MutationTimeout), state)
}

func (c *Controller) SetSerialSettings(settings SerialSettings) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	buttons := make([]client.QuickButton, 0, len(settings.Buttons))
	for _, button := range settings.Buttons {
		buttons = append(buttons, client.QuickButton{
			ID:      button.ID,
			Label:   button.Label,
			Command: button.Command,
			Terminator: client.Terminator{
				Label: button.Terminator.Label,
				Value: button.Terminator.Value,
			},
			Sort: button.Sort,
		})
	}
	if err := current.SetSerialSettings(withTimeout(context.Background(), c.cfg.MutationTimeout), client.SerialSettings{
		BaudRate: settings.BaudRate,
		DataBits: settings.DataBits,
		Parity:   settings.Parity,
		StopBits: settings.StopBits,
		Terminator: client.Terminator{
			Label: settings.Terminator.Label,
			Value: settings.Terminator.Value,
		},
		HideSerialSettings: settings.HideSerialSettings,
		EnableEcho:         settings.EnableEcho,
		NormalizeMode:      settings.NormalizeMode,
		NormalizeLineEnd:   settings.NormalizeLineEnd,
		TabRender:          settings.TabRender,
		PreserveANSI:       settings.PreserveANSI,
		ShowNLTag:          settings.ShowNLTag,
		Buttons:            buttons,
	}); err != nil {
		return err
	}
	settingsCopy := settings
	c.setState(func(s *Snapshot) {
		s.SerialSettings = &settingsCopy
		s.SerialConsoleError = ""
	})
	return nil
}

func (c *Controller) SetSerialCommandHistory(history []string) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SetSerialCommandHistory(withTimeout(context.Background(), c.cfg.MutationTimeout), history)
}

func (c *Controller) SendCustomCommand(command string) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SendCustomCommand(withTimeout(context.Background(), c.cfg.MutationTimeout), command)
}

func (c *Controller) SendSerialText(text string) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SendSerialText(text)
}

func (c *Controller) SendSerialRaw(text string) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SendSerialRaw(text)
}

func (c *Controller) SendSerialTerminator() error {
	c.mu.RLock()
	settings := c.snapshot.SerialSettings
	c.mu.RUnlock()

	terminator := "\n"
	if settings != nil && settings.Terminator.Value != "" {
		terminator = settings.Terminator.Value
	}
	return c.SendSerialRaw(terminator)
}

func (c *Controller) SetUSBDevices(devices USBDevices) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetUSBDevices(ctx, client.USBDevices{
			AbsoluteMouse: devices.AbsoluteMouse,
			RelativeMouse: devices.RelativeMouse,
			Keyboard:      devices.Keyboard,
			MassStorage:   devices.MassStorage,
			SerialConsole: devices.SerialConsole,
			Network:       devices.Network,
		})
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetUSBDevices(ctx)
		if err != nil {
			return false, err
		}
		return current == devices, nil
	})
}

func (c *Controller) SetUSBNetworkConfig(cfg USBNetworkConfig) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetUSBNetworkConfig(ctx, client.USBNetworkConfig{
			Enabled:         cfg.Enabled,
			HostPreset:      cfg.HostPreset,
			Protocol:        cfg.Protocol,
			SharingMode:     cfg.SharingMode,
			UplinkMode:      cfg.UplinkMode,
			UplinkInterface: cfg.UplinkInterface,
			IPv4SubnetCIDR:  cfg.IPv4SubnetCIDR,
			DHCPEnabled:     cfg.DHCPEnabled,
			DNSProxyEnabled: cfg.DNSProxyEnabled,
		})
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetUSBNetworkConfig(ctx)
		if err != nil {
			return false, err
		}
		return current == cfg, nil
	})
}

func (c *Controller) SetNetworkSettings(settings NetworkSettings) error {
	if strings.TrimSpace(settings.DHCPClient) == "" &&
		strings.TrimSpace(settings.Hostname) == "" &&
		strings.TrimSpace(settings.Domain) == "" &&
		strings.TrimSpace(settings.HTTPProxy) == "" &&
		strings.TrimSpace(settings.IPv4Mode) == "" &&
		settings.IPv4Static == nil &&
		strings.TrimSpace(settings.IPv6Mode) == "" &&
		settings.IPv6Static == nil &&
		strings.TrimSpace(settings.LLDPMode) == "" &&
		len(settings.LLDPTxTLVs) == 0 &&
		strings.TrimSpace(settings.MDNSMode) == "" &&
		strings.TrimSpace(settings.TimeSyncMode) == "" &&
		len(settings.TimeSyncOrdering) == 0 &&
		!settings.TimeSyncDisableFallback &&
		settings.TimeSyncParallel == 0 &&
		len(settings.TimeSyncNTPServers) == 0 &&
		len(settings.TimeSyncHTTPUrls) == 0 {
		return errors.New("network settings are required")
	}
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetNetworkSettings(ctx, networkSettingsToClient(settings))
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetNetworkSettings(ctx)
		if err != nil {
			return false, err
		}
		return reflect.DeepEqual(current, settings), nil
	})
}

func (c *Controller) GetCloudState(ctx context.Context) (CloudState, error) {
	current := c.clientIfConnected()
	if current == nil {
		return CloudState{}, errors.New("client not connected")
	}
	state, err := current.GetCloudState(ctx)
	if err != nil {
		return CloudState{}, err
	}
	return CloudState{
		Connected: state.Connected,
		URL:       state.URL,
		AppURL:    state.AppURL,
	}, nil
}

func (c *Controller) GetActiveExtension(ctx context.Context) (string, error) {
	current := c.clientIfConnected()
	if current == nil {
		return "", errors.New("client not connected")
	}
	return current.GetActiveExtension(ctx)
}

func (c *Controller) GetATXState(ctx context.Context) (*ATXState, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	state, err := current.GetATXState(ctx)
	if err != nil {
		return nil, err
	}
	return &ATXState{
		Power: state.Power,
		HDD:   state.HDD,
	}, nil
}

func (c *Controller) GetDCPowerState(ctx context.Context) (*DCPowerState, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	state, err := current.GetDCPowerState(ctx)
	if err != nil {
		return nil, err
	}
	return &DCPowerState{
		IsOn:         state.IsOn,
		Voltage:      state.Voltage,
		Current:      state.Current,
		Power:        state.Power,
		RestoreState: state.RestoreState,
	}, nil
}

func (c *Controller) GetSerialSettings(ctx context.Context) (*SerialSettings, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	settings, err := current.GetSerialSettings(ctx)
	if err != nil {
		return nil, err
	}
	buttons := make([]QuickButton, 0, len(settings.Buttons))
	for _, button := range settings.Buttons {
		buttons = append(buttons, QuickButton{
			ID:      button.ID,
			Label:   button.Label,
			Command: button.Command,
			Terminator: Terminator{
				Label: button.Terminator.Label,
				Value: button.Terminator.Value,
			},
			Sort: button.Sort,
		})
	}
	return &SerialSettings{
		BaudRate: settings.BaudRate,
		DataBits: settings.DataBits,
		Parity:   settings.Parity,
		StopBits: settings.StopBits,
		Terminator: Terminator{
			Label: settings.Terminator.Label,
			Value: settings.Terminator.Value,
		},
		HideSerialSettings: settings.HideSerialSettings,
		EnableEcho:         settings.EnableEcho,
		NormalizeMode:      settings.NormalizeMode,
		NormalizeLineEnd:   settings.NormalizeLineEnd,
		TabRender:          settings.TabRender,
		PreserveANSI:       settings.PreserveANSI,
		ShowNLTag:          settings.ShowNLTag,
		Buttons:            buttons,
	}, nil
}

func (c *Controller) GetSerialCommandHistory(ctx context.Context) ([]string, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	return current.GetSerialCommandHistory(ctx)
}

func (c *Controller) GetLocalAccessState(ctx context.Context) (LocalAuthMode, bool, error) {
	current := c.clientIfConnected()
	if current == nil {
		return LocalAuthModeUnknown, false, errors.New("client not connected")
	}
	info, err := current.DeviceInfo(ctx)
	if err != nil {
		return LocalAuthModeUnknown, false, err
	}
	return sessionLocalAuthMode(info.AuthMode), info.LoopbackOnly, nil
}

func (c *Controller) GetTLSState(ctx context.Context) (TLSState, error) {
	current := c.clientIfConnected()
	if current == nil {
		return TLSState{}, errors.New("client not connected")
	}
	state, err := current.GetTLSState(ctx)
	if err != nil {
		return TLSState{}, err
	}
	return TLSState{
		Mode:        TLSMode(state.Mode),
		Certificate: state.Certificate,
		PrivateKey:  state.PrivateKey,
	}, nil
}

func (c *Controller) SetTLSState(state TLSState) error {
	if state.Mode == TLSModeUnknown {
		return errors.New("tls mode is required")
	}
	if state.Mode == TLSModeCustom {
		if strings.TrimSpace(state.Certificate) == "" {
			return errors.New("certificate is required for custom TLS")
		}
		if strings.TrimSpace(state.PrivateKey) == "" {
			return errors.New("private key is required for custom TLS")
		}
	}
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetTLSState(ctx, client.TLSState{
			Mode:        string(state.Mode),
			Certificate: state.Certificate,
			PrivateKey:  state.PrivateKey,
		})
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetTLSState(ctx)
		if err != nil {
			return false, err
		}
		if current.Mode != state.Mode {
			return false, nil
		}
		if state.Mode != TLSModeCustom {
			return true, nil
		}
		return current.Certificate == state.Certificate && current.PrivateKey == state.PrivateKey, nil
	})
}

func (c *Controller) CreateLocalPassword(password string) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.CreateLocalPassword(ctx, password)
}

func (c *Controller) UpdateLocalPassword(oldPassword, newPassword string) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.UpdateLocalPassword(ctx, oldPassword, newPassword)
}

func (c *Controller) DeleteLocalPassword(password string) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.DeleteLocalPassword(ctx, password)
}

func (c *Controller) GetUSBEmulationState(ctx context.Context) (bool, error) {
	current := c.clientIfConnected()
	if current == nil {
		return false, errors.New("client not connected")
	}
	return current.GetUSBEmulationState(ctx)
}

func (c *Controller) GetUSBConfig(ctx context.Context) (USBConfig, error) {
	current := c.clientIfConnected()
	if current == nil {
		return USBConfig{}, errors.New("client not connected")
	}
	cfg, err := current.GetUSBConfig(ctx)
	if err != nil {
		return USBConfig{}, err
	}
	return USBConfig{
		VendorID:     cfg.VendorID,
		ProductID:    cfg.ProductID,
		SerialNumber: cfg.SerialNumber,
		Manufacturer: cfg.Manufacturer,
		Product:      cfg.Product,
	}, nil
}

func (c *Controller) GetUSBDevices(ctx context.Context) (USBDevices, error) {
	current := c.clientIfConnected()
	if current == nil {
		return USBDevices{}, errors.New("client not connected")
	}
	devices, err := current.GetUSBDevices(ctx)
	if err != nil {
		return USBDevices{}, err
	}
	return USBDevices{
		AbsoluteMouse: devices.AbsoluteMouse,
		RelativeMouse: devices.RelativeMouse,
		Keyboard:      devices.Keyboard,
		MassStorage:   devices.MassStorage,
		SerialConsole: devices.SerialConsole,
		Network:       devices.Network,
	}, nil
}

func (c *Controller) GetUSBNetworkConfig(ctx context.Context) (USBNetworkConfig, error) {
	current := c.clientIfConnected()
	if current == nil {
		return USBNetworkConfig{}, errors.New("client not connected")
	}
	cfg, err := current.GetUSBNetworkConfig(ctx)
	if err != nil {
		return USBNetworkConfig{}, err
	}
	return USBNetworkConfig{
		Enabled:         cfg.Enabled,
		HostPreset:      cfg.HostPreset,
		Protocol:        cfg.Protocol,
		SharingMode:     cfg.SharingMode,
		UplinkMode:      cfg.UplinkMode,
		UplinkInterface: cfg.UplinkInterface,
		IPv4SubnetCIDR:  cfg.IPv4SubnetCIDR,
		DHCPEnabled:     cfg.DHCPEnabled,
		DNSProxyEnabled: cfg.DNSProxyEnabled,
	}, nil
}

func (c *Controller) GetDisplayRotation(ctx context.Context) (DisplayRotation, error) {
	current := c.clientIfConnected()
	if current == nil {
		return DisplayRotationUnknown, errors.New("client not connected")
	}
	state, err := current.GetDisplayRotation(ctx)
	if err != nil {
		return DisplayRotationUnknown, err
	}
	return DisplayRotation(state.Rotation), nil
}

func (c *Controller) GetNetworkSettings(ctx context.Context) (NetworkSettings, error) {
	current := c.clientIfConnected()
	if current == nil {
		return NetworkSettings{}, errors.New("client not connected")
	}
	settings, err := current.GetNetworkSettings(ctx)
	if err != nil {
		return NetworkSettings{}, err
	}
	return networkSettingsFromClient(settings), nil
}

func (c *Controller) GetNetworkState(ctx context.Context) (NetworkState, error) {
	current := c.clientIfConnected()
	if current == nil {
		return NetworkState{}, errors.New("client not connected")
	}
	state, err := current.GetNetworkState(ctx)
	if err != nil {
		return NetworkState{}, err
	}
	var lease *DHCPLease
	if state.DHCPLease != nil {
		lease = &DHCPLease{
			IP:              state.DHCPLease.IP,
			Netmask:         state.DHCPLease.Netmask,
			DNSServers:      append([]string(nil), state.DHCPLease.DNSServers...),
			Broadcast:       state.DHCPLease.Broadcast,
			Domain:          state.DHCPLease.Domain,
			NTPServers:      append([]string(nil), state.DHCPLease.NTPServers...),
			Hostname:        state.DHCPLease.Hostname,
			Routers:         append([]string(nil), state.DHCPLease.Routers...),
			ServerID:        state.DHCPLease.ServerID,
			LeaseExpiry:     state.DHCPLease.LeaseExpiry,
			MTU:             state.DHCPLease.MTU,
			TTL:             state.DHCPLease.TTL,
			BootpNextServer: state.DHCPLease.BootpNextServer,
			BootpServerName: state.DHCPLease.BootpServerName,
			BootpFile:       state.DHCPLease.BootpFile,
			DHCPClient:      state.DHCPLease.DHCPClient,
		}
	}
	addresses := make([]IPv6Address, 0, len(state.IPv6Addresses))
	for _, address := range state.IPv6Addresses {
		addresses = append(addresses, IPv6Address{Address: address.Address, Prefix: address.Prefix})
	}
	return NetworkState{
		InterfaceName: state.InterfaceName,
		MACAddress:    state.MACAddress,
		IPv4:          state.IPv4,
		IPv4Addresses: append([]string(nil), state.IPv4Addresses...),
		IPv6:          state.IPv6,
		IPv6Addresses: addresses,
		IPv6LinkLocal: state.IPv6LinkLocal,
		IPv6Gateway:   state.IPv6Gateway,
		DHCPLease:     lease,
		Hostname:      state.Hostname,
	}, nil
}

func (c *Controller) RenewDHCPLease() error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.RenewDHCPLease(ctx)
}

func (c *Controller) GetDeveloperModeState(ctx context.Context) (*bool, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	state, err := current.GetDeveloperModeState(ctx)
	if err != nil {
		return nil, err
	}
	return &state.Enabled, nil
}

func (c *Controller) GetDevChannelState(ctx context.Context) (*bool, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	enabled, err := current.GetDevChannelState(ctx)
	if err != nil {
		return nil, err
	}
	return &enabled, nil
}

func (c *Controller) GetLocalLoopbackOnly(ctx context.Context) (*bool, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	enabled, err := current.GetLocalLoopbackOnly(ctx)
	if err != nil {
		return nil, err
	}
	return &enabled, nil
}

func (c *Controller) GetLocalVersion(ctx context.Context) (VersionInfo, error) {
	current := c.clientIfConnected()
	if current == nil {
		return VersionInfo{}, errors.New("client not connected")
	}
	version, err := current.GetLocalVersion(ctx)
	if err != nil {
		return VersionInfo{}, err
	}
	return VersionInfo{
		AppVersion:    version.AppVersion,
		SystemVersion: version.SystemVersion,
	}, nil
}

func (c *Controller) GetUpdateStatus(ctx context.Context) (UpdateStatus, error) {
	current := c.clientIfConnected()
	if current == nil {
		return UpdateStatus{}, errors.New("client not connected")
	}
	status, err := current.GetUpdateStatus(ctx)
	if err != nil {
		return UpdateStatus{}, err
	}
	return UpdateStatus{
		Local: VersionInfo{
			AppVersion:    status.Local.AppVersion,
			SystemVersion: status.Local.SystemVersion,
		},
		Remote: VersionInfo{
			AppVersion:    status.Remote.AppVersion,
			SystemVersion: status.Remote.SystemVersion,
		},
		AppUpdateAvailable:    status.AppUpdateAvailable,
		SystemUpdateAvailable: status.SystemUpdateAvailable,
	}, nil
}

func (c *Controller) GetPublicIPAddresses(ctx context.Context, refresh bool) ([]PublicIP, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	addresses, err := current.GetPublicIPAddresses(ctx, refresh)
	if err != nil {
		return nil, err
	}
	out := make([]PublicIP, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, PublicIP{
			IPAddress:   address.IPAddress,
			LastUpdated: address.LastUpdated,
		})
	}
	return out, nil
}

func (c *Controller) GetTailscaleStatus(ctx context.Context) (*TailscaleStatus, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	status, err := current.GetTailscaleStatus(ctx)
	if err != nil {
		return nil, err
	}
	var self *TailscalePeer
	if status.Self != nil {
		self = &TailscalePeer{
			HostName:     status.Self.HostName,
			DNSName:      status.Self.DNSName,
			TailscaleIPs: append([]string(nil), status.Self.TailscaleIPs...),
			Online:       status.Self.Online,
			OS:           status.Self.OS,
		}
	}
	return &TailscaleStatus{
		Installed:    status.Installed,
		Running:      status.Running,
		BackendState: status.BackendState,
		AuthURL:      status.AuthURL,
		ControlURL:   status.ControlURL,
		Self:         self,
		Health:       append([]string(nil), status.Health...),
	}, nil
}

func (c *Controller) GetVideoCodec(ctx context.Context) (VideoCodec, error) {
	current := c.clientIfConnected()
	if current == nil {
		return VideoCodecUnknown, errors.New("client not connected")
	}
	codec, err := current.GetVideoCodecPreference(ctx)
	if err != nil {
		return VideoCodecUnknown, err
	}
	return VideoCodec(codec), nil
}

func (c *Controller) GetEDID(ctx context.Context) (string, error) {
	current := c.clientIfConnected()
	if current == nil {
		return "", errors.New("client not connected")
	}
	return current.GetEDID(ctx)
}

func networkSettingsToClient(settings NetworkSettings) client.NetworkSettings {
	var ipv4Static *client.IPv4StaticConfig
	if settings.IPv4Static != nil {
		ipv4Static = &client.IPv4StaticConfig{
			Address: settings.IPv4Static.Address,
			Netmask: settings.IPv4Static.Netmask,
			Gateway: settings.IPv4Static.Gateway,
			DNS:     append([]string(nil), settings.IPv4Static.DNS...),
		}
	}
	var ipv6Static *client.IPv6StaticConfig
	if settings.IPv6Static != nil {
		ipv6Static = &client.IPv6StaticConfig{
			Prefix:  settings.IPv6Static.Prefix,
			Gateway: settings.IPv6Static.Gateway,
			DNS:     append([]string(nil), settings.IPv6Static.DNS...),
		}
	}
	return client.NetworkSettings{
		DHCPClient:              settings.DHCPClient,
		Hostname:                settings.Hostname,
		Domain:                  settings.Domain,
		HTTPProxy:               settings.HTTPProxy,
		IPv4Mode:                settings.IPv4Mode,
		IPv4Static:              ipv4Static,
		IPv6Mode:                settings.IPv6Mode,
		IPv6Static:              ipv6Static,
		LLDPMode:                settings.LLDPMode,
		LLDPTxTLVs:              append([]string(nil), settings.LLDPTxTLVs...),
		MDNSMode:                settings.MDNSMode,
		TimeSyncMode:            settings.TimeSyncMode,
		TimeSyncOrdering:        append([]string(nil), settings.TimeSyncOrdering...),
		TimeSyncDisableFallback: settings.TimeSyncDisableFallback,
		TimeSyncParallel:        settings.TimeSyncParallel,
		TimeSyncNTPServers:      append([]string(nil), settings.TimeSyncNTPServers...),
		TimeSyncHTTPUrls:        append([]string(nil), settings.TimeSyncHTTPUrls...),
	}
}

func networkSettingsFromClient(settings client.NetworkSettings) NetworkSettings {
	var ipv4Static *IPv4StaticConfig
	if settings.IPv4Static != nil {
		ipv4Static = &IPv4StaticConfig{
			Address: settings.IPv4Static.Address,
			Netmask: settings.IPv4Static.Netmask,
			Gateway: settings.IPv4Static.Gateway,
			DNS:     append([]string(nil), settings.IPv4Static.DNS...),
		}
	}
	var ipv6Static *IPv6StaticConfig
	if settings.IPv6Static != nil {
		ipv6Static = &IPv6StaticConfig{
			Prefix:  settings.IPv6Static.Prefix,
			Gateway: settings.IPv6Static.Gateway,
			DNS:     append([]string(nil), settings.IPv6Static.DNS...),
		}
	}
	return NetworkSettings{
		DHCPClient:              settings.DHCPClient,
		Hostname:                settings.Hostname,
		Domain:                  settings.Domain,
		HTTPProxy:               settings.HTTPProxy,
		IPv4Mode:                settings.IPv4Mode,
		IPv4Static:              ipv4Static,
		IPv6Mode:                settings.IPv6Mode,
		IPv6Static:              ipv6Static,
		LLDPMode:                settings.LLDPMode,
		LLDPTxTLVs:              append([]string(nil), settings.LLDPTxTLVs...),
		MDNSMode:                settings.MDNSMode,
		TimeSyncMode:            settings.TimeSyncMode,
		TimeSyncOrdering:        append([]string(nil), settings.TimeSyncOrdering...),
		TimeSyncDisableFallback: settings.TimeSyncDisableFallback,
		TimeSyncParallel:        settings.TimeSyncParallel,
		TimeSyncNTPServers:      append([]string(nil), settings.TimeSyncNTPServers...),
		TimeSyncHTTPUrls:        append([]string(nil), settings.TimeSyncHTTPUrls...),
	}
}

func (c *Controller) GetBacklightSettings(ctx context.Context) (BacklightSettings, error) {
	current := c.clientIfConnected()
	if current == nil {
		return BacklightSettings{}, errors.New("client not connected")
	}
	settings, err := current.GetBacklightSettings(ctx)
	if err != nil {
		return BacklightSettings{}, err
	}
	return BacklightSettings{
		MaxBrightness: settings.MaxBrightness,
		DimAfter:      settings.DimAfter,
		OffAfter:      settings.OffAfter,
	}, nil
}

func (c *Controller) GetVideoSleepMode(ctx context.Context) (*VideoSleepMode, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	state, err := current.GetVideoSleepMode(ctx)
	if err != nil {
		return nil, err
	}
	return &VideoSleepMode{
		Enabled:  state.Enabled,
		Duration: state.Duration,
	}, nil
}

func (c *Controller) GetSSHKeyState(ctx context.Context) (string, error) {
	current := c.clientIfConnected()
	if current == nil {
		return "", errors.New("client not connected")
	}
	return current.GetSSHKeyState(ctx)
}

func (c *Controller) GetKeyboardMacros(ctx context.Context) ([]KeyboardMacro, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	macros, err := current.GetKeyboardMacros(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]KeyboardMacro, 0, len(macros))
	for _, macro := range macros {
		steps := make([]KeyboardMacroStep, 0, len(macro.Steps))
		for _, step := range macro.Steps {
			steps = append(steps, KeyboardMacroStep{
				Keys:      append([]string(nil), step.Keys...),
				Modifiers: append([]string(nil), step.Modifiers...),
				Delay:     step.Delay,
			})
		}
		out = append(out, KeyboardMacro{
			ID:        macro.ID,
			Name:      macro.Name,
			Steps:     steps,
			SortOrder: macro.SortOrder,
		})
	}
	return out, nil
}

func (c *Controller) GetMQTTSettings(ctx context.Context) (MQTTSettings, error) {
	current := c.clientIfConnected()
	if current == nil {
		return MQTTSettings{}, errors.New("client not connected")
	}
	settings, err := current.GetMQTTSettings(ctx)
	if err != nil {
		return MQTTSettings{}, err
	}
	return MQTTSettings{
		Enabled:           settings.Enabled,
		Broker:            settings.Broker,
		Port:              settings.Port,
		Username:          settings.Username,
		Password:          settings.Password,
		BaseTopic:         settings.BaseTopic,
		UseTLS:            settings.UseTLS,
		TLSInsecure:       settings.TLSInsecure,
		EnableHADiscovery: settings.EnableHADiscovery,
		EnableActions:     settings.EnableActions,
		DebounceMs:        settings.DebounceMs,
	}, nil
}

func (c *Controller) SetMQTTSettings(settings MQTTSettings) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.SetMQTTSettings(ctx, client.MQTTSettings{
		Enabled:           settings.Enabled,
		Broker:            settings.Broker,
		Port:              settings.Port,
		Username:          settings.Username,
		Password:          settings.Password,
		BaseTopic:         settings.BaseTopic,
		UseTLS:            settings.UseTLS,
		TLSInsecure:       settings.TLSInsecure,
		EnableHADiscovery: settings.EnableHADiscovery,
		EnableActions:     settings.EnableActions,
		DebounceMs:        settings.DebounceMs,
	})
}

func (c *Controller) GetMQTTStatus(ctx context.Context) (MQTTStatus, error) {
	current := c.clientIfConnected()
	if current == nil {
		return MQTTStatus{}, errors.New("client not connected")
	}
	status, err := current.GetMQTTStatus(ctx)
	if err != nil {
		return MQTTStatus{}, err
	}
	return MQTTStatus{
		Connected: status.Connected,
		Error:     status.Error,
	}, nil
}

func (c *Controller) TestMQTTConnection(settings MQTTSettings) (MQTTTestResult, error) {
	current := c.clientIfConnected()
	if current == nil {
		return MQTTTestResult{}, errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	result, err := current.TestMQTTConnection(ctx, client.MQTTSettings{
		Enabled:           settings.Enabled,
		Broker:            settings.Broker,
		Port:              settings.Port,
		Username:          settings.Username,
		Password:          settings.Password,
		BaseTopic:         settings.BaseTopic,
		UseTLS:            settings.UseTLS,
		TLSInsecure:       settings.TLSInsecure,
		EnableHADiscovery: settings.EnableHADiscovery,
		EnableActions:     settings.EnableActions,
		DebounceMs:        settings.DebounceMs,
	})
	if err != nil {
		return MQTTTestResult{}, err
	}
	return MQTTTestResult{
		Success: result.Success,
		Error:   result.Error,
	}, nil
}

func (c *Controller) GetAutoUpdateState(ctx context.Context) (bool, error) {
	current := c.clientIfConnected()
	if current == nil {
		return false, errors.New("client not connected")
	}
	return current.GetAutoUpdateState(ctx)
}

func (c *Controller) SetAutoUpdateState(enabled bool) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetAutoUpdateState(ctx, enabled)
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetAutoUpdateState(ctx)
		if err != nil {
			return false, err
		}
		return current == enabled, nil
	})
}

func (c *Controller) SetDeveloperModeState(enabled bool) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetDeveloperModeState(ctx, enabled)
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetDeveloperModeState(ctx)
		if err != nil {
			return false, err
		}
		return current != nil && *current == enabled, nil
	})
}

func (c *Controller) SetDevChannelState(enabled bool) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetDevChannelState(ctx, enabled)
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetDevChannelState(ctx)
		if err != nil {
			return false, err
		}
		return current != nil && *current == enabled, nil
	})
}

func (c *Controller) SetLocalLoopbackOnly(enabled bool) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetLocalLoopbackOnly(ctx, enabled)
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetLocalLoopbackOnly(ctx)
		if err != nil {
			return false, err
		}
		return current != nil && *current == enabled, nil
	})
}

func (c *Controller) SetVideoCodec(codec VideoCodec) error {
	if codec == "" {
		return errors.New("video codec is required")
	}
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetVideoCodecPreference(ctx, string(codec))
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetVideoCodec(ctx)
		if err != nil {
			return false, err
		}
		return current == codec, nil
	})
}

func (c *Controller) SetEDID(edid string) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetEDID(ctx, edid)
	}, func(ctx context.Context) (bool, error) {
		current := c.clientIfConnected()
		if current == nil {
			return false, errors.New("client not connected")
		}
		currentEDID, err := current.GetEDID(ctx)
		if err != nil {
			return false, err
		}
		return currentEDID == edid, nil
	})
}

func (c *Controller) SetBacklightSettings(settings BacklightSettings) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetBacklightSettings(ctx, client.BacklightSettings{
			MaxBrightness: settings.MaxBrightness,
			DimAfter:      settings.DimAfter,
			OffAfter:      settings.OffAfter,
		})
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetBacklightSettings(ctx)
		if err != nil {
			return false, err
		}
		return current == settings, nil
	})
}

func (c *Controller) SetVideoSleepMode(duration int) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetVideoSleepMode(ctx, duration)
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetVideoSleepMode(ctx)
		if err != nil {
			return false, err
		}
		return current != nil && current.Duration == duration, nil
	})
}

func (c *Controller) SetSSHKeyState(sshKey string) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.SetSSHKeyState(ctx, sshKey)
}

func (c *Controller) SetKeyboardMacros(macros []KeyboardMacro) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	items := make([]client.KeyboardMacro, 0, len(macros))
	for _, macro := range macros {
		steps := make([]client.KeyboardMacroStep, 0, len(macro.Steps))
		for _, step := range macro.Steps {
			steps = append(steps, client.KeyboardMacroStep{
				Keys:      append([]string(nil), step.Keys...),
				Modifiers: append([]string(nil), step.Modifiers...),
				Delay:     step.Delay,
			})
		}
		items = append(items, client.KeyboardMacro{
			ID:        macro.ID,
			Name:      macro.Name,
			Steps:     steps,
			SortOrder: macro.SortOrder,
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.SetKeyboardMacros(ctx, items)
}

func (c *Controller) GetJigglerState(ctx context.Context) (bool, error) {
	current := c.clientIfConnected()
	if current == nil {
		return false, errors.New("client not connected")
	}
	return current.GetJigglerState(ctx)
}

func (c *Controller) SetJigglerState(enabled bool) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetJigglerState(ctx, enabled)
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetJigglerState(ctx)
		if err != nil {
			return false, err
		}
		return current == enabled, nil
	})
}

func (c *Controller) GetJigglerConfig(ctx context.Context) (JigglerConfig, error) {
	current := c.clientIfConnected()
	if current == nil {
		return JigglerConfig{}, errors.New("client not connected")
	}
	cfg, err := current.GetJigglerConfig(ctx)
	if err != nil {
		return JigglerConfig{}, err
	}
	return JigglerConfig{
		InactivityLimitSeconds: cfg.InactivityLimitSeconds,
		JitterPercentage:       cfg.JitterPercentage,
		ScheduleCronTab:        cfg.ScheduleCronTab,
		Timezone:               cfg.Timezone,
	}, nil
}

func (c *Controller) SetJigglerConfig(cfg JigglerConfig) error {
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetJigglerConfig(ctx, client.JigglerConfig{
			InactivityLimitSeconds: cfg.InactivityLimitSeconds,
			JitterPercentage:       cfg.JitterPercentage,
			ScheduleCronTab:        cfg.ScheduleCronTab,
			Timezone:               cfg.Timezone,
		})
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetJigglerConfig(ctx)
		if err != nil {
			return false, err
		}
		return current == cfg, nil
	})
}

func (c *Controller) GetVirtualMediaState(ctx context.Context) (*virtualmedia.State, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	return current.GetVirtualMediaState(ctx)
}

func (c *Controller) UnmountMedia() error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.UnmountMedia(ctx)
}

func (c *Controller) MountMediaURL(url string, mode virtualmedia.Mode) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("url is required")
	}
	if mode == "" {
		return errors.New("mode is required")
	}
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.MountMediaURL(ctx, strings.TrimSpace(url), mode)
}

func (c *Controller) GetStorageSpace(ctx context.Context) (virtualmedia.StorageSpace, error) {
	current := c.clientIfConnected()
	if current == nil {
		return virtualmedia.StorageSpace{}, errors.New("client not connected")
	}
	return current.GetStorageSpace(ctx)
}

func (c *Controller) ListStorageFiles(ctx context.Context) ([]virtualmedia.StorageFile, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	return current.ListStorageFiles(ctx)
}

func (c *Controller) DeleteStorageFile(filename string) error {
	if strings.TrimSpace(filename) == "" {
		return errors.New("filename is required")
	}
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.DeleteStorageFile(ctx, filename)
}

func (c *Controller) MountStorageFile(filename string, mode virtualmedia.Mode) error {
	if strings.TrimSpace(filename) == "" {
		return errors.New("filename is required")
	}
	if mode == "" {
		return errors.New("mode is required")
	}
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()
	return current.MountStorageFile(ctx, filename, mode)
}

func (c *Controller) UploadStorageFile(path string, progress func(virtualmedia.UploadProgress)) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("path must be a file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	uploadCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	start, err := current.StartStorageFileUpload(uploadCtx, filepath.Base(path), info.Size())
	if err != nil {
		return err
	}
	if start.AlreadyUploadedBytes > 0 {
		if _, err := file.Seek(start.AlreadyUploadedBytes, 0); err != nil {
			return err
		}
	}
	return current.UploadStorageFile(uploadCtx, start.DataChannel, file, start.AlreadyUploadedBytes, info.Size(), progress)
}

func (c *Controller) SendKeypress(key byte, press bool) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SendKeypress(key, press)
}

func (c *Controller) SendKeypressKeepAlive() error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SendKeypressKeepAlive()
}

func (c *Controller) SendAbsPointer(x, y int32, buttons byte) error {
	log := logging.Subsystem("session")
	current := c.clientIfConnected()
	if current == nil {
		err := errors.New("client not connected")
		log.Debug().Err(err).Int32("x", x).Int32("y", y).Uint8("buttons", buttons).Msg("failed to send absolute pointer")
		return err
	}
	if err := current.SendAbsPointer(x, y, buttons); err != nil {
		log.Debug().Err(err).Int32("x", x).Int32("y", y).Uint8("buttons", buttons).Msg("failed to forward absolute pointer")
		return err
	}
	return nil
}

func (c *Controller) SendRelMouse(dx, dy int8, buttons byte) error {
	log := logging.Subsystem("session")
	current := c.clientIfConnected()
	if current == nil {
		err := errors.New("client not connected")
		log.Debug().Err(err).Int8("dx", dx).Int8("dy", dy).Uint8("buttons", buttons).Msg("failed to send relative mouse")
		return err
	}
	if err := current.SendRelMouse(dx, dy, buttons); err != nil {
		log.Debug().Err(err).Int8("dx", dx).Int8("dy", dy).Uint8("buttons", buttons).Msg("failed to forward relative mouse")
		return err
	}
	return nil
}

func (c *Controller) SendWheel(wheelY, wheelX int8) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SendWheel(wheelY, wheelX)
}

func (c *Controller) ExecutePaste(text string, delay uint16) ([]rune, error) {
	current := c.clientIfConnected()
	if current == nil {
		return nil, errors.New("client not connected")
	}
	layout := c.Snapshot().KeyboardLayout
	steps, invalid := input.BuildPasteMacro(layout, text, delay)
	if len(steps) == 0 && len(invalid) > 0 {
		return invalid, errors.New("no pasteable characters in input")
	}
	if len(steps) == 0 {
		return invalid, nil
	}
	return invalid, current.ExecuteKeyboardMacro(true, steps)
}

func (c *Controller) CancelPaste() error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.CancelKeyboardMacro()
}

func (c *Controller) ExecuteRemoteHotkey(action hotkeys.Action) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	steps, err := hotkeys.MacroSteps(action)
	if err != nil {
		return err
	}
	return current.ExecuteKeyboardMacro(false, steps)
}

func (c *Controller) Stats() client.StatsSnapshot {
	c.mu.RLock()
	current := c.current
	snap := c.snapshot
	c.mu.RUnlock()
	if current == nil {
		return client.StatsSnapshot{
			SignalingMode: snap.SignalingMode,
			RTCState:      snap.RTCState,
			HIDReady:      snap.HIDReady,
			VideoReady:    snap.VideoReady,
			LastError:     snap.LastError,
		}
	}
	stats := current.Stats()
	stats.HIDReady = snap.HIDReady
	stats.VideoReady = snap.VideoReady
	if stats.SignalingMode == client.SignalingModeUnknown {
		stats.SignalingMode = snap.SignalingMode
	}
	if stats.RTCState == webrtc.PeerConnectionStateUnknown {
		stats.RTCState = snap.RTCState
	}
	if stats.LastError == "" {
		stats.LastError = snap.LastError
	}
	return stats
}

func (c *Controller) run(ctx context.Context) {
	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()
	var attempt int
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.setState(func(s *Snapshot) {
			if attempt == 0 {
				s.Phase = PhaseConnecting
				s.Status = "connecting"
			} else {
				s.Phase = PhaseReconnecting
				s.Status = fmt.Sprintf("reconnecting (attempt %d)", attempt)
			}
			s.HIDReady = false
			s.VideoReady = false
			s.SerialConsoleReady = false
			s.SerialConsoleError = ""
			s.SerialConsoleBuffer = ""
			s.SerialConsoleTruncated = false
			s.SerialSettings = nil
		})
		c.serialLog.Reset()

		cl := c.newClient()
		c.setClient(cl)

		err := cl.Connect(ctx)
		if err != nil {
			c.setConnectError(err)
			if !c.cfg.Reconnect || ctx.Err() != nil || isAuthError(err) {
				return
			}
			if !sleepWithContext(ctx, backoff(attempt, c.cfg.ReconnectBase, c.cfg.ReconnectMax)) {
				return
			}
			attempt++
			continue
		}

		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = cl.WaitForHID(waitCtx)
		cancel()
		if err != nil {
			c.setConnectError(err)
			if !c.cfg.Reconnect || ctx.Err() != nil {
				return
			}
			if !sleepWithContext(ctx, backoff(attempt, c.cfg.ReconnectBase, c.cfg.ReconnectMax)) {
				return
			}
			attempt++
			continue
		}

		_ = c.bootstrap(ctx, cl)
		reason, stop := c.watch(ctx, cl)
		if stop {
			return
		}

		switch reason {
		case "other_session":
			c.setState(func(s *Snapshot) {
				s.Phase = PhaseOtherSession
				s.Status = "another session took over"
				s.HIDReady = false
			})
			return
		case "rebooting":
			c.setState(func(s *Snapshot) {
				s.Phase = PhaseRebooting
				s.Status = "device rebooting"
			})
		default:
			c.setState(func(s *Snapshot) {
				s.Phase = PhaseReconnecting
				s.Status = "connection lost, retrying"
				s.HIDReady = false
			})
		}

		_ = cl.Close()
		if !c.cfg.Reconnect || !sleepWithContext(ctx, backoff(attempt, c.cfg.ReconnectBase, c.cfg.ReconnectMax)) {
			return
		}
		attempt++
	}
}

func (c *Controller) bootstrap(ctx context.Context, cl *client.Client) error {
	if deviceID, err := cl.GetDeviceID(ctx); err == nil {
		c.setState(func(s *Snapshot) { s.DeviceID = deviceID })
	}
	if activeExtension, err := cl.GetActiveExtension(ctx); err == nil {
		c.setState(func(s *Snapshot) { s.ActiveExtension = activeExtension })
		if activeExtension == "atx-power" {
			if atxState, stateErr := cl.GetATXState(ctx); stateErr == nil {
				c.setState(func(s *Snapshot) {
					s.ATXState = &ATXState{
						Power: atxState.Power,
						HDD:   atxState.HDD,
					}
				})
			}
		}
		if activeExtension == "serial-console" {
			settings, stateErr := cl.GetSerialSettings(ctx)
			if stateErr != nil {
				defaults := defaultSerialSettings()
				c.setState(func(s *Snapshot) {
					s.SerialSettings = &defaults
					s.SerialConsoleError = stateErr.Error()
				})
			} else {
				buttons := make([]QuickButton, 0, len(settings.Buttons))
				for _, button := range settings.Buttons {
					buttons = append(buttons, QuickButton{
						ID:      button.ID,
						Label:   button.Label,
						Command: button.Command,
						Terminator: Terminator{
							Label: button.Terminator.Label,
							Value: button.Terminator.Value,
						},
						Sort: button.Sort,
					})
				}
				c.setState(func(s *Snapshot) {
					s.SerialSettings = &SerialSettings{
						BaudRate: settings.BaudRate,
						DataBits: settings.DataBits,
						Parity:   settings.Parity,
						StopBits: settings.StopBits,
						Terminator: Terminator{
							Label: settings.Terminator.Label,
							Value: settings.Terminator.Value,
						},
						HideSerialSettings: settings.HideSerialSettings,
						EnableEcho:         settings.EnableEcho,
						NormalizeMode:      settings.NormalizeMode,
						NormalizeLineEnd:   settings.NormalizeLineEnd,
						TabRender:          settings.TabRender,
						PreserveANSI:       settings.PreserveANSI,
						ShowNLTag:          settings.ShowNLTag,
						Buttons:            buttons,
					}
				})
			}
		}
	}
	if quality, err := cl.GetStreamQualityFactor(ctx); err == nil {
		c.setState(func(s *Snapshot) { s.Quality = quality })
	}
	if keyboardLayout, err := cl.GetKeyboardLayout(ctx); err == nil {
		c.setState(func(s *Snapshot) { s.KeyboardLayout = normalizeKeyboardLayoutCode(keyboardLayout) })
	}
	if edid, err := cl.GetEDID(ctx); err == nil {
		c.setState(func(s *Snapshot) { s.EDID = edid })
	}
	if version, err := cl.GetLocalVersion(ctx); err == nil {
		c.setState(func(s *Snapshot) {
			s.AppVersion = version.AppVersion
			s.SystemVersion = version.SystemVersion
		})
	}
	if updateStatus, err := cl.GetUpdateStatus(ctx); err == nil {
		c.setState(func(s *Snapshot) {
			s.AppUpdateAvailable = updateStatus.AppUpdateAvailable
			s.SystemUpdateAvailable = updateStatus.SystemUpdateAvailable
		})
	}
	if network, err := cl.GetNetworkSettings(ctx); err == nil {
		c.setState(func(s *Snapshot) { s.Hostname = network.Hostname })
	}
	c.setState(func(s *Snapshot) {
		s.Phase = PhaseConnected
		s.Status = "connected"
		s.HIDReady = true
	})
	return nil
}

func (c *Controller) watch(ctx context.Context, cl *client.Client) (reason string, stop bool) {
	for {
		select {
		case <-ctx.Done():
			return "", true
		case evt, ok := <-cl.Events():
			if !ok {
				return "disconnected", false
			}
			switch evt.Method {
			case "otherSessionConnected":
				return "other_session", false
			case "willReboot":
				return "rebooting", false
			case "networkState":
				host := extractString(evt.Params, "hostname")
				c.setState(func(s *Snapshot) { s.Hostname = host })
			case "videoInputState":
				state := ""
				switch v := evt.Params.(type) {
				case string:
					state = v
				case map[string]any:
					state = extractString(v, "state")
				}
				if state != "" {
					c.setState(func(s *Snapshot) { s.Status = "video: " + state })
				}
			case "atxState":
				c.setState(func(s *Snapshot) {
					s.ATXState = &ATXState{
						Power: extractBool(evt.Params, "power"),
						HDD:   extractBool(evt.Params, "hdd"),
					}
				})
			}
		case serial, ok := <-cl.SerialEvents():
			if !ok {
				return "disconnected", false
			}
			c.mu.RLock()
			activeExtension := c.snapshot.ActiveExtension
			c.mu.RUnlock()
			if activeExtension != "serial-console" {
				continue
			}
			buffer, truncated := c.serialLog.Append(serial.Text)
			c.setState(func(s *Snapshot) {
				s.SerialConsoleBuffer = buffer
				s.SerialConsoleTruncated = truncated
			})
		case life, ok := <-cl.Lifecycle():
			if !ok {
				return "disconnected", false
			}
			switch life.Type {
			case "signaling_mode":
				c.setState(func(s *Snapshot) { s.SignalingMode = life.Signaling })
			case "hid_ready":
				c.setState(func(s *Snapshot) { s.HIDReady = true })
			case "video_ready":
				c.setState(func(s *Snapshot) { s.VideoReady = true })
			case "serial_ready":
				c.setState(func(s *Snapshot) {
					s.SerialConsoleReady = true
					s.SerialConsoleError = ""
				})
			case "paste_state":
				c.setState(func(s *Snapshot) { s.PasteInProgress = life.PasteState })
			case "peer_state":
				c.setState(func(s *Snapshot) { s.RTCState = life.Connection })
				if life.Connection == webrtc.PeerConnectionStateDisconnected ||
					life.Connection == webrtc.PeerConnectionStateFailed ||
					life.Connection == webrtc.PeerConnectionStateClosed {
					return "disconnected", false
				}
				if life.Connection == webrtc.PeerConnectionStateConnected {
					c.setState(func(s *Snapshot) {
						s.Phase = PhaseConnected
						s.Status = "connected"
					})
				}
			case "connect_error", "video_error":
				c.setState(func(s *Snapshot) { s.LastError = life.Err })
			case "serial_error":
				c.setState(func(s *Snapshot) { s.SerialConsoleError = life.Err })
			}
		}
	}
}

func (c *Controller) newClient() *client.Client {
	cl, _ := client.New(client.Config{
		BaseURL:    c.cfg.BaseURL,
		Password:   c.cfg.Password,
		RPCTimeout: c.cfg.RPCTimeout,
	})
	return cl
}

func (c *Controller) setConnectError(err error) {
	c.setState(func(s *Snapshot) {
		s.LastError = err.Error()
		s.HIDReady = false
		s.VideoReady = false
		s.SerialConsoleReady = false
		if isAuthError(err) {
			s.Phase = PhaseAuthFailed
			s.Status = "authentication failed"
		} else {
			s.Phase = PhaseDisconnected
			s.Status = "connection failed"
		}
	})
}

func (c *Controller) setState(update func(*Snapshot)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	update(&c.snapshot)
}

func (c *Controller) setClient(cl *client.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != nil {
		_ = c.current.Close()
	}
	c.current = cl
}

func (c *Controller) clientIfConnected() *client.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.current == nil || c.snapshot.Phase != PhaseConnected {
		return nil
	}
	return c.current
}

func (c *Controller) forceDisconnect(ctx context.Context) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.ForceDisconnect(ctx)
}

func (c *Controller) mutateAndConfirm(mutate func(context.Context) error, confirm func(context.Context) (bool, error)) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MutationTimeout)
	defer cancel()

	resultCh := make(chan error, 1)
	go func() {
		resultCh <- mutate(ctx)
	}()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-resultCh:
			if err == nil {
				return nil
			}
			confirmCtx, confirmCancel := context.WithTimeout(context.Background(), c.cfg.RPCTimeout)
			confirmed, confirmErr := confirm(confirmCtx)
			confirmCancel()
			if confirmErr == nil && confirmed {
				return nil
			}
			return err
		case <-ticker.C:
			confirmCtx, confirmCancel := context.WithTimeout(context.Background(), c.cfg.RPCTimeout)
			confirmed, err := confirm(confirmCtx)
			confirmCancel()
			if err == nil && confirmed {
				return nil
			}
		case <-ctx.Done():
			confirmCtx, confirmCancel := context.WithTimeout(context.Background(), c.cfg.RPCTimeout)
			confirmed, err := confirm(confirmCtx)
			confirmCancel()
			if err == nil && confirmed {
				return nil
			}
			return ctx.Err()
		}
	}
}

func backoff(attempt int, base, max time.Duration) time.Duration {
	d := base << attempt
	if d > max {
		return max
	}
	return d
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func withTimeout(ctx context.Context, d time.Duration) context.Context {
	timeoutCtx, cancel := context.WithTimeout(ctx, d)
	go func() {
		<-timeoutCtx.Done()
		cancel()
	}()
	return timeoutCtx
}

func sessionLocalAuthMode(mode auth.LocalAuthMode) LocalAuthMode {
	switch mode {
	case auth.LocalAuthModeNoPassword:
		return LocalAuthModeNoPassword
	case auth.LocalAuthModePassword:
		return LocalAuthModePassword
	default:
		return LocalAuthModeUnknown
	}
}

func isAuthError(err error) bool {
	var authErr *auth.Error
	if errors.As(err, &authErr) {
		switch authErr.StatusCode {
		case 401, 403, 429:
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "authentication failed")
}

func normalizeKeyboardLayoutCode(layout string) string {
	trimmed := strings.TrimSpace(layout)
	if trimmed == "" {
		return ""
	}
	return input.NormalizeKeyboardLayoutCode(trimmed)
}

func extractString(v any, key string) string {
	if m, ok := v.(map[string]any); ok {
		if s, ok := m[key].(string); ok {
			return s
		}
	}
	return ""
}

func extractBool(v any, key string) bool {
	if m, ok := v.(map[string]any); ok {
		if b, ok := m[key].(bool); ok {
			return b
		}
	}
	return false
}
