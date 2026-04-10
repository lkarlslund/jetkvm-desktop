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
	first := true
	for _, child := range c.Children {
		if child.Element == nil {
			continue
		}
		if !first {
			totalH += c.Spacing
		}
		first = false
		if child.Flex > 0 {
			maxW = max(maxW, constraints.MinW)
			continue
		}
		size := child.Element.Measure(ctx, Constraints{
			MaxW: constraints.MaxW,
			MaxH: max(0, available-totalH),
		})
		totalH += size.H
		maxW = max(maxW, size.W)
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
	first := true
	for _, child := range r.Children {
		if child.Element == nil {
			continue
		}
		if !first {
			totalW += r.Spacing
		}
		first = false
		if child.Flex > 0 {
			maxH = max(maxH, constraints.MinH)
			continue
		}
		size := child.Element.Measure(ctx, Constraints{
			MaxW: max(0, available-totalW),
			MaxH: constraints.MaxH,
		})
		totalW += size.W
		maxH = max(maxH, size.H)
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
