package main

import "testing"

func TestInitialWindowSizeForMonitor(t *testing.T) {
	tests := []struct {
		name          string
		monitorWidth  int
		monitorHeight int
		wantWidth     int
		wantHeight    int
	}{
		{
			name:          "unknown monitor uses default",
			monitorWidth:  0,
			monitorHeight: 0,
			wantWidth:     defaultWindowWidth,
			wantHeight:    defaultWindowHeight,
		},
		{
			name:          "large monitor prefers full hd",
			monitorWidth:  2560,
			monitorHeight: 1440,
			wantWidth:     targetWindowWidth,
			wantHeight:    targetWindowHeight,
		},
		{
			name:          "mid sized monitor scales up beyond default",
			monitorWidth:  1800,
			monitorHeight: 1000,
			wantWidth:     1600,
			wantHeight:    900,
		},
		{
			name:          "common laptop scales above default when there is room",
			monitorWidth:  1440,
			monitorHeight: 900,
			wantWidth:     1296,
			wantHeight:    729,
		},
		{
			name:          "small monitor scales down to fit",
			monitorWidth:  1280,
			monitorHeight: 720,
			wantWidth:     1152,
			wantHeight:    648,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWidth, gotHeight := initialWindowSizeForMonitor(tt.monitorWidth, tt.monitorHeight)
			if gotWidth != tt.wantWidth || gotHeight != tt.wantHeight {
				t.Fatalf("initialWindowSizeForMonitor(%d, %d) = (%d, %d), want (%d, %d)", tt.monitorWidth, tt.monitorHeight, gotWidth, gotHeight, tt.wantWidth, tt.wantHeight)
			}
		})
	}
}
