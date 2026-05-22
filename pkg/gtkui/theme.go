package gtkui

import (
	"fmt"
	"os"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type themeColors struct {
	panelBg   string
	panelFg   string
	border    string
	sidebarBg string
	accent    string
	accentFg  string
	dimLabel  string
	warnBg    string
}

var darkColors = themeColors{
	panelBg:   "#2d2d2d",
	panelFg:   "#e0e0e0",
	border:    "#555555",
	sidebarBg: "#262626",
	accent:    "#3584e4",
	accentFg:  "#ffffff",
	dimLabel:  "#999999",
	warnBg:    "#cd9309",
}

var lightColors = themeColors{
	panelBg:   "#f6f5f4",
	panelFg:   "#2e3436",
	border:    "#c0bfbc",
	sidebarBg: "#eeedeb",
	accent:    "#1c71d8",
	accentFg:  "#ffffff",
	dimLabel:  "#77767b",
	warnBg:    "#e5a50a",
}

func buildCSS(c themeColors) string {
	return fmt.Sprintf(`
/* Status bar */
.status-bar-box {
    background: alpha(black, 0.45);
    padding: 2px 14px;
    min-height: 18px;
}
.status-bar {
    color: alpha(white, 0.85);
    font-size: 12px;
}

/* Chrome toolbar */
.chrome-bar {
    background: alpha(black, 0.65);
    border-radius: 6px;
    padding: 4px;
}
.chrome-btn {
    color: white;
    min-width: 32px;
    min-height: 32px;
}
.chrome-handle {
    padding: 4px;
    margin: 2px;
}

/* 10-second settings hint */
.settings-hint {
    background: alpha(black, 0.55);
    color: white;
    border-radius: 50%%;
    min-width: 34px;
    min-height: 34px;
    margin: 16px;
}

/* Overlay panels (paste, media, serial, wol, stats) */
.overlay-panel {
    background-color: %[1]s;
    color: %[2]s;
    border-radius: 10px;
    padding: 20px;
    margin: 32px;
    border: 1px solid %[3]s;
    box-shadow: 0 4px 16px alpha(black, 0.35);
}
.overlay-panel label { color: %[2]s; }
.overlay-panel .dim-label { color: %[7]s; }
.overlay-panel entry, .overlay-panel textview {
    background-color: %[4]s;
    color: %[2]s;
}

/* Settings */
.settings-panel {
    background-color: %[1]s;
    color: %[2]s;
    border: 1px solid %[3]s;
    border-radius: 10px;
    padding: 12px;
    margin: 24px;
    box-shadow: 0 4px 16px alpha(black, 0.35);
}
.settings-panel label { color: %[2]s; }
.settings-panel .dim-label { color: %[7]s; }
.settings-panel entry {
    background-color: %[4]s;
    color: %[2]s;
}

/* Settings sidebar */
.settings-panel stacksidebar list {
    background-color: %[4]s;
    border-right: 1px solid %[3]s;
}
.settings-panel stacksidebar row:selected {
    background-color: %[5]s;
    color: %[6]s;
}
.settings-panel stacksidebar row:selected label {
    color: %[6]s;
}

/* Active toggle button highlight */
togglebutton:checked {
    background-color: %[5]s;
    color: %[6]s;
}
togglebutton:checked label {
    color: %[6]s;
}

/* Paste-in-progress banner */
.paste-banner {
    background: alpha(%[5]s, 0.92);
    color: white;
    border-radius: 8px;
    padding: 10px 20px;
    margin: 48px;
    min-width: 300px;
}

/* Backdrop behind overlay panels */
.overlay-backdrop {
    background-color: alpha(black, 0.6);
}

/* Connection banner */
.connection-banner {
    background: alpha(%[8]s, 0.9);
    border-radius: 6px;
    padding: 12px 16px;
    margin: 8px;
}

/* Pressed keys HUD */
.pressed-keys {
    background: alpha(black, 0.7);
    color: white;
    border-radius: 4px;
    padding: 4px 8px;
    font-size: 11px;
}
`, c.panelBg, c.panelFg, c.border, c.sidebarBg,
		c.accent, c.accentFg, c.dimLabel, c.warnBg)
}

func resolveEffectiveTheme(pref string) string {
	switch pref {
	case "dark", "light":
		return pref
	default:
		if isSystemDark() {
			return "dark"
		}
		return "light"
	}
}

func isSystemDark() bool {
	for _, key := range []string{"GTK_THEME", "XDG_CURRENT_DESKTOP"} {
		v := strings.ToLower(os.Getenv(key))
		if strings.Contains(v, "dark") {
			return true
		}
	}
	settings := gtk.SettingsGetDefault()
	if settings != nil {
		val := settings.ObjectProperty("gtk-application-prefer-dark-theme")
		if b, ok := val.(bool); ok && b {
			return true
		}
		theme := settings.ObjectProperty("gtk-theme-name")
		if s, ok := theme.(string); ok && strings.Contains(strings.ToLower(s), "dark") {
			return true
		}
	}
	return false
}

// applyTheme loads the CSS and configures dark/light preference.
func applyTheme(prefs Preferences) {
	effective := resolveEffectiveTheme(prefs.Theme)
	colors := darkColors
	if effective == "light" {
		colors = lightColors
	}

	provider := gtk.NewCSSProvider()
	provider.LoadFromString(buildCSS(colors))
	gtk.StyleContextAddProviderForDisplay(
		gdk.DisplayGetDefault(),
		provider,
		gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
	)

	settings := gtk.SettingsGetDefault()
	if settings == nil {
		return
	}
	switch prefs.Theme {
	case "dark":
		settings.SetObjectProperty("gtk-application-prefer-dark-theme", true)
	case "light":
		settings.SetObjectProperty("gtk-application-prefer-dark-theme", false)
	default:
		settings.SetObjectProperty("gtk-application-prefer-dark-theme", effective == "dark")
	}
}
