package gtkui

import (
	"testing"
	"time"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"192.168.1.50", "https://192.168.1.50"},
		{"jetkvm.local", "https://jetkvm.local"},
		{"http://jetkvm.local", "http://jetkvm.local"},
		{"https://10.0.0.1", "https://10.0.0.1"},
	}
	for _, tt := range tests {
		got := normalizeURL(tt.in)
		if got != tt.want {
			t.Errorf("normalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsValidHost(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"192.168.1.50", true},
		{"jetkvm.local", true},
		{"", false},
		{"  ", false},
		{"http://ok.com", true},
	}
	for _, tt := range tests {
		got := isValidHost(tt.in)
		if got != tt.want {
			t.Errorf("isValidHost(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "just now"},
		{30 * time.Second, "just now"},
		{5 * time.Minute, "5m ago"},
		{2 * time.Hour, "2h ago"},
		{48 * time.Hour, "2d ago"},
	}
	for _, tt := range tests {
		got := formatAge(tt.d)
		if got != tt.want {
			t.Errorf("formatAge(%s) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestPrefsRecentRoundtrip(t *testing.T) {
	p := defaultPrefs()
	p.addRecent("https://a.local", "A")
	p.addRecent("https://b.local", "B")
	p.addRecent("https://c.local", "C")

	if len(p.RecentConnections) != 3 {
		t.Fatalf("got %d recents, want 3", len(p.RecentConnections))
	}
	if p.RecentConnections[0].URL != "https://c.local" {
		t.Errorf("first recent = %q, want c.local", p.RecentConnections[0].URL)
	}

	p.removeRecent("https://b.local")
	if len(p.RecentConnections) != 2 {
		t.Fatalf("after remove, got %d recents, want 2", len(p.RecentConnections))
	}
}
