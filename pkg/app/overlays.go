package app

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
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
	ctx := a.newUIContext(screen, func(chromeButton) {})
	statsOverlayRootElement{
		app: a,
		child: statsOverlayElement{
			app:    a,
			lines:  lines,
			graphs: graphs,
		},
	}.Draw(ctx, ui.Rect{W: float64(screen.Bounds().Dx()), H: float64(screen.Bounds().Dy())})
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

func formatGraphValue(value float64, unit string) string {
	switch unit {
	case "fps":
		return fmt.Sprintf("%.1f %s", value, unit)
	default:
		return fmt.Sprintf("%.0f %s", value, unit)
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
	a.pastePanel = rect{}
	a.pasteButtons = a.pasteButtons[:0]
	ctx := a.newUIContext(screen, func(btn chromeButton) {
		a.pasteButtons = append(a.pasteButtons, btn)
	})
	pasteOverlayRootElement{
		app: a,
		child: pasteOverlayElement{
			app:  a,
			snap: snap,
		},
	}.Draw(ctx, ui.Rect{W: float64(bounds.Dx()), H: float64(bounds.Dy())})
}

type statsOverlayElement struct {
	app    *App
	lines  []string
	graphs []graphMetric
}

func (e statsOverlayElement) Measure(ctx *ui.Context, constraints ui.Constraints) ui.Size {
	return ui.Column{Children: e.children()}.Measure(ctx, constraints)
}

func (e statsOverlayElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Column{Children: e.children()}.Draw(ctx, bounds)
}

func (e statsOverlayElement) children() []ui.Child {
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
	return children
}

type statsGraphElement struct {
	app    *App
	metric graphMetric
}

func (statsGraphElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: 58})
}

func (e statsGraphElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.MetricGraph{
		Title:  e.metric.Title,
		Value:  formatGraphValue(e.metric.Value, e.metric.Unit),
		Series: e.metric.Series,
	}.Draw(ctx, bounds)
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

type statsOverlayRootElement struct {
	app   *App
	child ui.Element
}

func (statsOverlayRootElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e statsOverlayRootElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ui.Inset{
		Insets: ui.Insets{Top: 58, Right: 16, Bottom: 16, Left: 16},
		Child: ui.Align{
			Horizontal: ui.AlignEnd,
			Vertical:   ui.AlignStart,
			Child: ui.Panel{
				Fill:   color.RGBA{R: 9, G: 14, B: 22, A: 224},
				Stroke: color.RGBA{R: 88, G: 108, B: 126, A: 180},
				Insets: ui.UniformInsets(16),
				Child:  e.child,
			},
		},
	}.Draw(ctx, bounds)
}

type pasteOverlayRootElement struct {
	app   *App
	child ui.Element
}

func (pasteOverlayRootElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e pasteOverlayRootElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	ctx.FillRect(ui.Rect{W: bounds.W, H: bounds.H}, color.RGBA{A: 168})
	ui.Inset{
		Insets: ui.Insets{Top: 48, Right: 36, Bottom: 48, Left: 36},
		Child: ui.Align{
			Horizontal: ui.AlignCenter,
			Vertical:   ui.AlignCenter,
			Child: ui.Constrained{
				MaxW:  760,
				MaxH:  420,
				Child: pastePanelElement(e),
			},
		},
	}.Draw(ctx, bounds)
}

type pastePanelElement struct {
	app   *App
	child ui.Element
}

func (pastePanelElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e pastePanelElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	e.app.pastePanel = rect{x: bounds.X, y: bounds.Y, w: bounds.W, h: bounds.H}
	ui.Panel{
		Fill:   color.RGBA{R: 13, G: 20, B: 30, A: 246},
		Stroke: color.RGBA{R: 88, G: 102, B: 118, A: 180},
		Insets: ui.UniformInsets(22),
		Child:  e.child,
	}.Draw(ctx, bounds)
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
