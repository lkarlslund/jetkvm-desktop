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

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
	"github.com/lkarlslund/jetkvm-desktop/pkg/logging"
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
	DeviceID             string
	ActiveExtension      string
	ATXState             map[string]any
	DCPowerState         client.DCPowerState
	SerialSettings       client.SerialSettings
	SerialCommandHistory []string
	VideoState           string
	StreamQualityFactor  float64
	VideoCodec           string
	EDID                 string
	AutoUpdateEnabled    bool
	KeyboardLEDMask      byte
	KeyboardModifiers    byte
	KeysDown             []byte
	Hostname             string
	CloudURL             string
	CloudAppURL          string
	KeyboardLayout       string
	DeveloperMode        bool
	DevChannel           bool
	LoopbackOnly         bool
	SSHKey               string
	JigglerEnabled       bool
	JigglerConfig        client.JigglerConfig
	TLSMode              string
	TLSCertificate       string
	TLSPrivateKey        string
	DisplayRotation      string
	BacklightSettings    client.BacklightSettings
	VideoSleepDuration   int
	USBEmulation         bool
	USBConfig            client.USBConfig
	USBDevices           client.USBDevices
	USBNetworkConfig     client.USBNetworkConfig
	NetworkSettings      client.NetworkSettings
	NetworkState         client.NetworkState
	MQTTSettings         client.MQTTSettings
	MQTTConnected        bool
	MQTTError            string
	KeyboardMacros       []client.KeyboardMacro
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
			DeviceID:        "emu-jetkvm-001",
			ActiveExtension: "",
			ATXState:        map[string]any{"power": false, "hdd": false},
			DCPowerState: client.DCPowerState{
				IsOn:         false,
				Voltage:      12.0,
				Current:      0.0,
				Power:        0.0,
				RestoreState: 2,
			},
			SerialSettings: client.SerialSettings{
				BaudRate: 9600,
				DataBits: 8,
				Parity:   "none",
				StopBits: "1",
				Terminator: client.Terminator{
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
				Buttons: []client.QuickButton{
					{ID: "emu-btn-1", Label: "Help", Command: "help", Terminator: client.Terminator{Label: "LF (\\n)", Value: "\n"}, Sort: 0},
					{ID: "emu-btn-2", Label: "Status", Command: "status", Terminator: client.Terminator{Label: "CRLF (\\r\\n)", Value: "\r\n"}, Sort: 1},
				},
			},
			SerialCommandHistory: []string{"help", "status"},
			VideoState:           "ready",
			StreamQualityFactor:  0.75,
			VideoCodec:           "auto",
			EDID:                 "",
			AutoUpdateEnabled:    true,
			KeyboardLEDMask:      0,
			KeyboardModifiers:    0,
			KeysDown:             []byte{0, 0, 0, 0, 0, 0},
			Hostname:             "jetkvm-emulator",
			KeyboardLayout:       "en_US",
			DeveloperMode:        false,
			DevChannel:           false,
			LoopbackOnly:         false,
			SSHKey:               "",
			JigglerEnabled:       false,
			JigglerConfig: client.JigglerConfig{
				InactivityLimitSeconds: 60,
				JitterPercentage:       25,
				ScheduleCronTab:        "0 * * * * *",
			},
			TLSMode:         "disabled",
			TLSCertificate:  "",
			TLSPrivateKey:   "",
			DisplayRotation: "270",
			BacklightSettings: client.BacklightSettings{
				MaxBrightness: 64,
				DimAfter:      300,
				OffAfter:      600,
			},
			VideoSleepDuration: -1,
			USBEmulation:       true,
			USBConfig: client.USBConfig{
				VendorID:     "0xCafe",
				ProductID:    "0x4000",
				SerialNumber: "JETKVM-DESKTOP",
				Manufacturer: "JetKVM",
				Product:      "JetKVM Default",
			},
			USBDevices: client.USBDevices{
				Keyboard:      true,
				AbsoluteMouse: true,
				RelativeMouse: true,
				MassStorage:   true,
				SerialConsole: false,
				Network:       false,
			},
			USBNetworkConfig: client.USBNetworkConfig{
				Enabled:         false,
				HostPreset:      "auto",
				Protocol:        "ncm",
				SharingMode:     "nat",
				UplinkMode:      "auto",
				UplinkInterface: "",
				IPv4SubnetCIDR:  "10.55.0.0/24",
				DHCPEnabled:     true,
				DNSProxyEnabled: true,
			},
			NetworkSettings: client.NetworkSettings{
				DHCPClient:   "dhcpcd",
				Hostname:     "jetkvm-emulator",
				Domain:       "lab.example",
				HTTPProxy:    "",
				IPv4Mode:     "dhcp",
				MDNSMode:     "auto",
				IPv6Mode:     "slaac",
				TimeSyncMode: "ntp_only",
				IPv4Static: &client.IPv4StaticConfig{
					Address: "192.168.1.50",
					Netmask: "255.255.255.0",
					Gateway: "192.168.1.1",
					DNS:     []string{"1.1.1.1", "8.8.8.8"},
				},
				IPv6Static: &client.IPv6StaticConfig{
					Prefix:  "2001:db8::50/64",
					Gateway: "2001:db8::1",
					DNS:     []string{"2606:4700:4700::1111"},
				},
				TimeSyncNTPServers: []string{"0.pool.ntp.org", "1.pool.ntp.org"},
				TimeSyncHTTPUrls:   []string{"https://time.cloudflare.com"},
			},
			NetworkState: client.NetworkState{
				InterfaceName: "eth0",
				MACAddress:    "02:42:ac:11:00:02",
				IPv4:          "192.168.1.50",
				IPv4Addresses: []string{"192.168.1.50/24"},
				IPv6:          "2001:db8::50",
				IPv6Addresses: []client.IPv6Address{{Address: "2001:db8::50", Prefix: "64"}},
				IPv6LinkLocal: "fe80::1",
				IPv6Gateway:   "2001:db8::1",
				Hostname:      "jetkvm-emulator",
				DHCPLease: &client.DHCPLease{
					IP:          "192.168.1.50",
					Netmask:     "255.255.255.0",
					DNSServers:  []string{"1.1.1.1", "8.8.8.8"},
					Domain:      "lab.example",
					Routers:     []string{"192.168.1.1"},
					ServerID:    "192.168.1.1",
					LeaseExpiry: time.Now().Add(8 * time.Hour),
					DHCPClient:  "dhcpcd",
				},
			},
			MQTTSettings: client.MQTTSettings{
				Enabled:           false,
				Broker:            "mqtt.local",
				Port:              1883,
				Username:          "",
				Password:          "",
				BaseTopic:         "jetkvm",
				UseTLS:            false,
				TLSInsecure:       false,
				EnableHADiscovery: false,
				EnableActions:     false,
				DebounceMs:        0,
			},
			KeyboardMacros: []client.KeyboardMacro{},
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
	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

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
	s.mu.Lock()
	defer s.mu.Unlock()
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

func (s *Server) SetActiveExtension(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.ActiveExtension = strings.TrimSpace(name)
}

func (s *Server) SetATXState(power, hdd bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.ATXState = map[string]any{
		"power": power,
		"hdd":   hdd,
	}
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
				if s.state.ActiveExtension == "atx-power" {
					_ = sess.sendEvent("atxState", mapsClone(s.state.ATXState))
				}
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
	if s.pc != nil {
		_ = s.pc.Close()
	}
}

func decodeParams[T any](raw any) (T, error) {
	var out T
	if raw == nil {
		return out, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func cloneStrings(src []string) []string {
	if src == nil {
		return nil
	}
	return append([]string(nil), src...)
}

func cloneKeyboardMacros(src []client.KeyboardMacro) []client.KeyboardMacro {
	if src == nil {
		return nil
	}
	out := make([]client.KeyboardMacro, 0, len(src))
	for _, macro := range src {
		steps := make([]client.KeyboardMacroStep, 0, len(macro.Steps))
		for _, step := range macro.Steps {
			steps = append(steps, client.KeyboardMacroStep{
				Keys:      cloneStrings(step.Keys),
				Modifiers: cloneStrings(step.Modifiers),
				Delay:     step.Delay,
			})
		}
		out = append(out, client.KeyboardMacro{
			ID:        macro.ID,
			Name:      macro.Name,
			Steps:     steps,
			SortOrder: macro.SortOrder,
		})
	}
	return out
}

func cloneSerialSettings(src client.SerialSettings) client.SerialSettings {
	dst := src
	dst.Buttons = append([]client.QuickButton(nil), src.Buttons...)
	return dst
}

func cloneNetworkSettings(src client.NetworkSettings) client.NetworkSettings {
	dst := src
	if src.IPv4Static != nil {
		ipv4 := *src.IPv4Static
		ipv4.DNS = cloneStrings(src.IPv4Static.DNS)
		dst.IPv4Static = &ipv4
	}
	if src.IPv6Static != nil {
		ipv6 := *src.IPv6Static
		ipv6.DNS = cloneStrings(src.IPv6Static.DNS)
		dst.IPv6Static = &ipv6
	}
	dst.LLDPTxTLVs = cloneStrings(src.LLDPTxTLVs)
	dst.TimeSyncOrdering = cloneStrings(src.TimeSyncOrdering)
	dst.TimeSyncNTPServers = cloneStrings(src.TimeSyncNTPServers)
	dst.TimeSyncHTTPUrls = cloneStrings(src.TimeSyncHTTPUrls)
	return dst
}

func cloneNetworkState(src client.NetworkState) client.NetworkState {
	dst := src
	dst.IPv4Addresses = cloneStrings(src.IPv4Addresses)
	if src.DHCPLease != nil {
		lease := *src.DHCPLease
		lease.DNSServers = cloneStrings(src.DHCPLease.DNSServers)
		lease.NTPServers = cloneStrings(src.DHCPLease.NTPServers)
		lease.Routers = cloneStrings(src.DHCPLease.Routers)
		dst.DHCPLease = &lease
	}
	if src.IPv6Addresses != nil {
		dst.IPv6Addresses = append([]client.IPv6Address(nil), src.IPv6Addresses...)
	}
	return dst
}

func keysDownState(modifier byte, keys []byte) client.KeysDownState {
	return client.KeysDownState{
		Modifier: modifier,
		Keys:     append([]byte(nil), keys...),
	}
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
		resp = jsonrpc.NewResponse(req.ID, cloneNetworkSettings(s.serverRef.state.NetworkSettings))
	case "getNetworkState":
		resp = jsonrpc.NewResponse(req.ID, cloneNetworkState(s.serverRef.state.NetworkState))
	case "renewDHCPLease":
		if s.serverRef.state.NetworkState.DHCPLease != nil {
			lease := *s.serverRef.state.NetworkState.DHCPLease
			lease.LeaseExpiry = time.Now().Add(8 * time.Hour)
			s.serverRef.state.NetworkState.DHCPLease = &lease
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getPublicIPAddresses":
		resp = jsonrpc.NewResponse(req.ID, []client.PublicIP{
			{IPAddress: "198.51.100.10", LastUpdated: time.Now().Add(-4 * time.Minute)},
			{IPAddress: "2001:db8::10", LastUpdated: time.Now().Add(-4 * time.Minute)},
		})
	case "getTailscaleStatus":
		resp = jsonrpc.NewResponse(req.ID, client.TailscaleStatus{
			Installed:    true,
			Running:      true,
			BackendState: "Running",
			ControlURL:   "https://controlplane.tailscale.com",
			Self: &client.TailscalePeer{
				HostName:     "jetkvm-emulator",
				DNSName:      "jetkvm-emulator.tailnet.example.",
				TailscaleIPs: []string{"100.64.0.10", "fd7a:115c:a1e0::10"},
				Online:       true,
				OS:           "linux",
			},
		})
	case "setNetworkSettings":
		params, err := decodeParams[client.NetworkSettingsRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "invalid settings", nil)
			break
		}
		s.serverRef.state.NetworkSettings = cloneNetworkSettings(params.Settings)
		if hostname := strings.TrimSpace(params.Settings.Hostname); hostname != "" {
			s.serverRef.state.Hostname = hostname
			networkState := cloneNetworkState(s.serverRef.state.NetworkState)
			networkState.Hostname = hostname
			s.serverRef.state.NetworkState = networkState
		}
		if applyButDrop == req.Method {
			return nil
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getUsbNetworkConfig":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.USBNetworkConfig)
	case "setUsbNetworkConfig":
		params, err := decodeParams[client.USBNetworkConfigRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "invalid config", nil)
			break
		}
		s.serverRef.state.USBNetworkConfig = params.Config
		s.serverRef.state.USBDevices.Network = params.Config.Enabled
		if applyButDrop == req.Method {
			return nil
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getKeyboardLedState":
		resp = jsonrpc.NewResponse(req.ID, map[string]byte{"mask": s.serverRef.state.KeyboardLEDMask})
	case "getKeyDownState", "getKeysDownState":
		resp = jsonrpc.NewResponse(req.ID, keysDownState(s.serverRef.state.KeyboardModifiers, s.serverRef.state.KeysDown))
	case "getStreamQualityFactor":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.StreamQualityFactor)
	case "getAutoUpdateState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.AutoUpdateEnabled)
	case "setAutoUpdateState":
		params, err := decodeParams[client.EnabledStateRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "invalid enabled", nil)
			break
		}
		s.serverRef.state.AutoUpdateEnabled = params.Enabled
		resp = jsonrpc.NewResponse(req.ID, params.Enabled)
	case "setStreamQualityFactor":
		params, err := decodeParams[client.SetQualityRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "invalid factor", nil)
			break
		}
		s.serverRef.state.StreamQualityFactor = params.Factor
		if applyButDrop == req.Method {
			return nil
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "wheelReport":
		params, err := decodeParams[client.WheelReportRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "invalid wheel report", nil)
			break
		}
		s.serverRef.appendInput("rpc", "rpc.wheelReport", fmt.Sprintf("wheelY=%d wheelX=%d", int8(params.WheelY), int8(params.WheelX)))
		resp = jsonrpc.NewResponse(req.ID, true)
	case "reboot":
		if _, err := decodeParams[client.RebootRequest](req.Params); err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "invalid reboot params", nil)
			break
		}
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
		params, err := decodeParams[struct {
			Layout string `json:"layout"`
		}](req.Params)
		if err != nil || strings.TrimSpace(params.Layout) == "" {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing layout", nil)
			break
		}
		s.serverRef.state.KeyboardLayout = params.Layout
		if applyButDrop == req.Method {
			return nil
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getEDID":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.EDID)
	case "setEDID":
		params, err := decodeParams[client.SetEDIDRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing edid", nil)
			break
		}
		s.serverRef.state.EDID = params.EDID
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getVideoCodecPreference":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.VideoCodec)
	case "setVideoCodecPreference":
		params, err := decodeParams[client.SetCodecPreferenceRequest](req.Params)
		if err != nil || strings.TrimSpace(params.Codec) == "" {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing codec", nil)
			break
		}
		s.serverRef.state.VideoCodec = params.Codec
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getActiveExtension":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.ActiveExtension)
	case "setActiveExtension":
		extensionID, ok := params["extensionId"].(string)
		if !ok {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing extensionId", nil)
			break
		}
		s.serverRef.state.ActiveExtension = extensionID
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getDCPowerState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.DCPowerState)
	case "setDCPowerState":
		params, err := decodeParams[client.EnabledStateRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
			break
		}
		s.serverRef.state.DCPowerState.IsOn = params.Enabled
		if params.Enabled {
			s.serverRef.state.DCPowerState.Current = 1.2
			s.serverRef.state.DCPowerState.Power = 14.4
		} else {
			s.serverRef.state.DCPowerState.Current = 0.0
			s.serverRef.state.DCPowerState.Power = 0.0
		}
		s.serverRef.appendInput("rpc", "rpc.setDCPowerState", fmt.Sprintf("enabled=%t", params.Enabled))
		resp = jsonrpc.NewResponse(req.ID, true)
	case "setDCRestoreState":
		params, err := decodeParams[struct {
			State int `json:"state"`
		}](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing state", nil)
			break
		}
		s.serverRef.state.DCPowerState.RestoreState = params.State
		s.serverRef.appendInput("rpc", "rpc.setDCRestoreState", fmt.Sprintf("state=%d", params.State))
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getATXState":
		resp = jsonrpc.NewResponse(req.ID, mapsClone(s.serverRef.state.ATXState))
	case "setATXPowerAction":
		action, ok := params["action"].(string)
		if !ok || action == "" {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing action", nil)
			break
		}
		switch action {
		case "power-short", "power-long", "reset":
			s.serverRef.appendInput("rpc", "rpc.setATXPowerAction", action)
			resp = jsonrpc.NewResponse(req.ID, true)
		default:
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "invalid action", nil)
		}
	case "getSerialSettings":
		resp = jsonrpc.NewResponse(req.ID, cloneSerialSettings(s.serverRef.state.SerialSettings))
	case "setSerialSettings":
		params, err := decodeParams[struct {
			Settings client.SerialSettings `json:"settings"`
		}](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing settings", nil)
			break
		}
		s.serverRef.state.SerialSettings = cloneSerialSettings(params.Settings)
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getSerialCommandHistory":
		resp = jsonrpc.NewResponse(req.ID, append([]string(nil), s.serverRef.state.SerialCommandHistory...))
	case "setSerialCommandHistory":
		rawHistory, ok := params["commandHistory"].([]any)
		if !ok {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing commandHistory", nil)
			break
		}
		history := make([]string, 0, len(rawHistory))
		for _, item := range rawHistory {
			text, ok := item.(string)
			if !ok {
				continue
			}
			history = append(history, text)
		}
		s.serverRef.state.SerialCommandHistory = history
		resp = jsonrpc.NewResponse(req.ID, true)
	case "sendCustomCommand":
		command, ok := params["command"].(string)
		if !ok {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing command", nil)
			break
		}
		s.serverRef.appendInput("rpc", "rpc.sendCustomCommand", command)
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getUsbEmulationState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.USBEmulation)
	case "setUsbEmulationState":
		params, err := decodeParams[struct {
			Enabled bool `json:"enabled"`
		}](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
			break
		}
		s.serverRef.state.USBEmulation = params.Enabled
		if applyButDrop == req.Method {
			return nil
		}
		resp = jsonrpc.NewResponse(req.ID, params.Enabled)
	case "getCloudState":
		resp = jsonrpc.NewResponse(req.ID, client.CloudState{
			Connected: s.serverRef.state.CloudURL != "",
			URL:       s.serverRef.state.CloudURL,
			AppURL:    s.serverRef.state.CloudAppURL,
		})
	case "getTLSState":
		resp = jsonrpc.NewResponse(req.ID, client.TLSState{
			Mode:        s.serverRef.state.TLSMode,
			Certificate: s.serverRef.state.TLSCertificate,
			PrivateKey:  s.serverRef.state.TLSPrivateKey,
		})
	case "setTLSState":
		params, err := decodeParams[client.SetTLSStateRequest](req.Params)
		if err != nil || strings.TrimSpace(params.State.Mode) == "" {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing mode", nil)
			break
		}
		s.serverRef.state.TLSMode = params.State.Mode
		s.serverRef.state.TLSCertificate = params.State.Certificate
		s.serverRef.state.TLSPrivateKey = params.State.PrivateKey
		if applyButDrop == req.Method {
			return nil
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getUsbConfig":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.USBConfig)
	case "getUsbDevices":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.USBDevices)
	case "setUsbDevices":
		params, err := decodeParams[client.USBDevicesRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing devices", nil)
			break
		}
		s.serverRef.state.USBDevices = params.Devices
		if applyButDrop == req.Method {
			return nil
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getDisplayRotation":
		resp = jsonrpc.NewResponse(req.ID, client.DisplayRotationState{Rotation: s.serverRef.state.DisplayRotation})
	case "setDisplayRotation":
		params, err := decodeParams[client.SetDisplayRotationRequest](req.Params)
		if err != nil || strings.TrimSpace(params.Params.Rotation) == "" {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing rotation", nil)
			break
		}
		s.serverRef.state.DisplayRotation = params.Params.Rotation
		if applyButDrop == req.Method {
			return nil
		}
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getBacklightSettings":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.BacklightSettings)
	case "setBacklightSettings":
		params, err := decodeParams[client.SetBacklightSettingsRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing params", nil)
			break
		}
		s.serverRef.state.BacklightSettings = params.Params
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getVideoSleepMode":
		resp = jsonrpc.NewResponse(req.ID, client.VideoSleepMode{
			Enabled:  s.serverRef.state.VideoSleepDuration >= 0,
			Duration: s.serverRef.state.VideoSleepDuration,
		})
	case "setVideoSleepMode":
		params, err := decodeParams[client.SetVideoSleepModeRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing duration", nil)
			break
		}
		s.serverRef.state.VideoSleepDuration = params.Duration
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getMqttSettings":
		settings := s.serverRef.state.MQTTSettings
		if settings.Password != "" {
			settings.Password = mqttPasswordMask
		}
		resp = jsonrpc.NewResponse(req.ID, settings)
	case "setMqttSettings":
		params, err := decodeParams[client.MQTTSettingsRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing settings", nil)
			break
		}
		next := params.Settings
		if next.Password == mqttPasswordMask {
			next.Password = s.serverRef.state.MQTTSettings.Password
		}
		s.serverRef.state.MQTTSettings = next
		s.serverRef.state.MQTTConnected = next.Enabled
		s.serverRef.state.MQTTError = ""
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getMqttStatus":
		resp = jsonrpc.NewResponse(req.ID, client.MQTTStatus{Connected: s.serverRef.state.MQTTConnected, Error: s.serverRef.state.MQTTError})
	case "testMqttConnection":
		params, err := decodeParams[client.MQTTSettingsRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing settings", nil)
			break
		}
		if strings.TrimSpace(params.Settings.Broker) == "" {
			resp = jsonrpc.NewResponse(req.ID, client.MQTTTestResult{Success: false, Error: "broker address is required"})
		} else {
			resp = jsonrpc.NewResponse(req.ID, client.MQTTTestResult{Success: true})
		}
	case "getKeyboardMacros":
		resp = jsonrpc.NewResponse(req.ID, cloneKeyboardMacros(s.serverRef.state.KeyboardMacros))
	case "setKeyboardMacros":
		params, err := decodeParams[client.KeyboardMacrosRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing macros", nil)
			break
		}
		s.serverRef.state.KeyboardMacros = cloneKeyboardMacros(params.Params.Macros)
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getDevModeState":
		resp = jsonrpc.NewResponse(req.ID, client.DeveloperModeState{Enabled: s.serverRef.state.DeveloperMode})
	case "setDevModeState":
		params, err := decodeParams[client.EnabledStateRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
			break
		}
		s.serverRef.state.DeveloperMode = params.Enabled
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getDevChannelState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.DevChannel)
	case "setDevChannelState":
		params, err := decodeParams[client.EnabledStateRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
			break
		}
		s.serverRef.state.DevChannel = params.Enabled
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getLocalLoopbackOnly":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.LoopbackOnly)
	case "setLocalLoopbackOnly":
		params, err := decodeParams[client.EnabledStateRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
			break
		}
		s.serverRef.state.LoopbackOnly = params.Enabled
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getSSHKeyState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.SSHKey)
	case "setSSHKeyState":
		params, err := decodeParams[client.SetSSHKeyStateRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing sshKey", nil)
			break
		}
		s.serverRef.state.SSHKey = params.SSHKey
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getJigglerState":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.JigglerEnabled)
	case "setJigglerState":
		params, err := decodeParams[client.EnabledStateRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing enabled", nil)
			break
		}
		s.serverRef.state.JigglerEnabled = params.Enabled
		resp = jsonrpc.NewResponse(req.ID, true)
	case "getJigglerConfig":
		resp = jsonrpc.NewResponse(req.ID, s.serverRef.state.JigglerConfig)
	case "setJigglerConfig":
		params, err := decodeParams[client.JigglerConfigRequest](req.Params)
		if err != nil {
			resp = jsonrpc.NewErrorResponse(req.ID, -32602, "missing jigglerConfig", nil)
			break
		}
		s.serverRef.state.JigglerConfig = params.JigglerConfig
		resp = jsonrpc.NewResponse(req.ID, true)
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
		params, err := decodeParams[client.MountWithHTTPRequest](req.Params)
		if err != nil || strings.TrimSpace(params.URL) == "" || params.Mode == "" {
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
			Mode:   params.Mode,
			URL:    params.URL,
			Size:   2 * 1024 * 1024,
		}
		s.serverRef.mu.Unlock()
		resp = jsonrpc.NewResponse(req.ID, true)
	case "mountWithStorage":
		params, err := decodeParams[client.MountWithStorageRequest](req.Params)
		filename := filepath.Base(strings.TrimSpace(params.Filename))
		if err != nil || filename == "" || params.Mode == "" {
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
			Mode:     params.Mode,
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
		resp = jsonrpc.NewResponse(req.ID, client.StorageFilesResponse{Files: files})
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
		params, err := decodeParams[client.DeleteStorageFileRequest](req.Params)
		filename := filepath.Base(strings.TrimSpace(params.Filename))
		if err != nil || filename == "" {
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
		params, err := decodeParams[client.StorageUploadRequest](req.Params)
		filename := filepath.Base(strings.TrimSpace(params.Filename))
		if err != nil || filename == "" || params.Size <= 0 {
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
			size:     params.Size,
		}
		s.serverRef.mu.Unlock()
		resp = jsonrpc.NewResponse(req.ID, virtualmedia.UploadStart{
			AlreadyUploadedBytes: int64(len(file.data)),
			DataChannel:          uploadID,
		})
	case "getLocalVersion":
		resp = jsonrpc.NewResponse(req.ID, client.LocalVersion{AppVersion: "emulator-dev", SystemVersion: "emulator-dev"})
	case "getUpdateStatus":
		resp = jsonrpc.NewResponse(req.ID, client.UpdateStatus{
			Local:                 client.LocalVersion{AppVersion: "emulator-dev", SystemVersion: "emulator-dev"},
			Remote:                client.LocalVersion{AppVersion: "emulator-dev", SystemVersion: "emulator-dev"},
			SystemUpdateAvailable: false,
			AppUpdateAvailable:    false,
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
		s.serverRef.mu.Lock()
		state := keysDownState(s.serverRef.state.KeyboardModifiers, s.serverRef.state.KeysDown)
		s.serverRef.mu.Unlock()
		return s.sendEvent("keysDownState", state)
	case hidrpc.KeypressKeepAlive:
		return nil
	case hidrpc.Pointer:
		if v.Buttons != 0 {
			log := logging.Subsystem("emulator")
			log.Trace().Str("channel", channel).Int32("x", v.X).Int32("y", v.Y).Uint8("buttons", v.Buttons).Msg("received absolute pointer")
		}
		return nil
	case hidrpc.Mouse:
		if v.Buttons != 0 {
			log := logging.Subsystem("emulator")
			log.Trace().Str("channel", channel).Int8("dx", v.DX).Int8("dy", v.DY).Uint8("buttons", v.Buttons).Msg("received relative mouse")
		}
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
			state := keysDownState(s.serverRef.state.KeyboardModifiers, s.serverRef.state.KeysDown)
			s.serverRef.mu.Unlock()
			_ = s.sendEvent("keysDownState", state)
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
