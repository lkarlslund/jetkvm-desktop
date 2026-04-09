package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
	"github.com/lkarlslund/jetkvm-desktop/pkg/emulator"
	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/auth"
)

func TestControllerConnects(t *testing.T) {
	srv, ctx, cancel := startEmulator(t)
	defer cancel()

	controller := New(Config{
		BaseURL:    srv.BaseURL(),
		Password:   "secret",
		RPCTimeout: 2 * time.Second,
		Reconnect:  true,
	})
	controller.Start(ctx)
	defer controller.Stop()

	waitForPhase(t, controller, PhaseConnected, 5*time.Second)
	snapshot := controller.Snapshot()
	if snapshot.DeviceID == "" {
		t.Fatal("expected device ID after bootstrap")
	}
	if !snapshot.HIDReady {
		t.Fatal("expected HID to be ready")
	}
	if snapshot.SignalingMode != client.SignalingModeWebSocket {
		t.Fatalf("expected websocket signaling mode, got %q", snapshot.SignalingMode)
	}
}

func TestControllerReconnectsAfterDisconnect(t *testing.T) {
	srv, ctx, cancel := startEmulator(t)
	defer cancel()

	controller := New(Config{
		BaseURL:       srv.BaseURL(),
		Password:      "secret",
		RPCTimeout:    2 * time.Second,
		Reconnect:     true,
		ReconnectBase: 100 * time.Millisecond,
		ReconnectMax:  300 * time.Millisecond,
	})
	controller.Start(ctx)
	defer controller.Stop()

	waitForPhase(t, controller, PhaseConnected, 5*time.Second)
	if err := controller.call(context.Background(), "forceDisconnect", nil, nil); err != nil {
		t.Fatal(err)
	}
	waitForPhase(t, controller, PhaseConnected, 5*time.Second)
}

func TestControllerTransitionsToOtherSession(t *testing.T) {
	srv, ctx, cancel := startEmulator(t)
	defer cancel()

	first := New(Config{
		BaseURL:    srv.BaseURL(),
		Password:   "secret",
		RPCTimeout: 2 * time.Second,
		Reconnect:  true,
	})
	second := New(Config{
		BaseURL:    srv.BaseURL(),
		Password:   "secret",
		RPCTimeout: 2 * time.Second,
		Reconnect:  true,
	})
	first.Start(ctx)
	defer first.Stop()
	waitForPhase(t, first, PhaseConnected, 5*time.Second)

	second.Start(ctx)
	defer second.Stop()
	waitForPhase(t, second, PhaseConnected, 5*time.Second)
	waitForPhase(t, first, PhaseOtherSession, 5*time.Second)

	first.ReconnectNow()
	waitForPhase(t, first, PhaseConnected, 5*time.Second)
}

func TestControllerReceivesVideoAndForwardsInput(t *testing.T) {
	srv, ctx, cancel := startEmulator(t)
	defer cancel()

	controller := New(Config{
		BaseURL:    srv.BaseURL(),
		Password:   "secret",
		RPCTimeout: 2 * time.Second,
		Reconnect:  true,
	})
	controller.Start(ctx)
	defer controller.Stop()

	waitForPhase(t, controller, PhaseConnected, 5*time.Second)
	waitForFrame(t, controller, 5*time.Second)

	if err := controller.SendKeypress(4, true); err != nil {
		t.Fatal(err)
	}
	if err := controller.SendAbsPointer(1200, 3400, 1); err != nil {
		t.Fatal(err)
	}
	if err := controller.SendRelMouse(5, -3, 1); err != nil {
		t.Fatal(err)
	}
	if err := controller.SendWheel(-1); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		inputs := srv.Inputs()
		if len(inputs) < 4 {
			time.Sleep(25 * time.Millisecond)
			continue
		}
		foundKeypress := false
		foundPointer := false
		foundMouse := false
		foundWheel := false
		for _, input := range inputs {
			switch {
			case strings.Contains(input.Type, "Keypress"):
				foundKeypress = true
			case strings.Contains(input.Type, "Pointer"):
				foundPointer = true
			case strings.Contains(input.Type, "Mouse"):
				foundMouse = true
			case input.Type == "rpc.wheelReport":
				foundWheel = true
			}
		}
		if foundKeypress && foundPointer && foundMouse && foundWheel {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("expected keypress, pointer, mouse, and wheel inputs, got %+v", srv.Inputs())
}

func TestControllerSetQualitySucceedsWhenWriteAckIsDropped(t *testing.T) {
	srv, ctx, cancel := startFaultedEmulator(t, emulator.FaultConfig{
		ApplyButDropRPCMethod: "setStreamQualityFactor",
	})
	defer cancel()

	controller := New(Config{
		BaseURL:         srv.BaseURL(),
		Password:        "secret",
		RPCTimeout:      300 * time.Millisecond,
		MutationTimeout: 2 * time.Second,
		Reconnect:       true,
	})
	controller.Start(ctx)
	defer controller.Stop()

	waitForPhase(t, controller, PhaseConnected, 5*time.Second)
	if err := controller.SetQuality(0.5); err != nil {
		t.Fatalf("SetQuality returned error: %v", err)
	}
	if got := controller.Snapshot().Quality; got != 0.5 {
		t.Fatalf("snapshot quality = %v, want 0.5", got)
	}
}

func TestControllerSetKeyboardLayoutSucceedsWhenWriteAckIsDropped(t *testing.T) {
	srv, ctx, cancel := startFaultedEmulator(t, emulator.FaultConfig{
		ApplyButDropRPCMethod: "setKeyboardLayout",
	})
	defer cancel()

	controller := New(Config{
		BaseURL:         srv.BaseURL(),
		Password:        "secret",
		RPCTimeout:      300 * time.Millisecond,
		MutationTimeout: 2 * time.Second,
		Reconnect:       true,
	})
	controller.Start(ctx)
	defer controller.Stop()

	waitForPhase(t, controller, PhaseConnected, 5*time.Second)
	if err := controller.SetKeyboardLayout("da-DK"); err != nil {
		t.Fatalf("SetKeyboardLayout returned error: %v", err)
	}
	if got := controller.Snapshot().KeyboardLayout; got != "da-DK" {
		t.Fatalf("snapshot keyboard layout = %q, want da-DK", got)
	}
}

func TestIsAuthErrorMatchesDeviceMessages(t *testing.T) {
	tests := []error{
		&auth.Error{StatusCode: 401, Message: "Invalid password"},
		&auth.Error{StatusCode: 403, Message: "Forbidden"},
		&auth.Error{StatusCode: 429, Message: "Too many failed attempts"},
		errors.New("authentication failed"),
	}
	for _, err := range tests {
		if !isAuthError(err) {
			t.Fatalf("expected %v to be treated as auth error", err)
		}
	}
	if isAuthError(errors.New("connection reset by peer")) {
		t.Fatal("expected transport error to not be treated as auth error")
	}
}

func startEmulator(t *testing.T) (*emulator.Server, context.Context, context.CancelFunc) {
	t.Helper()
	return startFaultedEmulator(t, emulator.FaultConfig{})
}

func startFaultedEmulator(t *testing.T, faults emulator.FaultConfig) (*emulator.Server, context.Context, context.CancelFunc) {
	t.Helper()
	srv, err := emulator.NewServer(emulator.Config{
		ListenAddr: "127.0.0.1:0",
		AuthMode:   emulator.AuthModePassword,
		Password:   "secret",
		Faults:     faults,
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
	return srv, ctx, cancel
}

func waitForPhase(t *testing.T, controller *Controller, phase Phase, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if controller.Snapshot().Phase == phase {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for phase %s, got %+v", phase, controller.Snapshot())
}

func waitForFrame(t *testing.T, controller *Controller, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if controller.LatestFrame() != nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for first video frame")
}
