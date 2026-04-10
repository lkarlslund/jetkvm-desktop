package ui

import "image/color"

type MetricGraph struct {
	Title  string
	Value  string
	Series []float64
}

func (m MetricGraph) Measure(_ *Context, constraints Constraints) Size {
	return constraints.Clamp(Size{W: constraints.MaxW, H: 58})
}

func (m MetricGraph) Draw(ctx *Context, bounds Rect) {
	ctx.FillRect(bounds, color.RGBA{R: 15, G: 23, B: 34, A: 220})
	ctx.StrokeRect(bounds, 1, color.RGBA{R: 62, G: 80, B: 96, A: 180})
	DrawText(ctx.Screen, m.Title, bounds.X+10, bounds.Y+10, 12, color.RGBA{R: 240, G: 244, B: 248, A: 255})
	DrawText(ctx.Screen, m.Value, bounds.Right()-88, bounds.Y+10, 12, color.RGBA{R: 166, G: 200, B: 255, A: 255})

	chartX := bounds.X + 10
	chartY := bounds.Y + 24
	chartW := bounds.W - 20
	chartH := bounds.H - 32
	ctx.StrokeRect(Rect{X: chartX, Y: chartY, W: chartW, H: chartH}, 1, color.RGBA{R: 46, G: 60, B: 75, A: 120})

	minY, maxY := graphDomain(m.Series)
	for i := 1; i < 4; i++ {
		yy := chartY + chartH*(float64(i)/4)
		ctx.StrokeLine(Point{X: chartX, Y: yy}, Point{X: chartX + chartW, Y: yy}, 1, color.RGBA{R: 34, G: 46, B: 58, A: 120})
	}
	if len(m.Series) < 2 {
		return
	}
	prevX := chartX
	prevY := chartY + chartH
	for i, value := range m.Series {
		norm := 0.0
		if maxY > minY {
			norm = (value - minY) / (maxY - minY)
		}
		norm = clamp(norm, 0, 1)
		px := chartX + (float64(i)/float64(len(m.Series)-1))*chartW
		py := chartY + chartH - norm*chartH
		if i > 0 {
			ctx.StrokeLine(Point{X: prevX, Y: prevY}, Point{X: px, Y: py}, 2, color.RGBA{R: 108, G: 184, B: 255, A: 255})
		}
		prevX = px
		prevY = py
	}
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
	magnitude := 1.0
	for value/magnitude >= 10 {
		magnitude *= 10
	}
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
