package ui

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Point struct {
	X float64
	Y float64
}

type Size struct {
	W float64
	H float64
}

type Rect struct {
	X float64
	Y float64
	W float64
	H float64
}

func (r Rect) Inset(in Insets) Rect {
	x := r.X + in.Left
	y := r.Y + in.Top
	w := r.W - in.Left - in.Right
	h := r.H - in.Top - in.Bottom
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return Rect{X: x, Y: y, W: w, H: h}
}

func (r Rect) InnerSize(in Insets) Size {
	inner := r.Inset(in)
	return Size{W: inner.W, H: inner.H}
}

func (r Rect) Bottom() float64 {
	return r.Y + r.H
}

func (r Rect) Right() float64 {
	return r.X + r.W
}

type Insets struct {
	Top    float64
	Right  float64
	Bottom float64
	Left   float64
}

func UniformInsets(v float64) Insets {
	return Insets{Top: v, Right: v, Bottom: v, Left: v}
}

func SymmetricInsets(horizontal, vertical float64) Insets {
	return Insets{Top: vertical, Right: horizontal, Bottom: vertical, Left: horizontal}
}

type Constraints struct {
	MinW float64
	MaxW float64
	MinH float64
	MaxH float64
}

func NewConstraints(maxW, maxH float64) Constraints {
	return Constraints{MaxW: maxW, MaxH: maxH}
}

func (c Constraints) Tighten(size Size) Constraints {
	return Constraints{
		MinW: size.W,
		MaxW: size.W,
		MinH: size.H,
		MaxH: size.H,
	}
}

func (c Constraints) Clamp(size Size) Size {
	size.W = clamp(size.W, c.MinW, c.maxWidth())
	size.H = clamp(size.H, c.MinH, c.maxHeight())
	return size
}

func (c Constraints) Deflate(in Insets) Constraints {
	deflated := Constraints{
		MinW: max(0, c.MinW-in.Left-in.Right),
		MaxW: max(0, c.maxWidth()-in.Left-in.Right),
		MinH: max(0, c.MinH-in.Top-in.Bottom),
		MaxH: max(0, c.maxHeight()-in.Top-in.Bottom),
	}
	if deflated.MaxW == 0 && c.MaxW == 0 {
		deflated.MaxW = 0
	}
	if deflated.MaxH == 0 && c.MaxH == 0 {
		deflated.MaxH = 0
	}
	return deflated
}

func (c Constraints) maxWidth() float64 {
	if c.MaxW <= 0 {
		return math.MaxFloat64
	}
	return c.MaxW
}

func (c Constraints) maxHeight() float64 {
	if c.MaxH <= 0 {
		return math.MaxFloat64
	}
	return c.MaxH
}

type HitTarget struct {
	ID      string
	Rect    Rect
	Enabled bool
}

type Theme struct {
	Backdrop      color.Color
	ModalFill     color.Color
	ModalStroke   color.Color
	PanelFill     color.Color
	PanelStroke   color.Color
	SectionFill   color.Color
	SectionStroke color.Color
	Title         color.Color
	Body          color.Color
	Muted         color.Color
	Error         color.Color
	ButtonFill    color.Color
	ButtonStroke  color.Color
	ButtonText    color.Color
	ActiveFill    color.Color
	ActiveStroke  color.Color
	DisabledFill  color.Color
	DisabledText  color.Color
	InputFill     color.Color
	InputStroke   color.Color
	InputFocus    color.Color
	ProgressTrack color.Color
	ProgressFill  color.Color
}

func DefaultTheme() Theme {
	return Theme{
		Backdrop:      color.RGBA{A: 160},
		ModalFill:     color.RGBA{R: 10, G: 16, B: 24, A: 244},
		ModalStroke:   color.RGBA{R: 110, G: 130, B: 152, A: 110},
		PanelFill:     color.RGBA{R: 14, G: 22, B: 32, A: 234},
		PanelStroke:   color.RGBA{R: 84, G: 104, B: 122, A: 110},
		SectionFill:   color.RGBA{R: 18, G: 26, B: 38, A: 236},
		SectionStroke: color.RGBA{R: 94, G: 115, B: 136, A: 110},
		Title:         color.RGBA{R: 236, G: 241, B: 245, A: 255},
		Body:          color.RGBA{R: 236, G: 241, B: 245, A: 255},
		Muted:         color.RGBA{R: 166, G: 178, B: 190, A: 255},
		Error:         color.RGBA{R: 252, G: 165, B: 165, A: 255},
		ButtonFill:    color.RGBA{R: 18, G: 26, B: 38, A: 255},
		ButtonStroke:  color.RGBA{R: 84, G: 104, B: 122, A: 120},
		ButtonText:    color.RGBA{R: 236, G: 241, B: 245, A: 255},
		ActiveFill:    color.RGBA{R: 34, G: 78, B: 130, A: 255},
		ActiveStroke:  color.RGBA{R: 147, G: 197, B: 253, A: 180},
		DisabledFill:  color.RGBA{R: 18, G: 24, B: 32, A: 200},
		DisabledText:  color.RGBA{R: 110, G: 120, B: 132, A: 255},
		InputFill:     color.RGBA{R: 8, G: 12, B: 18, A: 255},
		InputStroke:   color.RGBA{R: 84, G: 104, B: 122, A: 120},
		InputFocus:    color.RGBA{R: 96, G: 165, B: 250, A: 180},
		ProgressTrack: color.RGBA{R: 22, G: 30, B: 44, A: 255},
		ProgressFill:  color.RGBA{R: 48, G: 123, B: 206, A: 255},
	}
}

type Context struct {
	Screen            *ebiten.Image
	Theme             Theme
	MeasureText       func(value string, size float64) (float64, float64)
	MeasureWrapped    func(value string, width, size float64) float64
	DrawText          func(dst *ebiten.Image, value string, x, y, size float64, clr color.Color)
	DrawWrappedText   func(dst *ebiten.Image, value string, x, y, width, size float64, clr color.Color) float64
	RegisterHitTarget func(HitTarget)
}

func (c *Context) FillRect(r Rect, clr color.Color) {
	vector.FillRect(c.Screen, float32(r.X), float32(r.Y), float32(r.W), float32(r.H), clr, false)
}

func (c *Context) StrokeRect(r Rect, width float64, clr color.Color) {
	vector.StrokeRect(c.Screen, float32(r.X), float32(r.Y), float32(r.W), float32(r.H), float32(width), clr, false)
}

func (c *Context) AddHit(id string, r Rect, enabled bool) {
	if c.RegisterHitTarget == nil || id == "" {
		return
	}
	c.RegisterHitTarget(HitTarget{ID: id, Rect: r, Enabled: enabled})
}

type Element interface {
	Measure(ctx *Context, constraints Constraints) Size
	Draw(ctx *Context, bounds Rect)
}

type fill struct {
	clr color.Color
}

func Fill(clr color.Color) Element {
	return fill{clr: clr}
}

func (f fill) Measure(_ *Context, constraints Constraints) Size {
	return constraints.Clamp(Size{})
}

func (f fill) Draw(ctx *Context, bounds Rect) {
	ctx.FillRect(bounds, f.clr)
}

func clamp(value, low, high float64) float64 {
	if high < low {
		high = low
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
