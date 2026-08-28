package capture

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework ApplicationServices
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>

// Forward declaration for the Go callback trampoline.
extern CGEventRef eventTapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *userInfo);

static CFMachPortRef createEventTap(void) {
    CGEventMask mask =
        (1 << kCGEventKeyDown) |
        (1 << kCGEventKeyUp) |
        (1 << kCGEventFlagsChanged) |
        (1 << kCGEventLeftMouseDown) |
        (1 << kCGEventLeftMouseUp) |
        (1 << kCGEventRightMouseDown) |
        (1 << kCGEventRightMouseUp) |
        (1 << kCGEventOtherMouseDown) |
        (1 << kCGEventOtherMouseUp) |
        (1 << kCGEventScrollWheel) |
        (1 << kCGEventMouseMoved) |
        (1 << kCGEventLeftMouseDragged) |
        (1 << kCGEventRightMouseDragged) |
        (1 << kCGEventOtherMouseDragged);

    return CGEventTapCreate(
        kCGHIDEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionDefault,
        mask,
        eventTapCallback,
        NULL
    );
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

type darwinGrabber struct {
	mu       sync.Mutex
	grabbed  bool
	tap      C.CFMachPortRef
	source   C.CFRunLoopSourceRef
	runLoop  C.CFRunLoopRef
	stopCh   chan struct{}
}

// New returns a Grabber that uses macOS CGEventTap.
func New() Grabber {
	return &darwinGrabber{}
}

func (g *darwinGrabber) IsSupported() bool {
	return true
}

var activeDarwinGrabber *darwinGrabber

//export eventTapCallback
func eventTapCallback(proxy C.CGEventTapProxy, eventType C.CGEventType, event C.CGEventRef, userInfo unsafe.Pointer) C.CGEventRef {
	if activeDarwinGrabber == nil || !activeDarwinGrabber.IsGrabbed() {
		return event
	}

	// If the tap was disabled by the system (timeout), re-enable it.
	if eventType == C.kCGEventTapDisabledByTimeout || eventType == C.kCGEventTapDisabledByUserInput {
		if activeDarwinGrabber.tap != 0 {
			C.CGEventTapEnable(activeDarwinGrabber.tap, C.bool(true))
		}
		return event
	}

	// Suppress the event from reaching the system; Ebiten still receives
	// input via its own NSApplication event handling.
	return C.CGEventRef(C.NULL)
}

func (g *darwinGrabber) Grab() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.grabbed {
		return nil
	}

	if C.AXIsProcessTrusted() == 0 {
		return fmt.Errorf("capture: Accessibility permission required — enable it in System Settings > Privacy & Security > Accessibility")
	}

	tap := C.createEventTap()
	if tap == 0 {
		return fmt.Errorf("capture: failed to create CGEventTap")
	}

	source := C.CFMachPortCreateRunLoopSource(C.kCFAllocatorDefault, tap, 0)
	if source == 0 {
		C.CFRelease(C.CFTypeRef(tap))
		return fmt.Errorf("capture: failed to create run loop source")
	}

	activeDarwinGrabber = g
	g.tap = tap
	g.source = source
	g.grabbed = true
	g.stopCh = make(chan struct{})

	go func() {
		rl := C.CFRunLoopGetCurrent()
		g.mu.Lock()
		g.runLoop = rl
		g.mu.Unlock()

		C.CFRunLoopAddSource(rl, source, C.kCFRunLoopCommonModes)
		C.CGEventTapEnable(tap, C.bool(true))
		C.CFRunLoopRun()
		close(g.stopCh)
	}()

	return nil
}

func (g *darwinGrabber) Release() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.grabbed {
		return nil
	}

	if g.tap != 0 {
		C.CGEventTapEnable(g.tap, C.bool(false))
	}

	if g.runLoop != 0 {
		C.CFRunLoopStop(g.runLoop)
	}

	g.grabbed = false
	activeDarwinGrabber = nil

	if g.stopCh != nil {
		g.mu.Unlock()
		<-g.stopCh
		g.mu.Lock()
	}

	if g.source != 0 {
		C.CFRelease(C.CFTypeRef(g.source))
		g.source = 0
	}
	if g.tap != 0 {
		C.CFRelease(C.CFTypeRef(g.tap))
		g.tap = 0
	}
	g.runLoop = 0

	return nil
}

func (g *darwinGrabber) IsGrabbed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.grabbed
}

func (g *darwinGrabber) PlatformNote() string {
	return "Requires Accessibility permission in System Settings."
}
