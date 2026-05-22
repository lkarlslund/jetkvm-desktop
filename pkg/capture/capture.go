package capture

// KeyEvent describes a key press/release captured at the OS level.
type KeyEvent struct {
	Keysym  uint32
	Pressed bool
}

// KeyCallback is invoked for each key event during capture.
// Return true to indicate the event was consumed (e.g. the capture toggle key).
type KeyCallback func(KeyEvent) bool

// Grabber captures all keyboard and mouse input at the OS level,
// preventing the host from reacting to key combinations like Alt+Tab
// while forwarding them through the normal Ebiten event pipeline to
// the KVM target.
type Grabber interface {
	// IsSupported reports whether total capture is available on this
	// platform/session (e.g. false on Wayland where X11 grabs don't work).
	IsSupported() bool
	// Grab activates exclusive input capture.
	// If cb is non-nil, key events are delivered via the callback instead
	// of being forwarded to the focused window via XSendEvent.
	Grab() error
	// GrabWithCallback is like Grab but delivers key events via cb.
	GrabWithCallback(cb KeyCallback) error
	// Release deactivates input capture and restores normal host behaviour.
	Release() error
	// IsGrabbed reports whether input capture is currently active.
	IsGrabbed() bool
	// PlatformNote returns a short user-facing string describing any
	// platform-specific limitations (empty if none).
	PlatformNote() string
}
