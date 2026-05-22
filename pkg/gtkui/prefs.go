package gtkui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const maxRecentConnections = 10

type RecentConnection struct {
	URL         string    `json:"url"`
	Name        string    `json:"name,omitempty"`
	ConnectedAt time.Time `json:"connected_at"`
}

// Preferences stores user-configurable settings.  The JSON layout is
// compatible with the original pkg/app format so existing configs carry over.
type Preferences struct {
	Theme                     string             `json:"theme"`
	PinChrome                 bool               `json:"pin_chrome"`
	HideHeaderBar             bool               `json:"hide_header_bar"`
	HideStatusBar             bool               `json:"hide_status_bar"`
	ChromeAnchor              string             `json:"chrome_anchor"`
	ChromeLayout              string             `json:"chrome_layout"`
	HideCursor                bool               `json:"hide_cursor"`
	InvertScroll              bool               `json:"invert_scroll"`
	ShowPressedKeys           bool               `json:"show_pressed_keys"`
	ExperimentalGlobalHotkeys bool               `json:"experimental_global_hotkeys"`
	AbsoluteSideButtonsViaRel bool               `json:"absolute_side_buttons_via_relative"`
	ScrollThrottleMs          int                `json:"scroll_throttle_ms,omitempty"`
	PointerMoveThrottleMs     int                `json:"pointer_move_throttle_ms,omitempty"`
	CaptureToggleKey          string             `json:"capture_toggle_key,omitempty"`
	ConnectWindowMode         string             `json:"connect_window_mode,omitempty"`
	ChromeCustomX             float64            `json:"chrome_custom_x,omitempty"`
	ChromeCustomY             float64            `json:"chrome_custom_y,omitempty"`
	ChromeCustomPos           bool               `json:"chrome_custom_pos,omitempty"`
	RecentConnections         []RecentConnection `json:"recent_connections,omitempty"`
}

func defaultPrefs() Preferences {
	return Preferences{
		Theme:                     "system",
		ChromeAnchor:              "top_right",
		ChromeLayout:              "horizontal",
		CaptureToggleKey:          "ScrollLock",
		AbsoluteSideButtonsViaRel: true,
		PointerMoveThrottleMs:     8,
		ConnectWindowMode:         "maximize",
	}
}

func loadPrefs() Preferences {
	path, err := prefsPath()
	if err != nil {
		return defaultPrefs()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultPrefs()
	}
	p := defaultPrefs()
	if err := json.Unmarshal(data, &p); err != nil {
		return defaultPrefs()
	}
	return p
}

func savePrefs(p Preferences) error {
	path, err := prefsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func prefsPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if root == "" {
		return "", errors.New("config directory unavailable")
	}
	return filepath.Join(root, "jetkvm-desktop", "preferences.json"), nil
}

func (p *Preferences) addRecent(url, name string) {
	now := time.Now()
	for i, rc := range p.RecentConnections {
		if rc.URL == url {
			p.RecentConnections[i].ConnectedAt = now
			if name != "" {
				p.RecentConnections[i].Name = name
			}
			p.sortRecents()
			return
		}
	}
	p.RecentConnections = append(p.RecentConnections, RecentConnection{
		URL:         url,
		Name:        name,
		ConnectedAt: now,
	})
	p.sortRecents()
	if len(p.RecentConnections) > maxRecentConnections {
		p.RecentConnections = p.RecentConnections[:maxRecentConnections]
	}
}

func (p *Preferences) removeRecent(url string) {
	p.RecentConnections = slices.DeleteFunc(p.RecentConnections, func(rc RecentConnection) bool {
		return rc.URL == url
	})
}

func (p *Preferences) sortRecents() {
	slices.SortFunc(p.RecentConnections, func(a, b RecentConnection) int {
		if a.ConnectedAt.After(b.ConnectedAt) {
			return -1
		}
		if a.ConnectedAt.Before(b.ConnectedAt) {
			return 1
		}
		return 0
	})
}
