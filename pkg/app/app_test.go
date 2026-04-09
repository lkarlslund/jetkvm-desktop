package app

import (
	"errors"
	"testing"

	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "192.168.1.50", want: "http://192.168.1.50"},
		{in: "jetkvm.local", want: "http://jetkvm.local"},
		{in: "https://jetkvm.local/view", want: "https://jetkvm.local"},
	}

	for _, tc := range tests {
		got, err := normalizeBaseURL(tc.in)
		if err != nil {
			t.Fatalf("normalizeBaseURL(%q) returned error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("normalizeBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeBaseURLRejectsEmpty(t *testing.T) {
	if _, err := normalizeBaseURL(""); err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestNormalizeBaseURLRejectsInvalidHost(t *testing.T) {
	for _, value := range []string{"bad host", "://broken", "http://bad host"} {
		if _, err := normalizeBaseURL(value); err == nil {
			t.Fatalf("expected error for %q", value)
		}
	}
}

func TestIsValidConnectHost(t *testing.T) {
	valid := []string{"192.168.1.50", "jetkvm.local", "jetkvm-22fef15037dbb5bb.isobits.local"}
	for _, value := range valid {
		if !isValidConnectHost(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}

	invalid := []string{"", "bad host", "-jetkvm.local", "jetkvm_.local", "foo/bar"}
	for _, value := range invalid {
		if isValidConnectHost(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestSettingsActionLifecycle(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	seq := app.beginSettingsAction(settingsGroupKeyboardLayout, "da_DK")
	state := app.settingsAction(settingsGroupKeyboardLayout)
	if !state.Pending {
		t.Fatal("expected action to be pending")
	}
	if state.PendingChoice != "da_DK" {
		t.Fatalf("pending choice = %q, want da_DK", state.PendingChoice)
	}
	if seq == 0 {
		t.Fatal("expected non-zero request sequence")
	}

	app.finishSettingsAction(settingsGroupKeyboardLayout, seq, nil)
	state = app.settingsAction(settingsGroupKeyboardLayout)
	if state.Pending {
		t.Fatal("expected action to settle")
	}
	if state.Error != "" {
		t.Fatalf("expected no error after success, got %q", state.Error)
	}
	if state.PendingChoice != "" {
		t.Fatalf("expected pending choice to clear, got %q", state.PendingChoice)
	}
}

func TestSettingsActionIgnoresStaleCompletion(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	first := app.beginSettingsAction(settingsGroupKeyboardLayout, "da_DK")
	second := app.beginSettingsAction(settingsGroupKeyboardLayout, "en_US")

	app.finishSettingsAction(settingsGroupKeyboardLayout, first, errors.New("stale failure"))
	state := app.settingsAction(settingsGroupKeyboardLayout)
	if !state.Pending {
		t.Fatal("expected stale completion to leave newer request pending")
	}
	if state.PendingChoice != "en_US" {
		t.Fatalf("pending choice = %q, want en_US", state.PendingChoice)
	}
	if state.Error != "" {
		t.Fatalf("expected stale error to be ignored, got %q", state.Error)
	}

	app.finishSettingsAction(settingsGroupKeyboardLayout, second, errors.New("latest failure"))
	state = app.settingsAction(settingsGroupKeyboardLayout)
	if state.Pending {
		t.Fatal("expected latest request to settle")
	}
	if state.Error != "latest failure" {
		t.Fatalf("error = %q, want latest failure", state.Error)
	}
}

func TestSectionLoadSeqMonotonic(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	first := app.markSettingsSectionLoading(sectionAccess)
	second := app.markSettingsSectionLoading(sectionAccess)
	if second <= first {
		t.Fatalf("expected section sequence to increase, got %d then %d", first, second)
	}
	if !app.sectionData.Access.Loading {
		t.Fatal("expected section to be marked loading")
	}
}

func TestPreferencesNormalizeChromeAnchor(t *testing.T) {
	prefs := Preferences{ChromeAnchor: "bad_anchor"}
	prefs.normalize()
	if prefs.ChromeAnchor != "top_right" {
		t.Fatalf("chrome anchor = %q, want top_right", prefs.ChromeAnchor)
	}
}

func TestPreferencesNormalizeChromeLayout(t *testing.T) {
	prefs := Preferences{ChromeLayout: "diagonal"}
	prefs.normalize()
	if prefs.ChromeLayout != "horizontal" {
		t.Fatalf("chrome layout = %q, want horizontal", prefs.ChromeLayout)
	}
}

func TestChromeAnchorOrigin(t *testing.T) {
	x, y := chromeAnchorOrigin("top_left", 1280, 720, 200, 34)
	if x != 18 || y != 18 {
		t.Fatalf("top_left origin = (%v,%v), want (18,18)", x, y)
	}
	x, y = chromeAnchorOrigin("bottom_center", 1280, 720, 200, 34)
	if x != 540 || y != 668 {
		t.Fatalf("bottom_center origin = (%v,%v), want (540,668)", x, y)
	}
	x, y = chromeAnchorOrigin("right_center", 1280, 720, 200, 34)
	if x != 1062 || y != 343 {
		t.Fatalf("right_center origin = (%v,%v), want (1062,343)", x, y)
	}
}

func TestLayoutChromeButtonsVertical(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	app.prefs.ChromeAnchor = "top_left"
	app.prefs.ChromeLayout = "vertical"

	buttons := app.layoutChromeButtons(1280, 720, session.Snapshot{Phase: session.PhaseConnected})
	if len(buttons) != 4 {
		t.Fatalf("button count = %d, want 4", len(buttons))
	}
	if buttons[0].rect.x != buttons[1].rect.x {
		t.Fatalf("expected vertical buttons to share x, got %v and %v", buttons[0].rect.x, buttons[1].rect.x)
	}
	if buttons[0].rect.y >= buttons[1].rect.y {
		t.Fatalf("expected later button to be lower, got %v then %v", buttons[0].rect.y, buttons[1].rect.y)
	}
}

func TestChromeRevealZoneTracksAnchor(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	app.prefs.ChromeAnchor = "left_center"
	app.prefs.ChromeLayout = "vertical"
	leftZone := app.chromeRevealZone(1280, 720, session.Snapshot{Phase: session.PhaseConnected})
	if !leftZone.contains(30, 360) {
		t.Fatalf("expected left-center hot zone to include left-side cursor, got %+v", leftZone)
	}
	if leftZone.contains(640, 20) {
		t.Fatalf("expected left-center hot zone to exclude top-center cursor, got %+v", leftZone)
	}

	app.prefs.ChromeAnchor = "top_right"
	app.prefs.ChromeLayout = "horizontal"
	topZone := app.chromeRevealZone(1280, 720, session.Snapshot{Phase: session.PhaseConnected})
	if !topZone.contains(1200, 30) {
		t.Fatalf("expected top-right hot zone to include top-right cursor, got %+v", topZone)
	}
	if topZone.contains(30, 360) {
		t.Fatalf("expected top-right hot zone to exclude left-center cursor, got %+v", topZone)
	}
}

func TestArmOverlayDismissSuppression(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	_ = app.keyboard.Update([]input.Key{input.KeyEscape, input.KeyShiftLeft})
	app.lastButtons = 1
	app.armOverlayDismissSuppression()

	if !app.suppressKeysUntilClear {
		t.Fatal("expected keyboard suppression to be armed")
	}
	if !app.suppressMouseUntilUp {
		t.Fatal("expected mouse suppression to be armed")
	}
	if app.lastButtons != 0 {
		t.Fatalf("lastButtons = %d, want 0", app.lastButtons)
	}
	if pressed := app.keyboard.Pressed(); len(pressed) != 0 {
		t.Fatalf("expected local keyboard state to clear, got %v", pressed)
	}
}

func TestCloseSettingsOverlayArmsDismissSuppression(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	app.settingsOpen = true
	app.closeSettingsOverlay()

	if app.settingsOpen {
		t.Fatal("expected settings overlay to close")
	}
	if !app.suppressMouseUntilUp {
		t.Fatal("expected mouse suppression after closing settings")
	}
	if !app.suppressKeysUntilClear {
		t.Fatal("expected keyboard suppression after closing settings")
	}
}
