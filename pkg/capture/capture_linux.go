package capture

/*
#cgo linux pkg-config: x11
#include <X11/Xlib.h>
#include <stdlib.h>

static int x11_init_threads(void) {
	return XInitThreads();
}

static Window x11_focused_window(Display *dpy) {
	Window w;
	int revert;
	XGetInputFocus(dpy, &w, &revert);
	return w;
}

// Drain one pending event from the grab connection.
// Key events are forwarded to the target (GLFW) window so Ebiten sees them.
// Pointer events are also forwarded so the Ebiten mouse pipeline keeps working.
// Returns 0 when no event was pending.
static int x11_pump_one(Display *dpy, Window target) {
	if (XPending(dpy) == 0)
		return 0;
	XEvent ev;
	XNextEvent(dpy, &ev);
	switch (ev.type) {
	case KeyPress:
	case KeyRelease:
		ev.xkey.window = target;
		XSendEvent(dpy, target, False, KeyPressMask | KeyReleaseMask, &ev);
		XFlush(dpy);
		break;
	case ButtonPress:
	case ButtonRelease:
		ev.xbutton.window = target;
		XSendEvent(dpy, target, False, ButtonPressMask | ButtonReleaseMask, &ev);
		XFlush(dpy);
		break;
	case MotionNotify:
		ev.xmotion.window = target;
		XSendEvent(dpy, target, False, PointerMotionMask, &ev);
		XFlush(dpy);
		break;
	}
	return 1;
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

func init() { C.x11_init_threads() }

// gsettings keys that GNOME/Mutter intercepts at the compositor level,
// bypassing X11 grabs. We temporarily disable them during capture.
var gnomeKeybindings = []struct {
	schema string
	key    string
}{
	{"org.gnome.desktop.wm.keybindings", "switch-applications"},
	{"org.gnome.desktop.wm.keybindings", "switch-applications-backward"},
	{"org.gnome.desktop.wm.keybindings", "switch-windows"},
	{"org.gnome.desktop.wm.keybindings", "switch-windows-backward"},
	{"org.gnome.desktop.wm.keybindings", "panel-main-menu"},
	{"org.gnome.desktop.wm.keybindings", "switch-group"},
	{"org.gnome.desktop.wm.keybindings", "switch-group-backward"},
	{"org.gnome.mutter.keybindings", "toggle-tiled-left"},
	{"org.gnome.mutter.keybindings", "toggle-tiled-right"},
	{"org.gnome.mutter", "overlay-key"},
}

type savedBinding struct {
	Schema string `json:"schema"`
	Key    string `json:"key"`
	Value  string `json:"value"`
}

type x11Grabber struct {
	mu            sync.Mutex
	grabbed       atomic.Bool
	display       *C.Display
	target        C.Window
	done          chan struct{}
	savedBindings []savedBinding
	sigChan       chan os.Signal

	supportedOnce sync.Once
	supportedVal  bool
}

func New() Grabber {
	g := &x11Grabber{}
	g.recoverBindingsFromDisk()
	return g
}

func (g *x11Grabber) IsSupported() bool {
	g.supportedOnce.Do(func() {
		if os.Getenv("XDG_SESSION_TYPE") == "wayland" && os.Getenv("DISPLAY") == "" {
			g.supportedVal = false
			return
		}
		dpy := C.XOpenDisplay((*C.char)(unsafe.Pointer(nil)))
		if dpy == nil {
			g.supportedVal = false
			return
		}
		C.XCloseDisplay(dpy)
		g.supportedVal = true
	})
	return g.supportedVal
}

func (g *x11Grabber) Grab() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.grabbed.Load() {
		return nil
	}

	dpy := C.XOpenDisplay((*C.char)(unsafe.Pointer(nil)))
	if dpy == nil {
		return fmt.Errorf("capture: cannot open X11 display (Wayland is not supported)")
	}

	target := C.x11_focused_window(dpy)
	if target == C.None {
		C.XCloseDisplay(dpy)
		return fmt.Errorf("capture: no focused X11 window")
	}

	rc := C.XGrabKeyboard(dpy, target, C.True,
		C.GrabModeAsync, C.GrabModeAsync, C.CurrentTime)
	if rc != C.GrabSuccess {
		C.XCloseDisplay(dpy)
		return fmt.Errorf("capture: XGrabKeyboard failed (code %d)", int(rc))
	}

	prc := C.XGrabPointer(dpy, target, C.True,
		C.uint(C.ButtonPressMask|C.ButtonReleaseMask|C.PointerMotionMask),
		C.GrabModeAsync, C.GrabModeAsync, C.None, C.None, C.CurrentTime)
	if prc != C.GrabSuccess {
		C.XUngrabKeyboard(dpy, C.CurrentTime)
		C.XFlush(dpy)
		C.XCloseDisplay(dpy)
		return fmt.Errorf("capture: XGrabPointer failed (code %d)", int(prc))
	}

	C.XFlush(dpy)
	g.display = dpy
	g.target = target
	g.done = make(chan struct{})

	g.disableCompositorShortcuts()
	g.installSignalHandler()

	g.grabbed.Store(true)

	go g.pump()
	return nil
}

func (g *x11Grabber) pump() {
	defer close(g.done)
	for g.grabbed.Load() {
		if C.x11_pump_one(g.display, g.target) == 0 {
			time.Sleep(time.Millisecond)
		}
	}
}

func (g *x11Grabber) Release() error {
	if !g.grabbed.Load() {
		return nil
	}
	g.grabbed.Store(false)

	if g.done != nil {
		<-g.done
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	g.removeSignalHandler()
	g.restoreCompositorShortcuts()

	if g.display != nil {
		C.XUngrabPointer(g.display, C.CurrentTime)
		C.XUngrabKeyboard(g.display, C.CurrentTime)
		C.XFlush(g.display)
		C.XCloseDisplay(g.display)
		g.display = nil
	}
	return nil
}

func (g *x11Grabber) IsGrabbed() bool {
	return g.grabbed.Load()
}

func (g *x11Grabber) PlatformNote() string {
	return "Total Capture requires X11. Wayland is not supported."
}

func backupPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "jetkvm-desktop", ".keybindings-backup.json")
}

// disableCompositorShortcuts saves originals to disk, then blanks them.
func (g *x11Grabber) disableCompositorShortcuts() {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return
	}

	g.savedBindings = nil
	for _, kb := range gnomeKeybindings {
		out, err := exec.Command("gsettings", "get", kb.schema, kb.key).Output()
		if err != nil {
			continue
		}
		original := strings.TrimSpace(string(out))
		g.savedBindings = append(g.savedBindings, savedBinding{
			Schema: kb.schema,
			Key:    kb.key,
			Value:  original,
		})
	}

	g.writeBackupToDisk()

	for _, sb := range g.savedBindings {
		blank := "['']"
		if sb.Key == "overlay-key" {
			blank = "''"
		}
		_ = exec.Command("gsettings", "set", sb.Schema, sb.Key, blank).Run()
	}
}

// restoreCompositorShortcuts puts back the original bindings and removes the backup.
func (g *x11Grabber) restoreCompositorShortcuts() {
	for _, sb := range g.savedBindings {
		_ = exec.Command("gsettings", "set", sb.Schema, sb.Key, sb.Value).Run()
	}
	g.savedBindings = nil
	g.removeBackupFromDisk()
}

func (g *x11Grabber) writeBackupToDisk() {
	if len(g.savedBindings) == 0 {
		return
	}
	data, err := json.Marshal(g.savedBindings)
	if err != nil {
		return
	}
	p := backupPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, data, 0o644)
}

func (g *x11Grabber) removeBackupFromDisk() {
	_ = os.Remove(backupPath())
}

// recoverBindingsFromDisk restores keybindings from a previous crash.
func (g *x11Grabber) recoverBindingsFromDisk() {
	p := backupPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var bindings []savedBinding
	if json.Unmarshal(data, &bindings) != nil || len(bindings) == 0 {
		_ = os.Remove(p)
		return
	}

	if _, err := exec.LookPath("gsettings"); err != nil {
		_ = os.Remove(p)
		return
	}

	for _, sb := range bindings {
		_ = exec.Command("gsettings", "set", sb.Schema, sb.Key, sb.Value).Run()
	}
	_ = os.Remove(p)
}

// installSignalHandler catches SIGTERM/SIGINT to restore keybindings before exit.
func (g *x11Grabber) installSignalHandler() {
	g.sigChan = make(chan os.Signal, 1)
	signal.Notify(g.sigChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig, ok := <-g.sigChan
		if !ok {
			return
		}
		g.mu.Lock()
		g.restoreCompositorShortcuts()
		g.mu.Unlock()
		signal.Reset(sig)
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(sig)
	}()
}

func (g *x11Grabber) removeSignalHandler() {
	if g.sigChan != nil {
		signal.Stop(g.sigChan)
		close(g.sigChan)
		g.sigChan = nil
	}
}
