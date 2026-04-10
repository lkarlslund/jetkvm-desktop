package app

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.design/x/clipboard"

	"github.com/lkarlslund/jetkvm-desktop/pkg/client"
	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
	"github.com/lkarlslund/jetkvm-desktop/pkg/ui"
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
	a.appendStatsHistory(a.stats, time.Now())
	a.lastStatsPoll = time.Now()
}

func (a *App) appendStatsHistory(stats client.StatsSnapshot, now time.Time) {
	a.statsHistory = append(a.statsHistory, statsPoint{
		At:              now,
		BitrateKbps:     stats.BitrateKbps,
		JitterMs:        stats.JitterMs,
		RoundTripMs:     stats.RoundTripMs,
		FramesPerSecond: stats.FramesPerSecond,
	})
	cutoff := now.Add(-2 * time.Minute)
	trimmed := a.statsHistory[:0]
	for _, sample := range a.statsHistory {
		if sample.At.Before(cutoff) {
			continue
		}
		trimmed = append(trimmed, sample)
	}
	a.statsHistory = trimmed
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

	graphs := []graphMetric{
		{
			Title:  "Bitrate",
			Unit:   "kbps",
			Value:  stats.BitrateKbps,
			Series: statsSeries(a.statsHistory, func(p statsPoint) float64 { return p.BitrateKbps }),
		},
		{
			Title:  "Jitter",
			Unit:   "ms",
			Value:  stats.JitterMs,
			Series: statsSeries(a.statsHistory, func(p statsPoint) float64 { return p.JitterMs }),
		},
		{
			Title:  "RTT",
			Unit:   "ms",
			Value:  stats.RoundTripMs,
			Series: statsSeries(a.statsHistory, func(p statsPoint) float64 { return p.RoundTripMs }),
		},
		{
			Title:  "Decode FPS",
			Unit:   "fps",
			Value:  stats.FramesPerSecond,
			Series: statsSeries(a.statsHistory, func(p statsPoint) float64 { return p.FramesPerSecond }),
		},
	}
	w := 0.0
	for _, line := range lines {
		lineW, _ := measureText(line, 12)
		if lineW > w {
			w = lineW
		}
	}
	graphAreaW := 320.0
	if w > graphAreaW {
		graphAreaW = w
	}
	const pad = 16.0
	boxW := graphAreaW + pad*2
	boxH := float64(len(lines))*18 + pad*2 + 18 + float64(len(graphs))*72
	x := float64(screen.Bounds().Dx()) - boxW - 16
	y := 58.0
	ctx := a.newUIContext(screen, func(chromeButton) {})
	ui.Panel{
		Fill:   color.RGBA{R: 9, G: 14, B: 22, A: 224},
		Stroke: color.RGBA{R: 88, G: 108, B: 126, A: 180},
		Insets: ui.UniformInsets(pad),
		Child: statsOverlayElement{
			app:    a,
			lines:  lines,
			graphs: graphs,
		},
	}.Draw(ctx, ui.Rect{X: x, Y: y, W: boxW, H: boxH})
}

type graphMetric struct {
	Title  string
	Unit   string
	Value  float64
	Series []float64
}

func statsSeries(history []statsPoint, pick func(statsPoint) float64) []float64 {
	values := make([]float64, 0, len(history))
	for _, sample := range history {
		values = append(values, pick(sample))
	}
	return values
}

func graphDomain(values []float64) (float64, float64) {
	maxValue := 0.0
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	if maxValue <= 0 {
		return 0, 1
	}
	return 0, niceCeil(maxValue * 1.1)
}

func niceCeil(value float64) float64 {
	if value <= 0 {
		return 1
	}
	magnitude := math.Pow(10, math.Floor(math.Log10(value)))
	normalized := value / magnitude
	switch {
	case normalized <= 1:
		return 1 * magnitude
	case normalized <= 2:
		return 2 * magnitude
	case normalized <= 5:
		return 5 * magnitude
	default:
		return 10 * magnitude
	}
}

func formatGraphValue(value float64, unit string) string {
	switch unit {
	case "fps":
		return fmt.Sprintf("%.1f %s", value, unit)
	default:
		return fmt.Sprintf("%.0f %s", value, unit)
	}
}

func (a *App) drawStatsGraph(screen *ebiten.Image, x, y, w, h float64, metric graphMetric) {
	vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), color.RGBA{R: 15, G: 23, B: 34, A: 220}, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 1, color.RGBA{R: 62, G: 80, B: 96, A: 180}, false)

	drawText(screen, metric.Title, x+10, y+10, 12, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	drawText(screen, formatGraphValue(metric.Value, metric.Unit), x+w-88, y+10, 12, color.RGBA{R: 166, G: 200, B: 255, A: 255})

	chartX := x + 10
	chartY := y + 24
	chartW := w - 20
	chartH := h - 32
	vector.StrokeRect(screen, float32(chartX), float32(chartY), float32(chartW), float32(chartH), 1, color.RGBA{R: 46, G: 60, B: 75, A: 120}, false)

	minY, maxY := graphDomain(metric.Series)
	for i := 1; i < 4; i++ {
		yy := chartY + chartH*(float64(i)/4)
		vector.StrokeLine(screen, float32(chartX), float32(yy), float32(chartX+chartW), float32(yy), 1, color.RGBA{R: 34, G: 46, B: 58, A: 120}, false)
	}
	if len(metric.Series) < 2 {
		return
	}
	prevX := chartX
	prevY := chartY + chartH
	for i, value := range metric.Series {
		norm := 0.0
		if maxY > minY {
			norm = (value - minY) / (maxY - minY)
		}
		if norm < 0 {
			norm = 0
		}
		if norm > 1 {
			norm = 1
		}
		px := chartX + (float64(i)/float64(len(metric.Series)-1))*chartW
		py := chartY + chartH - norm*chartH
		if i > 0 {
			vector.StrokeLine(screen, float32(prevX), float32(prevY), float32(px), float32(py), 2, color.RGBA{R: 108, G: 184, B: 255, A: 255}, false)
		}
		prevX = px
		prevY = py
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
	panelW := min(760, float64(bounds.Dx()-72))
	panelH := min(420, float64(bounds.Dy()-96))
	panelX := (float64(bounds.Dx()) - panelW) / 2
	panelY := (float64(bounds.Dy()) - panelH) / 2
	a.pastePanel = rect{x: panelX, y: panelY, w: panelW, h: panelH}
	a.pasteButtons = a.pasteButtons[:0]
	ctx := a.newUIContext(screen, func(btn chromeButton) {
		a.pasteButtons = append(a.pasteButtons, btn)
	})
	ctx.FillRect(ui.Rect{W: float64(bounds.Dx()), H: float64(bounds.Dy())}, color.RGBA{A: 168})
	ui.Panel{
		Fill:   color.RGBA{R: 13, G: 20, B: 30, A: 246},
		Stroke: color.RGBA{R: 88, G: 102, B: 118, A: 180},
		Insets: ui.UniformInsets(22),
		Child: pasteOverlayElement{
			app:  a,
			snap: snap,
		},
	}.Draw(ctx, ui.Rect{X: panelX, Y: panelY, W: panelW, H: panelH})
}

type statsOverlayElement struct {
	app    *App
	lines  []string
	graphs []graphMetric
}

func (e statsOverlayElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e statsOverlayElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: "Connection Stats", Size: 14, Color: color.RGBA{R: 240, G: 244, B: 248, A: 255}}),
		ui.Fixed(ui.Spacer{H: 8}),
	}
	for i, line := range e.lines {
		if i > 0 {
			children = append(children, ui.Fixed(ui.Spacer{H: 6}))
		}
		children = append(children, ui.Fixed(ui.Label{Text: line, Size: 12, Color: color.RGBA{R: 210, G: 218, B: 226, A: 255}}))
	}
	children = append(children, ui.Fixed(ui.Spacer{H: 18}))
	for i, graph := range e.graphs {
		if i > 0 {
			children = append(children, ui.Fixed(ui.Spacer{H: 14}))
		}
		children = append(children, ui.Fixed(statsGraphElement{app: e.app, metric: graph}))
	}
	ui.Column{Children: children}.Draw(ctx, bounds)
}

type statsGraphElement struct {
	app    *App
	metric graphMetric
}

func (statsGraphElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 58})
}

func (e statsGraphElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	e.app.drawStatsGraph(ctx.Screen, bounds.X, bounds.Y, bounds.W, bounds.H, e.metric)
}

type pasteOverlayElement struct {
	app  *App
	snap session.Snapshot
}

func (e pasteOverlayElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e pasteOverlayElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	metaRow := ui.Row{
		Children: []ui.Child{
			ui.Fixed(ui.KeyValue{Label: "Keyboard Layout", Value: fallbackLabel(e.snap.KeyboardLayout, "en-US"), LabelWidth: 98}),
			ui.Fixed(ui.Spacer{W: 18}),
			ui.Fixed(ui.KeyValue{Label: "Delay", Value: fmt.Sprintf("%dms", e.app.pasteDelay), LabelWidth: 28}),
		},
	}
	bodyChildren := []ui.Child{
		ui.Fixed(ui.Label{Text: "Paste Text", Size: 22, Color: color.RGBA{R: 240, G: 244, B: 248, A: 255}}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(ui.Paragraph{
			Text:  "Send clipboard text to the remote host using keyboard macro steps over HID-RPC. Unsupported characters are skipped.",
			Size:  13,
			Color: color.RGBA{R: 166, G: 178, B: 190, A: 255},
		}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(metaRow),
		ui.Fixed(ui.Spacer{H: 16}),
		ui.Flex(ui.Panel{
			Fill:   color.RGBA{R: 18, G: 28, B: 40, A: 255},
			Stroke: color.RGBA{R: 54, G: 68, B: 84, A: 180},
			Insets: ui.UniformInsets(14),
			Child:  pasteTextElement{app: e.app},
		}, 1),
		ui.Fixed(ui.Spacer{H: 16}),
	}
	if e.app.pasteInvalid != "" {
		bodyChildren = append(bodyChildren,
			ui.Fixed(ui.Paragraph{
				Text:  "Skipped characters: " + e.app.pasteInvalid,
				Size:  12,
				Color: color.RGBA{R: 236, G: 180, B: 126, A: 255},
			}),
			ui.Fixed(ui.Spacer{H: 6}),
		)
	}
	if e.app.pasteError != "" {
		bodyChildren = append(bodyChildren,
			ui.Fixed(ui.Paragraph{Text: e.app.pasteError, Size: 12, Color: color.RGBA{R: 228, G: 142, B: 142, A: 255}}),
			ui.Fixed(ui.Spacer{H: 6}),
		)
	}
	if e.snap.PasteInProgress {
		bodyChildren = append(bodyChildren,
			ui.Fixed(ui.Label{Text: "Paste in progress…", Size: 12, Color: color.RGBA{R: 166, G: 200, B: 255, A: 255}}),
			ui.Fixed(ui.Spacer{H: 6}),
		)
	}
	bodyChildren = append(bodyChildren,
		ui.Fixed(ui.Row{
			Children: []ui.Child{
				ui.Flex(ui.Spacer{}, 1),
				ui.Fixed(ui.Button{ID: "paste_load_clipboard", Label: "Load Clipboard", Enabled: true}),
				ui.Fixed(ui.Button{ID: "paste_cancel", Label: "Cancel", Enabled: true}),
				ui.Fixed(ui.Button{ID: "paste_send", Label: "Send", Enabled: !e.snap.PasteInProgress && strings.TrimSpace(e.app.pasteText) != ""}),
			},
			Spacing: 12,
		}),
	)
	ui.Column{Children: bodyChildren}.Draw(ctx, bounds)
}

type pasteTextElement struct {
	app *App
}

func (pasteTextElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e pasteTextElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	if strings.TrimSpace(e.app.pasteText) == "" {
		ui.Label{
			Text:  "Paste from host clipboard or type here",
			Size:  14,
			Color: color.RGBA{R: 108, G: 122, B: 136, A: 255},
		}.Draw(ctx, bounds)
		return
	}
	ui.Paragraph{
		Text:  e.app.pasteText,
		Size:  14,
		Color: color.RGBA{R: 236, G: 241, B: 245, A: 255},
	}.Draw(ctx, bounds)
}
