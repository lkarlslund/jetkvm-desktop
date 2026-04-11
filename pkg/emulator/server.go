package emulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/hidrpc"
	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/jsonrpc"
	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/signaling"
	"github.com/lkarlslund/jetkvm-desktop/pkg/video"
	"github.com/lkarlslund/jetkvm-desktop/pkg/virtualmedia"
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
	RPCDelay              time.Duration
	DropRPCMethod         string
	ApplyButDropRPCMethod string
	DisconnectAfter       time.Duration
	HIDHandshakeDelay     time.Duration
	InitialVideoState     string
}

type DeviceState struct {
	DeviceID            string
	VideoState          string
	StreamQualityFactor float64
	VideoCodec          string
	EDID                string
	AutoUpdateEnabled   bool
	KeyboardLEDMask     byte
	KeyboardModifiers   byte
	KeysDown            []byte
	Hostname            string
	CloudURL            string
	CloudAppURL         string
	KeyboardLayout      string
	DeveloperMode       bool
	DevChannel          bool
	LoopbackOnly        bool
	SSHKey              string
	JigglerEnabled      bool
	JigglerConfig       map[string]any
	TLSMode             string
	TLSCertificate      string
	TLSPrivateKey       string
	DisplayRotation     string
	BacklightSettings   map[string]any
	VideoSleepDuration  int
	USBEmulation        bool
	USBConfig           map[string]any
	USBDevices          map[string]any
	USBNetworkConfig    map[string]any
	NetworkSettings     map[string]any
	NetworkState        map[string]any
	MQTTSettings        map[string]any
	MQTTConnected       bool
	MQTTError           string
	KeyboardMacros      []map[string]any
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

	mu      sync.Mutex
	session *session
	token   string
	state   DeviceState
	inputs  []InputRecord
	media   *virtualmedia.State
	storage map[string]storedFile
	uploads map[string]*pendingUpload
}

type storedFile struct {
	data      []byte
	createdAt time.Time
}

type pendingUpload struct {
	filename string
	size     int64
}

type session struct {
	pc         *webrtc.PeerConnection
	rpc        *webrtc.DataChannel
	hid        *webrtc.DataChannel
	hidOrdered *webrtc.DataChannel
	hidLoose   *webrtc.DataChannel
	opened     map[string]bool
	openedMu   sync.Mutex
	serverRef  *Server
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
		cfg:   cfg,
		token: "jetkvm-desktop-emulator-token",
		state: DeviceState{
			DeviceID:            "emu-jetkvm-001",
			VideoState:          "ready",
			StreamQualityFactor: 0.75,
			VideoCodec:          "auto",
			EDID:                "",
			AutoUpdateEnabled:   true,
			KeyboardLEDMask:     0,
			KeyboardModifiers:   0,
			KeysDown:            []byte{0, 0, 0, 0, 0, 0},
			Hostname:            "jetkvm-emulator",
			KeyboardLayout:      "en_US",
			DeveloperMode:       false,
			DevChannel:          false,
			LoopbackOnly:        false,
			SSHKey:              "",
			JigglerEnabled:      false,
			JigglerConfig: map[string]any{
				"inactivity_limit_seconds": 60,
				"jitter_percentage":        25,
				"schedule_cron_tab":        "0 * * * * *",
			},
			TLSMode:         "disabled",
			TLSCertificate:  "",
			TLSPrivateKey:   "",
			DisplayRotation: "270",
			BacklightSettings: map[string]any{
				"max_brightness": 64,
				"dim_after":      300,
				"off_after":      600,
			},
			VideoSleepDuration: -1,
			USBEmulation:       true,
			USBConfig: map[string]any{
				"vendor_id":     "0xCafe",
				"product_id":    "0x4000",
				"serial_number": "JETKVM-DESKTOP",
				"manufacturer":  "JetKVM",
				"product":       "JetKVM Default",
			},
			USBDevices: map[string]any{
				"keyboard":       true,
				"absolute_mouse": true,
				"relative_mouse": true,
				"mass_storage":   true,
				"serial_console": false,
				"network":        false,
			},
			USBNetworkConfig: map[string]any{
				"enabled":           false,
				"host_preset":       "auto",
				"protocol":          "ncm",
				"sharing_mode":      "nat",
				"uplink_mode":       "auto",
				"uplink_interface":  "",
				"ipv4_subnet_cidr":  "10.55.0.0/24",
				"dhcp_enabled":      true,
				"dns_proxy_enabled": true,
			},
			NetworkSettings: map[string]any{
				"dhcp_client":    "dhcpcd",
				"hostname":       "jetkvm-emulator",
				"domain":         "lab.example",
				"http_proxy":     "",
				"ipv4_mode":      "dhcp",
				"mdns_mode":      "auto",
				"ipv6_mode":      "slaac",
				"time_sync_mode": "ntp_only",
				"ipv4_static": map[string]any{
					"address": "192.168.1.50",
					"netmask": "255.255.255.0",
					"gateway": "192.168.1.1",
					"dns":     []string{"1.1.1.1", "8.8.8.8"},
				},
				"ipv6_static": map[string]any{
					"prefix":  "2001:db8::50/64",
					"gateway": "2001:db8::1",
					"dns":     []string{"2606:4700:4700::1111"},
				},
				"time_sync_ntp_servers": []string{"0.pool.ntp.org", "1.pool.ntp.org"},
				"time_sync_http_urls":   []string{"https://time.cloudflare.com"},
			},
			NetworkState: map[string]any{
				"interface_name":  "eth0",
				"mac_address":     "02:42:ac:11:00:02",
				"ipv4":            "192.168.1.50",
				"ipv4_addresses":  []string{"192.168.1.50/24"},
				"ipv6":            "2001:db8::50",
				"ipv6_addresses":  []map[string]any{{"address": "2001:db8::50", "prefix": "64"}},
				"ipv6_link_local": "fe80::1",
				"ipv6_gateway":    "2001:db8::1",
				"hostname":        "jetkvm-emulator",
				"dhcp_lease": map[string]any{
					"ip":           "192.168.1.50",
					"netmask":      "255.255.255.0",
					"dns_servers":  []string{"1.1.1.1", "8.8.8.8"},
					"domain":       "lab.example",
					"routers":      []string{"192.168.1.1"},
					"server_id":    "192.168.1.1",
					"lease_expiry": time.Now().Add(8 * time.Hour),
					"dhcp_client":  "dhcpcd",
				},
			},
			MQTTSettings: map[string]any{
				"enabled":             false,
				"broker":              "mqtt.local",
				"port":                1883,
				"username":            "",
				"password":            "",
				"base_topic":          "jetkvm",
				"use_tls":             false,
				"tls_insecure":        false,
				"enable_ha_discovery": false,
				"enable_actions":      false,
				"debounce_ms":         0,
			},
			KeyboardMacros: []map[string]any{},
		},
		inputs: make([]InputRecord, 0, 32),
		storage: map[string]storedFile{
			"debian.iso": {data: bytes.Repeat([]byte("D"), 8*1024), createdAt: time.Now().Add(-2 * time.Hour)},
			"tools.img":  {data: bytes.Repeat([]byte("I"), 4*1024), createdAt: time.Now().Add(-1 * time.Hour)},
		},
		uploads: make(map[string]*pendingUpload),
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
	mux.HandleFunc("/storage/upload", s.handleStorageUpload)
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
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
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
			"deviceVersion": "jetkvm-desktop-emulator",
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
		"loopbackOnly": s.state.LoopbackOnly,
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

func (s *Server) handleStorageUpload(w http.ResponseWriter, r *http.Request) {
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
	uploadID := strings.TrimSpace(r.URL.Query().Get("uploadId"))
	if uploadID == "" {
		http.Error(w, "missing uploadId", http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read upload", http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	upload, ok := s.uploads[uploadID]
	if !ok {
		http.Error(w, "upload not found", http.StatusNotFound)
		return
	}
	incompleteName := upload.filename + ".incomplete"
	file := s.storage[incompleteName]
	file.data = append(file.data, data...)
	if file.createdAt.IsZero() {
		file.createdAt = time.Now()
	}
	if int64(len(file.data)) >= upload.size {
		s.storage[upload.filename] = storedFile{data: file.data[:upload.size], createdAt: file.createdAt}
		delete(s.storage, incompleteName)
		delete(s.uploads, uploadID)
	} else {
		s.storage[incompleteName] = file
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Upload completed"})
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
			sess.hidOrdered = dc
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				_ = sess.handleHID("hidrpc-unreliable-ordered", msg.Data)
			})
		case "hidrpc-unreliable-nonordered":
			sess.hidLoose = dc
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				_ = sess.handleHID("hidrpc-unreliable-nonordered", msg.Data)
			})
		}
	})

	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"jetkvm-desktop-emulator",
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
				sess.closeTransport()
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

func (s *session) closeTransport() {
	if s.rpc != nil {
		_ = s.rpc.Close()
	}
	if s.hid != nil {
		_ = s.hid.Close()
	}
	if s.hidOrdered != nil {
		_ = s.hidOrdered.Close()
	}
	if s.hidLoose != nil {
		_ = s.hidLoose.Close()
	}
	if s.pc != nil {
		_ = s.pc.Close()
	}
}

func requestParams(raw any) map[string]any {
	params, _ := raw.(map[string]any)
	if params == nil {
		return map[string]any{}
	}
	return params
}

func mapsClone(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
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
	applyButDrop := s.serverRef.cfg.Faults.ApplyButDropRPCMethod
	params := requestParams(req.Params)
	const mqttPasswordMask = "********"

	var resp jsonrpc.Response
	switch req.Method {
	case "ping":
		resp = jsonrpc.NewResponse(req.ID, "pong")
	case "getDeviceID":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.DeviceID)
	case "getVideoState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.VideoState)
	case "getNetworkSettings":
		resp = jsonrpc.NewResponse(req.ID, mapsClone(s.serverRef.state.NetworkSettings))
	case "getNetworkState":
		resp = jsonrpc.NewResponse(req.ID, mapsClone(s.serverRef.state.NetworkState))
	case "renewDHCPLease":
		if lease, ok := s.serverRef.state.NetworkState["dhcp_lease"].(map[string]any); ok {
			next := mapsClone(lease)
			next["lease_expiry"] = time.Now().Add(8 * time.Hour)
			s.serverRef.state.NetworkState["dhcp_lease"] = next
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getPublicIPAddresses":
		resp = jsonrpc.NewResponse(req.ID, []map[string]any{
			{"ip": "198.51.100.10", "last_updated": time.Now().Add(-4 * time.Minute)},
			{"ip": "2001:db8::10", "last_updated": time.Now().Add(-4 * time.Minute)},
		})
	case "getTailscaleStatus":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{
			"installed":    true,
			"running":      true,
			"backendState": "Running",
			"controlURL":   "https://controlplane.tailscale.com",
			"self": map[string]any{
				"hostName":     "jetkvm-emulator",
				"dnsName":      "jetkvm-emulator.tailnet.example.",
				"tailscaleIPs": []string{"100.64.0.10", "fd7a:115c:a1e0::10"},
				"online":       true,
				"os":           "linux",
			},
		})
	case "setNetworkSettings":
		if settings, ok := params["settings"].(map[string]any); ok {
			next := mapsClone(s.serverRef.state.NetworkSettings)
			for key, value := range settings {
				next[key] = value
			}
			s.serverRef.state.NetworkSettings = next
			if hostname, ok := next["hostname"].(string); ok {
				s.serverRef.state.Hostname = hostname
				networkState := mapsClone(s.serverRef.state.NetworkState)
				networkState["hostname"] = hostname
				s.serverRef.state.NetworkState = networkState
			}
			if applyButDrop == req.Method {
				return nil
			}
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getUsbNetworkConfig":
		resp = jsonrpc.NewResponse(req.ID, mapsClone(s.serverRef.state.USBNetworkConfig))
	case "setUsbNetworkConfig":
		if cfg, ok := params["config"].(map[string]any); ok {
			next := mapsClone(s.serverRef.state.USBNetworkConfig)
			for key, value := range cfg {
				next[key] = value
			}
			s.serverRef.state.USBNetworkConfig = next
			if enabled, ok := next["enabled"].(bool); ok {
				devices := mapsClone(s.serverRef.state.USBDevices)
				devices["network"] = enabled
				s.serverRef.state.USBDevices = devices
			}
			if applyButDrop == req.Method {
				return nil
			}
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getKeyboardLedState":
		resp = jsonrpc.NewResponse(req.ID, map[string]byte{"mask": s.serverRef.state.KeyboardLEDMask})
	case "getKeyDownState", "getKeysDownState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"modifier": s.serverRef.state.KeyboardModifiers, "keys": s.serverRef.state.KeysDown})
	case "getStreamQualityFactor":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.StreamQualityFactor)
	case "getAutoUpdateState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.AutoUpdateEnabled)
	case "setAutoUpdateState":
		if enabled, ok := params["enabled"].(bool); ok {
			s.serverRef.state.AutoUpdateEnabled = enabled
			resp = jsonrpc.NewResponse(req.ID, enabled)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
		}
	case "setStreamQualityFactor":
		if factor, ok := params["factor"].(float64); ok {
			s.serverRef.state.StreamQualityFactor = factor
			if applyButDrop == req.Method {
				return nil
			}
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing factor", nil)
		}
	case "wheelReport":
		if wheelY, ok := params["wheelY"].(float64); ok {
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
	case "tryUpdate":
		resp = jsonrpc.NewResponse(req.ID, true)
	case "factoryReset":
		resp = jsonrpc.NewResponse(req.ID, true)
	case "forceDisconnect":
		resp = jsonrpc.NewResponse(req.ID, true)
		go func() {
			time.Sleep(50 * time.Millisecond)
			s.closeTransport()
		}()
	case "getKeyboardLayout":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.KeyboardLayout)
	case "setKeyboardLayout":
		if layout, ok := params["layout"].(string); ok && layout != "" {
			s.serverRef.state.KeyboardLayout = layout
			if applyButDrop == req.Method {
				return nil
			}
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getEDID":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.EDID)
	case "setEDID":
		if edid, ok := params["edid"].(string); ok {
			s.serverRef.state.EDID = edid
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing edid", nil)
		}
	case "getVideoCodecPreference":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.VideoCodec)
	case "setVideoCodecPreference":
		if codec, ok := params["codec"].(string); ok && codec != "" {
			s.serverRef.state.VideoCodec = codec
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing codec", nil)
		}
	case "getUsbEmulationState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.USBEmulation)
	case "setUsbEmulationState":
		if enabled, ok := params["enabled"].(bool); ok {
			s.serverRef.state.USBEmulation = enabled
			if applyButDrop == req.Method {
				return nil
			}
			resp = jsonrpc.NewResponse(req.ID, enabled)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
		}
	case "getCloudState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"connected": s.serverRef.state.CloudURL != "", "url": s.serverRef.state.CloudURL, "appUrl": s.serverRef.state.CloudAppURL})
	case "getTLSState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{
			"mode":        s.serverRef.state.TLSMode,
			"certificate": s.serverRef.state.TLSCertificate,
			"privateKey":  s.serverRef.state.TLSPrivateKey,
		})
	case "setTLSState":
		if state, ok := params["state"].(map[string]any); ok {
			if mode, ok := state["mode"].(string); ok && mode != "" {
				s.serverRef.state.TLSMode = mode
				if certificate, ok := state["certificate"].(string); ok {
					s.serverRef.state.TLSCertificate = certificate
				}
				if privateKey, ok := state["privateKey"].(string); ok {
					s.serverRef.state.TLSPrivateKey = privateKey
				}
				if applyButDrop == req.Method {
					return nil
				}
				resp = jsonrpc.NewResponse(req.ID, true)
			} else {
				resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing mode", nil)
			}
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing state", nil)
		}
	case "getUsbConfig":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.USBConfig)
	case "getUsbDevices":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.USBDevices)
	case "setUsbDevices":
		if devices, ok := params["devices"].(map[string]any); ok {
			s.serverRef.state.USBDevices = devices
			if applyButDrop == req.Method {
				return nil
			}
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing devices", nil)
		}
	case "getDisplayRotation":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"rotation": s.serverRef.state.DisplayRotation})
	case "setDisplayRotation":
		if rotationParams, ok := params["params"].(map[string]any); ok {
			if rotation, ok := rotationParams["rotation"].(string); ok && rotation != "" {
				s.serverRef.state.DisplayRotation = rotation
				if applyButDrop == req.Method {
					return nil
				}
				resp = jsonrpc.NewResponse(req.ID, true)
			} else {
				resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing rotation", nil)
			}
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing params", nil)
		}
	case "getBacklightSettings":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.BacklightSettings)
	case "setBacklightSettings":
		if settings, ok := params["params"].(map[string]any); ok {
			s.serverRef.state.BacklightSettings = mapsClone(settings)
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing params", nil)
		}
	case "getVideoSleepMode":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{
			"enabled":  s.serverRef.state.VideoSleepDuration >= 0,
			"duration": s.serverRef.state.VideoSleepDuration,
		})
	case "setVideoSleepMode":
		if duration, ok := params["duration"].(float64); ok {
			s.serverRef.state.VideoSleepDuration = int(duration)
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing duration", nil)
		}
	case "getMqttSettings":
		settings := mapsClone(s.serverRef.state.MQTTSettings)
		if password, ok := settings["password"].(string); ok && password != "" {
			settings["password"] = mqttPasswordMask
		}
		resp = jsonrpc.NewResponse(req.ID, settings)
	case "setMqttSettings":
		if settings, ok := params["settings"].(map[string]any); ok {
			next := mapsClone(settings)
			if password, ok := next["password"].(string); ok && password == mqttPasswordMask {
				if currentPassword, ok := s.serverRef.state.MQTTSettings["password"].(string); ok {
					next["password"] = currentPassword
				}
			}
			s.serverRef.state.MQTTSettings = next
			if enabled, ok := next["enabled"].(bool); ok {
				s.serverRef.state.MQTTConnected = enabled
			}
			s.serverRef.state.MQTTError = ""
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing settings", nil)
		}
	case "getMqttStatus":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"connected": s.serverRef.state.MQTTConnected, "error": s.serverRef.state.MQTTError})
	case "testMqttConnection":
		if settings, ok := params["settings"].(map[string]any); ok {
			broker, _ := settings["broker"].(string)
			if strings.TrimSpace(broker) == "" {
				resp = jsonrpc.NewResponse(req.ID, map[string]any{"success": false, "error": "broker address is required"})
			} else {
				resp = jsonrpc.NewResponse(req.ID, map[string]any{"success": true})
			}
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing settings", nil)
		}
	case "getKeyboardMacros":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.KeyboardMacros)
	case "setKeyboardMacros":
		if wrapper, ok := params["params"].(map[string]any); ok {
			if macros, ok := wrapper["macros"].([]any); ok {
				out := make([]map[string]any, 0, len(macros))
				for _, item := range macros {
					macro, ok := item.(map[string]any)
					if !ok {
						continue
					}
					out = append(out, mapsClone(macro))
				}
				s.serverRef.state.KeyboardMacros = out
				resp = jsonrpc.NewResponse(req.ID, true)
			} else {
				resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing macros", nil)
			}
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing params", nil)
		}
	case "getDevModeState":
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"enabled": s.serverRef.state.DeveloperMode})
	case "setDevModeState":
		if enabled, ok := params["enabled"].(bool); ok {
			s.serverRef.state.DeveloperMode = enabled
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
		}
	case "getDevChannelState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.DevChannel)
	case "setDevChannelState":
		if enabled, ok := params["enabled"].(bool); ok {
			s.serverRef.state.DevChannel = enabled
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
		}
	case "getLocalLoopbackOnly":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.LoopbackOnly)
	case "setLocalLoopbackOnly":
		if enabled, ok := params["enabled"].(bool); ok {
			s.serverRef.state.LoopbackOnly = enabled
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
		}
	case "getSSHKeyState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.SSHKey)
	case "setSSHKeyState":
		if sshKey, ok := params["sshKey"].(string); ok {
			s.serverRef.state.SSHKey = sshKey
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing sshKey", nil)
		}
	case "getJigglerState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.JigglerEnabled)
	case "setJigglerState":
		if enabled, ok := params["enabled"].(bool); ok {
			s.serverRef.state.JigglerEnabled = enabled
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
		}
	case "getJigglerConfig":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.JigglerConfig)
	case "setJigglerConfig":
		if cfg, ok := params["jigglerConfig"].(map[string]any); ok {
			s.serverRef.state.JigglerConfig = cfg
			resp = jsonrpc.NewResponse(req.ID, true)
		} else {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing jigglerConfig", nil)
		}
	case "getVirtualMediaState":
		s.serverRef.mu.Lock()
		state := s.serverRef.media
		s.serverRef.mu.Unlock()
		resp = jsonrpc.NewResponse(req.ID, state)
	case "unmountImage":
		s.serverRef.mu.Lock()
		s.serverRef.media = nil
		s.serverRef.mu.Unlock()
		resp = jsonrpc.NewResponse(req.ID, true)
	case "mountWithHTTP":
		rawURL, _ := params["url"].(string)
		rawMode, _ := params["mode"].(string)
		if strings.TrimSpace(rawURL) == "" || strings.TrimSpace(rawMode) == "" {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing mount params", nil)
			break
		}
		s.serverRef.mu.Lock()
		if s.serverRef.media != nil {
			s.serverRef.mu.Unlock()
			resp = jsonrpc.NewErrorResponse(req.ID, -32000, "another virtual media is already mounted", nil)
			break
		}
		s.serverRef.media = &virtualmedia.State{
			Source: virtualmedia.SourceHTTP,
			Mode:   virtualmedia.Mode(rawMode),
			URL:    rawURL,
			Size:   2 * 1024 * 1024,
		}
		s.serverRef.mu.Unlock()
		resp = jsonrpc.NewResponse(req.ID, true)
	case "mountWithStorage":
		filename, _ := params["filename"].(string)
		rawMode, _ := params["mode"].(string)
		filename = filepath.Base(strings.TrimSpace(filename))
		if filename == "" || strings.TrimSpace(rawMode) == "" {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing mount params", nil)
			break
		}
		s.serverRef.mu.Lock()
		if s.serverRef.media != nil {
			s.serverRef.mu.Unlock()
			resp = jsonrpc.NewErrorResponse(req.ID, -32000, "another virtual media is already mounted", nil)
			break
		}
		file, ok := s.serverRef.storage[filename]
		if !ok || strings.HasSuffix(filename, ".incomplete") {
			s.serverRef.mu.Unlock()
			resp = jsonrpc.NewErrorResponse(req.ID, -32000, "storage file not found", nil)
			break
		}
		s.serverRef.media = &virtualmedia.State{
			Source:   virtualmedia.SourceStorage,
			Mode:     virtualmedia.Mode(rawMode),
			Filename: filename,
			Size:     int64(len(file.data)),
		}
		s.serverRef.mu.Unlock()
		resp = jsonrpc.NewResponse(req.ID, true)
	case "listStorageFiles":
		s.serverRef.mu.Lock()
		files := make([]virtualmedia.StorageFile, 0, len(s.serverRef.storage))
		for name, file := range s.serverRef.storage {
			files = append(files, virtualmedia.StorageFile{
				Filename:  name,
				Size:      int64(len(file.data)),
				CreatedAt: file.createdAt,
			})
		}
		s.serverRef.mu.Unlock()
		sort.Slice(files, func(i, j int) bool {
			return files[i].CreatedAt.After(files[j].CreatedAt)
		})
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"files": files})
	case "getStorageSpace":
		s.serverRef.mu.Lock()
		var used int64
		for _, file := range s.serverRef.storage {
			used += int64(len(file.data))
		}
		s.serverRef.mu.Unlock()
		total := int64(2 * 1024 * 1024 * 1024)
		resp = jsonrpc.NewResponse(req.ID, virtualmedia.StorageSpace{
			BytesUsed: used,
			BytesFree: total - used,
		})
	case "deleteStorageFile":
		filename, _ := params["filename"].(string)
		filename = filepath.Base(strings.TrimSpace(filename))
		if filename == "" {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing filename", nil)
			break
		}
		s.serverRef.mu.Lock()
		if _, ok := s.serverRef.storage[filename]; !ok {
			s.serverRef.mu.Unlock()
			resp = jsonrpc.NewErrorResponse(req.ID, -32000, "file does not exist", nil)
			break
		}
		delete(s.serverRef.storage, filename)
		s.serverRef.mu.Unlock()
		resp = jsonrpc.NewResponse(req.ID, true)
	case "startStorageFileUpload":
		filename, _ := params["filename"].(string)
		filename = filepath.Base(strings.TrimSpace(filename))
		size, _ := params["size"].(float64)
		if filename == "" || size <= 0 {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "invalid upload params", nil)
			break
		}
		s.serverRef.mu.Lock()
		if _, ok := s.serverRef.storage[filename]; ok {
			s.serverRef.mu.Unlock()
			resp = jsonrpc.NewErrorResponse(req.ID, -32000, "file already exists", nil)
			break
		}
		incompleteName := filename + ".incomplete"
		file := s.serverRef.storage[incompleteName]
		uploadID := fmt.Sprintf("upload_%d", time.Now().UnixNano())
		s.serverRef.uploads[uploadID] = &pendingUpload{
			filename: filename,
			size:     int64(size),
		}
		s.serverRef.mu.Unlock()
		resp = jsonrpc.NewResponse(req.ID, virtualmedia.UploadStart{
			AlreadyUploadedBytes: int64(len(file.data)),
			DataChannel:          uploadID,
		})
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
	case hidrpc.KeypressKeepAlive:
		return nil
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
