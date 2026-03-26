package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

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
		cfg:        cfg,
		authClient: authClient,
		eventCh:    make(chan jsonrpc.Event, 32),
		hidReady:   make(chan struct{}),
	}, nil
}

func (c *Client) Events() <-chan jsonrpc.Event {
	return c.eventCh
}

func (c *Client) Connect(ctx context.Context) error {
	if err := c.authClient.Login(ctx, c.cfg.BaseURL, c.cfg.Password); err != nil {
		return err
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return err
	}
	c.pc = pc

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {})

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		stream, err := video.AttachRemoteTrack(ctx, track)
		if err != nil {
			return
		}
		c.videoStream = stream
	})
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		return err
	}

	if err := c.openDataChannels(); err != nil {
		return err
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return err
	}
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

	rawAnswer, err := signaling.DecodeSDP(resp.SD)
	if err != nil {
		return err
	}
	var answer webrtc.SessionDescription
	if err := json.Unmarshal(rawAnswer, &answer); err != nil {
		return err
	}
	return pc.SetRemoteDescription(answer)
}

func (c *Client) Close() error {
	if c.videoStream != nil {
		c.videoStream.Close()
	}
	if c.pc == nil {
		return nil
	}
	return c.pc.Close()
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

func (c *Client) SendAbsPointer(x, y uint16, buttons byte) error {
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
		hello := hidrpc.Handshake{Version: hidrpc.Version}
		data, err := hello.MarshalBinary()
		if err == nil {
			_ = c.hidDC.Send(data)
		}
	})
	c.hidDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		decoded, err := hidrpc.Decode(msg.Data)
		if err != nil {
			return
		}
		if hs, ok := decoded.(hidrpc.Handshake); ok && hs.Version == hidrpc.Version {
			c.hidReadyOnce.Do(func() { close(c.hidReady) })
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
