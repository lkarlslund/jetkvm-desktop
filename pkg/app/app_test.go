package app

import (
	"errors"
	"testing"
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
