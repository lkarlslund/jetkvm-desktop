package app

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.design/x/clipboard"

	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

var clipboardReady bool

func init() {
	clipboardReady = clipboard.Init() == nil
}

func (a *App) syncStats() {
	if !a.statsOpen && time.Since(a.lastStatsPoll) < time.Second {
		return
	}
	if time.Since(a.lastStatsPoll) < time.Second {
		return
	}
	a.stats = a.ctrl.Stats()
	a.lastStatsPoll = time.Now()
}

func (a *App) syncPasteInput() {
	if !a.pasteOpen {
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) && (ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight) || ebiten.IsKeyPressed(ebiten.KeyMetaLeft) || ebiten.IsKeyPressed(ebiten.KeyMetaRight)) {
		go a.submitPaste()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		runes := []rune(a.pasteText)
		if len(runes) > 0 {
			a.pasteText = string(runes[:len(runes)-1])
			a.updatePastePreview()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		a.pasteText += "\n"
		a.updatePastePreview()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyV) && (ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight) || ebiten.IsKeyPressed(ebiten.KeyMetaLeft) || ebiten.IsKeyPressed(ebiten.KeyMetaRight)) {
		a.loadClipboardText()
		return
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		if r >= 32 || r == '\t' {
			a.pasteText += string(r)
		}
	}
	a.updatePastePreview()
}

func (a *App) updatePastePreview() {
	_, invalid := input.BuildPasteMacro(a.ctrl.Snapshot().KeyboardLayout, a.pasteText, a.pasteDelay)
	a.pasteInvalid = input.InvalidRunesString(invalid)
}

func (a *App) loadClipboardText() {
	a.pasteError = ""
	if !clipboardReady {
		a.pasteError = "System clipboard is not available on this platform/session"
		return
	}
	data := clipboard.Read(clipboard.FmtText)
	if len(data) == 0 {
		a.pasteText = ""
		a.pasteInvalid = ""
		return
	}
	a.pasteText = string(data)
	a.updatePastePreview()
}

func (a *App) submitPaste() {
	a.pasteError = ""
	invalid, err := a.ctrl.ExecutePaste(a.pasteText, a.pasteDelay)
	a.pasteInvalid = input.InvalidRunesString(invalid)
	if err != nil {
		a.pasteError = err.Error()
		return
	}
	a.pasteOpen = false
	a.applyCursorMode()
}

func (a *App) drawStatsOverlay(screen *ebiten.Image) {
	if !a.statsOpen {
		return
	}
	stats := a.stats
	lines := []string{
		fmt.Sprintf("Signaling: %s", signalingLabel(stats.SignalingMode)),
		fmt.Sprintf("RTC: %s", rtcLabel(stats.RTCState)),
		fmt.Sprintf("HID: %s", readyWord(stats.HIDReady)),
		fmt.Sprintf("Video: %s", readyWord(stats.VideoReady)),
		fmt.Sprintf("Resolution: %dx%d", stats.FrameWidth, stats.FrameHeight),
		fmt.Sprintf("Quality: %.0f%%", a.ctrl.Snapshot().Quality*100),
		fmt.Sprintf("Frame age: %s", humanFrameAge(a.lastFrameAt)),
	}
	if stats.BitrateKbps > 0 {
		lines = append(lines, fmt.Sprintf("Bitrate: %.0f kbps", stats.BitrateKbps))
	}
	if stats.FramesPerSecond > 0 {
		lines = append(lines, fmt.Sprintf("Decode FPS: %.1f", stats.FramesPerSecond))
	}
	if stats.JitterMs > 0 || stats.RoundTripMs > 0 {
		lines = append(lines, fmt.Sprintf("Jitter / RTT: %.1fms / %.1fms", stats.JitterMs, stats.RoundTripMs))
	}
	if stats.PacketsLost != 0 {
		lines = append(lines, fmt.Sprintf("Packets lost: %d", stats.PacketsLost))
	}
	if stats.LastError != "" {
		lines = append(lines, "Error: "+trimForFooter(stats.LastError))
	}

	const pad = 16.0
	w := 0.0
	for _, line := range lines {
		lineW, _ := measureText(line, 12)
		if lineW > w {
			w = lineW
		}
	}
	boxW := w + pad*2
	boxH := float64(len(lines))*18 + pad*2
	x := float64(screen.Bounds().Dx()) - boxW - 16
	y := 58.0
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(boxW), float32(boxH), color.RGBA{R: 9, G: 14, B: 22, A: 224}, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(boxW), float32(boxH), 1, color.RGBA{R: 88, G: 108, B: 126, A: 180}, false)
	drawText(screen, "Connection Stats", x+pad, y+12, 14, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	for i, line := range lines {
		drawText(screen, line, x+pad, y+34+float64(i*18), 12, color.RGBA{R: 210, G: 218, B: 226, A: 255})
	}
}

func humanFrameAge(at time.Time) string {
	if at.IsZero() {
		return "n/a"
	}
	age := time.Since(at)
	switch {
	case age < 100*time.Millisecond:
		return "<100ms"
	case age < time.Second:
		return fmt.Sprintf("%dms", (age/(100*time.Millisecond))*100)
	case age < 10*time.Second:
		return fmt.Sprintf("%.1fs", float64((age/(100*time.Millisecond))*100)/1000)
	default:
		return fmt.Sprintf("%ds", int(age.Seconds()))
	}
}

func (a *App) drawPasteOverlay(screen *ebiten.Image, snap session.Snapshot) {
	if !a.pasteOpen {
		a.pasteButtons = nil
		return
	}
	bounds := screen.Bounds()
	vector.DrawFilledRect(screen, 0, 0, float32(bounds.Dx()), float32(bounds.Dy()), color.RGBA{A: 168}, false)

	panelW := min(760, float64(bounds.Dx()-72))
	panelH := min(420, float64(bounds.Dy()-96))
	panelX := (float64(bounds.Dx()) - panelW) / 2
	panelY := (float64(bounds.Dy()) - panelH) / 2
	a.pastePanel = rect{x: panelX, y: panelY, w: panelW, h: panelH}

	vector.DrawFilledRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), color.RGBA{R: 13, G: 20, B: 30, A: 246}, false)
	vector.StrokeRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), 1, color.RGBA{R: 88, G: 102, B: 118, A: 180}, false)

	drawText(screen, "Paste Text", panelX+22, panelY+18, 22, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	drawWrappedText(screen, "Send clipboard text to the remote host using keyboard macro steps over HID-RPC. Unsupported characters are skipped.", panelX+22, panelY+48, panelW-44, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})

	drawText(screen, "Keyboard Layout", panelX+22, panelY+88, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fallbackLabel(snap.KeyboardLayout, "en_US"), panelX+132, panelY+88, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	drawText(screen, "Delay", panelX+246, panelY+88, 13, color.RGBA{R: 166, G: 178, B: 190, A: 255})
	drawText(screen, fmt.Sprintf("%dms", a.pasteDelay), panelX+286, panelY+88, 13, color.RGBA{R: 236, G: 241, B: 245, A: 255})

	textRect := rect{x: panelX + 22, y: panelY + 114, w: panelW - 44, h: panelH - 182}
	vector.DrawFilledRect(screen, float32(textRect.x), float32(textRect.y), float32(textRect.w), float32(textRect.h), color.RGBA{R: 18, G: 28, B: 40, A: 255}, false)
	vector.StrokeRect(screen, float32(textRect.x), float32(textRect.y), float32(textRect.w), float32(textRect.h), 1, color.RGBA{R: 54, G: 68, B: 84, A: 180}, false)

	text := a.pasteText
	if text == "" {
		drawText(screen, "Paste from host clipboard or type here", textRect.x+14, textRect.y+14, 14, color.RGBA{R: 108, G: 122, B: 136, A: 255})
	} else {
		drawWrappedText(screen, text, textRect.x+14, textRect.y+14, textRect.w-28, 14, color.RGBA{R: 236, G: 241, B: 245, A: 255})
	}

	infoY := panelY + panelH - 58
	if a.pasteInvalid != "" {
		drawWrappedText(screen, "Skipped characters: "+a.pasteInvalid, panelX+22, infoY-18, panelW-240, 12, color.RGBA{R: 236, G: 180, B: 126, A: 255})
	}
	if a.pasteError != "" {
		drawWrappedText(screen, a.pasteError, panelX+22, infoY, panelW-240, 12, color.RGBA{R: 228, G: 142, B: 142, A: 255})
	}
	if snap.PasteInProgress {
		drawText(screen, "Paste in progress…", panelX+22, infoY, 12, color.RGBA{R: 166, G: 200, B: 255, A: 255})
	}

	a.pasteButtons = a.pasteButtons[:0]
	a.drawPasteButton(screen, "paste_load_clipboard", "Load Clipboard", panelX+panelW-356, panelY+panelH-48, 120, true, false)
	a.drawPasteButton(screen, "paste_cancel", "Cancel", panelX+panelW-224, panelY+panelH-48, 96, true, false)
	a.drawPasteButton(screen, "paste_send", "Send", panelX+panelW-116, panelY+panelH-48, 96, !snap.PasteInProgress && strings.TrimSpace(a.pasteText) != "", false)
}

func (a *App) drawPasteButton(screen *ebiten.Image, id, label string, x, y, w float64, enabled, active bool) {
	btn := chromeButton{id: id, label: label, enabled: enabled, active: active, rect: rect{x: x, y: y, w: w, h: 32}}
	a.pasteButtons = append(a.pasteButtons, btn)
	fill := color.RGBA{R: 30, G: 42, B: 58, A: 255}
	stroke := color.RGBA{R: 80, G: 96, B: 112, A: 180}
	textClr := color.RGBA{R: 228, G: 236, B: 244, A: 255}
	if !enabled {
		fill = color.RGBA{R: 24, G: 30, B: 38, A: 255}
		stroke = color.RGBA{R: 60, G: 68, B: 76, A: 150}
		textClr = color.RGBA{R: 128, G: 136, B: 144, A: 255}
	}
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), 32, fill, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), 32, 1, stroke, false)
	drawText(screen, label, x+12, y+9, 13, textClr)
}
