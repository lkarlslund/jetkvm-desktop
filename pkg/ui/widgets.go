package ui

import (
	"image/color"
	"strings"
	"time"
)

type Label struct {
	Text  string
	Size  float64
	Color color.Color
}

func (l Label) Measure(ctx *Context, constraints Constraints) Size {
	if ctx.MeasureText == nil || l.Text == "" {
		return constraints.Clamp(Size{})
	}
	w, h := ctx.MeasureText(l.Text, l.Size)
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
}

func (b Button) Measure(ctx *Context, constraints Constraints) Size {
	labelW, labelH := 0.0, 0.0
	if ctx.MeasureText != nil {
		labelW, labelH = ctx.MeasureText(b.Label, 13)
	}
	return constraints.Clamp(Size{W: max(64, labelW+24), H: max(30, labelH+14)})
}

func (b Button) Draw(ctx *Context, bounds Rect) {
	fill := ctx.Theme.ButtonFill
	stroke := ctx.Theme.ButtonStroke
	label := ctx.Theme.ButtonText
	if b.Active {
		fill = ctx.Theme.ActiveFill
		stroke = ctx.Theme.ActiveStroke
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
	tw, th := ctx.MeasureText(b.Label, 13)
	ctx.DrawText(ctx.Screen, b.Label, bounds.X+(bounds.W-tw)/2, bounds.Y+(bounds.H-th)/2, 13, label)
}

type KeyValue struct {
	Label      string
	Value      string
	LabelWidth float64
}

func (kv KeyValue) Measure(ctx *Context, constraints Constraints) Size {
	labelW, labelH := ctx.MeasureText(kv.Label, 13)
	valueW, valueH := ctx.MeasureText(kv.Value, 13)
	return constraints.Clamp(Size{W: max(kv.LabelWidth, labelW) + 12 + valueW, H: max(labelH, valueH)})
}

func (kv KeyValue) Draw(ctx *Context, bounds Rect) {
	ctx.DrawText(ctx.Screen, kv.Label, bounds.X, bounds.Y, 13, ctx.Theme.Muted)
	ctx.DrawText(ctx.Screen, kv.Value, bounds.X+kv.LabelWidth+12, bounds.Y, 13, ctx.Theme.Body)
}

type TextField struct {
	ID          string
	Value       string
	Placeholder string
	Focused     bool
	Enabled     bool
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
	if t.Focused {
		stroke = ctx.Theme.InputFocus
	}
	ctx.FillRect(bounds, ctx.Theme.InputFill)
	ctx.StrokeRect(bounds, 1, stroke)
	ctx.AddHit(t.ID, bounds, t.Enabled)
	if ctx.DrawText == nil {
		return
	}
	text := t.Value
	textColor := ctx.Theme.Body
	if strings.TrimSpace(text) == "" {
		text = t.Placeholder
		textColor = ctx.Theme.DisabledText
	}
	if t.Focused && strings.TrimSpace(t.Value) != "" && time.Now().UnixNano()/500_000_000%2 == 0 {
		text += "|"
	}
	ctx.DrawText(ctx.Screen, text, bounds.X+12, bounds.Y+12, 13, textColor)
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
