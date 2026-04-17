package emulator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
)

func TestHTTPBootstrapFlow(t *testing.T) {
	srv, err := NewServer(Config{
		ListenAddr: "127.0.0.1:0",
		AuthMode:   AuthModeUnset,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil && ctx.Err() == nil {
				t.Errorf("server: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("server did not shut down")
		}
	})
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for srv.BaseURL() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.BaseURL() == "" {
		t.Fatal("server did not start")
	}

	httpClient := &http.Client{}

	resp, err := httpClient.Get(srv.BaseURL() + "/device/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var status struct {
		IsSetup bool `json:"isSetup"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.IsSetup {
		t.Fatal("expected device to start unconfigured")
	}

	setupReq, err := http.NewRequest(http.MethodPost, srv.BaseURL()+"/device/setup", strings.NewReader(`{"localAuthMode":"password","password":"secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	setupReq.Header.Set("Content-Type", "application/json")
	setupResp, err := httpClient.Do(setupReq)
	if err != nil {
		t.Fatal(err)
	}
	defer setupResp.Body.Close()
	if setupResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected setup status %s", setupResp.Status)
	}

	resp, err = httpClient.Get(srv.BaseURL() + "/device/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !status.IsSetup {
		t.Fatal("expected device to be configured after setup")
	}
}

func TestClientConnectsAndRPCWorks(t *testing.T) {
	srv, err := NewServer(Config{
		ListenAddr: "127.0.0.1:0",
		AuthMode:   AuthModePassword,
		Password:   "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil && ctx.Err() == nil {
				t.Errorf("server: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("server did not shut down")
		}
	})
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for srv.BaseURL() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.BaseURL() == "" {
		t.Fatal("server did not start")
	}

	c, err := client.New(client.Config{
		BaseURL:    srv.BaseURL(),
		Password:   "secret",
		RPCTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()
	if err := c.WaitForHID(waitCtx); err != nil {
		t.Fatal(err)
	}
	if c.SignalingMode() != client.SignalingModeWebSocket {
		t.Fatalf("expected websocket signaling mode, got %q", c.SignalingMode())
	}

	var pong string
	if err := c.Call(waitCtx, "ping", nil, &pong); err != nil {
		t.Fatal(err)
	}
	if pong != "pong" {
		t.Fatalf("expected pong, got %q", pong)
	}

	var quality float64
	if err := c.Call(waitCtx, "getStreamQualityFactor", nil, &quality); err != nil {
		t.Fatal(err)
	}
	if quality != 0.75 {
		t.Fatalf("expected default quality 0.75, got %v", quality)
	}
	if err := c.SetStreamQualityFactor(waitCtx, 0.5); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(waitCtx, "getStreamQualityFactor", nil, &quality); err != nil {
		t.Fatal(err)
	}
	if quality != 0.5 {
		t.Fatalf("expected updated quality 0.5, got %v", quality)
	}

	if err := c.SendKeypress(4, true); err != nil {
		t.Fatal(err)
	}
	if err := c.SendAbsPointer(1000, 2000, 1); err != nil {
		t.Fatal(err)
	}
	if err := c.SendRelMouse(3, -2, 1); err != nil {
		t.Fatal(err)
	}
	if err := c.SendWheel(-1, 0); err != nil {
		t.Fatal(err)
	}

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		stream := c.VideoStream()
		if stream != nil && stream.Latest() != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if c.VideoStream() == nil || c.VideoStream().Latest() == nil {
		t.Fatal("expected at least one decoded video frame")
	}

	deadline = time.Now().Add(2 * time.Second)
	for len(srv.Inputs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(srv.Inputs()) == 0 {
		t.Fatal("expected HID input to be recorded")
	}

	foundPointer := false
	foundRelative := false
	foundWheel := false
	for _, input := range srv.Inputs() {
		if input.Channel == "hidrpc-unreliable-ordered" {
			foundPointer = true
		}
		if input.Type == "hidrpc.Mouse" {
			foundRelative = true
		}
		if input.Type == "rpc.wheelReport" && input.Data == "wheelY=-1 wheelX=0" {
			foundWheel = true
		}
	}
	if !foundPointer {
		t.Fatal("expected pointer input on hidrpc-unreliable-ordered channel")
	}
	if !foundRelative {
		t.Fatal("expected relative mouse input on hidrpc channel")
	}
	if !foundWheel {
		t.Fatal("expected wheel input via rpc wheelReport")
	}

	if err := c.Reboot(waitCtx); err != nil {
		t.Fatal(err)
	}

	deadline = time.Now().Add(2 * time.Second)
	foundRebootEvent := false
	for time.Now().Before(deadline) && !foundRebootEvent {
		select {
		case evt := <-c.Events():
			if evt.Method == "videoInputState" {
				foundRebootEvent = true
			}
		case <-time.After(25 * time.Millisecond):
		}
	}
	if !foundRebootEvent {
		t.Fatal("expected reboot-driven videoInputState event")
	}
}

func TestClientRPCTimeoutWhenMethodDropped(t *testing.T) {
	srv, err := NewServer(Config{
		ListenAddr: "127.0.0.1:0",
		AuthMode:   AuthModePassword,
		Password:   "secret",
		Faults: FaultConfig{
			DropRPCMethod: "ping",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil && ctx.Err() == nil {
				t.Errorf("server: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("server did not shut down")
		}
	})
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()
	waitForBaseURL(t, srv)

	c, err := client.New(client.Config{
		BaseURL:    srv.BaseURL(),
		Password:   "secret",
		RPCTimeout: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
	defer waitCancel()
	if err := c.WaitForHID(waitCtx); err != nil {
		t.Fatal(err)
	}

	var pong string
	if err := c.Call(context.Background(), "ping", nil, &pong); err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}

func TestClientCallRespectsCallerDeadlineOverride(t *testing.T) {
	srv, err := NewServer(Config{
		ListenAddr: "127.0.0.1:0",
		AuthMode:   AuthModePassword,
		Password:   "secret",
		Faults: FaultConfig{
			DropRPCMethod: "ping",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil && ctx.Err() == nil {
				t.Errorf("server: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("server did not shut down")
		}
	})
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()
	waitForBaseURL(t, srv)

	c, err := client.New(client.Config{
		BaseURL:    srv.BaseURL(),
		Password:   "secret",
		RPCTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
	defer waitCancel()
	if err := c.WaitForHID(waitCtx); err != nil {
		t.Fatal(err)
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer callCancel()
	start := time.Now()
	var pong string
	err = c.Call(callCtx, "ping", nil, &pong)
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected rpc timeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("expected caller deadline to override default timeout, call ended after %v", elapsed)
	}
}

func TestKeyboardStateTracksModifiersAndReleases(t *testing.T) {
	srv, err := NewServer(Config{
		ListenAddr: "127.0.0.1:0",
		AuthMode:   AuthModePassword,
		Password:   "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil && ctx.Err() == nil {
				t.Errorf("server: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("server did not shut down")
		}
	})
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()
	waitForBaseURL(t, srv)

	c, err := client.New(client.Config{
		BaseURL:    srv.BaseURL(),
		Password:   "secret",
		RPCTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
	defer waitCancel()
	if err := c.WaitForHID(waitCtx); err != nil {
		t.Fatal(err)
	}

	if err := c.SendKeypress(225, true); err != nil {
		t.Fatal(err)
	}
	if err := c.SendKeypress(4, true); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	var state struct {
		Modifier byte   `json:"modifier"`
		Keys     []byte `json:"keys"`
	}
	if err := c.Call(waitCtx, "getKeysDownState", nil, &state); err != nil {
		t.Fatal(err)
	}
	if state.Modifier != 0x02 {
		t.Fatalf("expected left shift modifier bit, got %08b", state.Modifier)
	}
	if len(state.Keys) < 1 || state.Keys[0] != 4 {
		t.Fatalf("expected key A in keysDown state, got %+v", state.Keys)
	}

	if err := c.SendKeypress(4, false); err != nil {
		t.Fatal(err)
	}
	if err := c.SendKeypress(225, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := c.Call(waitCtx, "getKeysDownState", nil, &state); err != nil {
		t.Fatal(err)
	}
	if state.Modifier != 0 {
		t.Fatalf("expected modifiers to clear, got %08b", state.Modifier)
	}
	for _, key := range state.Keys {
		if key != 0 {
			t.Fatalf("expected all keys released, got %+v", state.Keys)
		}
	}
}

func TestForcedDisconnectFaultClosesPeerConnection(t *testing.T) {
	srv, err := NewServer(Config{
		ListenAddr: "127.0.0.1:0",
		AuthMode:   AuthModePassword,
		Password:   "secret",
		Faults: FaultConfig{
			DisconnectAfter: 150 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil && ctx.Err() == nil {
				t.Errorf("server: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("server did not shut down")
		}
	})
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()
	waitForBaseURL(t, srv)

	c, err := client.New(client.Config{
		BaseURL:    srv.BaseURL(),
		Password:   "secret",
		RPCTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	lifecycle := make([]string, 0, 16)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case evt := <-c.Lifecycle():
			lifecycle = append(lifecycle, formatLifecycleEvent(evt))
			if evt.Type == "peer_state" && (evt.Connection == webrtc.PeerConnectionStateClosed || evt.Connection == webrtc.PeerConnectionStateDisconnected || evt.Connection == webrtc.PeerConnectionStateFailed) {
				return
			}
		case <-time.After(25 * time.Millisecond):
		}
	}
	t.Fatalf("expected peer connection to close after disconnect fault: stats=%+v lifecycle=%v", c.Stats(), lifecycle)
}

func formatLifecycleEvent(evt client.LifecycleEvent) string {
	switch evt.Type {
	case "peer_state":
		return fmt.Sprintf("%s:%s", evt.Type, evt.Connection)
	case "connect_error", "video_error":
		return fmt.Sprintf("%s:%s", evt.Type, evt.Err)
	case "signaling_mode":
		return fmt.Sprintf("%s:%s", evt.Type, evt.Signaling)
	case "paste_state":
		return fmt.Sprintf("%s:%t", evt.Type, evt.PasteState)
	default:
		return evt.Type
	}
}

func waitForBaseURL(t *testing.T, srv *Server) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for srv.BaseURL() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.BaseURL() == "" {
		t.Fatal("server did not start")
	}
}
