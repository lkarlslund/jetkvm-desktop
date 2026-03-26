package app

import (
	"context"
	"fmt"
	"image/color"
	"sort"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/lkarlslund/jetkvm-native/internal/client"
)

type Config struct {
	BaseURL    string
	Password   string
	RPCTimeout time.Duration
}

type App struct {
	cfg    Config
	client *client.Client

	mu       sync.RWMutex
	status   string
	deviceID string
	quality  float64
	lastImg  *ebiten.Image
	keys     map[ebiten.Key]bool
	lastX    int
	lastY    int
	relative bool
}

func New(cfg Config) (*App, error) {
	c, err := client.New(client.Config{
		BaseURL:    cfg.BaseURL,
		Password:   cfg.Password,
		RPCTimeout: cfg.RPCTimeout,
	})
	if err != nil {
		return nil, err
	}
	return &App{
		cfg:    cfg,
		client: c,
		status: "starting",
		keys:   map[ebiten.Key]bool{},
	}, nil
}

func (a *App) Connect(ctx context.Context) error {
	a.SetStatus("connecting")
	if err := a.client.Connect(ctx); err != nil {
		return err
	}

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := a.client.WaitForHID(waitCtx); err != nil {
		return err
	}

	var deviceID string
	if err := a.client.Call(ctx, "getDeviceID", nil, &deviceID); err != nil {
		return err
	}
	var quality float64
	if err := a.client.Call(ctx, "getStreamQualityFactor", nil, &quality); err != nil {
		return err
	}
	a.mu.Lock()
	a.deviceID = deviceID
	a.quality = quality
	a.mu.Unlock()
	a.SetStatus(fmt.Sprintf("connected to %s", deviceID))

	go func() {
		for evt := range a.client.Events() {
			a.SetStatus("event: " + evt.Method)
		}
	}()

	go func() {
		for {
			stream := a.client.VideoStream()
			if stream == nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			for frame := range stream.Frames() {
				img := ebiten.NewImageFromImage(frame.Image)
				a.mu.Lock()
				a.lastImg = img
				a.mu.Unlock()
			}
			return
		}
	}()
	return nil
}

func (a *App) SetStatus(status string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = status
}

func (a *App) statusLine() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

func (a *App) Update() error {
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF8) {
		a.relative = !a.relative
		if a.relative {
			ebiten.SetCursorMode(ebiten.CursorModeCaptured)
			a.SetStatus("relative mouse enabled")
		} else {
			ebiten.SetCursorMode(ebiten.CursorModeVisible)
			a.SetStatus("absolute mouse enabled")
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF5) {
		go func() {
			_ = a.client.Call(context.Background(), "reboot", map[string]any{"force": false}, nil)
		}()
		a.SetStatus("reboot requested")
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) {
		a.adjustStreamQuality(+0.05)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) {
		a.adjustStreamQuality(-0.05)
	}

	a.syncKeyboard()
	a.syncMouse()
	return nil
}

func (a *App) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 12, G: 20, B: 32, A: 255})
	a.mu.RLock()
	img := a.lastImg
	a.mu.RUnlock()
	if img != nil {
		w, h := img.Size()
		op := &ebiten.DrawImageOptions{}
		sw, sh := screen.Size()
		scale := min(float64(sw)/float64(w), float64(sh)/float64(h))
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate((float64(sw)-float64(w)*scale)/2, (float64(sh)-float64(h)*scale)/2)
		screen.DrawImage(img, op)
	}
	mode := "absolute"
	if a.relative {
		mode = "relative"
	}
	a.mu.RLock()
	deviceID := a.deviceID
	quality := a.quality
	a.mu.RUnlock()
	ebitenutil.DebugPrint(
		screen,
		fmt.Sprintf(
			"jetkvm-native\n%s\ndevice: %s\nquality: %.2f (+/-)\nmouse: %s (F8 toggles)\nF5 reboot\nEsc to quit",
			a.statusLine(),
			deviceID,
			quality,
			mode,
		),
	)
}

func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func (a *App) syncKeyboard() {
	keys := inpututil.AppendPressedKeys(nil)
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	current := make(map[ebiten.Key]bool, len(keys))
	for _, key := range keys {
		current[key] = true
		if !a.keys[key] {
			if hid, ok := keyToHID(key); ok {
				_ = a.client.SendKeypress(hid, true)
			}
		}
	}
	for key := range a.keys {
		if current[key] {
			continue
		}
		if hid, ok := keyToHID(key); ok {
			_ = a.client.SendKeypress(hid, false)
		}
	}
	a.keys = current
}

func (a *App) syncMouse() {
	x, y := ebiten.CursorPosition()
	buttons := byte(0)
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		buttons |= 1
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		buttons |= 2
	}
	if a.relative {
		dx := int8(clamp(float64(x-a.lastX), -127, 127))
		dy := int8(clamp(float64(y-a.lastY), -127, 127))
		if dx != 0 || dy != 0 || buttons != 0 {
			_ = a.client.SendRelMouse(dx, dy, buttons)
		}
	} else {
		sw, sh := ebiten.WindowSize()
		if sw == 0 || sh == 0 {
			return
		}
		nx := uint16(clamp(float64(x)/float64(sw)*32767.0, 0, 32767))
		ny := uint16(clamp(float64(y)/float64(sh)*32767.0, 0, 32767))
		_ = a.client.SendAbsPointer(nx, ny, buttons)
	}
	_, wheelY := ebiten.Wheel()
	if wheelY != 0 {
		_ = a.client.SendWheel(int8(clamp(-wheelY, -127, 127)))
	}
	a.lastX = x
	a.lastY = y
}

func keyToHID(key ebiten.Key) (byte, bool) {
	switch key {
	case ebiten.KeyA:
		return 4, true
	case ebiten.KeyB:
		return 5, true
	case ebiten.KeyC:
		return 6, true
	case ebiten.KeyD:
		return 7, true
	case ebiten.KeyE:
		return 8, true
	case ebiten.KeyF:
		return 9, true
	case ebiten.KeyG:
		return 10, true
	case ebiten.KeyH:
		return 11, true
	case ebiten.KeyI:
		return 12, true
	case ebiten.KeyJ:
		return 13, true
	case ebiten.KeyK:
		return 14, true
	case ebiten.KeyL:
		return 15, true
	case ebiten.KeyM:
		return 16, true
	case ebiten.KeyN:
		return 17, true
	case ebiten.KeyO:
		return 18, true
	case ebiten.KeyP:
		return 19, true
	case ebiten.KeyQ:
		return 20, true
	case ebiten.KeyR:
		return 21, true
	case ebiten.KeyS:
		return 22, true
	case ebiten.KeyT:
		return 23, true
	case ebiten.KeyU:
		return 24, true
	case ebiten.KeyV:
		return 25, true
	case ebiten.KeyW:
		return 26, true
	case ebiten.KeyX:
		return 27, true
	case ebiten.KeyY:
		return 28, true
	case ebiten.KeyZ:
		return 29, true
	case ebiten.Key1:
		return 30, true
	case ebiten.Key2:
		return 31, true
	case ebiten.Key3:
		return 32, true
	case ebiten.Key4:
		return 33, true
	case ebiten.Key5:
		return 34, true
	case ebiten.Key6:
		return 35, true
	case ebiten.Key7:
		return 36, true
	case ebiten.Key8:
		return 37, true
	case ebiten.Key9:
		return 38, true
	case ebiten.Key0:
		return 39, true
	case ebiten.KeyEnter:
		return 40, true
	case ebiten.KeyEscape:
		return 41, true
	case ebiten.KeyBackspace:
		return 42, true
	case ebiten.KeyTab:
		return 43, true
	case ebiten.KeySpace:
		return 44, true
	case ebiten.KeyMinus:
		return 45, true
	case ebiten.KeyEqual:
		return 46, true
	case ebiten.KeyLeftBracket:
		return 47, true
	case ebiten.KeyRightBracket:
		return 48, true
	case ebiten.KeyBackslash:
		return 49, true
	case ebiten.KeySemicolon:
		return 51, true
	case ebiten.KeyApostrophe:
		return 52, true
	case ebiten.KeyGraveAccent:
		return 53, true
	case ebiten.KeyComma:
		return 54, true
	case ebiten.KeyPeriod:
		return 55, true
	case ebiten.KeySlash:
		return 56, true
	case ebiten.KeyCapsLock:
		return 57, true
	case ebiten.KeyF1:
		return 58, true
	case ebiten.KeyF2:
		return 59, true
	case ebiten.KeyF3:
		return 60, true
	case ebiten.KeyF4:
		return 61, true
	case ebiten.KeyF5:
		return 62, true
	case ebiten.KeyF6:
		return 63, true
	case ebiten.KeyF7:
		return 64, true
	case ebiten.KeyF8:
		return 65, true
	case ebiten.KeyF9:
		return 66, true
	case ebiten.KeyF10:
		return 67, true
	case ebiten.KeyF11:
		return 68, true
	case ebiten.KeyF12:
		return 69, true
	case ebiten.KeyPrintScreen:
		return 70, true
	case ebiten.KeyScrollLock:
		return 71, true
	case ebiten.KeyPause:
		return 72, true
	case ebiten.KeyInsert:
		return 73, true
	case ebiten.KeyHome:
		return 74, true
	case ebiten.KeyPageUp:
		return 75, true
	case ebiten.KeyDelete:
		return 76, true
	case ebiten.KeyEnd:
		return 77, true
	case ebiten.KeyPageDown:
		return 78, true
	case ebiten.KeyRight:
		return 79, true
	case ebiten.KeyLeft:
		return 80, true
	case ebiten.KeyDown:
		return 81, true
	case ebiten.KeyUp:
		return 82, true
	case ebiten.KeyNumLock:
		return 83, true
	case ebiten.KeyNumpadDivide:
		return 84, true
	case ebiten.KeyNumpadMultiply:
		return 85, true
	case ebiten.KeyNumpadSubtract:
		return 86, true
	case ebiten.KeyNumpadAdd:
		return 87, true
	case ebiten.KeyNumpadEnter:
		return 88, true
	case ebiten.KeyNumpad1:
		return 89, true
	case ebiten.KeyNumpad2:
		return 90, true
	case ebiten.KeyNumpad3:
		return 91, true
	case ebiten.KeyNumpad4:
		return 92, true
	case ebiten.KeyNumpad5:
		return 93, true
	case ebiten.KeyNumpad6:
		return 94, true
	case ebiten.KeyNumpad7:
		return 95, true
	case ebiten.KeyNumpad8:
		return 96, true
	case ebiten.KeyNumpad9:
		return 97, true
	case ebiten.KeyNumpad0:
		return 98, true
	case ebiten.KeyNumpadDecimal:
		return 99, true
	case ebiten.KeyControlLeft:
		return 224, true
	case ebiten.KeyShiftLeft:
		return 225, true
	case ebiten.KeyAltLeft:
		return 226, true
	case ebiten.KeyMetaLeft:
		return 227, true
	case ebiten.KeyControlRight:
		return 228, true
	case ebiten.KeyShiftRight:
		return 229, true
	case ebiten.KeyAltRight:
		return 230, true
	case ebiten.KeyMetaRight:
		return 231, true
	default:
		return 0, false
	}
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func (a *App) adjustStreamQuality(delta float64) {
	a.mu.RLock()
	next := clamp(a.quality+delta, 0.1, 1.0)
	a.mu.RUnlock()

	go func(value float64) {
		if err := a.client.Call(context.Background(), "setStreamQualityFactor", map[string]any{"factor": value}, nil); err != nil {
			a.SetStatus("quality update failed")
			return
		}
		a.mu.Lock()
		a.quality = value
		a.mu.Unlock()
		a.SetStatus(fmt.Sprintf("quality set to %.2f", value))
	}(next)
}
