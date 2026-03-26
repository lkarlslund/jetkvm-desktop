package emulator

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-native/internal/protocol/hidrpc"
	"github.com/lkarlslund/jetkvm-native/internal/protocol/jsonrpc"
	"github.com/lkarlslund/jetkvm-native/internal/protocol/signaling"
	"github.com/lkarlslund/jetkvm-native/internal/video"
)

type AuthMode string

const (
	AuthModeUnset      AuthMode = ""
	AuthModeNoPassword AuthMode = "noPassword"
	AuthModePassword   AuthMode = "password"
)

type Config struct {
	ListenAddr string
	AuthMode   AuthMode
	Password   string
	Width      int
	Height     int
	FPS        int
}

type DeviceState struct {
	DeviceID            string
	VideoState          string
	StreamQualityFactor float64
	KeyboardLEDMask     byte
	KeysDown            []byte
	Hostname            string
	CloudURL            string
	CloudAppURL         string
	KeyboardLayout      string
}

type InputRecord struct {
	Channel string
	Type    string
	Data    string
	At      time.Time
}

type Server struct {
	cfg        Config
	httpServer *http.Server
	listener   net.Listener

	mu        sync.Mutex
	session   *session
	token     string
	state     DeviceState
	inputs    []InputRecord
	startedCh chan struct{}
}

type session struct {
	pc        *webrtc.PeerConnection
	rpc       *webrtc.DataChannel
	hid       *webrtc.DataChannel
	opened    map[string]bool
	openedMu  sync.Mutex
	serverRef *Server
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:8080"
	}
	if cfg.Width == 0 {
		cfg.Width = 960
	}
	if cfg.Height == 0 {
		cfg.Height = 540
	}
	if cfg.FPS == 0 {
		cfg.FPS = 15
	}
	s := &Server{
		cfg:    cfg,
		token:  "jetkvm-native-emulator-token",
		state:  DeviceState{DeviceID: "emu-jetkvm-001", VideoState: "ok", StreamQualityFactor: 0.75, KeyboardLEDMask: 0, KeysDown: []byte{0, 0, 0, 0, 0, 0}, Hostname: "jetkvm-emulator", KeyboardLayout: "en_US"},
		inputs: make([]InputRecord, 0, 32),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/device/status", s.handleDeviceStatus)
	mux.HandleFunc("/device/setup", s.handleSetup)
	mux.HandleFunc("/device", s.handleDevice)
	mux.HandleFunc("/auth/login-local", s.handleLogin)
	mux.HandleFunc("/auth/logout", s.handleLogout)
	mux.HandleFunc("/auth/password-local", s.handlePasswordLocal)
	mux.HandleFunc("/auth/local-password", s.handleDeletePassword)
	mux.HandleFunc("/cloud/state", s.handleCloudState)
	mux.HandleFunc("/webrtc/session", s.handleSession)
	mux.HandleFunc("/healthz", s.handleHealth)

	s.httpServer = &http.Server{Handler: mux}
	return s, nil
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return err
	}
	s.listener = ln

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (s *Server) BaseURL() string {
	if s.listener == nil {
		return ""
	}
	return "http://" + s.listener.Addr().String()
}

func (s *Server) Inputs() []InputRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]InputRecord, len(s.inputs))
	copy(out, s.inputs)
	return out
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeCORS(w, r)
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.handlePreflight(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.AuthMode == AuthModeNoPassword {
		http.Error(w, "login disabled in noPassword mode", http.StatusBadRequest)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Password != s.cfg.Password {
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "authToken",
		Value:    s.token,
		Path:     "/",
		HttpOnly: true,
	})
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Login successful"})
}

func (s *Server) authorized(r *http.Request) bool {
	if s.cfg.AuthMode == AuthModeNoPassword || s.cfg.AuthMode == AuthModeUnset {
		return true
	}
	cookie, err := r.Cookie("authToken")
	return err == nil && cookie.Value == s.token
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if s.handlePreflight(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req signaling.ExchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	answer, err := s.exchangeOffer(req.SD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(signaling.ExchangeResponse{SD: answer})
}

func (s *Server) handleDeviceStatus(w http.ResponseWriter, r *http.Request) {
	if s.handlePreflight(w, r) {
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"isSetup": s.cfg.AuthMode != AuthModeUnset})
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request) {
	if s.handlePreflight(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	authMode := string(s.cfg.AuthMode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"authMode":     &authMode,
		"deviceId":     s.state.DeviceID,
		"loopbackOnly": false,
	})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s.handlePreflight(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.AuthMode != AuthModeUnset {
		http.Error(w, "Device is already set up", http.StatusBadRequest)
		return
	}
	var req struct {
		LocalAuthMode string `json:"localAuthMode"`
		Password      string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	switch AuthMode(req.LocalAuthMode) {
	case AuthModeNoPassword:
		s.cfg.AuthMode = AuthModeNoPassword
	case AuthModePassword:
		if req.Password == "" {
			http.Error(w, "Password is required for password mode", http.StatusBadRequest)
			return
		}
		s.cfg.AuthMode = AuthModePassword
		s.cfg.Password = req.Password
		http.SetCookie(w, &http.Cookie{Name: "authToken", Value: s.token, Path: "/", HttpOnly: true})
	default:
		http.Error(w, "Invalid localAuthMode", http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Device setup completed successfully"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.handlePreflight(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "authToken", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Logout successful"})
}

func (s *Server) handlePasswordLocal(w http.ResponseWriter, r *http.Request) {
	if s.handlePreflight(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		if s.cfg.AuthMode != AuthModeNoPassword {
			http.Error(w, "Password mode is not enabled", http.StatusBadRequest)
			return
		}
		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		s.cfg.AuthMode = AuthModePassword
		s.cfg.Password = req.Password
		http.SetCookie(w, &http.Cookie{Name: "authToken", Value: s.token, Path: "/", HttpOnly: true})
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Password set successfully"})
	case http.MethodPut:
		if s.cfg.AuthMode != AuthModePassword {
			http.Error(w, "Password mode is not enabled", http.StatusBadRequest)
			return
		}
		var req struct {
			OldPassword string `json:"oldPassword"`
			NewPassword string `json:"newPassword"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NewPassword == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if s.cfg.Password != "" && req.OldPassword != s.cfg.Password {
			http.Error(w, "Incorrect old password", http.StatusUnauthorized)
			return
		}
		s.cfg.Password = req.NewPassword
		http.SetCookie(w, &http.Cookie{Name: "authToken", Value: s.token, Path: "/", HttpOnly: true})
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Password updated successfully"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDeletePassword(w http.ResponseWriter, r *http.Request) {
	if s.handlePreflight(w, r) {
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.AuthMode != AuthModePassword {
		http.Error(w, "Password mode is not enabled", http.StatusBadRequest)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.Password != s.cfg.Password {
		http.Error(w, "Incorrect password", http.StatusUnauthorized)
		return
	}
	s.cfg.Password = ""
	s.cfg.AuthMode = AuthModeNoPassword
	http.SetCookie(w, &http.Cookie{Name: "authToken", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Password disabled successfully"})
}

func (s *Server) handleCloudState(w http.ResponseWriter, r *http.Request) {
	if s.handlePreflight(w, r) {
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"connected": s.state.CloudURL != "",
		"url":       s.state.CloudURL,
		"appUrl":    s.state.CloudAppURL,
	})
}

func (s *Server) handlePreflight(w http.ResponseWriter, r *http.Request) bool {
	s.writeCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func (s *Server) writeCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	if origin != "*" {
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Content-Type", "application/json")
}

func (s *Server) exchangeOffer(encoded string) (string, error) {
	rawOffer, err := signaling.DecodeSDP(encoded)
	if err != nil {
		return "", err
	}

	var offer webrtc.SessionDescription
	if err := json.Unmarshal(rawOffer, &offer); err != nil {
		return "", err
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return "", err
	}

	sess := &session{pc: pc, opened: map[string]bool{}, serverRef: s}
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		sess.openedMu.Lock()
		sess.opened[dc.Label()] = true
		sess.openedMu.Unlock()

		switch dc.Label() {
		case "rpc":
			sess.rpc = dc
			dc.OnOpen(func() {
				_ = sess.sendEvent("videoInputState", map[string]string{"state": s.state.VideoState})
				_ = sess.sendEvent("keyboardLedState", map[string]byte{"mask": s.state.KeyboardLEDMask})
				_ = sess.sendEvent("networkState", map[string]any{"hostname": s.state.Hostname, "ip": "127.0.0.1"})
				_ = sess.sendEvent("usbState", map[string]any{"keyboard": true, "mouse": true, "massStorage": false})
				_ = sess.sendEvent("failsafeMode", map[string]any{"active": false, "reason": ""})
			})
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				if !msg.IsString {
					return
				}
				_ = sess.handleRPC(msg.Data)
			})
		case "hidrpc":
			sess.hid = dc
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				_ = sess.handleHID("hidrpc", msg.Data)
			})
		case "hidrpc-unreliable-ordered":
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				_ = sess.handleHID("hidrpc-unreliable-ordered", msg.Data)
			})
		case "hidrpc-unreliable-nonordered":
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				_ = sess.handleHID("hidrpc-unreliable-nonordered", msg.Data)
			})
		}
	})

	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"jetkvm-native-emulator",
	)
	if err != nil {
		return "", err
	}
	sender, err := pc.AddTrack(videoTrack)
	if err != nil {
		return "", err
	}
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, err := sender.Read(rtcpBuf); err != nil {
				return
			}
		}
	}()

	if err := pc.SetRemoteDescription(offer); err != nil {
		return "", err
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		return "", err
	}
	<-webrtc.GatheringCompletePromise(pc)

	s.mu.Lock()
	if s.session != nil && s.session.rpc != nil {
		_ = s.session.sendEvent("otherSessionConnected", nil)
		_ = s.session.pc.Close()
	}
	s.session = sess
	s.mu.Unlock()

	streamCtx, cancel := context.WithCancel(context.Background())
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateClosed ||
			state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateDisconnected {
			cancel()
		}
	})
	if err := video.StartTestPattern(streamCtx, s.cfg.Width, s.cfg.Height, s.cfg.FPS, videoTrack); err != nil {
		cancel()
		return "", err
	}

	rawAnswer, err := json.Marshal(pc.LocalDescription())
	if err != nil {
		return "", err
	}
	return signaling.EncodeSDP(rawAnswer), nil
}

func (s *Server) appendInput(channel, typ, data string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputs = append(s.inputs, InputRecord{
		Channel: channel,
		Type:    typ,
		Data:    data,
		At:      time.Now(),
	})
}

func (s *session) sendEvent(method string, params any) error {
	if s.rpc == nil {
		return nil
	}
	data, err := jsonrpc.Marshal(jsonrpc.NewEvent(method, params))
	if err != nil {
		return err
	}
	return s.rpc.SendText(string(data))
}

func (s *session) handleRPC(data []byte) error {
	decoded, err := jsonrpc.DecodeMessage(data)
	if err != nil {
		return err
	}
	req, ok := decoded.(jsonrpc.Request)
	if !ok {
		return nil
	}

	var resp jsonrpc.Response
	switch req.Method {
	case "ping":
		resp = jsonrpc.NewResponse(req.ID, "pong")
	case "getDeviceID":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.DeviceID)
	case "getVideoState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.VideoState)
	case "getNetworkSettings":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"hostname": s.serverRef.state.Hostname, "ip": "127.0.0.1"})
	case "getNetworkState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"hostname": s.serverRef.state.Hostname, "ip": "127.0.0.1", "dhcp": true})
	case "setNetworkSettings":
		if settings, ok := req.Params["settings"].(map[string]any); ok {
			if hostname, ok := settings["hostname"].(string); ok && hostname != "" {
				s.serverRef.state.Hostname = hostname
			}
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getKeyboardLedState":
		resp = jsonrpc.NewResponse(req.ID, map[string]byte{"mask": s.serverRef.state.KeyboardLEDMask})
	case "getKeyDownState", "getKeysDownState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"modifier": 0, "keys": s.serverRef.state.KeysDown})
	case "getStreamQualityFactor":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.StreamQualityFactor)
	case "setStreamQualityFactor":
		if factor, ok := req.Params["factor"].(float64); ok {
			s.serverRef.state.StreamQualityFactor = factor
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing factor", nil)
		}
	case "reboot":
		resp = jsonrpc.NewResponse(req.ID, true)
		go func() {
			time.Sleep(100 * time.Millisecond)
			_ = s.sendEvent("videoInputState", map[string]string{"state": "rebooting"})
		}()
	case "getKeyboardLayout":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.KeyboardLayout)
	case "setKeyboardLayout":
		if layout, ok := req.Params["layout"].(string); ok && layout != "" {
			s.serverRef.state.KeyboardLayout = layout
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getAutoUpdateState":
		resp = jsonrpc.NewResponse(req.ID, false)
	case "getBacklightSettings":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"max_brightness": 100, "dim_after": 60, "off_after": 300})
	case "getVideoSleepMode":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"duration": 0})
	case "getDisplayRotation":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"rotation": "0"})
	case "getEDID":
		resp = jsonrpc.NewResponse(req.ID, "")
	case "getVideoLogStatus":
		resp = jsonrpc.NewResponse(req.ID, "disabled")
	case "getDevModeState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"enabled": false})
	case "getSSHKeyState":
		resp = jsonrpc.NewResponse(req.ID, "")
	case "getUsbEmulationState":
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getDevChannelState":
		resp = jsonrpc.NewResponse(req.ID, false)
	case "getLocalLoopbackOnly":
		resp = jsonrpc.NewResponse(req.ID, false)
	case "getCloudState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"connected": s.serverRef.state.CloudURL != "", "url": s.serverRef.state.CloudURL, "appUrl": s.serverRef.state.CloudAppURL})
	case "getTLSState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"enabled": false})
	case "getMqttSettings":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"enabled": false})
	case "getMqttStatus":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"connected": false})
	case "getActiveExtension":
		resp = jsonrpc.NewResponse(req.ID, "")
	case "getVirtualMediaState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"mounted": false, "uploading": false})
	case "getStorageSpace":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"free": 1024 * 1024 * 1024, "total": 2 * 1024 * 1024 * 1024})
	case "getJigglerState":
		resp = jsonrpc.NewResponse(req.ID, false)
	case "getJigglerConfig":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"enabled": false})
	case "getTimezones":
		resp = jsonrpc.NewResponse(req.ID, []string{"UTC", "Europe/Copenhagen"})
	case "getSerialCommandHistory":
		resp = jsonrpc.NewResponse(req.ID, []string{})
	case "getWakeOnLanDevices":
		resp = jsonrpc.NewResponse(req.ID, []any{})
	case "getPublicIPAddresses":
		resp = jsonrpc.NewResponse(req.ID, []string{"127.0.0.1"})
	case "getTailscaleStatus":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"connected": false})
	case "getSerialSettings":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"baudRate": 115200, "dataBits": 8, "stopBits": 1, "parity": "none"})
	case "getDCPowerState":
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getATXState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"power": "on", "hdd": false})
	case "getUsbConfig":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"vendor_id": "0xCafe", "product_id": "0x4000"})
	case "getUsbDevices":
		resp = jsonrpc.NewResponse(req.ID, []any{})
	default:
		resp = jsonrpc.NewErrorResponse(req.ID, -32601, "method not found", req.Method)
	}

	payload, err := jsonrpc.Marshal(resp)
	if err != nil {
		return err
	}
	return s.rpc.SendText(string(payload))
}

func (s *session) handleHID(channel string, data []byte) error {
	msg, err := hidrpc.Decode(data)
	if err != nil {
		return err
	}
	s.serverRef.appendInput(channel, fmt.Sprintf("%T", msg), fmt.Sprintf("%v", msg))

	switch v := msg.(type) {
	case hidrpc.Handshake:
		reply, err := hidrpc.Handshake{Version: v.Version}.MarshalBinary()
		if err != nil {
			return err
		}
		return s.hid.Send(reply)
	case hidrpc.Keypress:
		s.serverRef.state.KeysDown = []byte{v.Key, 0, 0, 0, 0, 0}
		return s.sendEvent("keysDownState", map[string]any{"modifier": 0, "keys": s.serverRef.state.KeysDown})
	case hidrpc.Pointer:
		return s.sendEvent("pointerState", map[string]any{"x": v.X, "y": v.Y, "buttons": v.Buttons})
	case hidrpc.Mouse:
		return s.sendEvent("relativePointerState", map[string]any{"dx": v.DX, "dy": v.DY, "buttons": v.Buttons})
	case hidrpc.Wheel:
		return s.sendEvent("wheelState", map[string]any{"delta": v.Delta})
	}
	return nil
}

func TrimBaseURL(addr string) string {
	return "http://" + strings.TrimPrefix(addr, "http://")
}
