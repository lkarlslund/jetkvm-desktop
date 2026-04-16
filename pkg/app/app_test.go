package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/lkarlslund/jetkvm-desktop/pkg/emulator"
	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
	"github.com/lkarlslund/jetkvm-desktop/pkg/ui"
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

func TestCurrentMouseButtonsMapsAllSupportedButtons(t *testing.T) {
	pressed := map[ebiten.MouseButton]bool{
		ebiten.MouseButtonLeft:   true,
		ebiten.MouseButtonRight:  true,
		ebiten.MouseButtonMiddle: true,
		ebiten.MouseButton3:      true,
		ebiten.MouseButton4:      true,
	}

	got := currentMouseButtons(func(button ebiten.MouseButton) bool {
		return pressed[button]
	})
	want := mouseButtonLeftMask | mouseButtonRightMask | mouseButtonMiddleMask | mouseButtonBackMask | mouseButtonForwardMask
	if got != want {
		t.Fatalf("currentMouseButtons(...) = %08b, want %08b", got, want)
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

func TestSettingsToggleRowUpdatesRuntimeStateOnClick(t *testing.T) {
	runtime := &ui.Runtime{}
	ctx := &ui.Context{
		Screen:   ebiten.NewImage(240, 80),
		Theme:    ui.DefaultTheme(),
		Runtime:  runtime,
		OnAction: func(string) {},
	}
	row := settingsToggleRowElement("toggle", "Example", settingsActionVisual{
		Enabled: true,
		Active:  false,
	})
	bounds := ui.Rect{X: 0, Y: 0, W: 220, H: 32}

	runtime.BeginFrame()
	row.Draw(ctx, bounds)
	point := ui.Point{X: 20, Y: 16}
	runtime.HandlePointer(point, true, true, false)
	runtime.HandlePointer(point, false, false, true)

	if !runtime.ToggleValue("toggle", false) {
		t.Fatal("expected settings toggle row to flip runtime toggle state on click")
	}
}

func TestSettingsAccessEditorModalConsumesBackdropClicks(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	app.accessEditor = accessEditorState{Mode: accessEditorModeUpdate}

	runtime := &ui.Runtime{}
	ctx := &ui.Context{
		Screen:   ebiten.NewImage(800, 600),
		Theme:    ui.DefaultTheme(),
		Runtime:  runtime,
		OnAction: app.invokeAction,
	}

	runtime.BeginFrame()
	settingsAccessEditorModalElement{app: app}.Draw(ctx, ui.Rect{X: 0, Y: 0, W: 800, H: 600})

	point := ui.Point{X: 20, Y: 20}
	if !runtime.HandlePointer(point, true, true, false) {
		t.Fatal("expected backdrop click to be consumed on press")
	}
	if !runtime.HandlePointer(point, false, false, true) {
		t.Fatal("expected backdrop click to be consumed on release")
	}
}

func TestSettingsATXModalConsumesBackdropClicks(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	app.atxConfirmAction = session.ATXPowerActionReset

	runtime := &ui.Runtime{}
	ctx := &ui.Context{
		Screen:   ebiten.NewImage(800, 600),
		Theme:    ui.DefaultTheme(),
		Runtime:  runtime,
		OnAction: app.invokeAction,
	}

	runtime.BeginFrame()
	settingsATXConfirmElement{app: app}.Draw(ctx, ui.Rect{X: 0, Y: 0, W: 800, H: 600})

	point := ui.Point{X: 20, Y: 20}
	if !runtime.HandlePointer(point, true, true, false) {
		t.Fatal("expected ATX backdrop click to be consumed on press")
	}
	if !runtime.HandlePointer(point, false, false, true) {
		t.Fatal("expected ATX backdrop click to be consumed on release")
	}
}

func TestShouldDismissOverlayOnOutsidePress(t *testing.T) {
	if shouldDismissOverlayOnOutsidePress("settings") {
		t.Fatal("settings overlay should not dismiss on outside click")
	}
	if !shouldDismissOverlayOnOutsidePress("media") {
		t.Fatal("media overlay should still dismiss on outside click")
	}
	if !shouldDismissOverlayOnOutsidePress("paste") {
		t.Fatal("paste overlay should still dismiss on outside click")
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

func TestNewDefaultsExperimentalUSBNetworkDisabled(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if app.cfg.ExperimentalUSBNetwork {
		t.Fatal("expected experimental USB network to default to disabled")
	}
}

func TestNewPreservesExperimentalUSBNetworkFlag(t *testing.T) {
	app, err := New(Config{ExperimentalUSBNetwork: true})
	if err != nil {
		t.Fatal(err)
	}
	if !app.cfg.ExperimentalUSBNetwork {
		t.Fatal("expected experimental USB network flag to be preserved")
	}
}

func TestPreferencesNormalizeChromeAnchor(t *testing.T) {
	prefs := Preferences{ChromeAnchor: ChromeAnchor(255)}
	prefs.normalize()
	if prefs.ChromeAnchor != chromeAnchorTopRight {
		t.Fatalf("chrome anchor = %v, want %v", prefs.ChromeAnchor, chromeAnchorTopRight)
	}
}

func TestPreferencesNormalizeChromeLayout(t *testing.T) {
	prefs := Preferences{ChromeLayout: ChromeLayout(255)}
	prefs.normalize()
	if prefs.ChromeLayout != chromeLayoutHorizontal {
		t.Fatalf("chrome layout = %v, want %v", prefs.ChromeLayout, chromeLayoutHorizontal)
	}
}

func TestPreferencesNormalizeThrottleMs(t *testing.T) {
	prefs := Preferences{ScrollThrottleMs: 999, PointerMoveThrottleMs: -2}
	prefs.normalize()
	if prefs.ScrollThrottleMs != maxScrollThrottleMs {
		t.Fatalf("scroll throttle ms = %d, want %d", prefs.ScrollThrottleMs, maxScrollThrottleMs)
	}
	if prefs.PointerMoveThrottleMs != 0 {
		t.Fatalf("pointer move throttle ms = %d, want 0", prefs.PointerMoveThrottleMs)
	}
}

func TestNormalizeWheelDeltaYRespectsInvertScroll(t *testing.T) {
	if got := normalizeWheelDeltaY(1, false); got != 1 {
		t.Fatalf("normalizeWheelDeltaY(1, false) = %d, want 1", got)
	}
	if got := normalizeWheelDeltaY(1, true); got != -1 {
		t.Fatalf("normalizeWheelDeltaY(1, true) = %d, want -1", got)
	}
	if got := normalizeWheelDeltaY(-2, false); got != -2 {
		t.Fatalf("normalizeWheelDeltaY(-2, false) = %d, want -2", got)
	}
}

func TestNormalizeWheelDeltaXRespectsInvertScroll(t *testing.T) {
	if got := normalizeWheelDeltaX(1, false); got != 1 {
		t.Fatalf("normalizeWheelDeltaX(1, false) = %d, want 1", got)
	}
	if got := normalizeWheelDeltaX(1, true); got != 1 {
		t.Fatalf("normalizeWheelDeltaX(1, true) = %d, want 1", got)
	}
	if got := normalizeWheelDeltaX(-2, false); got != -2 {
		t.Fatalf("normalizeWheelDeltaX(-2, false) = %d, want -2", got)
	}
	if got := normalizeWheelDeltaX(-2, true); got != -2 {
		t.Fatalf("normalizeWheelDeltaX(-2, true) = %d, want -2", got)
	}
}

func TestShouldThrottlePointerMovement(t *testing.T) {
	base := time.Unix(1000, 0)
	throttle := 8 * time.Millisecond
	if shouldThrottlePointerMovement(throttle, time.Time{}, base, true, false) {
		t.Fatal("expected first movement send to bypass throttle")
	}
	if !shouldThrottlePointerMovement(throttle, base, base.Add(throttle/2), true, false) {
		t.Fatal("expected movement-only send inside throttle window to be throttled")
	}
	if shouldThrottlePointerMovement(throttle, base, base.Add(throttle/2), true, true) {
		t.Fatal("expected button changes to bypass movement throttle")
	}
	if shouldThrottlePointerMovement(throttle, base, base.Add(throttle/2), false, false) {
		t.Fatal("expected unchanged pointer state to bypass throttle helper")
	}
	if shouldThrottlePointerMovement(throttle, base, base.Add(throttle), true, false) {
		t.Fatal("expected movement send at throttle boundary to proceed")
	}
}

func TestShouldSendRelativeMouseFlushesMovementOnButtonChange(t *testing.T) {
	base := time.Unix(1000, 0)
	throttle := 8 * time.Millisecond
	dx, dy, send := shouldSendRelativeMouse(10, 20, 18, 27, 0, mouseButtonLeftMask, throttle, base, base.Add(throttle/2))
	if !send {
		t.Fatal("expected button change to force an immediate relative mouse send")
	}
	if dx != 8 || dy != 7 {
		t.Fatalf("shouldSendRelativeMouse(...) = (%d, %d), want (8, 7)", dx, dy)
	}
}

func TestShouldSendRelativeMouseThrottlesMovementOnly(t *testing.T) {
	base := time.Unix(1000, 0)
	throttle := 8 * time.Millisecond
	dx, dy, send := shouldSendRelativeMouse(10, 20, 18, 27, 0, 0, throttle, base, base.Add(throttle/2))
	if send {
		t.Fatal("expected movement-only relative mouse send to be throttled")
	}
	if dx != 8 || dy != 7 {
		t.Fatalf("shouldSendRelativeMouse(...) = (%d, %d), want (8, 7)", dx, dy)
	}
}

func TestSavePreferencesUsesThrottleDurations(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	app.scrollThrottle = 50 * time.Millisecond
	app.pointerMoveThrottle = 24 * time.Millisecond
	app.savePreferences()

	if app.prefs.ScrollThrottleMs != 50 {
		t.Fatalf("prefs.ScrollThrottleMs = %d, want 50", app.prefs.ScrollThrottleMs)
	}
	if app.prefs.PointerMoveThrottleMs != 24 {
		t.Fatalf("prefs.PointerMoveThrottleMs = %d, want 24", app.prefs.PointerMoveThrottleMs)
	}
}

func TestVideoCodecH265ClickOpensConfirmation(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	app.sectionData.Video.State.Codec = session.VideoCodecH264
	app.openH265CodecConfirm()

	if !app.h265ConfirmOpen {
		t.Fatal("expected H265 confirmation to open")
	}
	if state := app.settingsAction(settingsGroupVideoCodec); state.Pending {
		t.Fatal("expected codec change to wait for confirmation")
	}
}

func TestVideoCodecH265CancelClosesConfirmation(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	app.h265ConfirmOpen = true
	app.h265ConfirmOpen = false

	if app.h265ConfirmOpen {
		t.Fatal("expected H265 confirmation to close")
	}
}

func TestVideoCodecH265ConfirmStartsCodecChange(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	app.ctrl = session.New(session.Config{})
	app.h265ConfirmOpen = true
	app.confirmH265CodecAction()

	if app.h265ConfirmOpen {
		t.Fatal("expected H265 confirmation to close on confirm")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state := app.settingsAction(settingsGroupVideoCodec)
		if !state.Pending {
			if state.Error == "" {
				t.Fatal("expected codec change to fail without a connected client")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for codec action to settle")
}

func TestDefaultPreferencesEnableAbsoluteSideButtonFallback(t *testing.T) {
	prefs := defaultPreferences()
	if !prefs.AbsoluteSideButtonsViaRel {
		t.Fatal("expected absolute side-button fallback to default on")
	}
}

func TestShouldRouteSideButtonsToRelative(t *testing.T) {
	app := &App{}
	app.prefs.AbsoluteSideButtonsViaRel = true
	app.hardwareConn.USBDevicesLoaded = true
	app.hardwareConn.USBDevices = session.USBDevices{
		AbsoluteMouse: true,
		RelativeMouse: true,
	}
	if !app.shouldRouteSideButtonsToRelative() {
		t.Fatal("expected side-button fallback to be enabled when both mouse gadgets are active")
	}

	app.relative = true
	if app.shouldRouteSideButtonsToRelative() {
		t.Fatal("expected side-button fallback to be disabled in relative mode")
	}
	app.relative = false

	app.hardwareConn.USBDevices.RelativeMouse = false
	if app.shouldRouteSideButtonsToRelative() {
		t.Fatal("expected side-button fallback to be disabled without relative mouse support")
	}
}

func TestShouldRouteSideButtonsToRelativeForRelease(t *testing.T) {
	app := &App{}
	app.prefs.AbsoluteSideButtonsViaRel = true
	app.hardwareConn.USBDevicesLoaded = true
	app.hardwareConn.USBDevices = session.USBDevices{
		AbsoluteMouse: true,
		RelativeMouse: true,
	}

	if !app.shouldRouteSideButtonsToRelativeFor(0, mouseButtonBackMask) {
		t.Fatal("expected side-button release to stay on relative routing")
	}
	if !app.shouldRouteSideButtonsToRelativeFor(0, mouseButtonForwardMask) {
		t.Fatal("expected forward-button release to stay on relative routing")
	}
	if app.shouldRouteSideButtonsToRelativeFor(0, 0) {
		t.Fatal("expected no routing when neither current nor last state contains side buttons")
	}
}

func TestValidateJigglerConfig(t *testing.T) {
	if err := validateJigglerConfig(standardJigglerConfig()); err != nil {
		t.Fatalf("standard config rejected: %v", err)
	}
	if err := validateJigglerConfig(session.JigglerConfig{}); err == nil {
		t.Fatal("expected empty jiggler config to fail validation")
	}
}

func TestChromeAnchorOrigin(t *testing.T) {
	x, y := chromeAnchorOrigin(chromeAnchorTopLeft, 1280, 720, 200, 34)
	if x != 18 || y != 18 {
		t.Fatalf("top_left origin = (%v,%v), want (18,18)", x, y)
	}
	x, y = chromeAnchorOrigin(chromeAnchorBottomCenter, 1280, 720, 200, 34)
	if x != 540 || y != 668 {
		t.Fatalf("bottom_center origin = (%v,%v), want (540,668)", x, y)
	}
	x, y = chromeAnchorOrigin(chromeAnchorRightCenter, 1280, 720, 200, 34)
	if x != 1062 || y != 343 {
		t.Fatalf("right_center origin = (%v,%v), want (1062,343)", x, y)
	}
}

func TestLayoutChromeButtonsVertical(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	app.prefs.ChromeAnchor = chromeAnchorTopLeft
	app.prefs.ChromeLayout = chromeLayoutVertical

	buttons := app.layoutChromeButtons(1280, 720, session.Snapshot{Phase: session.PhaseConnected})
	if len(buttons) != 5 {
		t.Fatalf("button count = %d, want 5", len(buttons))
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

	app.prefs.ChromeAnchor = chromeAnchorLeftCenter
	app.prefs.ChromeLayout = chromeLayoutVertical
	leftZone := app.chromeRevealZone(1280, 720, session.Snapshot{Phase: session.PhaseConnected})
	if !leftZone.contains(30, 360) {
		t.Fatalf("expected left-center hot zone to include left-side cursor, got %+v", leftZone)
	}
	if leftZone.contains(640, 20) {
		t.Fatalf("expected left-center hot zone to exclude top-center cursor, got %+v", leftZone)
	}

	app.prefs.ChromeAnchor = chromeAnchorTopRight
	app.prefs.ChromeLayout = chromeLayoutHorizontal
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

	_ = app.keyboard.Update([]input.Key{input.KeyEscape, input.KeyShiftLeft}, time.Unix(0, 0))
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

func TestCloseMediaOverlayArmsDismissSuppression(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	app.mediaOpen = true
	app.closeMediaOverlay()

	if app.mediaOpen {
		t.Fatal("expected media overlay to close")
	}
	if !app.suppressMouseUntilUp {
		t.Fatal("expected mouse suppression after closing media")
	}
	if !app.suppressKeysUntilClear {
		t.Fatal("expected keyboard suppression after closing media")
	}
}

func TestIsValidMediaURL(t *testing.T) {
	if !isValidMediaURL("https://example.com/test.iso") {
		t.Fatal("expected valid media URL")
	}
	for _, value := range []string{"", "/relative/path.iso", "example.com/test.iso"} {
		if isValidMediaURL(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestNewLauncherStartsInBrowseMode(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if !app.launcherOpen {
		t.Fatal("expected launcher to be open without a target")
	}
	if app.launcherMode != launcherModeBrowse {
		t.Fatalf("launcher mode = %v, want %v", app.launcherMode, launcherModeBrowse)
	}
}

func TestConnectFromLauncherLeavesLauncherWhileConnecting(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	app.launcherOpen = true
	app.launcherMode = launcherModePassword
	app.launcherPassword = "secret"

	app.connectFromLauncher("192.168.1.50")

	if app.launcherOpen {
		t.Fatal("expected launcher to close while connecting")
	}
	if app.pendingTarget != "http://192.168.1.50" {
		t.Fatalf("pending target = %q, want normalized target", app.pendingTarget)
	}
}

func TestShowPasswordPromptSwitchesLauncherMode(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	app.launcherOpen = false
	app.launcherMode = launcherModeBrowse
	app.showPasswordPrompt("http://jetkvm.local", "Password required for http://jetkvm.local")

	if !app.launcherOpen {
		t.Fatal("expected launcher to open")
	}
	if app.launcherMode != launcherModePassword {
		t.Fatalf("launcher mode = %v, want %v", app.launcherMode, launcherModePassword)
	}
	if app.pendingTarget != "http://jetkvm.local" {
		t.Fatalf("pending target = %q, want http://jetkvm.local", app.pendingTarget)
	}
}

func TestAuthPromptError(t *testing.T) {
	if got := authPromptError(""); got != "Authentication failed" {
		t.Fatalf("empty error = %q, want Authentication failed", got)
	}
	if got := authPromptError("login failed with status 401 Unauthorized"); got != "login failed with status 401 Unauthorized" {
		t.Fatalf("unexpected auth prompt error %q", got)
	}
}

func TestAppPasswordRetryFlowConnects(t *testing.T) {
	srv, ctx, cancel := startAppEmulator(t)
	defer cancel()

	app, err := New(Config{BaseURL: srv.BaseURL(), RPCTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	app.Start(ctx)
	defer func() {
		if app.ctrl != nil {
			app.ctrl.Stop()
		}
	}()

	waitForAppPhase(t, app, session.PhaseAuthFailed, 5*time.Second)
	app.syncSessionState()
	if !app.launcherOpen {
		t.Fatal("expected launcher to open after auth failure")
	}
	if app.launcherMode != launcherModePassword {
		t.Fatalf("launcher mode = %v, want %v", app.launcherMode, launcherModePassword)
	}

	app.launcherPassword = "secret"
	app.connectFromLauncher(app.pendingTarget)
	if app.launcherOpen {
		t.Fatal("expected launcher to close while retrying with password")
	}

	waitForAppPhase(t, app, session.PhaseConnected, 5*time.Second)
	app.syncSessionState()
	if app.launcherOpen {
		t.Fatal("expected launcher to remain closed after successful password retry")
	}
}

func TestAppCLIConfigPasswordConnectsWithoutPrompt(t *testing.T) {
	srv, ctx, cancel := startAppEmulator(t)
	defer cancel()

	app, err := New(Config{
		BaseURL:    srv.BaseURL(),
		Password:   "secret",
		RPCTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	app.Start(ctx)
	defer func() {
		if app.ctrl != nil {
			app.ctrl.Stop()
		}
	}()

	waitForAppPhase(t, app, session.PhaseConnected, 5*time.Second)
	app.syncSessionState()
	if app.launcherOpen {
		t.Fatal("expected launcher to remain closed when config password succeeds")
	}
}

func TestAppWrongPasswordReturnsToPasswordPromptWithError(t *testing.T) {
	srv, ctx, cancel := startAppEmulator(t)
	defer cancel()

	app, err := New(Config{BaseURL: srv.BaseURL(), RPCTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	app.Start(ctx)
	defer func() {
		if app.ctrl != nil {
			app.ctrl.Stop()
		}
	}()

	waitForAppPhase(t, app, session.PhaseAuthFailed, 5*time.Second)
	app.syncSessionState()
	app.launcherPassword = "wrongpass"
	app.connectFromLauncher(app.pendingTarget)

	if app.launcherOpen {
		t.Fatal("expected launcher to close while retrying wrong password")
	}

	waitForAppPhase(t, app, session.PhaseAuthFailed, 5*time.Second)
	app.syncSessionState()
	if !app.launcherOpen {
		t.Fatal("expected password prompt to reopen after auth failure")
	}
	if app.launcherMode != launcherModePassword {
		t.Fatalf("launcher mode = %v, want %v", app.launcherMode, launcherModePassword)
	}
	if app.launcherError == "" {
		t.Fatal("expected auth error to be shown in password prompt")
	}
}

func TestLoadATXSection(t *testing.T) {
	srv, ctx, cancel := startAppEmulator(t)
	defer cancel()
	srv.SetActiveExtension("atx-power")
	srv.SetATXState(true, true)

	app, err := New(Config{
		BaseURL:    srv.BaseURL(),
		Password:   "secret",
		RPCTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	app.Start(ctx)
	defer func() {
		if app.ctrl != nil {
			app.ctrl.Stop()
		}
	}()

	waitForAppPhase(t, app, session.PhaseConnected, 5*time.Second)
	seq := app.markSettingsSectionLoading(sectionATX)
	if err := app.loadSettingsSection(sectionATX, seq); err != nil {
		t.Fatalf("loadSettingsSection(sectionATX) returned error: %v", err)
	}

	state := app.sectionData.ATX
	if state.Loading {
		t.Fatal("expected ATX section load to complete")
	}
	if state.ActiveExtension != "atx-power" {
		t.Fatalf("active extension = %q, want atx-power", state.ActiveExtension)
	}
	if state.State == nil || !state.State.Power || !state.State.HDD {
		t.Fatalf("ATX state = %+v, want power=true hdd=true", state.State)
	}
}

func startAppEmulator(t *testing.T) (*emulator.Server, context.Context, context.CancelFunc) {
	t.Helper()
	srv, err := emulator.NewServer(emulator.Config{
		ListenAddr: "127.0.0.1:0",
		AuthMode:   emulator.AuthModePassword,
		Password:   "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil && ctx.Err() == nil {
				t.Errorf("server: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("server did not shut down")
		}
	})
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for srv.BaseURL() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.BaseURL() == "" {
		t.Fatal("server did not start")
	}
	return srv, ctx, cancel
}

func waitForAppPhase(t *testing.T, app *App, phase session.Phase, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if app.ctrl != nil && app.ctrl.Snapshot().Phase == phase {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	if app.ctrl == nil {
		t.Fatalf("timed out waiting for phase %v: controller is nil", phase)
	}
	t.Fatalf("timed out waiting for phase %v, got %+v", phase, app.ctrl.Snapshot())
}
