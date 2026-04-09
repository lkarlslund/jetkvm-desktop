package client

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-native/pkg/protocol/auth"
	"github.com/lkarlslund/jetkvm-native/pkg/protocol/hidrpc"
	"github.com/lkarlslund/jetkvm-native/pkg/protocol/jsonrpc"
	"github.com/lkarlslund/jetkvm-native/pkg/protocol/signaling"
	"github.com/lkarlslund/jetkvm-native/pkg/video"
)

type Config struct {
	BaseURL    string
	Password   string
	RPCTimeout time.Duration
}

type SignalingMode string

const (
	SignalingModeLegacyHTTP SignalingMode = "legacy_http"
	SignalingModeWebSocket  SignalingMode = "websocket"
)

type LifecycleEvent struct {
	Type       string
	Connection webrtc.PeerConnectionState
	Err        string
	Signaling  SignalingMode
}

type Client struct {
	cfg        Config
	authClient *auth.Client
	pc         *webrtc.PeerConnection

	rpcDC          *webrtc.DataChannel
	hidDC          *webrtc.DataChannel
	hidUnreliable  *webrtc.DataChannel
	hidNonOrdered  *webrtc.DataChannel
	eventCh        chan jsonrpc.Event
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
		hidReady:    make(chan struct{}),
		lifecycleCh: make(chan LifecycleEvent, 32),
		closeCh:     make(chan struct{}),
	}, nil
}

func (c *Client) Events() <-chan jsonrpc.Event {
	return c.eventCh
}

func (c *Client) Lifecycle() <-chan LifecycleEvent {
	return c.lifecycleCh
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
		c.emitLifecycle(LifecycleEvent{Type: "peer_state", Connection: state})
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		stream, err := video.AttachRemoteTrack(ctx, track)
		if err != nil {
			c.emitLifecycle(LifecycleEvent{Type: "video_error", Err: err.Error()})
			return
		}
		c.videoStream = stream
		c.emitLifecycle(LifecycleEvent{Type: "video_ready"})
	})
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
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
		if err := c.writeSignal(map[string]any{
			"type": "offer",
			"data": map[string]string{"sd": signaling.EncodeSDP(rawOffer)},
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

func (c *Client) Close() error {
	var err error
	select {
	case <-c.closeCh:
	default:
		close(c.closeCh)
	}
	if c.videoStream != nil {
		c.videoStream.Close()
	}
	if c.signalConn != nil {
		_ = c.signalConn.Close()
	}
	if c.pc != nil {
		err = c.pc.Close()
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

func (c *Client) Call(ctx context.Context, method string, params map[string]any, out any) error {
	if c.rpcDC == nil {
		return fmt.Errorf("rpc data channel not ready")
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

	timeout := time.NewTimer(c.cfg.RPCTimeout)
	defer timeout.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timeout.C:
		return fmt.Errorf("rpc timeout for %s", method)
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

func (c *Client) SendAbsPointer(x, y int32, buttons byte) error {
	if c.hidUnreliable == nil {
		return fmt.Errorf("pointer channel not ready")
	}
	msg := hidrpc.Pointer{X: x, Y: y, Buttons: buttons}
	data, err := msg.MarshalBinary()
	if err != nil {
		return err
	}
	return c.hidUnreliable.Send(data)
}

func (c *Client) SendRelMouse(dx, dy int8, buttons byte) error {
	if c.hidDC == nil {
		return fmt.Errorf("hid channel not ready")
	}
	msg := hidrpc.Mouse{DX: dx, DY: dy, Buttons: buttons}
	data, err := msg.MarshalBinary()
	if err != nil {
		return err
	}
	return c.hidDC.Send(data)
}

func (c *Client) SendWheel(delta int8) error {
	if c.hidDC == nil {
		return fmt.Errorf("hid channel not ready")
	}
	msg := hidrpc.Wheel{Delta: delta}
	data, err := msg.MarshalBinary()
	if err != nil {
		return err
	}
	return c.hidDC.Send(data)
}

func (c *Client) VideoStream() *video.Stream {
	return c.videoStream
}

func (c *Client) openDataChannels() error {
	var err error
	c.rpcDC, err = c.pc.CreateDataChannel("rpc", nil)
	if err != nil {
		return err
	}
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
	c.hidDC.OnOpen(func() {
		go c.runHIDHandshake(c.hidDC)
	})
	c.hidDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		decoded, err := hidrpc.Decode(msg.Data)
		if err != nil {
			return
		}
		if hs, ok := decoded.(hidrpc.Handshake); ok && hs.Version <= hidrpc.Version {
			c.hidReadyOnce.Do(func() {
				close(c.hidReady)
				c.emitLifecycle(LifecycleEvent{Type: "hid_ready"})
			})
		}
	})

	c.hidUnreliable, err = c.pc.CreateDataChannel("hidrpc-unreliable-ordered", &webrtc.DataChannelInit{
		Ordered:        &[]bool{true}[0],
		MaxRetransmits: &[]uint16{0}[0],
	})
	if err != nil {
		return err
	}
	c.hidNonOrdered, err = c.pc.CreateDataChannel("hidrpc-unreliable-nonordered", &webrtc.DataChannelInit{
		Ordered:        &[]bool{false}[0],
		MaxRetransmits: &[]uint16{0}[0],
	})
	return err
}

func (c *Client) HTTPClient() *http.Client {
	return c.authClient.HTTPClient()
}

func (c *Client) LatestFrame() image.Image {
	if c.videoStream == nil || c.videoStream.Latest() == nil {
		return nil
	}
	return c.videoStream.Latest().Image
}

func (c *Client) LatestFrameInfo() (image.Image, time.Time) {
	if c.videoStream == nil {
		return nil, time.Time{}
	}
	frame := c.videoStream.Latest()
	if frame == nil {
		return nil, time.Time{}
	}
	return frame.Image, frame.At
}

func (c *Client) emitLifecycle(evt LifecycleEvent) {
	select {
	case c.lifecycleCh <- evt:
	default:
	}
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
		_ = c.writeSignal(map[string]any{
			"type": "new-ice-candidate",
			"data": init,
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
