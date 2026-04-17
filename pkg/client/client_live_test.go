package client

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

func TestLiveDeviceConnectsAndStreams(t *testing.T) {
	baseURL := os.Getenv("JETKVM_BASE_URL")
	if baseURL == "" {
		t.Skip("JETKVM_BASE_URL not set")
	}

	password := os.Getenv("JETKVM_PASSWORD")

	c, err := New(Config{
		BaseURL:    baseURL,
		Password:   password,
		RPCTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var (
		mu     sync.Mutex
		events []LifecycleEvent
	)
	logCtx, logCancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-logCtx.Done():
				return
			case evt := <-c.Lifecycle():
				mu.Lock()
				events = append(events, evt)
				mu.Unlock()
			}
		}
	}()
	defer func() {
		logCancel()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
		}
		mu.Lock()
		defer mu.Unlock()
		for _, evt := range events {
			t.Logf("lifecycle: type=%s state=%s err=%s", evt.Type, evt.Connection.String(), evt.Err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.WaitForHID(ctx); err != nil {
		t.Fatal(err)
	}
	if c.SignalingMode() != SignalingModeWebSocket {
		t.Fatalf("expected websocket signaling mode, got %v", c.SignalingMode())
	}

	var deviceID string
	if err := c.Call(ctx, "getDeviceID", nil, &deviceID); err != nil {
		t.Fatal(err)
	}
	if deviceID == "" {
		t.Fatal("expected non-empty device ID")
	}

	var quality float64
	if err := c.Call(ctx, "getStreamQualityFactor", nil, &quality); err != nil {
		t.Fatal(err)
	}
	if quality < 0 || quality > 1 {
		t.Fatalf("unexpected stream quality factor %v", quality)
	}
	restoreQuality := quality
	if os.Getenv("JETKVM_LIVE_ALLOW_SETTING_MUTATIONS") == "1" {
		testQuality := 0.5
		if quality == testQuality {
			testQuality = 0.6
		}
		if testQuality != quality {
			mutationCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := c.SetStreamQualityFactor(mutationCtx, testQuality); err != nil {
				t.Fatal(err)
			}
			defer func() {
				restoreCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				_ = c.SetStreamQualityFactor(restoreCtx, restoreQuality)
			}()
			var updatedQuality float64
			if err := c.Call(ctx, "getStreamQualityFactor", nil, &updatedQuality); err != nil {
				t.Fatal(err)
			}
			if updatedQuality != testQuality {
				t.Fatalf("expected updated quality %v, got %v", testQuality, updatedQuality)
			}
		}
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		stream := c.VideoStream()
		if stream != nil && stream.Latest() != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	if stream := c.VideoStream(); stream != nil && stream.Err() != nil {
		t.Logf("video error: %v", stream.Err())
	}
	if stream := c.VideoStream(); stream == nil || stream.Latest() == nil {
		t.Fatal("expected at least one decoded video frame from live device")
	}

	if err := c.SendKeypress(4, true); err != nil {
		t.Fatal(err)
	}
	if err := c.SendKeypress(4, false); err != nil {
		t.Fatal(err)
	}
	if err := c.SendAbsPointer(1024, 2048, 0); err != nil {
		t.Fatal(err)
	}
	if err := c.SendRelMouse(3, -2, 0); err != nil {
		t.Fatal(err)
	}
	if err := c.SendWheel(-1, 0); err != nil {
		t.Fatal(err)
	}
}
