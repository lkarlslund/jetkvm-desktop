package capture

// Grabber captures all keyboard and mouse input at the OS level,
// preventing the host from reacting to key combinations like Alt+Tab
// while forwarding them through the normal Ebiten event pipeline to
// the KVM target.
type Grabber interface {
	// IsSupported reports whether total capture is available on this
	// platform/session (e.g. false on Wayland where X11 grabs don't work).
	IsSupported() bool
	// Grab activates exclusive input capture.
	Grab() error
	// Release deactivates input capture and restores normal host behaviour.
	Release() error
	// IsGrabbed reports whether input capture is currently active.
	IsGrabbed() bool
	// PlatformNote returns a short user-facing string describing any
	// platform-specific limitations (empty if none).
	PlatformNote() string
}
