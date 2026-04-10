package session

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
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
	Phase                 Phase
	Status                string
	BaseURL               string
	DeviceID              string
	Hostname              string
	Quality               float64
	KeyboardLayout        string
	EDID                  string
	AppVersion            string
	SystemVersion         string
	AppUpdateAvailable    bool
	SystemUpdateAvailable bool
	HIDReady              bool
	VideoReady            bool
	LastError             string
	RTCState              webrtc.PeerConnectionState
	SignalingMode         client.SignalingMode
	PasteInProgress       bool
}

type Controller struct {
	cfg Config

	mu        sync.RWMutex
	snapshot  Snapshot
	current   *client.Client
	runParent context.Context
	cancelRun context.CancelFunc
	running   bool
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
	return current.Reboot(withTimeout(context.Background(), c.cfg.MutationTimeout))
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

func (c *Controller) SetNetworkSettings(settings NetworkSettings) error {
	if settings.Hostname == "" && settings.IP == "" {
		return errors.New("network settings are required")
	}
	return c.mutateAndConfirm(func(ctx context.Context) error {
		current := c.clientIfConnected()
		if current == nil {
			return errors.New("client not connected")
		}
		return current.SetNetworkSettings(ctx, client.NetworkSettings{
			Hostname: settings.Hostname,
			IP:       settings.IP,
		})
	}, func(ctx context.Context) (bool, error) {
		current, err := c.GetNetworkSettings(ctx)
		if err != nil {
			return false, err
		}
		return current == settings, nil
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
	return current.CreateLocalPassword(withTimeout(context.Background(), c.cfg.MutationTimeout), password)
}

func (c *Controller) UpdateLocalPassword(oldPassword, newPassword string) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.UpdateLocalPassword(withTimeout(context.Background(), c.cfg.MutationTimeout), oldPassword, newPassword)
}

func (c *Controller) DeleteLocalPassword(password string) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.DeleteLocalPassword(withTimeout(context.Background(), c.cfg.MutationTimeout), password)
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
	return NetworkSettings{
		Hostname: settings.Hostname,
		IP:       settings.IP,
	}, nil
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
	return NetworkState{
		Hostname: state.Hostname,
		IP:       state.IP,
		DHCP:     &state.DHCP,
	}, nil
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
	return current.SetMQTTSettings(withTimeout(context.Background(), c.cfg.MutationTimeout), client.MQTTSettings{
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
	result, err := current.TestMQTTConnection(withTimeout(context.Background(), c.cfg.MutationTimeout), client.MQTTSettings{
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
	return current.SetSSHKeyState(withTimeout(context.Background(), c.cfg.MutationTimeout), sshKey)
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
	return current.SetKeyboardMacros(withTimeout(context.Background(), c.cfg.MutationTimeout), items)
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
	return c.mutate("unmountImage", nil, nil)
}

func (c *Controller) MountMediaURL(url string, mode virtualmedia.Mode) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("url is required")
	}
	if mode == "" {
		return errors.New("mode is required")
	}
	return c.mutate("mountWithHTTP", map[string]any{
		"url":  strings.TrimSpace(url),
		"mode": mode,
	}, nil)
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
	return c.mutate("deleteStorageFile", map[string]any{"filename": filename}, nil)
}

func (c *Controller) MountStorageFile(filename string, mode virtualmedia.Mode) error {
	if strings.TrimSpace(filename) == "" {
		return errors.New("filename is required")
	}
	if mode == "" {
		return errors.New("mode is required")
	}
	return c.mutate("mountWithStorage", map[string]any{
		"filename": filename,
		"mode":     mode,
	}, nil)
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

	uploadCtx := withTimeout(context.Background(), 30*time.Minute)
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
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SendAbsPointer(x, y, buttons)
}

func (c *Controller) SendRelMouse(dx, dy int8, buttons byte) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SendRelMouse(dx, dy, buttons)
}

func (c *Controller) SendWheel(delta int8) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SendWheel(delta)
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
		})

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

		if err := cl.WaitForHID(withTimeout(ctx, 5*time.Second)); err != nil {
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
			}
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

func (c *Controller) call(ctx context.Context, method string, params map[string]any, out any) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.Call(ctx, method, params, out)
}

func (c *Controller) mutate(method string, params map[string]any, out any) error {
	return c.call(withTimeout(context.Background(), c.cfg.MutationTimeout), method, params, out)
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
			confirmed, confirmErr := confirm(withTimeout(context.Background(), c.cfg.RPCTimeout))
			if confirmErr == nil && confirmed {
				return nil
			}
			return err
		case <-ticker.C:
			confirmed, err := confirm(withTimeout(context.Background(), c.cfg.RPCTimeout))
			if err == nil && confirmed {
				return nil
			}
		case <-ctx.Done():
			confirmed, err := confirm(withTimeout(context.Background(), c.cfg.RPCTimeout))
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
