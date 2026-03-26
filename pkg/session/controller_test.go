package session

import (
	"context"
	"testing"
	"time"

	"github.com/lkarlslund/jetkvm-native/pkg/emulator"
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
}

func startEmulator(t *testing.T) (*emulator.Server, context.Context, context.CancelFunc) {
	t.Helper()
	srv, err := emulator.NewServer(emulator.Config{
		ListenAddr: "127.0.0.1:0",
		AuthMode:   emulator.AuthModePassword,
		Password:   "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
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
