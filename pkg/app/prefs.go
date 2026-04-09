package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Preferences struct {
	Theme           string `json:"theme"`
	PinChrome       bool   `json:"pin_chrome"`
	ChromeAnchor    string `json:"chrome_anchor"`
	ChromeLayout    string `json:"chrome_layout"`
	HideCursor      bool   `json:"hide_cursor"`
	ShowPressedKeys bool   `json:"show_pressed_keys"`
	ScrollThrottle  string `json:"scroll_throttle"`
}

func defaultPreferences() Preferences {
	return Preferences{
		Theme:           "dark",
		PinChrome:       false,
		ChromeAnchor:    "top_right",
		ChromeLayout:    "horizontal",
		HideCursor:      false,
		ShowPressedKeys: false,
		ScrollThrottle:  "0",
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
	var prefs Preferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return defaultPreferences()
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
	if p.Theme == "" {
		p.Theme = "dark"
	}
	switch p.ScrollThrottle {
	case "0", "10", "25", "50", "100":
	default:
		p.ScrollThrottle = "0"
	}
	switch p.ChromeAnchor {
	case "top_left", "top_center", "top_right", "left_center", "right_center", "bottom_left", "bottom_center", "bottom_right":
	default:
		p.ChromeAnchor = "top_right"
	}
	switch p.ChromeLayout {
	case "horizontal", "vertical":
	default:
		p.ChromeLayout = "horizontal"
	}
}

func scrollThrottleFromPref(value string) time.Duration {
	switch value {
	case "10":
		return 10 * time.Millisecond
	case "25":
		return 25 * time.Millisecond
	case "50":
		return 50 * time.Millisecond
	case "100":
		return 100 * time.Millisecond
	default:
		return 0
	}
}

func scrollThrottlePref(value time.Duration) string {
	switch value {
	case 10 * time.Millisecond:
		return "10"
	case 25 * time.Millisecond:
		return "25"
	case 50 * time.Millisecond:
		return "50"
	case 100 * time.Millisecond:
		return "100"
	default:
		return "0"
	}
}
