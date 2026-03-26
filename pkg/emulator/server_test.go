package emulator

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/lkarlslund/jetkvm-native/pkg/client"
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
	defer cancel()
	go func() {
		if err := srv.ListenAndServe(ctx); err != nil {
			t.Errorf("server: %v", err)
		}
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
	defer cancel()

	go func() {
		if err := srv.ListenAndServe(ctx); err != nil {
			t.Errorf("server: %v", err)
		}
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
	if err := c.Call(waitCtx, "setStreamQualityFactor", map[string]any{"factor": 0.5}, nil); err != nil {
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
	if err := c.SendWheel(-1); err != nil {
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
		if input.Type == "hidrpc.Wheel" {
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
		t.Fatal("expected wheel input on hidrpc channel")
	}

	if err := c.Call(waitCtx, "reboot", map[string]any{"force": false}, nil); err != nil {
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
	defer cancel()
	go func() {
		if err := srv.ListenAndServe(ctx); err != nil {
			t.Errorf("server: %v", err)
		}
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
	if err := c.Call(waitCtx, "ping", nil, &pong); err == nil || !strings.Contains(err.Error(), "rpc timeout") {
		t.Fatalf("expected rpc timeout, got %v", err)
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
	defer cancel()
	go func() {
		if err := srv.ListenAndServe(ctx); err != nil {
			t.Errorf("server: %v", err)
		}
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

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case evt := <-c.Lifecycle():
			if evt.Type == "peer_state" && (evt.Connection == webrtc.PeerConnectionStateClosed || evt.Connection == webrtc.PeerConnectionStateDisconnected || evt.Connection == webrtc.PeerConnectionStateFailed) {
				return
			}
		case <-time.After(25 * time.Millisecond):
		}
	}
	t.Fatal("expected peer connection to close after disconnect fault")
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
