package client

import (
	"testing"
	"time"
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
