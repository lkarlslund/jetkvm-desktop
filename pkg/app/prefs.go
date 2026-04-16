package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

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
	AbsoluteSideButtonsViaRel bool           `json:"absolute_side_buttons_via_relative"`
	ScrollThrottle            ScrollThrottle `json:"scroll_throttle"`
	ScrollThrottleMs          int            `json:"scroll_throttle_ms,omitempty"`
	PointerMoveThrottleMs     int            `json:"pointer_move_throttle_ms,omitempty"`
}

//go:generate go tool github.com/dmarkham/enumer -type=Theme,ChromeAnchor,ChromeLayout,ScrollThrottle -linecomment -json -text -output prefs_enums.go

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
		AbsoluteSideButtonsViaRel: true,
		ScrollThrottle:            scrollThrottleOff,
		ScrollThrottleMs:          0,
		PointerMoveThrottleMs:     8,
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
	switch p.ChromeLayout {
	case chromeLayoutHorizontal, chromeLayoutVertical:
	default:
		p.ChromeLayout = chromeLayoutHorizontal
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
