package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
	"github.com/lkarlslund/jetkvm-desktop/pkg/emulator"
	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/auth"
	"github.com/lkarlslund/jetkvm-desktop/pkg/virtualmedia"
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
		t.Fatalf("expected websocket signaling mode, got %v", snapshot.SignalingMode)
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
	if err := controller.forceDisconnect(context.Background()); err != nil {
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
	if err := controller.SendWheel(-1, 0); err != nil {
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
			case input.Type == "rpc.wheelReport" && strings.Contains(input.Data, "wheelY=-1") && strings.Contains(input.Data, "wheelX=0"):
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

func TestControllerSetUSBDevicesSucceedsWhenWriteAckIsDropped(t *testing.T) {
	srv, ctx, cancel := startFaultedEmulator(t, emulator.FaultConfig{
		ApplyButDropRPCMethod: "setUsbDevices",
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
	want := USBDevices{
		Keyboard:      true,
		AbsoluteMouse: false,
		RelativeMouse: false,
		MassStorage:   false,
		SerialConsole: true,
		Network:       false,
	}
	if err := controller.SetUSBDevices(want); err != nil {
		t.Fatalf("SetUSBDevices returned error: %v", err)
	}
	got, err := controller.GetUSBDevices(context.Background())
	if err != nil {
		t.Fatalf("GetUSBDevices returned error: %v", err)
	}
	if got != want {
		t.Fatalf("usb devices = %+v, want %+v", got, want)
	}
}

func TestControllerVirtualMediaURLMountAndUnmount(t *testing.T) {
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

	if err := controller.MountMediaURL("https://example.com/debian.iso", virtualmedia.ModeCDROM); err != nil {
		t.Fatalf("MountMediaURL returned error: %v", err)
	}
	state, err := controller.GetVirtualMediaState(context.Background())
	if err != nil {
		t.Fatalf("GetVirtualMediaState returned error: %v", err)
	}
	if state == nil || state.Source != virtualmedia.SourceHTTP || state.URL != "https://example.com/debian.iso" {
		t.Fatalf("unexpected media state: %+v", state)
	}

	if err := controller.UnmountMedia(); err != nil {
		t.Fatalf("UnmountMedia returned error: %v", err)
	}
	state, err = controller.GetVirtualMediaState(context.Background())
	if err != nil {
		t.Fatalf("GetVirtualMediaState after unmount returned error: %v", err)
	}
	if state != nil {
		t.Fatalf("expected media to be unmounted, got %+v", state)
	}
}

func TestControllerLocalPasswordLifecycle(t *testing.T) {
	srv, err := emulator.NewServer(emulator.Config{
		ListenAddr: "127.0.0.1:0",
		AuthMode:   emulator.AuthModeNoPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()
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
	deadline := time.Now().Add(2 * time.Second)
	for srv.BaseURL() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.BaseURL() == "" {
		t.Fatal("server did not start")
	}

	controller := New(Config{
		BaseURL:    srv.BaseURL(),
		RPCTimeout: 2 * time.Second,
		Reconnect:  true,
	})
	controller.Start(ctx)
	defer controller.Stop()

	waitForPhase(t, controller, PhaseConnected, 5*time.Second)

	mode, _, err := controller.GetLocalAccessState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mode != LocalAuthModeNoPassword {
		t.Fatalf("initial auth mode = %v, want noPassword", mode)
	}

	if err := controller.CreateLocalPassword("password123"); err != nil {
		t.Fatal(err)
	}
	mode, _, err = controller.GetLocalAccessState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mode != LocalAuthModePassword {
		t.Fatalf("auth mode after create = %v, want password", mode)
	}

	if err := controller.UpdateLocalPassword("password123", "password456"); err != nil {
		t.Fatal(err)
	}
	controller.SetPassword("password456")
	if err := controller.DeleteLocalPassword("password456"); err != nil {
		t.Fatal(err)
	}
	mode, _, err = controller.GetLocalAccessState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mode != LocalAuthModeNoPassword {
		t.Fatalf("auth mode after delete = %v, want noPassword", mode)
	}
}

func TestControllerUploadAndMountStorageFile(t *testing.T) {
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

	tempDir := t.TempDir()
	imagePath := filepath.Join(tempDir, "test.iso")
	if err := os.WriteFile(imagePath, []byte("virtual-media-test-image"), 0o644); err != nil {
		t.Fatal(err)
	}
	progressCalls := 0
	if err := controller.UploadStorageFile(imagePath, func(progress virtualmedia.UploadProgress) {
		progressCalls++
		if progress.Total <= 0 {
			t.Fatalf("expected upload total to be positive, got %+v", progress)
		}
	}); err != nil {
		t.Fatalf("UploadStorageFile returned error: %v", err)
	}
	if progressCalls == 0 {
		t.Fatal("expected upload progress callback to run")
	}

	files, err := controller.ListStorageFiles(context.Background())
	if err != nil {
		t.Fatalf("ListStorageFiles returned error: %v", err)
	}
	found := false
	for _, file := range files {
		if file.Filename == "test.iso" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected uploaded file in storage list, got %+v", files)
	}

	if err := controller.MountStorageFile("test.iso", virtualmedia.ModeCDROM); err != nil {
		t.Fatalf("MountStorageFile returned error: %v", err)
	}
	state, err := controller.GetVirtualMediaState(context.Background())
	if err != nil {
		t.Fatalf("GetVirtualMediaState returned error: %v", err)
	}
	if state == nil || state.Source != virtualmedia.SourceStorage || state.Filename != "test.iso" {
		t.Fatalf("unexpected mounted storage state: %+v", state)
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
	t.Fatalf("timed out waiting for phase %v, got %+v", phase, controller.Snapshot())
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
