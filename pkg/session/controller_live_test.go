package session

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
)

func TestLiveControllerConnectsAndForwardsSafeInput(t *testing.T) {
	baseURL := os.Getenv("JETKVM_BASE_URL")
	if baseURL == "" {
		t.Skip("JETKVM_BASE_URL not set")
	}

	controller := New(Config{
		BaseURL:       baseURL,
		Password:      os.Getenv("JETKVM_PASSWORD"),
		RPCTimeout:    5 * time.Second,
		Reconnect:     true,
		ReconnectBase: 200 * time.Millisecond,
		ReconnectMax:  time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	controller.Start(ctx)
	defer controller.Stop()

	waitForPhase(t, controller, PhaseConnected, 20*time.Second)
	waitForFrame(t, controller, 10*time.Second)

	snapshot := controller.Snapshot()
	if snapshot.DeviceID == "" {
		t.Fatal("expected device ID in controller snapshot")
	}
	if !snapshot.HIDReady {
		t.Fatal("expected HID to be ready in controller snapshot")
	}
	if !snapshot.VideoReady {
		t.Fatal("expected video to be ready in controller snapshot")
	}
	if snapshot.SignalingMode != client.SignalingModeWebSocket {
		t.Fatalf("expected websocket signaling mode, got %q", snapshot.SignalingMode)
	}

	if err := controller.SendKeypress(4, true); err != nil {
		t.Fatal(err)
	}
	if err := controller.SendKeypress(4, false); err != nil {
		t.Fatal(err)
	}
	if err := controller.SendAbsPointer(1500, 2500, 0); err != nil {
		t.Fatal(err)
	}
	if err := controller.SendRelMouse(2, -1, 0); err != nil {
		t.Fatal(err)
	}
	if err := controller.SendWheel(-1); err != nil {
		t.Fatal(err)
	}

	if os.Getenv("JETKVM_LIVE_ALLOW_SETTING_MUTATIONS") == "1" {
		if err := controller.SetQuality(0.5); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLiveControllerRebootsWhenAllowed(t *testing.T) {
	if os.Getenv("JETKVM_LIVE_ALLOW_DISRUPTIVE") != "1" {
		t.Skip("JETKVM_LIVE_ALLOW_DISRUPTIVE not set")
	}

	baseURL := os.Getenv("JETKVM_BASE_URL")
	if baseURL == "" {
		t.Skip("JETKVM_BASE_URL not set")
	}

	controller := New(Config{
		BaseURL:       baseURL,
		Password:      os.Getenv("JETKVM_PASSWORD"),
		RPCTimeout:    5 * time.Second,
		Reconnect:     true,
		ReconnectBase: 500 * time.Millisecond,
		ReconnectMax:  2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	controller.Start(ctx)
	defer controller.Stop()

	waitForPhase(t, controller, PhaseConnected, 20*time.Second)
	if err := controller.Reboot(); err != nil {
		t.Fatal(err)
	}
	waitForAnyPhase(t, controller, 20*time.Second, PhaseRebooting, PhaseReconnecting, PhaseDisconnected)
	waitForPhase(t, controller, PhaseConnected, 90*time.Second)
	waitForFrame(t, controller, 20*time.Second)
}

func waitForAnyPhase(t *testing.T, controller *Controller, timeout time.Duration, phases ...Phase) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		current := controller.Snapshot().Phase
		for _, phase := range phases {
			if current == phase {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for any phase %v, got %+v", phases, controller.Snapshot())
}
