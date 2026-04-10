package ui

import "image/color"

type Child struct {
	Element Element
	Flex    float64
}

func Fixed(element Element) Child {
	return Child{Element: element}
}

func Flex(element Element, weight float64) Child {
	return Child{Element: element, Flex: weight}
}

type Spacer struct {
	W float64
	H float64
}

func (s Spacer) Measure(_ *Context, constraints Constraints) Size {
	return constraints.Clamp(Size(s))
}

func (Spacer) Draw(_ *Context, _ Rect) {}

type Column struct {
	Children []Child
	Spacing  float64
}

func (c Column) Measure(ctx *Context, constraints Constraints) Size {
	available := constraints.maxHeight()
	totalH := 0.0
	maxW := 0.0
	totalFlex := 0.0
	count := 0
	for _, child := range c.Children {
		if child.Element == nil {
			continue
		}
		if count > 0 {
			totalH += c.Spacing
		}
		count++
		if child.Flex > 0 {
			totalFlex += child.Flex
			continue
		}
		size := child.Element.Measure(ctx, Constraints{
			MaxW: constraints.MaxW,
			MaxH: max(0, available-totalH),
		})
		totalH += size.H
		maxW = max(maxW, size.W)
	}
	if totalFlex > 0 {
		remaining := max(0, available-totalH)
		for _, child := range c.Children {
			if child.Element == nil || child.Flex <= 0 {
				continue
			}
			height := remaining * (child.Flex / totalFlex)
			size := child.Element.Measure(ctx, Constraints{
				MaxW: constraints.MaxW,
				MaxH: height,
			})
			maxW = max(maxW, size.W)
		}
		totalH += remaining
	}
	return constraints.Clamp(Size{W: maxW, H: totalH})
}

func (c Column) Draw(ctx *Context, bounds Rect) {
	y := bounds.Y
	totalFlex := 0.0
	fixedHeight := 0.0
	count := 0
	for _, child := range c.Children {
		if child.Element == nil {
			continue
		}
		if count > 0 {
			fixedHeight += c.Spacing
		}
		count++
		if child.Flex > 0 {
			totalFlex += child.Flex
			continue
		}
		fixedHeight += child.Element.Measure(ctx, Constraints{MaxW: bounds.W, MaxH: bounds.H}).H
	}
	remaining := max(0, bounds.H-fixedHeight)
	index := 0
	for _, child := range c.Children {
		if child.Element == nil {
			continue
		}
		if index > 0 {
			y += c.Spacing
		}
		index++
		height := child.Element.Measure(ctx, Constraints{MaxW: bounds.W, MaxH: bounds.H}).H
		if child.Flex > 0 && totalFlex > 0 {
			height = remaining * (child.Flex / totalFlex)
		}
		childBounds := Rect{X: bounds.X, Y: y, W: bounds.W, H: height}
		child.Element.Draw(ctx, childBounds)
		y += height
	}
}

type Row struct {
	Children []Child
	Spacing  float64
}

func (r Row) Measure(ctx *Context, constraints Constraints) Size {
	available := constraints.maxWidth()
	totalW := 0.0
	maxH := 0.0
	totalFlex := 0.0
	count := 0
	for _, child := range r.Children {
		if child.Element == nil {
			continue
		}
		if count > 0 {
			totalW += r.Spacing
		}
		count++
		if child.Flex > 0 {
			totalFlex += child.Flex
			continue
		}
		size := child.Element.Measure(ctx, Constraints{
			MaxW: max(0, available-totalW),
			MaxH: constraints.MaxH,
		})
		totalW += size.W
		maxH = max(maxH, size.H)
	}
	if totalFlex > 0 {
		remaining := max(0, available-totalW)
		for _, child := range r.Children {
			if child.Element == nil || child.Flex <= 0 {
				continue
			}
			width := remaining * (child.Flex / totalFlex)
			size := child.Element.Measure(ctx, Constraints{
				MaxW: width,
				MaxH: constraints.MaxH,
			})
			maxH = max(maxH, size.H)
		}
		totalW += remaining
	}
	return constraints.Clamp(Size{W: totalW, H: maxH})
}

func (r Row) Draw(ctx *Context, bounds Rect) {
	x := bounds.X
	totalFlex := 0.0
	fixedWidth := 0.0
	count := 0
	for _, child := range r.Children {
		if child.Element == nil {
			continue
		}
		if count > 0 {
			fixedWidth += r.Spacing
		}
		count++
		if child.Flex > 0 {
			totalFlex += child.Flex
			continue
		}
		fixedWidth += child.Element.Measure(ctx, Constraints{MaxW: bounds.W, MaxH: bounds.H}).W
	}
	remaining := max(0, bounds.W-fixedWidth)
	index := 0
	for _, child := range r.Children {
		if child.Element == nil {
			continue
		}
		if index > 0 {
			x += r.Spacing
		}
		index++
		width := child.Element.Measure(ctx, Constraints{MaxW: bounds.W, MaxH: bounds.H}).W
		if child.Flex > 0 && totalFlex > 0 {
			width = remaining * (child.Flex / totalFlex)
		}
		childBounds := Rect{X: x, Y: bounds.Y, W: width, H: bounds.H}
		child.Element.Draw(ctx, childBounds)
		x += width
	}
}

type Inset struct {
	Insets Insets
	Child  Element
}

func (i Inset) Measure(ctx *Context, constraints Constraints) Size {
	if i.Child == nil {
		return constraints.Clamp(Size{})
	}
	childSize := i.Child.Measure(ctx, constraints.Deflate(i.Insets))
	return constraints.Clamp(Size{
		W: childSize.W + i.Insets.Left + i.Insets.Right,
		H: childSize.H + i.Insets.Top + i.Insets.Bottom,
	})
}

func (i Inset) Draw(ctx *Context, bounds Rect) {
	if i.Child == nil {
		return
	}
	i.Child.Draw(ctx, bounds.Inset(i.Insets))
}

type Panel struct {
	Fill   color.Color
	Stroke color.Color
	Insets Insets
	Child  Element
}

func (p Panel) Measure(ctx *Context, constraints Constraints) Size {
	return Inset{Insets: p.Insets, Child: p.Child}.Measure(ctx, constraints)
}

func (p Panel) Draw(ctx *Context, bounds Rect) {
	if p.Fill != nil {
		ctx.FillRect(bounds, p.Fill)
	}
	if p.Stroke != nil {
		ctx.StrokeRect(bounds, 1, p.Stroke)
	}
	if p.Child != nil {
		p.Child.Draw(ctx, bounds.Inset(p.Insets))
	}
}

type Alignment uint8

const (
	AlignStart Alignment = iota
	AlignCenter
	AlignEnd
)

type Constrained struct {
	MinW  float64
	MaxW  float64
	MinH  float64
	MaxH  float64
	Child Element
}

func (c Constrained) Measure(ctx *Context, constraints Constraints) Size {
	if c.Child == nil {
		return constraints.Clamp(Size{})
	}
	childConstraints := constraints
	if c.MinW > 0 {
		childConstraints.MinW = c.MinW
	}
	if c.MaxW > 0 && (childConstraints.MaxW == 0 || c.MaxW < childConstraints.MaxW) {
		childConstraints.MaxW = c.MaxW
	}
	if c.MinH > 0 {
		childConstraints.MinH = c.MinH
	}
	if c.MaxH > 0 && (childConstraints.MaxH == 0 || c.MaxH < childConstraints.MaxH) {
		childConstraints.MaxH = c.MaxH
	}
	return constraints.Clamp(c.Child.Measure(ctx, childConstraints))
}

func (c Constrained) Draw(ctx *Context, bounds Rect) {
	if c.Child == nil {
		return
	}
	childSize := c.Measure(ctx, Constraints{MaxW: bounds.W, MaxH: bounds.H})
	c.Child.Draw(ctx, Rect{X: bounds.X, Y: bounds.Y, W: childSize.W, H: childSize.H})
}

type Align struct {
	Horizontal Alignment
	Vertical   Alignment
	Child      Element
}

func (a Align) Measure(_ *Context, constraints Constraints) Size {
	return constraints.Clamp(Size{W: constraints.MaxW, H: constraints.MaxH})
}

func (a Align) Draw(ctx *Context, bounds Rect) {
	if a.Child == nil {
		return
	}
	childSize := a.Child.Measure(ctx, Constraints{MaxW: bounds.W, MaxH: bounds.H})
	x := bounds.X
	y := bounds.Y
	switch a.Horizontal {
	case AlignCenter:
		x += (bounds.W - childSize.W) / 2
	case AlignEnd:
		x += bounds.W - childSize.W
	}
	switch a.Vertical {
	case AlignCenter:
		y += (bounds.H - childSize.H) / 2
	case AlignEnd:
		y += bounds.H - childSize.H
	}
	a.Child.Draw(ctx, Rect{X: x, Y: y, W: childSize.W, H: childSize.H})
}

type Wrap struct {
	Children    []Element
	Spacing     float64
	LineSpacing float64
}

func (w Wrap) Measure(ctx *Context, constraints Constraints) Size {
	maxWidth := constraints.MaxW
	if maxWidth <= 0 {
		maxWidth = constraints.maxWidth()
	}
	x := 0.0
	lineH := 0.0
	totalH := 0.0
	usedW := 0.0
	count := 0
	for _, child := range w.Children {
		if child == nil {
			continue
		}
		size := child.Measure(ctx, Constraints{MaxW: maxWidth, MaxH: constraints.MaxH})
		if count > 0 && x+size.W > maxWidth {
			totalH += lineH + w.LineSpacing
			usedW = max(usedW, x-w.Spacing)
			x = 0
			lineH = 0
			count = 0
		}
		if count > 0 {
			x += w.Spacing
		}
		x += size.W
		lineH = max(lineH, size.H)
		count++
	}
	if count > 0 {
		totalH += lineH
		usedW = max(usedW, x)
	}
	return constraints.Clamp(Size{W: usedW, H: totalH})
}

func (w Wrap) Draw(ctx *Context, bounds Rect) {
	x := bounds.X
	y := bounds.Y
	lineH := 0.0
	count := 0
	for _, child := range w.Children {
		if child == nil {
			continue
		}
		size := child.Measure(ctx, Constraints{MaxW: bounds.W, MaxH: bounds.H})
		if count > 0 && x+size.W > bounds.X+bounds.W {
			x = bounds.X
			y += lineH + w.LineSpacing
			lineH = 0
			count = 0
		}
		if count > 0 {
			x += w.Spacing
		}
		child.Draw(ctx, Rect{X: x, Y: y, W: size.W, H: size.H})
		x += size.W
		lineH = max(lineH, size.H)
		count++
	}
}
