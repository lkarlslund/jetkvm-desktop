package emulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-native/pkg/protocol/hidrpc"
	"github.com/lkarlslund/jetkvm-native/pkg/protocol/jsonrpc"
	"github.com/lkarlslund/jetkvm-native/pkg/protocol/signaling"
	"github.com/lkarlslund/jetkvm-native/pkg/video"
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
	Faults     FaultConfig
}

type FaultConfig struct {
	RPCDelay          time.Duration
	DropRPCMethod     string
	DisconnectAfter   time.Duration
	HIDHandshakeDelay time.Duration
	InitialVideoState string
}

type DeviceState struct {
	DeviceID            string
	VideoState          string
	StreamQualityFactor float64
	KeyboardLEDMask     byte
	KeyboardModifiers   byte
	KeysDown            []byte
	Hostname            string
	CloudURL            string
	CloudAppURL         string
	KeyboardLayout      string
	TLSMode             string
	DisplayRotation     string
	USBEmulation        bool
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
		state:  DeviceState{DeviceID: "emu-jetkvm-001", VideoState: "ready", StreamQualityFactor: 0.75, KeyboardLEDMask: 0, KeyboardModifiers: 0, KeysDown: []byte{0, 0, 0, 0, 0, 0}, Hostname: "jetkvm-emulator", KeyboardLayout: "en_US", TLSMode: "disabled", DisplayRotation: "270", USBEmulation: true},
		inputs: make([]InputRecord, 0, 32),
	}
	if cfg.Faults.InitialVideoState != "" {
		s.state.VideoState = cfg.Faults.InitialVideoState
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
	mux.HandleFunc("/webrtc/signaling/client", s.handleSignalingClient)
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
		_ = ln.Close()
		_ = s.httpServer.Shutdown(shutdownCtx)
		err := <-errCh
		if err == nil || err == http.ErrServerClosed {
			return nil
		}
		return err
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

func (s *Server) handleSignalingClient(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	_ = conn.WriteJSON(map[string]any{
		"type": "device-metadata",
		"data": map[string]any{
			"deviceVersion": "jetkvm-native-emulator",
		},
	})

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		if bytes.Equal(msg, []byte("ping")) {
			_ = conn.WriteMessage(websocket.TextMessage, []byte("pong"))
			continue
		}

		var message struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg, &message); err != nil {
			continue
		}

		switch message.Type {
		case "offer":
			var req signaling.ExchangeRequest
			if err := json.Unmarshal(message.Data, &req); err != nil {
				continue
			}
			answer, err := s.exchangeOffer(req.SD)
			if err != nil {
				continue
			}
			_ = conn.WriteJSON(map[string]any{"type": "answer", "data": answer})
		case "new-ice-candidate":
			var candidate webrtc.ICECandidateInit
			if err := json.Unmarshal(message.Data, &candidate); err != nil {
				continue
			}
			s.mu.Lock()
			current := s.session
			s.mu.Unlock()
			if current != nil {
				_ = current.pc.AddICECandidate(candidate)
			}
		}
	}
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
				_ = sess.sendEvent("videoInputState", s.state.VideoState)
				_ = sess.sendEvent("keyboardLedState", map[string]byte{"mask": s.state.KeyboardLEDMask})
				_ = sess.sendEvent("networkState", map[string]any{"hostname": s.state.Hostname, "ip": "127.0.0.1"})
				_ = sess.sendEvent("usbState", "attached")
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
		prev := s.session
		go func() {
			_ = prev.sendEvent("otherSessionConnected", nil)
			time.Sleep(100 * time.Millisecond)
			_ = prev.pc.Close()
		}()
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
	if s.cfg.Faults.DisconnectAfter > 0 {
		go func() {
			timer := time.NewTimer(s.cfg.Faults.DisconnectAfter)
			defer timer.Stop()
			select {
			case <-streamCtx.Done():
			case <-timer.C:
				_ = pc.Close()
			}
		}()
	}
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
	if delay := s.serverRef.cfg.Faults.RPCDelay; delay > 0 {
		time.Sleep(delay)
	}
	if method := s.serverRef.cfg.Faults.DropRPCMethod; method != "" && method == req.Method {
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
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"modifier": s.serverRef.state.KeyboardModifiers, "keys": s.serverRef.state.KeysDown})
	case "getStreamQualityFactor":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.StreamQualityFactor)
	case "setStreamQualityFactor":
		if factor, ok := req.Params["factor"].(float64); ok {
			s.serverRef.state.StreamQualityFactor = factor
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing factor", nil)
		}
	case "wheelReport":
		if wheelY, ok := req.Params["wheelY"].(float64); ok {
			s.serverRef.appendInput("rpc", "rpc.wheelReport", fmt.Sprintf("wheelY=%d", int8(wheelY)))
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing wheelY", nil)
		}
	case "reboot":
		resp = jsonrpc.NewResponse(req.ID, true)
		go func() {
			s.serverRef.mu.Lock()
			s.serverRef.state.VideoState = "rebooting"
			s.serverRef.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
			_ = s.sendEvent("videoInputState", "rebooting")
			time.Sleep(250 * time.Millisecond)
			s.serverRef.mu.Lock()
			s.serverRef.state.VideoState = "ready"
			s.serverRef.mu.Unlock()
			_ = s.sendEvent("videoInputState", "ready")
		}()
	case "forceDisconnect":
		resp = jsonrpc.NewResponse(req.ID, true)
		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = s.pc.Close()
		}()
	case "getKeyboardLayout":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.KeyboardLayout)
	case "setKeyboardLayout":
		if layout, ok := req.Params["layout"].(string); ok && layout != "" {
			s.serverRef.state.KeyboardLayout = layout
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getEDID":
		resp = jsonrpc.NewResponse(req.ID, "")
	case "getUsbEmulationState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.USBEmulation)
	case "setUsbEmulationState":
		if enabled, ok := req.Params["enabled"].(bool); ok {
			s.serverRef.state.USBEmulation = enabled
			resp = jsonrpc.NewResponse(req.ID, enabled)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
		}
	case "getCloudState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"connected": s.serverRef.state.CloudURL != "", "url": s.serverRef.state.CloudURL, "appUrl": s.serverRef.state.CloudAppURL})
	case "getTLSState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"mode": s.serverRef.state.TLSMode})
	case "setTLSState":
		if state, ok := req.Params["state"].(map[string]any); ok {
			if mode, ok := state["mode"].(string); ok && mode != "" {
				s.serverRef.state.TLSMode = mode
				resp = jsonrpc.NewResponse(req.ID, true)
			} else {
				resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing mode", nil)
			}
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing state", nil)
		}
	case "getUsbConfig":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"vendor_id": "0xCafe", "product_id": "0x4000"})
	case "getUsbDevices":
		resp = jsonrpc.NewResponse(req.ID, []any{})
	case "getDisplayRotation":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"rotation": s.serverRef.state.DisplayRotation})
	case "setDisplayRotation":
		if params, ok := req.Params["params"].(map[string]any); ok {
			if rotation, ok := params["rotation"].(string); ok && rotation != "" {
				s.serverRef.state.DisplayRotation = rotation
				resp = jsonrpc.NewResponse(req.ID, true)
			} else {
				resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing rotation", nil)
			}
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing params", nil)
		}
	case "getKeyboardMacros":
		resp = jsonrpc.NewResponse(req.ID, []any{})
	case "getLocalVersion":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"appVersion": "emulator-dev", "systemVersion": "emulator-dev"})
	case "getUpdateStatus":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{
			"local": map[string]any{
				"appVersion":    "emulator-dev",
				"systemVersion": "emulator-dev",
			},
			"remote": map[string]any{
				"appVersion":    "emulator-dev",
				"systemVersion": "emulator-dev",
			},
			"systemUpdateAvailable": false,
			"appUpdateAvailable":    false,
		})
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
		if delay := s.serverRef.cfg.Faults.HIDHandshakeDelay; delay > 0 {
			time.Sleep(delay)
		}
		reply, err := hidrpc.Handshake{Version: v.Version}.MarshalBinary()
		if err != nil {
			return err
		}
		return s.hid.Send(reply)
	case hidrpc.Keypress:
		s.serverRef.applyKeypress(v.Key, v.Press)
		return s.sendEvent("keysDownState", map[string]any{"modifier": s.serverRef.state.KeyboardModifiers, "keys": s.serverRef.state.KeysDown})
	case hidrpc.KeyboardMacroReport:
		if v.IsPaste {
			stateMsg, err := hidrpc.KeyboardMacroState{State: true, IsPaste: true}.MarshalBinary()
			if err == nil {
				_ = s.hid.Send(stateMsg)
			}
		}
		for _, step := range v.Steps {
			s.serverRef.mu.Lock()
			s.serverRef.state.KeyboardModifiers = step.Modifier
			keys := make([]byte, 0, len(step.Keys))
			for _, key := range step.Keys {
				keys = append(keys, key)
			}
			s.serverRef.state.KeysDown = keys
			s.serverRef.mu.Unlock()
			_ = s.sendEvent("keysDownState", map[string]any{"modifier": s.serverRef.state.KeyboardModifiers, "keys": s.serverRef.state.KeysDown})
		}
		if v.IsPaste {
			stateMsg, err := hidrpc.KeyboardMacroState{State: false, IsPaste: true}.MarshalBinary()
			if err == nil {
				_ = s.hid.Send(stateMsg)
			}
		}
		return nil
	case hidrpc.CancelKeyboardMacro:
		stateMsg, err := hidrpc.KeyboardMacroState{State: false, IsPaste: true}.MarshalBinary()
		if err == nil {
			_ = s.hid.Send(stateMsg)
		}
		return nil
	}
	return nil
}

func TrimBaseURL(addr string) string {
	return "http://" + strings.TrimPrefix(addr, "http://")
}

func (s *Server) applyKeypress(key byte, press bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if modifierBit, ok := modifierMask(key); ok {
		if press {
			s.state.KeyboardModifiers |= modifierBit
		} else {
			s.state.KeyboardModifiers &^= modifierBit
		}
		return
	}

	keys := append([]byte(nil), s.state.KeysDown...)
	if len(keys) < 6 {
		padded := make([]byte, 6)
		copy(padded, keys)
		keys = padded
	}
	if press {
		for _, existing := range keys {
			if existing == key {
				s.state.KeysDown = keys
				return
			}
		}
		for i, existing := range keys {
			if existing == 0 {
				keys[i] = key
				s.state.KeysDown = keys
				return
			}
		}
		copy(keys, keys[1:])
		keys[len(keys)-1] = key
		s.state.KeysDown = keys
		return
	}

	next := make([]byte, 0, len(keys))
	for _, existing := range keys {
		if existing != 0 && existing != key {
			next = append(next, existing)
		}
	}
	for len(next) < 6 {
		next = append(next, 0)
	}
	s.state.KeysDown = next
}

func modifierMask(key byte) (byte, bool) {
	switch key {
	case 224:
		return 0x01, true
	case 225:
		return 0x02, true
	case 226:
		return 0x04, true
	case 227:
		return 0x08, true
	case 228:
		return 0x10, true
	case 229:
		return 0x20, true
	case 230:
		return 0x40, true
	case 231:
		return 0x80, true
	default:
		return 0, false
	}
}
