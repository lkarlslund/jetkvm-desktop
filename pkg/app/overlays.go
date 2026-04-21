package app

import (
	"fmt"
	"image/color"
	"math"
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

func (a *App) openSerialConsoleOverlay() {
	if a.ctrl == nil {
		return
	}
	snap := a.ctrl.Snapshot()
	if snap.ActiveExtension != "serial-console" {
		return
	}
	a.releaseAllKeys(true)
	a.settingsOpen = false
	a.pasteOpen = false
	a.mediaOpen = false
	a.serialConsoleOpen = true
	a.serialConsoleScroll = 0
	a.settingsInputFocus = settingsInputNone
	a.applyCursorMode()
}

func (a *App) syncSerialConsoleInput() {
	if !a.serialConsoleOpen || !a.focused || a.ctrl == nil || a.ctrl.Snapshot().Phase != session.PhaseConnected {
		return
	}
	rawKeys := inpututil.AppendPressedKeys(nil)
	if a.suppressKeysUntilClear {
		if len(rawKeys) == 0 {
			a.suppressKeysUntilClear = false
		}
		return
	}

	_, wheelY := ebiten.Wheel()
	if wheelY != 0 {
		lines := int(math.Round(math.Abs(wheelY) * 3))
		if lines < 1 {
			lines = 1
		}
		if wheelY > 0 {
			a.serialConsoleScroll += lines
		} else {
			a.serialConsoleScroll -= lines
			if a.serialConsoleScroll < 0 {
				a.serialConsoleScroll = 0
			}
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		_ = a.ctrl.SendSerialTerminator()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		_ = a.ctrl.SendSerialText("\x7f")
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		_ = a.ctrl.SendSerialText("\t")
	}
	chars := ebiten.AppendInputChars(nil)
	if len(chars) != 0 {
		_ = a.ctrl.SendSerialText(string(chars))
	}
}

func (a *App) updatePastePreview() {
	_, invalid := input.BuildPasteMacro(a.ctrl.Snapshot().KeyboardLayout, a.pasteText, a.pasteDelay)
	a.pasteInvalid = input.InvalidRunesString(invalid)
}

func (a *App) loadClipboardText() {
	a.pasteError = ""
	text, err := readClipboardText()
	if err != nil {
		a.pasteError = err.Error()
		return
	}
	if text == "" {
		a.pasteText = ""
		a.pasteInvalid = ""
		return
	}
	a.pasteText = text
	a.updatePastePreview()
}

func readClipboardText() (string, error) {
	if !clipboardReady {
		return "", fmt.Errorf("system clipboard is not available on this platform/session")
	}
	data := clipboard.Read(clipboard.FmtText)
	if len(data) == 0 {
		return "", nil
	}
	return string(data), nil
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
	a.drawUIRoot(screen, nil, func(chromeButton) {}, statsOverlayRootElement{
		app: a,
		child: statsOverlayElement{
			app:    a,
			lines:  lines,
			graphs: graphs,
		},
	})
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

func (a *App) drawSerialConsoleOverlay(screen *ebiten.Image, snap session.Snapshot) {
	if !a.serialConsoleOpen {
		return
	}
	a.drawUIRoot(screen, &a.serialConsoleRuntime, func(chromeButton) {}, serialConsoleOverlayRootElement{
		app:  a,
		snap: snap,
	})
}

type serialConsoleOverlayRootElement struct {
	app  *App
	snap session.Snapshot
}

func (serialConsoleOverlayRootElement) Measure(_ *ui.Context, constraints ui.Constraints) ui.Size {
	return constraints.Clamp(ui.Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (e serialConsoleOverlayRootElement) Draw(ctx *ui.Context, bounds ui.Rect) {
	if ctx.Runtime != nil {
		ctx.Runtime.Register(ui.Control{
			ID:      "serial_console_backdrop",
			Rect:    bounds,
			Enabled: true,
			OnClick: func(ui.PointerEvent) {},
		})
	}
	ui.Backdrop{Color: colorRGBA(0, 0, 0, 132)}.Draw(ctx, bounds)
	panelW := min(bounds.W-72, 1040)
	panelH := min(bounds.H-96, 680)
	if panelW < 320 {
		panelW = bounds.W - 24
	}
	if panelH < 220 {
		panelH = bounds.H - 24
	}
	panel := ui.Rect{
		X: bounds.X + (bounds.W-panelW)/2,
		Y: bounds.Y + (bounds.H-panelH)/2,
		W: panelW,
		H: panelH,
	}
	e.app.serialConsolePanel = rect{x: panel.X, y: panel.Y, w: panel.W, h: panel.H}
	ctx.FillStrokedRoundedRect(panel, 1, 8, ctx.Theme.ModalStroke, ctx.Theme.ModalFill)

	titleY := panel.Y + 18
	ctx.DrawText(ctx.Screen, "Serial Console", panel.X+18, titleY, 20, ctx.Theme.Title)

	closeRect := ui.Rect{X: panel.Right() - 54, Y: panel.Y + 14, W: 36, H: 28}
	ui.Button{Label: "X", Enabled: true, OnClick: e.app.closeSerialConsoleOverlay}.Draw(ctx, closeRect)

	status := "Connected"
	if e.snap.ActiveExtension != "serial-console" {
		status = "Serial extension is not active"
	} else if !e.snap.SerialConsoleReady {
		status = "Waiting for serial channel"
	}
	statusColor := ctx.Theme.Muted
	if e.snap.SerialConsoleError != "" {
		status = e.snap.SerialConsoleError
		statusColor = ctx.Theme.Error
	}
	ctx.DrawText(ctx.Screen, status, panel.X+18, panel.Y+48, 12, statusColor)

	textRect := ui.Rect{
		X: panel.X + 18,
		Y: panel.Y + 72,
		W: panel.W - 36,
		H: panel.H - 118,
	}
	ctx.FillRect(textRect, ctx.Theme.InputFill)
	ctx.StrokeRect(textRect, 1, ctx.Theme.InputStroke)

	lines := serialConsoleDisplayLines(e.snap.SerialConsoleBuffer)
	lineHeight := ui.LineHeight(12)
	visibleLines := maxInt(1, int((textRect.H-12)/lineHeight))
	maxScroll := maxInt(0, len(lines)-visibleLines)
	if e.app.serialConsoleScroll > maxScroll {
		e.app.serialConsoleScroll = maxScroll
	}
	start := maxInt(0, len(lines)-visibleLines-e.app.serialConsoleScroll)
	end := minInt(len(lines), start+visibleLines)
	y := textRect.Y + 8
	for _, line := range lines[start:end] {
		ctx.DrawText(ctx.Screen, line, textRect.X+8, y, 12, ctx.Theme.Body)
		y += lineHeight
	}
	if len(lines) == 0 {
		ctx.DrawText(ctx.Screen, "No serial output yet.", textRect.X+8, textRect.Y+8, 12, ctx.Theme.Muted)
	}

	hint := "Typing is sent directly to the serial console. Esc closes."
	if e.snap.SerialConsoleTruncated {
		hint = "Typing is sent directly to the serial console. Older scrollback has been truncated."
	}
	ctx.DrawText(ctx.Screen, hint, panel.X+18, panel.Bottom()-28, 12, ctx.Theme.Muted)
}

func serialConsoleDisplayLines(buffer string) []string {
	if buffer == "" {
		return nil
	}
	normalized := strings.ReplaceAll(buffer, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}

func colorRGBA(r, g, b, a uint8) color.Color {
	return color.RGBA{R: r, G: g, B: b, A: a}
}

func (a *App) drawPasteOverlay(screen *ebiten.Image, snap session.Snapshot) {
	if !a.pasteOpen {
		a.pasteRuntime.BeginFrame()
		return
	}
	bounds := screen.Bounds()
	a.pastePanel = rect{}
	_ = bounds
	a.drawUIRoot(screen, &a.pasteRuntime, func(chromeButton) {}, pasteOverlayRootElement{
		app: a,
		child: pasteOverlayElement{
			app:  a,
			snap: snap,
		},
	})
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
	theme := e.app.currentTheme()
	children := []ui.Child{
		ui.Fixed(ui.Label{Text: "Connection Stats", Size: 14, Color: theme.Title}),
		ui.Fixed(ui.Spacer{H: 8}),
	}
	for i, line := range e.lines {
		if i > 0 {
			children = append(children, ui.Fixed(ui.Spacer{H: 6}))
		}
		children = append(children, ui.Fixed(ui.Label{Text: line, Size: 12, Color: theme.Body}))
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
		ui.Fixed(ui.Label{Text: "Paste Text", Size: 22, Color: ctx.Theme.Title}),
		ui.Fixed(ui.Spacer{H: 12}),
		ui.Fixed(ui.Paragraph{
			Text:  "Send clipboard text to the remote host using keyboard macro steps over HID-RPC. Unsupported characters are skipped.",
			Size:  13,
			Color: ctx.Theme.Muted,
		}),
		ui.Fixed(ui.Spacer{H: 18}),
		ui.Fixed(metaRow),
		ui.Fixed(ui.Spacer{H: 16}),
		ui.Flex(ui.Panel{
			Fill:   ctx.Theme.PanelFill,
			Stroke: ctx.Theme.PanelStroke,
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
				Color: ctx.Theme.WarningStroke,
			}),
			ui.Fixed(ui.Spacer{H: 6}),
		)
	}
	if e.app.pasteError != "" {
		bodyChildren = append(bodyChildren,
			ui.Fixed(ui.Paragraph{Text: e.app.pasteError, Size: 12, Color: ctx.Theme.Error}),
			ui.Fixed(ui.Spacer{H: 6}),
		)
	}
	if e.snap.PasteInProgress {
		bodyChildren = append(bodyChildren,
			ui.Fixed(ui.Label{Text: "Paste in progress…", Size: 12, Color: ctx.Theme.AccentText}),
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
				Fill:   ctx.Theme.ModalFill,
				Stroke: ctx.Theme.ModalStroke,
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
	ui.Stack{Children: []ui.Element{
		ui.Backdrop{Color: ctx.Theme.Backdrop},
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
		},
	}}.Draw(ctx, bounds)
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
		Fill:   ctx.Theme.ModalFill,
		Stroke: ctx.Theme.ModalStroke,
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
			Color: ctx.Theme.DisabledText,
		}.Draw(ctx, bounds)
		return
	}
	ui.Paragraph{
		Text:  e.app.pasteText,
		Size:  14,
		Color: ctx.Theme.Body,
	}.Draw(ctx, bounds)
}
