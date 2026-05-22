package capture

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32              = windows.NewLazyDLL("user32.dll")
	procSetWindowsHookEx   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx     = user32.NewProc("CallNextHookEx")
	procGetMessage         = user32.NewProc("GetMessageW")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procPostThreadMessage  = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadId = windows.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")
)

const (
	whKeyboardLL = 13
	whMouseLL    = 14
	wmQuit       = 0x0012
)

// KBDLLHOOKSTRUCT fields we inspect.
type kbdLLHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// Bit flag in KBDLLHOOKSTRUCT.Flags for injected events.
const llkhfInjected = 0x00000010

// Virtual-key codes we suppress when our window is foreground.
const (
	vkLWin  = 0x5B
	vkRWin  = 0x5C
	vkTab   = 0x09
	vkEsc   = 0x1B
	vkMenu  = 0x12 // Alt
	vkF4    = 0x73
)

type winGrabber struct {
	mu       sync.Mutex
	grabbed  bool
	kbHook   uintptr
	mouseHook uintptr
	threadID uint32
	done     chan struct{}
}

// New returns a Grabber that uses low-level Windows hooks.
func New() Grabber {
	return &winGrabber{}
}

func (g *winGrabber) IsSupported() bool {
	return true
}

// Global reference so hook callbacks can reach the grabber state.
// Only one grabber instance is active at a time.
var activeGrabber *winGrabber

func (g *winGrabber) GrabWithCallback(cb KeyCallback) error {
	return g.Grab()
}

func (g *winGrabber) Grab() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.grabbed {
		return nil
	}

	activeGrabber = g
	g.done = make(chan struct{})

	errCh := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		tid, _, _ := procGetCurrentThreadId.Call()
		g.mu.Lock()
		g.threadID = uint32(tid)
		g.mu.Unlock()

		kb, _, err := procSetWindowsHookEx.Call(
			whKeyboardLL,
			windows.NewCallback(keyboardHookProc),
			0,
			0,
		)
		if kb == 0 {
			errCh <- fmt.Errorf("capture: SetWindowsHookEx(keyboard) failed: %w", err)
			close(g.done)
			return
		}

		mouse, _, err := procSetWindowsHookEx.Call(
			whMouseLL,
			windows.NewCallback(mouseHookProc),
			0,
			0,
		)
		if mouse == 0 {
			procUnhookWindowsHookEx.Call(kb)
			errCh <- fmt.Errorf("capture: SetWindowsHookEx(mouse) failed: %w", err)
			close(g.done)
			return
		}

		g.mu.Lock()
		g.kbHook = kb
		g.mouseHook = mouse
		g.grabbed = true
		g.mu.Unlock()
		errCh <- nil

		// Message pump required for low-level hooks.
		var msg [48]byte
		for {
			ret, _, _ := procGetMessage.Call(
				uintptr(unsafe.Pointer(&msg[0])),
				0, 0, 0,
			)
			if ret == 0 || ret == uintptr(^uintptr(0)) {
				break
			}
		}
		close(g.done)
	}()

	return <-errCh
}

func (g *winGrabber) Release() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.grabbed {
		return nil
	}

	if g.kbHook != 0 {
		procUnhookWindowsHookEx.Call(g.kbHook)
		g.kbHook = 0
	}
	if g.mouseHook != 0 {
		procUnhookWindowsHookEx.Call(g.mouseHook)
		g.mouseHook = 0
	}

	if g.threadID != 0 {
		procPostThreadMessage.Call(uintptr(g.threadID), wmQuit, 0, 0)
		g.threadID = 0
	}

	g.grabbed = false
	activeGrabber = nil

	if g.done != nil {
		g.mu.Unlock()
		<-g.done
		g.mu.Lock()
	}
	return nil
}

func (g *winGrabber) IsGrabbed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.grabbed
}

func (g *winGrabber) PlatformNote() string {
	return "Ctrl+Alt+Del cannot be captured on Windows."
}

func isOurWindowForeground() bool {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}
	var pid uint32
	windows.GetWindowThreadProcessId(windows.HWND(hwnd), &pid)
	return pid == uint32(windows.GetCurrentProcessId())
}

func shouldSuppressKey(vk uint32) bool {
	switch vk {
	case vkLWin, vkRWin:
		return true
	case vkTab:
		return true // suppressed only when Alt is held (checked via flag in the hook)
	case vkEsc:
		return true
	case vkF4:
		return true
	}
	return false
}

func keyboardHookProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && activeGrabber != nil && activeGrabber.IsGrabbed() && isOurWindowForeground() {
		kb := (*kbdLLHookStruct)(unsafe.Pointer(lParam))
		if kb.Flags&llkhfInjected == 0 && shouldSuppressKey(kb.VkCode) {
			return 1
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func mouseHookProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	// Let all mouse events through to our window; the hook is installed
	// primarily so that shell gestures (e.g. edge swipes) are blocked.
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}
