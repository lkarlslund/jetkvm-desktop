package client

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestComputeSmoothedRates(t *testing.T) {
	base := time.Unix(1000, 0)
	history := []statsSample{
		{at: base, bytesReceived: 1000, framesDecoded: 10},
		{at: base.Add(1500 * time.Millisecond), bytesReceived: 2500, framesDecoded: 40},
		{at: base.Add(3 * time.Second), bytesReceived: 4000, framesDecoded: 70},
	}

	bitrateKbps, fps := computeSmoothedRates(history)
	if bitrateKbps != 8 {
		t.Fatalf("expected 8 kbps, got %v", bitrateKbps)
	}
	if fps != 20 {
		t.Fatalf("expected 20 fps, got %v", fps)
	}
}

func TestComputeSmoothedRatesHandlesShortHistory(t *testing.T) {
	bitrateKbps, fps := computeSmoothedRates([]statsSample{{at: time.Unix(1000, 0)}})
	if bitrateKbps != 0 || fps != 0 {
		t.Fatalf("expected zero rates, got bitrate=%v fps=%v", bitrateKbps, fps)
	}
}

func TestHandleTransportDisconnectEmitsLifecycleAndCloses(t *testing.T) {
	c, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	c.handleTransportDisconnect(webrtc.PeerConnectionStateDisconnected, "test")

	select {
	case evt := <-c.Lifecycle():
		if evt.Type != "peer_state" {
			t.Fatalf("expected peer_state event, got %q", evt.Type)
		}
		if evt.Connection != webrtc.PeerConnectionStateDisconnected {
			t.Fatalf("expected disconnected state, got %s", evt.Connection)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for lifecycle event")
	}

	select {
	case <-c.closeCh:
	case <-time.After(time.Second):
		t.Fatal("expected client to close after transport disconnect")
	}
}

func TestHandleTransportDisconnectNoopsAfterClose(t *testing.T) {
	c, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	c.handleTransportDisconnect(webrtc.PeerConnectionStateDisconnected, "test")

	select {
	case evt := <-c.Lifecycle():
		t.Fatalf("unexpected lifecycle event after close: %+v", evt)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestStatsSkipsPeerConnectionPollingAfterClose(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}

	c, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	c.pc = pc

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if c.pc != nil {
		t.Fatal("expected Close to clear peer connection reference")
	}

	_ = c.Stats()
}
