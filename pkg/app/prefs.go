package app

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

type Preferences struct {
	Theme                     Theme          `json:"theme"`
	PinChrome                 bool           `json:"pin_chrome"`
	HideHeaderBar             bool           `json:"hide_header_bar"`
	HideStatusBar             bool           `json:"hide_status_bar"`
	ChromeAnchor              ChromeAnchor   `json:"chrome_anchor"`
	ChromeLayout              ChromeLayout   `json:"chrome_layout"`
	HideCursor                bool           `json:"hide_cursor"`
	InvertScroll              bool           `json:"invert_scroll"`
	ShowPressedKeys           bool           `json:"show_pressed_keys"`
	ExperimentalGlobalHotkeys bool           `json:"experimental_global_hotkeys"`
	AbsoluteSideButtonsViaRel bool           `json:"absolute_side_buttons_via_relative"`
	ScrollThrottle            ScrollThrottle `json:"scroll_throttle"`
	ScrollThrottleMs          int                `json:"scroll_throttle_ms,omitempty"`
	PointerMoveThrottleMs     int                `json:"pointer_move_throttle_ms,omitempty"`
	CaptureToggleKey          string             `json:"capture_toggle_key,omitempty"`
	ConnectWindowMode         ConnectWindowMode  `json:"connect_window_mode,omitempty"`
	ChromeCustomX             float64            `json:"chrome_custom_x,omitempty"`
	ChromeCustomY             float64            `json:"chrome_custom_y,omitempty"`
	ChromeCustomPos           bool               `json:"chrome_custom_pos,omitempty"`
	RecentConnections         []RecentConnection `json:"recent_connections,omitempty"`
}

//go:generate go tool github.com/dmarkham/enumer -type=Theme,ChromeAnchor,ChromeLayout,ScrollThrottle,ConnectWindowMode -linecomment -json -text -output prefs_enums.go

type Theme uint8

const (
	themeUnknown Theme = iota // unknown
	themeSystem               // system
	themeDark                 // dark
	themeLight                // light
)

type ChromeAnchor uint8

const (
	chromeAnchorUnknown      ChromeAnchor = iota // unknown
	chromeAnchorTopLeft                          // top_left
	chromeAnchorTopCenter                        // top_center
	chromeAnchorTopRight                         // top_right
	chromeAnchorLeftCenter                       // left_center
	chromeAnchorRightCenter                      // right_center
	chromeAnchorBottomLeft                       // bottom_left
	chromeAnchorBottomCenter                     // bottom_center
	chromeAnchorBottomRight                      // bottom_right
)

type ChromeLayout uint8

const (
	chromeLayoutUnknown    ChromeLayout = iota // unknown
	chromeLayoutHorizontal                     // horizontal
	chromeLayoutVertical                       // vertical
)

type ScrollThrottle uint8

const (
	scrollThrottleUnknown ScrollThrottle = iota // unknown
	scrollThrottleOff                           // 0
	scrollThrottle10ms                          // 10
	scrollThrottle25ms                          // 25
	scrollThrottle50ms                          // 50
	scrollThrottle100ms                         // 100
)

type ConnectWindowMode uint8

const (
	connectWindowUnchanged   ConnectWindowMode = iota // unchanged
	connectWindowMaximize                             // maximize
	connectWindowPixelRatio                           // pixel_ratio
	connectWindowFullscreen                           // fullscreen
)

func defaultPreferences() Preferences {
	return Preferences{
		Theme:                     themeSystem,
		PinChrome:                 false,
		HideHeaderBar:             false,
		HideStatusBar:             false,
		ChromeAnchor:              chromeAnchorTopRight,
		ChromeLayout:              chromeLayoutHorizontal,
		HideCursor:                false,
		InvertScroll:              false,
		ShowPressedKeys:           false,
		ExperimentalGlobalHotkeys: false,
		AbsoluteSideButtonsViaRel: true,
		ScrollThrottle:            scrollThrottleOff,
		ScrollThrottleMs:          0,
		PointerMoveThrottleMs:     8,
		ConnectWindowMode:         connectWindowMaximize,
	}
}

func loadPreferences() Preferences {
	path, err := preferencesPath()
	if err != nil {
		return defaultPreferences()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultPreferences()
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return defaultPreferences()
	}
	var prefs Preferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return defaultPreferences()
	}
	if _, ok := raw["absolute_side_buttons_via_relative"]; !ok {
		prefs.AbsoluteSideButtonsViaRel = true
	}
	if _, ok := raw["scroll_throttle_ms"]; !ok {
		prefs.ScrollThrottleMs = int(scrollThrottleFromPref(prefs.ScrollThrottle) / time.Millisecond)
	}
	if _, ok := raw["pointer_move_throttle_ms"]; !ok {
		prefs.PointerMoveThrottleMs = defaultPointerMoveThrottleMs
	}
	if _, ok := raw["connect_window_mode"]; !ok {
		prefs.ConnectWindowMode = connectWindowMaximize
	}
	prefs.normalize()
	return prefs
}

func savePreferences(prefs Preferences) error {
	path, err := preferencesPath()
	if err != nil {
		return err
	}
	prefs.normalize()
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func preferencesPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if root == "" {
		return "", errors.New("config directory unavailable")
	}
	return filepath.Join(root, "jetkvm-desktop", "preferences.json"), nil
}

func (p *Preferences) normalize() {
	if p.Theme == themeUnknown {
		p.Theme = themeSystem
	}
	switch p.ScrollThrottle {
	case scrollThrottleOff, scrollThrottle10ms, scrollThrottle25ms, scrollThrottle50ms, scrollThrottle100ms:
	default:
		p.ScrollThrottle = scrollThrottleOff
	}
	p.ScrollThrottleMs = clampInt(p.ScrollThrottleMs, 0, maxScrollThrottleMs)
	p.PointerMoveThrottleMs = clampInt(p.PointerMoveThrottleMs, 0, maxPointerMoveThrottleMs)
	switch p.ChromeAnchor {
	case chromeAnchorTopLeft, chromeAnchorTopCenter, chromeAnchorTopRight, chromeAnchorLeftCenter, chromeAnchorRightCenter, chromeAnchorBottomLeft, chromeAnchorBottomCenter, chromeAnchorBottomRight:
	default:
		p.ChromeAnchor = chromeAnchorTopRight
	}
	if !isValidCaptureToggleKey(p.CaptureToggleKey) {
		p.CaptureToggleKey = defaultCaptureToggleKey
	}
	switch p.ChromeLayout {
	case chromeLayoutHorizontal, chromeLayoutVertical:
	default:
		p.ChromeLayout = chromeLayoutHorizontal
	}
	switch p.ConnectWindowMode {
	case connectWindowUnchanged, connectWindowMaximize, connectWindowPixelRatio, connectWindowFullscreen:
	default:
		p.ConnectWindowMode = connectWindowMaximize
	}
}

func scrollThrottleFromPref(value ScrollThrottle) time.Duration {
	switch value {
	case scrollThrottle10ms:
		return 10 * time.Millisecond
	case scrollThrottle25ms:
		return 25 * time.Millisecond
	case scrollThrottle50ms:
		return 50 * time.Millisecond
	case scrollThrottle100ms:
		return 100 * time.Millisecond
	default:
		return 0
	}
}

func scrollThrottlePref(value time.Duration) ScrollThrottle {
	switch value {
	case 10 * time.Millisecond:
		return scrollThrottle10ms
	case 25 * time.Millisecond:
		return scrollThrottle25ms
	case 50 * time.Millisecond:
		return scrollThrottle50ms
	case 100 * time.Millisecond:
		return scrollThrottle100ms
	default:
		return scrollThrottleOff
	}
}

func throttleDurationFromMs(value int) time.Duration {
	return time.Duration(value) * time.Millisecond
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func (p *Preferences) addRecentConnection(url, name string) {
	now := time.Now()
	for i, rc := range p.RecentConnections {
		if rc.URL == url {
			p.RecentConnections[i].ConnectedAt = now
			if name != "" {
				p.RecentConnections[i].Name = name
			}
			p.sortRecentConnections()
			return
		}
	}
	p.RecentConnections = append(p.RecentConnections, RecentConnection{
		URL:         url,
		Name:        name,
		ConnectedAt: now,
	})
	p.sortRecentConnections()
	if len(p.RecentConnections) > maxRecentConnections {
		p.RecentConnections = p.RecentConnections[:maxRecentConnections]
	}
}

func (p *Preferences) removeRecentConnection(url string) {
	p.RecentConnections = slices.DeleteFunc(p.RecentConnections, func(rc RecentConnection) bool {
		return rc.URL == url
	})
}

func (p *Preferences) sortRecentConnections() {
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


const defaultCaptureToggleKey = "ScrollLock"

var allowedCaptureToggleKeys = []string{
	"F1", "F2", "F3", "F4", "F5", "F6",
	"F7", "F8", "F9", "F10", "F11", "F12",
	"Pause", "ScrollLock",
}

func isValidCaptureToggleKey(key string) bool {
	for _, k := range allowedCaptureToggleKeys {
		if k == key {
			return true
		}
	}
	return false
}
