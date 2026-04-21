package discovery

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

func TestScannerStartStopRunning(t *testing.T) {
	originalEnumerate := discoveryEnumerateTargets
	originalProbe := discoveryProbeTarget
	t.Cleanup(func() {
		discoveryEnumerateTargets = originalEnumerate
		discoveryProbeTarget = originalProbe
	})

	discoveryEnumerateTargets = func() []netip.Addr {
		return []netip.Addr{netip.MustParseAddr("192.168.1.10")}
	}
	probeStarted := make(chan struct{}, 1)
	releaseProbe := make(chan struct{})
	discoveryProbeTarget = func(ctx context.Context, addr netip.Addr) (Device, bool) {
		select {
		case probeStarted <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return Device{}, false
		case <-releaseProbe:
			return Device{BaseURL: "http://" + addr.String()}, true
		}
	}

	scanner := NewScanner()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scanner.Start(ctx)

	select {
	case <-probeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for discovery probe to start")
	}

	if !scanner.Running() {
		t.Fatal("expected scanner to report running after Start")
	}

	stopDone := make(chan struct{})
	go func() {
		scanner.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scanner to stop")
	}

	if scanner.Running() {
		t.Fatal("expected scanner to report stopped after Stop")
	}

	scanner.Stop()
	close(releaseProbe)
}
