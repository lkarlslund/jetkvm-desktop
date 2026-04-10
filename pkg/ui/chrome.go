package ui

import "image/color"

type IconKind uint8

const (
	IconReconnect IconKind = iota
	IconMouse
	IconPaste
	IconMedia
	IconStats
	IconMinus
	IconPlus
	IconPower
	IconSettings
	IconFullscreen
	IconClose
)

type IconButton struct {
	Kind    IconKind
	Active  bool
	Enabled bool
	Alpha   float64
}

func (b IconButton) Measure(_ *Context, constraints Constraints) Size {
	return constraints.Clamp(Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (b IconButton) Draw(ctx *Context, bounds Rect) {
	fill := rgba(20, 30, 42, 220, b.Alpha)
	stroke := rgba(130, 146, 162, 160, b.Alpha)
	icon := rgba(236, 241, 245, 255, b.Alpha)
	if b.Active {
		fill = rgba(28, 66, 116, 232, b.Alpha)
		stroke = rgba(148, 198, 255, 210, b.Alpha)
	}
	if !b.Enabled {
		fill = rgba(20, 24, 32, 160, b.Alpha)
		stroke = rgba(86, 96, 108, 100, b.Alpha)
		icon = rgba(126, 136, 146, 180, b.Alpha)
	}
	ctx.FillRect(bounds, fill)
	ctx.StrokeRect(bounds, 1, stroke)
	drawIcon(ctx, b.Kind, bounds, icon, b.Active)
}

type Tooltip struct {
	Text  string
	Alpha float64
}

func (t Tooltip) Measure(_ *Context, constraints Constraints) Size {
	return constraints.Clamp(Size{W: constraints.MaxW, H: 28})
}

func (t Tooltip) Draw(ctx *Context, bounds Rect) {
	ctx.FillRect(bounds, rgba(8, 12, 18, 220, t.Alpha))
	ctx.StrokeRect(bounds, 1, rgba(112, 128, 148, 120, t.Alpha))
	DrawText(ctx.Screen, t.Text, bounds.X+10, bounds.Y+8, 13, rgba(236, 241, 245, 255, t.Alpha))
}

func drawIcon(ctx *Context, kind IconKind, r Rect, clr color.Color, active bool) {
	cx := r.X + r.W/2
	cy := r.Y + r.H/2
	left := r.X + 9
	right := r.X + r.W - 9
	top := r.Y + 9
	bottom := r.Y + r.H - 9
	mid := r.Y + r.H/2
	switch kind {
	case IconReconnect:
		ctx.StrokeLine(Point{left + 3, top + 1}, Point{right - 2, top + 1}, 1.5, clr)
		ctx.StrokeLine(Point{right - 2, top + 1}, Point{right - 2, bottom - 4}, 1.5, clr)
		ctx.StrokeLine(Point{right - 2, bottom - 4}, Point{left + 5, bottom - 4}, 1.5, clr)
		ctx.StrokeLine(Point{left + 5, bottom - 4}, Point{left + 5, mid + 1}, 1.5, clr)
		ctx.StrokeLine(Point{left + 5, mid + 1}, Point{left + 1, mid - 3}, 1.5, clr)
		ctx.StrokeLine(Point{left + 5, mid + 1}, Point{left + 9, mid - 3}, 1.5, clr)
	case IconMouse:
		if active {
			ctx.StrokeLine(Point{cx, top}, Point{cx, bottom}, 1.5, clr)
			ctx.StrokeLine(Point{left, cy}, Point{right, cy}, 1.5, clr)
			ctx.StrokeLine(Point{cx, top}, Point{cx - 3, top + 3}, 1.5, clr)
			ctx.StrokeLine(Point{cx, top}, Point{cx + 3, top + 3}, 1.5, clr)
			ctx.StrokeLine(Point{cx, bottom}, Point{cx - 3, bottom - 3}, 1.5, clr)
			ctx.StrokeLine(Point{cx, bottom}, Point{cx + 3, bottom - 3}, 1.5, clr)
		} else {
			ctx.StrokeLine(Point{left + 2, top}, Point{left + 2, bottom - 1}, 1.5, clr)
			ctx.StrokeLine(Point{left + 2, top}, Point{right - 1, cy}, 1.5, clr)
			ctx.StrokeLine(Point{left + 2, top}, Point{cx + 1, bottom - 2}, 1.5, clr)
		}
	case IconPaste:
		ctx.StrokeRect(Rect{X: left, Y: top + 2, W: right - left, H: bottom - top - 2}, 1.4, clr)
		ctx.StrokeLine(Point{left + 3, top + 6}, Point{right - 3, top + 6}, 1.4, clr)
		ctx.StrokeLine(Point{cx, top + 6}, Point{cx, top + 1}, 1.4, clr)
	case IconMedia:
		ctx.StrokeRect(Rect{X: left + 1, Y: top + 2, W: right - left - 2, H: bottom - top - 5}, 1.4, clr)
		ctx.StrokeLine(Point{left + 5, top + 2}, Point{left + 8, top - 1}, 1.4, clr)
		ctx.StrokeLine(Point{right - 5, top + 2}, Point{right - 8, top - 1}, 1.4, clr)
		ctx.StrokeLine(Point{left + 4, cy}, Point{right - 4, cy}, 1.4, clr)
	case IconStats:
		ctx.StrokeLine(Point{left + 2, bottom}, Point{left + 2, mid + 4}, 2, clr)
		ctx.StrokeLine(Point{cx, bottom}, Point{cx, top + 5}, 2, clr)
		ctx.StrokeLine(Point{right - 2, bottom}, Point{right - 2, mid - 1}, 2, clr)
	case IconMinus:
		ctx.StrokeLine(Point{left, cy}, Point{right, cy}, 2, clr)
	case IconPlus:
		ctx.StrokeLine(Point{left, cy}, Point{right, cy}, 2, clr)
		ctx.StrokeLine(Point{cx, top}, Point{cx, bottom}, 2, clr)
	case IconPower:
		ctx.StrokeLine(Point{cx, top - 1}, Point{cx, cy - 2}, 2, clr)
		ctx.StrokeLine(Point{left + 3, top + 4}, Point{left, mid}, 1.5, clr)
		ctx.StrokeLine(Point{left, mid}, Point{left + 4, bottom - 1}, 1.5, clr)
		ctx.StrokeLine(Point{left + 4, bottom - 1}, Point{right - 4, bottom - 1}, 1.5, clr)
		ctx.StrokeLine(Point{right - 4, bottom - 1}, Point{right, mid}, 1.5, clr)
		ctx.StrokeLine(Point{right, mid}, Point{right - 3, top + 4}, 1.5, clr)
	case IconSettings:
		ctx.StrokeLine(Point{left, top + 2}, Point{right, top + 2}, 1.5, clr)
		ctx.StrokeLine(Point{left, cy}, Point{right, cy}, 1.5, clr)
		ctx.StrokeLine(Point{left, bottom - 2}, Point{right, bottom - 2}, 1.5, clr)
		ctx.FillRect(Rect{X: cx - 6.5, Y: top - 0.5, W: 5, H: 5}, clr)
		ctx.FillRect(Rect{X: cx + 2.5, Y: cy - 2.5, W: 5, H: 5}, clr)
		ctx.FillRect(Rect{X: cx - 3.5, Y: bottom - 4.5, W: 5, H: 5}, clr)
	case IconFullscreen:
		ctx.StrokeLine(Point{left, top + 4}, Point{left, top}, 1.6, clr)
		ctx.StrokeLine(Point{left, top}, Point{left + 4, top}, 1.6, clr)
		ctx.StrokeLine(Point{right, top + 4}, Point{right, top}, 1.6, clr)
		ctx.StrokeLine(Point{right - 4, top}, Point{right, top}, 1.6, clr)
		ctx.StrokeLine(Point{left, bottom - 4}, Point{left, bottom}, 1.6, clr)
		ctx.StrokeLine(Point{left, bottom}, Point{left + 4, bottom}, 1.6, clr)
		ctx.StrokeLine(Point{right, bottom - 4}, Point{right, bottom}, 1.6, clr)
		ctx.StrokeLine(Point{right - 4, bottom}, Point{right, bottom}, 1.6, clr)
	case IconClose:
		ctx.StrokeLine(Point{left, top}, Point{right, bottom}, 1.8, clr)
		ctx.StrokeLine(Point{right, top}, Point{left, bottom}, 1.8, clr)
	}
}

func rgba(r, g, b, a uint8, alpha float64) color.Color {
	if alpha <= 0 {
		return color.RGBA{}
	}
	if alpha > 1 {
		alpha = 1
	}
	return color.RGBA{
		R: r,
		G: g,
		B: b,
		A: uint8(float64(a) * alpha),
	}
}
