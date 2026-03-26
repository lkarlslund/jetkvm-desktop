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
		state:  DeviceState{DeviceID: "emu-jetkvm-001", VideoState: "ok", StreamQualityFactor: 0.75, KeyboardLEDMask: 0, KeysDown: []byte{0, 0, 0, 0, 0, 0}},
		inputs: make([]InputRecord, 0, 32),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/login-local", s.handleLogin)
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
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
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
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) authorized(r *http.Request) bool {
	if s.cfg.AuthMode == AuthModeNoPassword {
		return true
	}
	cookie, err := r.Cookie("authToken")
	return err == nil && cookie.Value == s.token
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
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
		resp = jsonrpc.NewResponse(req.ID, map[string]any{"hostname": "jetkvm-emulator", "ip": "127.0.0.1"})
	case "getKeyboardLedState":
		resp = jsonrpc.NewResponse(req.ID, map[string]byte{"mask": s.serverRef.state.KeyboardLEDMask})
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
