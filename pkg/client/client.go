package client

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/logging"
	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/auth"
	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/hidrpc"
	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/jsonrpc"
	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/signaling"
	"github.com/lkarlslund/jetkvm-desktop/pkg/video"
)

type Config struct {
	BaseURL    string
	Password   string
	RPCTimeout time.Duration
}

//go:generate go tool github.com/dmarkham/enumer -type=SignalingMode -linecomment -text -output client_enums.go

type SignalingMode uint8

const (
	SignalingModeUnknown    SignalingMode = iota // unknown
	SignalingModeLegacyHTTP                      // legacy_http
	SignalingModeWebSocket                       // websocket
)

type LifecycleEvent struct {
	Type       string
	Connection webrtc.PeerConnectionState
	Err        string
	Signaling  SignalingMode
	PasteState bool
}

type SerialConsoleEvent struct {
	Text string
}

type StatsSnapshot struct {
	SignalingMode   SignalingMode
	RTCState        webrtc.PeerConnectionState
	HIDReady        bool
	VideoReady      bool
	FrameWidth      int
	FrameHeight     int
	BytesReceived   uint64
	BitrateKbps     float64
	PacketsLost     int32
	JitterMs        float64
	FramesDecoded   uint32
	FramesRendered  uint32
	FramesPerSecond float64
	RoundTripMs     float64
	LastError       string
	TransportDebug  string
}

type Client struct {
	cfg        Config
	authClient *auth.Client
	pc         *webrtc.PeerConnection

	rpcDC          *webrtc.DataChannel
	hidDC          *webrtc.DataChannel
	hidUnreliable  *webrtc.DataChannel
	hidNonOrdered  *webrtc.DataChannel
	serialDC       *webrtc.DataChannel
	eventCh        chan jsonrpc.Event
	serialCh       chan SerialConsoleEvent
	pending        sync.Map
	requestCounter atomic.Uint64
	hidReady       chan struct{}
	hidReadyOnce   sync.Once
	videoStream    *video.Stream
	lifecycleCh    chan LifecycleEvent
	closeCh        chan struct{}
	signalConn     *websocket.Conn
	signalMu       sync.Mutex
	signalMode     SignalingMode
	statsMu        sync.Mutex
	videoMu        sync.RWMutex
	closeMu        sync.Mutex
	statsHistory   []statsSample
	disconnectOnce sync.Once
	lastError      atomic.Value
	transportDebug atomic.Value
}

type statsSample struct {
	at            time.Time
	bytesReceived uint64
	framesDecoded uint32
}

func computeSmoothedRates(history []statsSample) (bitrateKbps, framesPerSecond float64) {
	if len(history) < 2 {
		return 0, 0
	}
	first := history[0]
	last := history[len(history)-1]
	elapsed := last.at.Sub(first.at).Seconds()
	if elapsed <= 0 {
		return 0, 0
	}
	if last.bytesReceived >= first.bytesReceived {
		bitrateKbps = float64(last.bytesReceived-first.bytesReceived) * 8 / elapsed / 1000
	}
	if last.framesDecoded >= first.framesDecoded {
		framesPerSecond = float64(last.framesDecoded-first.framesDecoded) / elapsed
	}
	return bitrateKbps, framesPerSecond
}

type pendingCall struct {
	ch chan jsonrpc.Response
}

func New(cfg Config) (*Client, error) {
	authClient, err := auth.NewClient()
	if err != nil {
		return nil, err
	}
	if cfg.RPCTimeout == 0 {
		cfg.RPCTimeout = 5 * time.Second
	}
	return &Client{
		cfg:         cfg,
		authClient:  authClient,
		eventCh:     make(chan jsonrpc.Event, 32),
		serialCh:    make(chan SerialConsoleEvent, 128),
		hidReady:    make(chan struct{}),
		lifecycleCh: make(chan LifecycleEvent, 32),
		closeCh:     make(chan struct{}),
	}, nil
}

func (c *Client) Events() <-chan jsonrpc.Event {
	return c.eventCh
}

func (c *Client) SerialEvents() <-chan SerialConsoleEvent {
	return c.serialCh
}

func (c *Client) Lifecycle() <-chan LifecycleEvent {
	return c.lifecycleCh
}

func (c *Client) DeviceInfo(ctx context.Context) (auth.DeviceInfo, error) {
	return c.authClient.GetDeviceInfo(ctx, c.cfg.BaseURL)
}

func (c *Client) CreateLocalPassword(ctx context.Context, password string) error {
	return c.authClient.CreateLocalPassword(ctx, c.cfg.BaseURL, password)
}

func (c *Client) UpdateLocalPassword(ctx context.Context, oldPassword, newPassword string) error {
	return c.authClient.UpdateLocalPassword(ctx, c.cfg.BaseURL, oldPassword, newPassword)
}

func (c *Client) DeleteLocalPassword(ctx context.Context, password string) error {
	return c.authClient.DeleteLocalPassword(ctx, c.cfg.BaseURL, password)
}

func (c *Client) SignalingMode() SignalingMode {
	return c.signalMode
}

func (c *Client) Connect(ctx context.Context) error {
	if err := c.authClient.Login(ctx, c.cfg.BaseURL, c.cfg.Password); err != nil {
		c.emitLifecycle(LifecycleEvent{Type: "connect_error", Err: err.Error()})
		return err
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return err
	}
	c.pc = pc

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.noteTransportDebug(fmt.Sprintf("pc_state=%s", state))
		c.emitLifecycle(LifecycleEvent{Type: "peer_state", Connection: state})
	})
	if sctp := pc.SCTP(); sctp != nil {
		sctp.OnClose(func(err error) {
			c.handleTransportDisconnect(webrtc.PeerConnectionStateDisconnected, "sctp_close")
		})
	}

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		stream, err := video.AttachRemoteTrack(ctx, track)
		if err != nil {
			c.emitLifecycle(LifecycleEvent{Type: "video_error", Err: err.Error()})
			return
		}
		c.videoMu.Lock()
		c.videoStream = stream
		c.videoMu.Unlock()
		c.emitLifecycle(LifecycleEvent{Type: "video_ready"})
	})
	videoTransceiver, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		return err
	}
	if err := restrictVideoTransceiverToH264(videoTransceiver); err != nil {
		return err
	}

	if err := c.openDataChannels(); err != nil {
		return err
	}

	answerCh := make(chan webrtc.SessionDescription, 1)
	wsErrCh := make(chan error, 1)
	signalConn, useLegacySignaling, err := c.openSignaling(ctx, pc, answerCh, wsErrCh)
	if err != nil {
		return err
	}
	if signalConn != nil {
		c.signalConn = signalConn
	}
	if useLegacySignaling {
		c.signalMode = SignalingModeLegacyHTTP
	} else {
		c.signalMode = SignalingModeWebSocket
	}
	c.emitLifecycle(LifecycleEvent{Type: "signaling_mode", Signaling: c.signalMode})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return err
	}

	if useLegacySignaling {
		<-webrtc.GatheringCompletePromise(pc)

		rawOffer, err := json.Marshal(pc.LocalDescription())
		if err != nil {
			return err
		}
		resp, err := signaling.Exchange(ctx, c.authClient.HTTPClient(), c.cfg.BaseURL, signaling.ExchangeRequest{
			SD: signaling.EncodeSDP(rawOffer),
		})
		if err != nil {
			return err
		}

		answer, err := decodeAnswer(resp.SD)
		if err != nil {
			return err
		}
		if err := pc.SetRemoteDescription(answer); err != nil {
			c.emitLifecycle(LifecycleEvent{Type: "connect_error", Err: err.Error()})
			return err
		}
	} else {
		rawOffer, err := json.Marshal(pc.LocalDescription())
		if err != nil {
			return err
		}
		if err := c.writeSignal(signalingMessage{
			Type: "offer",
			Data: offerSignalData{SD: signaling.EncodeSDP(rawOffer)},
		}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-wsErrCh:
			return err
		case answer := <-answerCh:
			if err := pc.SetRemoteDescription(answer); err != nil {
				c.emitLifecycle(LifecycleEvent{Type: "connect_error", Err: err.Error()})
				return err
			}
		}
	}
	c.emitLifecycle(LifecycleEvent{Type: "connected"})
	return nil
}

func restrictVideoTransceiverToH264(transceiver *webrtc.RTPTransceiver) error {
	return transceiver.SetCodecPreferences([]webrtc.RTPCodecParameters{{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeH264,
			ClockRate: 90000,
		},
	}})
}

func offerAdvertisesH265(offer webrtc.SessionDescription) bool {
	return strings.Contains(strings.ToUpper(offer.SDP), "H265")
}

func (c *Client) handleTransportDisconnect(state webrtc.PeerConnectionState, source string) {
	select {
	case <-c.closeCh:
		return
	default:
	}
	c.noteTransportDebug(fmt.Sprintf("disconnect source=%s state=%s", source, state))
	c.disconnectOnce.Do(func() {
		c.emitLifecycle(LifecycleEvent{Type: "peer_state", Connection: state})
		go func() {
			_ = c.Close()
		}()
	})
}

func (c *Client) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	var err error
	select {
	case <-c.closeCh:
	default:
		close(c.closeCh)
	}
	c.noteTransportDebug("client_close")
	c.videoMu.Lock()
	if c.videoStream != nil {
		c.videoStream.Close()
		c.videoStream = nil
	}
	c.videoMu.Unlock()
	if c.signalConn != nil {
		_ = c.signalConn.Close()
		c.signalConn = nil
	}
	if c.pc != nil {
		err = c.pc.Close()
		c.pc = nil
	}
	return err
}

func (c *Client) WaitForHID(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.hidReady:
		return nil
	}
}

func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	if c.rpcDC == nil {
		return fmt.Errorf("rpc data channel not ready")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.RPCTimeout)
		defer cancel()
	}

	id := fmt.Sprintf("rpc-%d", c.requestCounter.Add(1))
	req := jsonrpc.NewRequest(method, params, id)
	data, err := jsonrpc.Marshal(req)
	if err != nil {
		return err
	}

	respCh := make(chan jsonrpc.Response, 1)
	c.pending.Store(id, pendingCall{ch: respCh})
	defer c.pending.Delete(id)

	if err := c.rpcDC.SendText(string(data)); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return fmt.Errorf("%s: %s", method, resp.Error.Message)
		}
		if out == nil {
			return nil
		}
		raw, err := json.Marshal(resp.Result)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, out)
	}
}

func (c *Client) SendKeypress(key byte, press bool) error {
	if c.hidDC == nil {
		return fmt.Errorf("hid channel not ready")
	}
	msg := hidrpc.Keypress{Key: key, Press: press}
	data, err := msg.MarshalBinary()
	if err != nil {
		return err
	}
	return c.hidDC.Send(data)
}

func (c *Client) SendKeypressKeepAlive() error {
	if c.hidDC == nil {
		return fmt.Errorf("hid channel not ready")
	}
	msg := hidrpc.KeypressKeepAlive{}
	data, err := msg.MarshalBinary()
	if err != nil {
		return err
	}
	return c.hidDC.Send(data)
}

func (c *Client) SendAbsPointer(x, y int32, buttons byte) error {
	log := logging.Subsystem("client")
	if c.hidUnreliable == nil {
		err := fmt.Errorf("pointer channel not ready")
		log.Debug().Err(err).Int32("x", x).Int32("y", y).Uint8("buttons", buttons).Msg("failed to send absolute pointer")
		return err
	}
	msg := hidrpc.Pointer{X: x, Y: y, Buttons: buttons}
	data, err := msg.MarshalBinary()
	if err != nil {
		log.Debug().Err(err).Int32("x", x).Int32("y", y).Uint8("buttons", buttons).Msg("failed to marshal absolute pointer")
		return err
	}
	if buttons != 0 {
		log.Trace().Int32("x", x).Int32("y", y).Uint8("buttons", buttons).Msg("sending absolute pointer")
	}
	if err := c.hidUnreliable.Send(data); err != nil {
		log.Debug().Err(err).Int32("x", x).Int32("y", y).Uint8("buttons", buttons).Msg("failed to send absolute pointer")
		return err
	}
	return nil
}

func (c *Client) SendRelMouse(dx, dy int8, buttons byte) error {
	log := logging.Subsystem("client")
	if c.hidDC == nil {
		err := fmt.Errorf("hid channel not ready")
		log.Debug().Err(err).Int8("dx", dx).Int8("dy", dy).Uint8("buttons", buttons).Msg("failed to send relative mouse")
		return err
	}
	msg := hidrpc.Mouse{DX: dx, DY: dy, Buttons: buttons}
	data, err := msg.MarshalBinary()
	if err != nil {
		log.Debug().Err(err).Int8("dx", dx).Int8("dy", dy).Uint8("buttons", buttons).Msg("failed to marshal relative mouse")
		return err
	}
	if buttons != 0 || (dx == 0 && dy == 0) {
		log.Trace().Int8("dx", dx).Int8("dy", dy).Uint8("buttons", buttons).Msg("sending relative mouse")
	}
	if err := c.hidDC.Send(data); err != nil {
		log.Debug().Err(err).Int8("dx", dx).Int8("dy", dy).Uint8("buttons", buttons).Msg("failed to send relative mouse")
		return err
	}
	return nil
}

func (c *Client) SendWheel(wheelY, wheelX int8) error {
	return c.sendWheelReport(context.Background(), wheelY, wheelX)
}

func (c *Client) ExecuteKeyboardMacro(isPaste bool, steps []hidrpc.KeyboardMacroStep) error {
	if c.hidDC == nil {
		return fmt.Errorf("hid channel not ready")
	}
	msg := hidrpc.KeyboardMacroReport{IsPaste: isPaste, Steps: steps}
	data, err := msg.MarshalBinary()
	if err != nil {
		return err
	}
	return c.hidDC.Send(data)
}

func (c *Client) SendSerialText(text string) error {
	if text == "" {
		return nil
	}
	if c.serialDC == nil {
		return fmt.Errorf("serial channel not ready")
	}
	data, err := json.Marshal(struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}{
		Type: "serial",
		Data: text,
	})
	if err != nil {
		return err
	}
	return c.serialDC.SendText(string(data))
}

func (c *Client) SendSerialRaw(text string) error {
	if text == "" {
		return nil
	}
	if c.serialDC == nil {
		return fmt.Errorf("serial channel not ready")
	}
	return c.serialDC.SendText(text)
}

func (c *Client) CancelKeyboardMacro() error {
	if c.hidDC == nil {
		return fmt.Errorf("hid channel not ready")
	}
	msg := hidrpc.CancelKeyboardMacro{}
	data, err := msg.MarshalBinary()
	if err != nil {
		return err
	}
	return c.hidDC.Send(data)
}

func (c *Client) VideoStream() *video.Stream {
	c.videoMu.RLock()
	defer c.videoMu.RUnlock()
	return c.videoStream
}

func (c *Client) openDataChannels() error {
	var err error
	c.rpcDC, err = c.pc.CreateDataChannel("rpc", nil)
	if err != nil {
		return err
	}
	c.watchControlChannel(c.rpcDC)
	c.rpcDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		if !msg.IsString {
			return
		}
		decoded, err := jsonrpc.DecodeMessage(msg.Data)
		if err != nil {
			return
		}
		switch v := decoded.(type) {
		case jsonrpc.Response:
			if call, ok := c.pending.Load(fmt.Sprint(v.ID)); ok {
				call.(pendingCall).ch <- v
			}
		case jsonrpc.Event:
			select {
			case c.eventCh <- v:
			default:
			}
		}
	})

	c.hidDC, err = c.pc.CreateDataChannel("hidrpc", nil)
	if err != nil {
		return err
	}
	c.watchControlChannel(c.hidDC)
	c.hidDC.OnOpen(func() {
		go c.runHIDHandshake(c.hidDC)
	})
	c.hidDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		decoded, err := hidrpc.Decode(msg.Data)
		if err != nil {
			return
		}
		switch v := decoded.(type) {
		case hidrpc.Handshake:
			if v.Version <= hidrpc.Version {
				c.hidReadyOnce.Do(func() {
					close(c.hidReady)
					c.emitLifecycle(LifecycleEvent{Type: "hid_ready"})
				})
			}
		case hidrpc.KeyboardMacroState:
			c.emitLifecycle(LifecycleEvent{Type: "paste_state", PasteState: v.State && v.IsPaste})
		}
	})

	c.hidUnreliable, err = c.pc.CreateDataChannel("hidrpc-unreliable-ordered", &webrtc.DataChannelInit{
		Ordered:        &[]bool{true}[0],
		MaxRetransmits: &[]uint16{0}[0],
	})
	if err != nil {
		return err
	}
	c.watchControlChannel(c.hidUnreliable)
	c.hidNonOrdered, err = c.pc.CreateDataChannel("hidrpc-unreliable-nonordered", &webrtc.DataChannelInit{
		Ordered:        &[]bool{false}[0],
		MaxRetransmits: &[]uint16{0}[0],
	})
	if err == nil {
		c.watchControlChannel(c.hidNonOrdered)
	}
	if err != nil {
		return err
	}

	c.serialDC, err = c.pc.CreateDataChannel("serial", nil)
	if err != nil {
		return err
	}
	c.watchControlChannel(c.serialDC)
	c.serialDC.OnOpen(func() {
		c.emitLifecycle(LifecycleEvent{Type: "serial_ready"})
	})
	c.serialDC.OnError(func(err error) {
		if err != nil {
			c.emitLifecycle(LifecycleEvent{Type: "serial_error", Err: err.Error()})
		}
	})
	c.serialDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		text := string(msg.Data)
		if text == "" {
			return
		}
		select {
		case c.serialCh <- SerialConsoleEvent{Text: text}:
		default:
		}
	})
	return nil
}

func (c *Client) watchControlChannel(dc *webrtc.DataChannel) {
	if dc == nil {
		return
	}
	dc.OnClose(func() {
		c.handleTransportDisconnect(webrtc.PeerConnectionStateDisconnected, "datachannel:"+dc.Label()+":close")
	})
	dc.OnError(func(err error) {
		if err != nil {
			c.lastError.Store(err.Error())
		}
		c.handleTransportDisconnect(webrtc.PeerConnectionStateFailed, "datachannel:"+dc.Label()+":error")
	})
}

func (c *Client) noteTransportDebug(msg string) {
	c.transportDebug.Store(msg)
}

func (c *Client) HTTPClient() *http.Client {
	return c.authClient.HTTPClient()
}

func (c *Client) LatestFrame() image.Image {
	stream := c.VideoStream()
	if stream == nil || stream.Latest() == nil {
		return nil
	}
	return stream.Latest().Image
}

func (c *Client) LatestFrameInfo() (image.Image, time.Time) {
	stream := c.VideoStream()
	if stream == nil {
		return nil, time.Time{}
	}
	frame := stream.Latest()
	if frame == nil {
		return nil, time.Time{}
	}
	return frame.Image, frame.At
}

func (c *Client) emitLifecycle(evt LifecycleEvent) {
	if evt.Err != "" {
		c.lastError.Store(evt.Err)
	}
	select {
	case c.lifecycleCh <- evt:
	default:
	}
}

func (c *Client) Stats() StatsSnapshot {
	stats := StatsSnapshot{
		SignalingMode: c.signalMode,
	}
	select {
	case <-c.closeCh:
		if err, ok := c.lastError.Load().(string); ok {
			stats.LastError = err
		}
		if debug, ok := c.transportDebug.Load().(string); ok {
			stats.TransportDebug = debug
		}
		return stats
	default:
	}
	if frame, _ := c.LatestFrameInfo(); frame != nil {
		b := frame.Bounds()
		stats.FrameWidth = b.Dx()
		stats.FrameHeight = b.Dy()
	}
	if c.pc != nil {
		stats.RTCState = c.pc.ConnectionState()
		if stats.RTCState == webrtc.PeerConnectionStateClosed {
			if err, ok := c.lastError.Load().(string); ok {
				stats.LastError = err
			}
			if debug, ok := c.transportDebug.Load().(string); ok {
				stats.TransportDebug = debug
			}
			return stats
		}
		report := c.pc.GetStats()
		now := time.Now()
		for _, raw := range report {
			switch v := raw.(type) {
			case webrtc.InboundRTPStreamStats:
				if v.Kind != "video" {
					continue
				}
				if stats.FrameWidth == 0 && stats.FrameHeight == 0 {
					stats.FrameWidth = int(v.FrameWidth)
					stats.FrameHeight = int(v.FrameHeight)
				}
				stats.BytesReceived = v.BytesReceived
				stats.PacketsLost = v.PacketsLost
				stats.JitterMs = v.Jitter * 1000
				stats.FramesDecoded = v.FramesDecoded
				stats.FramesRendered = v.FramesRendered
				c.statsMu.Lock()
				c.statsHistory = append(c.statsHistory, statsSample{
					at:            now,
					bytesReceived: v.BytesReceived,
					framesDecoded: v.FramesDecoded,
				})
				cutoff := now.Add(-3 * time.Second)
				trimmed := c.statsHistory[:0]
				for _, sample := range c.statsHistory {
					if sample.at.Before(cutoff) && len(c.statsHistory) > 2 {
						continue
					}
					trimmed = append(trimmed, sample)
				}
				c.statsHistory = trimmed
				stats.BitrateKbps, stats.FramesPerSecond = computeSmoothedRates(c.statsHistory)
				c.statsMu.Unlock()
			case webrtc.ICECandidatePairStats:
				if v.State == webrtc.StatsICECandidatePairStateSucceeded || v.Nominated {
					stats.RoundTripMs = v.CurrentRoundTripTime * 1000
				}
			}
		}
	}
	select {
	case <-c.hidReady:
		stats.HIDReady = true
	default:
	}
	stats.VideoReady = c.VideoStream() != nil && c.VideoStream().Latest() != nil
	if err, ok := c.lastError.Load().(string); ok {
		stats.LastError = err
	}
	if debug, ok := c.transportDebug.Load().(string); ok {
		stats.TransportDebug = debug
	}
	return stats
}

func (c *Client) runHIDHandshake(dc *webrtc.DataChannel) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	send := func() bool {
		hello := hidrpc.Handshake{Version: hidrpc.Version}
		data, err := hello.MarshalBinary()
		if err != nil {
			return false
		}
		if dc.ReadyState() != webrtc.DataChannelStateOpen {
			return false
		}
		return dc.Send(data) == nil
	}

	_ = send()

	for {
		select {
		case <-c.closeCh:
			return
		case <-c.hidReady:
			return
		case <-ticker.C:
			if dc.ReadyState() != webrtc.DataChannelStateOpen {
				return
			}
			_ = send()
		}
	}
}

func (c *Client) openSignaling(ctx context.Context, pc *webrtc.PeerConnection, answerCh chan<- webrtc.SessionDescription, wsErrCh chan<- error) (*websocket.Conn, bool, error) {
	conn, _, err := signaling.DialWebsocket(ctx, c.authClient.HTTPClient(), c.cfg.BaseURL)
	if err != nil {
		return nil, true, nil
	}

	_, data, err := conn.ReadMessage()
	if err != nil {
		_ = conn.Close()
		return nil, true, nil
	}

	var msg signaling.WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	if msg.Type != "device-metadata" {
		_ = conn.Close()
		return nil, false, fmt.Errorf("unexpected signaling message type %q", msg.Type)
	}

	var meta signaling.DeviceMetadata
	if err := json.Unmarshal(msg.Data, &meta); err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	if meta.DeviceVersion == "" {
		_ = conn.Close()
		return nil, true, nil
	}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		init := candidate.ToJSON()
		if init.Candidate == "" {
			return
		}
		_ = c.writeSignal(signalingMessage{
			Type: "new-ice-candidate",
			Data: init,
		})
	})

	go c.readSignaling(conn, pc, answerCh, wsErrCh)
	return conn, false, nil
}

func (c *Client) readSignaling(conn *websocket.Conn, pc *webrtc.PeerConnection, answerCh chan<- webrtc.SessionDescription, wsErrCh chan<- error) {
	defer conn.Close()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			c.noteTransportDebug("signaling_read_error")
			select {
			case <-c.closeCh:
			default:
				select {
				case wsErrCh <- err:
				default:
				}
			}
			return
		}

		var msg signaling.WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "answer":
			var encoded string
			if err := json.Unmarshal(msg.Data, &encoded); err != nil {
				continue
			}
			answer, err := decodeAnswer(encoded)
			if err != nil {
				select {
				case wsErrCh <- err:
				default:
				}
				return
			}
			select {
			case answerCh <- answer:
			default:
			}
		case "new-ice-candidate":
			var candidate webrtc.ICECandidateInit
			if err := json.Unmarshal(msg.Data, &candidate); err != nil {
				continue
			}
			if pc.RemoteDescription() == nil {
				time.AfterFunc(100*time.Millisecond, func() {
					_ = pc.AddICECandidate(candidate)
				})
				continue
			}
			_ = pc.AddICECandidate(candidate)
		}
	}
}

func decodeAnswer(encoded string) (webrtc.SessionDescription, error) {
	rawAnswer, err := signaling.DecodeSDP(encoded)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	var answer webrtc.SessionDescription
	if err := json.Unmarshal(rawAnswer, &answer); err != nil {
		return webrtc.SessionDescription{}, err
	}
	return answer, nil
}

func (c *Client) writeSignal(msg any) error {
	if c.signalConn == nil {
		return fmt.Errorf("signaling connection not ready")
	}
	c.signalMu.Lock()
	defer c.signalMu.Unlock()
	return c.signalConn.WriteJSON(msg)
}
