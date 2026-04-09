package session

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-native/pkg/client"
	"github.com/lkarlslund/jetkvm-native/pkg/input"
)

type Phase string

const (
	PhaseIdle         Phase = "idle"
	PhaseConnecting   Phase = "connecting"
	PhaseConnected    Phase = "connected"
	PhaseReconnecting Phase = "reconnecting"
	PhaseDisconnected Phase = "disconnected"
	PhaseAuthFailed   Phase = "auth_failed"
	PhaseOtherSession Phase = "other_session"
	PhaseRebooting    Phase = "rebooting"
	PhaseFatal        Phase = "fatal_error"
)

type Config struct {
	BaseURL       string
	Password      string
	RPCTimeout    time.Duration
	Reconnect     bool
	ReconnectBase time.Duration
	ReconnectMax  time.Duration
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
	cancelRun context.CancelFunc
}

func New(cfg Config) *Controller {
	if cfg.RPCTimeout == 0 {
		cfg.RPCTimeout = 5 * time.Second
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
	if c.cancelRun != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancelRun = cancel
	go c.run(ctx)
}

func (c *Controller) Stop() {
	if c.cancelRun != nil {
		c.cancelRun()
		c.cancelRun = nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != nil {
		_ = c.current.Close()
		c.current = nil
	}
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
	c.mu.Unlock()
	if current != nil {
		_ = current.Close()
	}
}

func (c *Controller) Reboot() error {
	return c.call(context.Background(), "reboot", map[string]any{"force": false}, nil)
}

func (c *Controller) SetQuality(value float64) error {
	if err := c.call(context.Background(), "setStreamQualityFactor", map[string]any{"factor": value}, nil); err != nil {
		return err
	}
	c.setState(func(s *Snapshot) {
		s.Quality = value
	})
	return nil
}

func (c *Controller) SetKeyboardLayout(layout string) error {
	if layout == "" {
		return errors.New("keyboard layout is required")
	}
	if err := c.call(context.Background(), "setKeyboardLayout", map[string]any{"layout": layout}, nil); err != nil {
		return err
	}
	c.setState(func(s *Snapshot) {
		s.KeyboardLayout = layout
	})
	return nil
}

func (c *Controller) SetTLSMode(mode string) error {
	if mode == "" {
		return errors.New("tls mode is required")
	}
	return c.call(context.Background(), "setTLSState", map[string]any{
		"state": map[string]any{"mode": mode},
	}, nil)
}

func (c *Controller) SetDisplayRotation(rotation string) error {
	if rotation == "" {
		return errors.New("display rotation is required")
	}
	return c.call(context.Background(), "setDisplayRotation", map[string]any{
		"params": map[string]any{"rotation": rotation},
	}, nil)
}

func (c *Controller) SetUSBEmulation(enabled bool) error {
	return c.call(context.Background(), "setUsbEmulationState", map[string]any{"enabled": enabled}, nil)
}

func (c *Controller) SetNetworkSettings(settings map[string]any) error {
	if len(settings) == 0 {
		return errors.New("network settings are required")
	}
	return c.call(context.Background(), "setNetworkSettings", map[string]any{"settings": settings}, nil)
}

func (c *Controller) SendKeypress(key byte, press bool) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.SendKeypress(key, press)
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
	if stats.SignalingMode == "" {
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
			if !c.cfg.Reconnect || ctx.Err() != nil {
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
	var deviceID string
	var quality float64
	var keyboardLayout string
	var edid string
	var version struct {
		AppVersion    string `json:"appVersion"`
		SystemVersion string `json:"systemVersion"`
	}
	var updateStatus struct {
		AppUpdateAvailable    bool `json:"appUpdateAvailable"`
		SystemUpdateAvailable bool `json:"systemUpdateAvailable"`
	}
	var network struct {
		Hostname string `json:"hostname"`
	}

	if err := cl.Call(ctx, "getDeviceID", nil, &deviceID); err == nil {
		c.setState(func(s *Snapshot) { s.DeviceID = deviceID })
	}
	if err := cl.Call(ctx, "getStreamQualityFactor", nil, &quality); err == nil {
		c.setState(func(s *Snapshot) { s.Quality = quality })
	}
	if err := cl.Call(ctx, "getKeyboardLayout", nil, &keyboardLayout); err == nil {
		c.setState(func(s *Snapshot) { s.KeyboardLayout = keyboardLayout })
	}
	if err := cl.Call(ctx, "getEDID", nil, &edid); err == nil {
		c.setState(func(s *Snapshot) { s.EDID = edid })
	}
	if err := cl.Call(ctx, "getLocalVersion", nil, &version); err == nil {
		c.setState(func(s *Snapshot) {
			s.AppVersion = version.AppVersion
			s.SystemVersion = version.SystemVersion
		})
	}
	if err := cl.Call(ctx, "getUpdateStatus", nil, &updateStatus); err == nil {
		c.setState(func(s *Snapshot) {
			s.AppUpdateAvailable = updateStatus.AppUpdateAvailable
			s.SystemUpdateAvailable = updateStatus.SystemUpdateAvailable
		})
	}
	if err := cl.Call(ctx, "getNetworkSettings", nil, &network); err == nil {
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

func (c *Controller) call(ctx context.Context, method string, params map[string]any, out any) error {
	current := c.clientIfConnected()
	if current == nil {
		return errors.New("client not connected")
	}
	return current.Call(ctx, method, params, out)
}

func (c *Controller) Query(ctx context.Context, method string, params map[string]any, out any) error {
	return c.call(ctx, method, params, out)
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

func isAuthError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unauthorized") || strings.Contains(msg, "login failed")
}

func extractString(v any, key string) string {
	if m, ok := v.(map[string]any); ok {
		if s, ok := m[key].(string); ok {
			return s
		}
	}
	return ""
}
