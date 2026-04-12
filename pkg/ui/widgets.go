package ui

import (
	"image/color"
	"time"
)

type Label struct {
	Text  string
	Size  float64
	Color color.Color
}

type Backdrop struct {
	Color color.Color
}

func (Backdrop) Measure(_ *Context, constraints Constraints) Size {
	return constraints.Clamp(Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (b Backdrop) Draw(ctx *Context, bounds Rect) {
	ctx.FillRect(bounds, b.Color)
}

func (l Label) Measure(ctx *Context, constraints Constraints) Size {
	if ctx.MeasureText == nil || l.Text == "" {
		return constraints.Clamp(Size{})
	}
	w, _ := ctx.MeasureText(l.Text, l.Size)
	h := LineHeight(l.Size)
	return constraints.Clamp(Size{W: w, H: h})
}

func (l Label) Draw(ctx *Context, bounds Rect) {
	if l.Text == "" || ctx.DrawText == nil {
		return
	}
	ctx.DrawText(ctx.Screen, l.Text, bounds.X, bounds.Y, l.Size, l.Color)
}

type Paragraph struct {
	Text  string
	Size  float64
	Color color.Color
}

func (p Paragraph) Measure(ctx *Context, constraints Constraints) Size {
	if ctx.MeasureWrapped == nil || p.Text == "" {
		return constraints.Clamp(Size{})
	}
	width := constraints.MaxW
	if width <= 0 {
		width = constraints.maxWidth()
	}
	height := ctx.MeasureWrapped(p.Text, width, p.Size)
	return constraints.Clamp(Size{W: width, H: height})
}

func (p Paragraph) Draw(ctx *Context, bounds Rect) {
	if p.Text == "" || ctx.DrawWrappedText == nil {
		return
	}
	ctx.DrawWrappedText(ctx.Screen, p.Text, bounds.X, bounds.Y, bounds.W, p.Size, p.Color)
}

type Button struct {
	ID      string
	Label   string
	Enabled bool
	Active  bool
	Pending bool
	MinW    float64
	Width   float64
}

type Toggle struct {
	ID      string
	Enabled bool
	Active  bool
	Pending bool
}

func (t Toggle) Measure(_ *Context, constraints Constraints) Size {
	return constraints.Clamp(Size{W: 46, H: 24})
}

func (t Toggle) Draw(ctx *Context, bounds Rect) {
	trackFill := ctx.Theme.ButtonFill
	trackStroke := ctx.Theme.ButtonStroke
	knobFill := ctx.Theme.ButtonText
	knobStroke := ctx.Theme.ButtonStroke
	if t.Active {
		trackFill = ctx.Theme.ActiveFill
		trackStroke = ctx.Theme.ActiveStroke
	}
	if t.Pending {
		trackFill = ctx.Theme.WarningFill
		trackStroke = ctx.Theme.WarningStroke
		knobStroke = trackStroke
	}
	if !t.Enabled {
		trackFill = ctx.Theme.DisabledFill
		trackStroke = ctx.Theme.ButtonStroke
		knobFill = ctx.Theme.DisabledText
		knobStroke = ctx.Theme.ButtonStroke
	}
	cy := bounds.Y + bounds.H/2
	ctx.FillStrokedRoundedRect(bounds, 1, bounds.H/2, trackStroke, trackFill)
	knobRadius := (bounds.H - 6) / 2
	knobCx := bounds.X + 3 + knobRadius
	if t.Active {
		knobCx = bounds.Right() - 3 - knobRadius
	}
	ctx.FillCircle(Point{X: knobCx, Y: cy}, knobRadius, knobFill)
	ctx.StrokeLine(Point{X: knobCx, Y: cy}, Point{X: knobCx, Y: cy}, knobRadius*2, knobStroke)
	ctx.AddHit(t.ID, bounds, t.Enabled)
}

func (b Button) Measure(ctx *Context, constraints Constraints) Size {
	labelW, labelH := 0.0, 0.0
	if ctx.MeasureText != nil {
		labelW, _ = ctx.MeasureText(b.Label, 13)
		labelH = LineHeight(13)
	}
	width := b.Width
	if width <= 0 {
		minW := b.MinW
		if minW <= 0 {
			minW = 64
		}
		width = max(minW, labelW+24)
	}
	return constraints.Clamp(Size{W: width, H: max(30, labelH+14)})
}

func (b Button) Draw(ctx *Context, bounds Rect) {
	fill := ctx.Theme.ButtonFill
	stroke := ctx.Theme.ButtonStroke
	label := ctx.Theme.ButtonText
	if b.Active {
		fill = ctx.Theme.ActiveFill
		stroke = ctx.Theme.ActiveStroke
	}
	if b.Pending {
		fill = ctx.Theme.WarningFill
		stroke = ctx.Theme.WarningStroke
	}
	if !b.Enabled {
		fill = ctx.Theme.DisabledFill
		label = ctx.Theme.DisabledText
	}
	ctx.FillRect(bounds, fill)
	ctx.StrokeRect(bounds, 1, stroke)
	ctx.AddHit(b.ID, bounds, b.Enabled)
	if ctx.DrawText == nil || b.Label == "" {
		return
	}
	tw, _ := ctx.MeasureText(b.Label, 13)
	th := LineHeight(13)
	ctx.DrawText(ctx.Screen, b.Label, bounds.X+(bounds.W-tw)/2, bounds.Y+(bounds.H-th)/2, 13, label)
}

type KeyValue struct {
	Label      string
	Value      string
	LabelWidth float64
}

func (kv KeyValue) Measure(ctx *Context, constraints Constraints) Size {
	labelW, _ := ctx.MeasureText(kv.Label, 13)
	valueW, _ := ctx.MeasureText(kv.Value, 13)
	labelH := LineHeight(13)
	valueH := LineHeight(13)
	return constraints.Clamp(Size{W: max(kv.LabelWidth, labelW) + 12 + valueW, H: max(labelH, valueH)})
}

func (kv KeyValue) Draw(ctx *Context, bounds Rect) {
	ctx.DrawText(ctx.Screen, kv.Label, bounds.X, bounds.Y, 13, ctx.Theme.Muted)
	ctx.DrawText(ctx.Screen, kv.Value, bounds.X+kv.LabelWidth+12, bounds.Y, 13, ctx.Theme.Body)
}

type TextField struct {
	ID               string
	Value            string
	DisplayValue     string
	Placeholder      string
	CaretIndex       int
	SelectionStart   int
	SelectionEnd     int
	Focused          bool
	Enabled          bool
	TextSize         float64
	FillColor        color.Color
	StrokeColor      color.Color
	FocusColor       color.Color
	TextColor        color.Color
	PlaceholderColor color.Color
	CaretColor       color.Color
}

func (t TextField) Measure(_ *Context, constraints Constraints) Size {
	height := 38.0
	width := constraints.MaxW
	if width <= 0 {
		width = constraints.maxWidth()
	}
	return constraints.Clamp(Size{W: width, H: height})
}

func (t TextField) Draw(ctx *Context, bounds Rect) {
	stroke := ctx.Theme.InputStroke
	if t.StrokeColor != nil {
		stroke = t.StrokeColor
	}
	if t.Focused {
		stroke = ctx.Theme.InputFocus
		if t.FocusColor != nil {
			stroke = t.FocusColor
		}
	}
	fill := ctx.Theme.InputFill
	if t.FillColor != nil {
		fill = t.FillColor
	}
	ctx.FillRect(bounds, fill)
	ctx.StrokeRect(bounds, 1, stroke)
	ctx.AddHit(t.ID, bounds, t.Enabled)
	if ctx.DrawText == nil {
		return
	}
	textSize := t.TextSize
	if textSize <= 0 {
		textSize = 13
	}
	text := t.DisplayValue
	if text == "" {
		text = t.Value
	}
	textColor := ctx.Theme.Body
	if t.TextColor != nil {
		textColor = t.TextColor
	}
	showPlaceholder := text == ""
	if text == "" {
		text = t.Placeholder
		textColor = ctx.Theme.DisabledText
		if t.PlaceholderColor != nil {
			textColor = t.PlaceholderColor
		}
	}
	textY := bounds.Y + (bounds.H-LineHeight(textSize))/2
	textX := bounds.X + 12
	textWidth := max(0, bounds.W-24)
	runes := []rune(text)
	caretIndex := 0
	var advances []float64
	if !showPlaceholder {
		caretIndex = t.CaretIndex
		if caretIndex < 0 {
			caretIndex = 0
		}
		if caretIndex > len(runes) {
			caretIndex = len(runes)
		}
		advances = PrefixAdvances(text, textSize)
	}
	scrollX := 0.0
	if !showPlaceholder && textWidth > 0 {
		caretPixel := advances[caretIndex]
		textPixel := advances[len(advances)-1]
		if textPixel > textWidth {
			scrollX = caretPixel - textWidth + 8
			if scrollX < 0 {
				scrollX = 0
			}
			maxScroll := textPixel - textWidth
			if scrollX > maxScroll {
				scrollX = maxScroll
			}
		}
	}
	visibleStart := 0
	visibleEnd := len(runes)
	if !showPlaceholder && textWidth > 0 && len(runes) > 0 {
		for visibleStart < len(runes) {
			if advances[visibleStart+1] > scrollX {
				break
			}
			visibleStart++
		}
		visibleEnd = len(runes)
		for visibleEnd > visibleStart {
			visibleW := advances[visibleEnd] - advances[visibleStart]
			if visibleW <= textWidth {
				break
			}
			visibleEnd--
		}
		if visibleEnd < visibleStart {
			visibleEnd = visibleStart
		}
	}
	visibleText := text
	if !showPlaceholder {
		visibleText = string(runes[visibleStart:visibleEnd])
	}
	if !showPlaceholder && t.SelectionStart != t.SelectionEnd {
		startIndex := t.SelectionStart
		if startIndex < 0 {
			startIndex = 0
		}
		if startIndex > len(runes) {
			startIndex = len(runes)
		}
		endIndex := t.SelectionEnd
		if endIndex < 0 {
			endIndex = 0
		}
		if endIndex > len(runes) {
			endIndex = len(runes)
		}
		if endIndex < startIndex {
			startIndex, endIndex = endIndex, startIndex
		}
		highlightStart := max(float64(startIndex), float64(visibleStart))
		highlightEnd := min(float64(endIndex), float64(visibleEnd))
		if highlightEnd > highlightStart {
			prefixW := advances[int(highlightStart)] - advances[visibleStart]
			selectedW := advances[int(highlightEnd)] - advances[int(highlightStart)]
			ctx.FillRect(Rect{X: textX + prefixW - 1, Y: textY - 1, W: selectedW + 2, H: LineHeight(textSize) + 2}, ctx.Theme.ActiveFill)
		}
	}
	ctx.DrawText(ctx.Screen, visibleText, textX, textY, textSize, textColor)
	if !t.Focused || time.Now().UnixNano()/500_000_000%2 != 0 {
		return
	}
	caretX := textX
	if !showPlaceholder {
		caretBase := caretIndex
		if caretBase < visibleStart {
			caretBase = visibleStart
		}
		if caretBase > visibleEnd {
			caretBase = visibleEnd
		}
		textW := advances[caretBase] - advances[visibleStart]
		caretX += textW + 2
	}
	caretH := LineHeight(textSize)
	caretColor := ctx.Theme.Body
	if t.CaretColor != nil {
		caretColor = t.CaretColor
	}
	ctx.FillRect(Rect{X: caretX, Y: textY, W: 2, H: caretH}, caretColor)
}

type ProgressBar struct {
	Progress float64
}

func (p ProgressBar) Measure(_ *Context, constraints Constraints) Size {
	width := constraints.MaxW
	if width <= 0 {
		width = constraints.maxWidth()
	}
	return constraints.Clamp(Size{W: width, H: 18})
}

func (p ProgressBar) Draw(ctx *Context, bounds Rect) {
	progress := clamp(p.Progress, 0, 1)
	ctx.FillRect(bounds, ctx.Theme.ProgressTrack)
	ctx.FillRect(Rect{X: bounds.X, Y: bounds.Y, W: bounds.W * progress, H: bounds.H}, ctx.Theme.ProgressFill)
	ctx.StrokeRect(bounds, 1, ctx.Theme.PanelStroke)
}

type ModalFrame struct {
	Title       string
	Subtitle    string
	CloseButton *Button
	State       Element
	Body        Element
	Footer      Element
	Width       float64
	MaxWidth    float64
	Height      float64
	MaxHeight   float64
}

type FooterStatus struct {
	Left       string
	Right      string
	Size       float64
	LeftColor  color.Color
	RightColor color.Color
	Insets     Insets
}

func (f FooterStatus) Measure(_ *Context, constraints Constraints) Size {
	height := LineHeight(max(f.Size, 12)) + f.Insets.Top + f.Insets.Bottom
	return constraints.Clamp(Size{W: constraints.MaxW, H: height})
}

func (f FooterStatus) Draw(ctx *Context, bounds Rect) {
	size := f.Size
	if size <= 0 {
		size = 12
	}
	leftColor := f.LeftColor
	if leftColor == nil {
		leftColor = ctx.Theme.Muted
	}
	rightColor := f.RightColor
	if rightColor == nil {
		rightColor = ctx.Theme.Error
	}
	textY := bounds.Y + f.Insets.Top
	if f.Left != "" {
		ctx.DrawText(ctx.Screen, f.Left, bounds.X+f.Insets.Left, textY, size, leftColor)
	}
	if f.Right != "" {
		w, _ := ctx.MeasureText(f.Right, size)
		ctx.DrawText(ctx.Screen, f.Right, bounds.Right()-f.Insets.Right-w, textY, size, rightColor)
	}
}

func (m ModalFrame) Measure(_ *Context, constraints Constraints) Size {
	w := m.Width
	if m.MaxWidth > 0 {
		w = minPositive(w, m.MaxWidth)
	}
	if w <= 0 || w > constraints.MaxW {
		w = constraints.MaxW
	}
	h := m.Height
	if m.MaxHeight > 0 {
		h = minPositive(h, m.MaxHeight)
	}
	if h <= 0 || h > constraints.MaxH {
		h = constraints.MaxH
	}
	return constraints.Clamp(Size{W: w, H: h})
}

func (m ModalFrame) Draw(ctx *Context, bounds Rect) {
	ctx.FillRect(Rect{W: bounds.W + bounds.X, H: bounds.H + bounds.Y}, color.Transparent)
	panel := Rect{
		X: bounds.X + (bounds.W-m.Width)/2,
		Y: bounds.Y + (bounds.H-m.Height)/2,
		W: m.Width,
		H: m.Height,
	}
	ctx.FillRect(Rect{X: bounds.X, Y: bounds.Y, W: bounds.W, H: bounds.H}, ctx.Theme.Backdrop)
	ctx.FillRect(panel, ctx.Theme.ModalFill)
	ctx.StrokeRect(panel, 1, ctx.Theme.ModalStroke)

	headerX := panel.X + 18
	headerY := panel.Y + 18
	if ctx.DrawText != nil {
		ctx.DrawText(ctx.Screen, m.Title, headerX, headerY, 26, ctx.Theme.Title)
	}
	if m.Subtitle != "" && ctx.DrawWrappedText != nil {
		ctx.DrawWrappedText(ctx.Screen, m.Subtitle, headerX, panel.Y+52, panel.W-72, 12, ctx.Theme.Muted)
	}
	if m.CloseButton != nil {
		m.CloseButton.Draw(ctx, Rect{X: panel.Right() - 38, Y: panel.Y + 12, W: 24, H: 24})
	}
	stateTop := panel.Y + 88
	if m.State != nil {
		m.State.Draw(ctx, Rect{X: panel.X + 18, Y: stateTop, W: panel.W - 36, H: 112})
	}
	bodyTop := stateTop + 112 + 18 + 44
	bodyHeight := panel.H - (bodyTop - panel.Y) - 18
	bodyRect := Rect{X: panel.X + 18, Y: bodyTop, W: panel.W - 36, H: bodyHeight}
	if m.Body != nil {
		Panel{
			Fill:   ctx.Theme.PanelFill,
			Stroke: ctx.Theme.PanelStroke,
			Child:  m.Body,
		}.Draw(ctx, bodyRect)
	}
	if m.Footer != nil {
		m.Footer.Draw(ctx, bodyRect)
	}
}

func minPositive(a, b float64) float64 {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}
