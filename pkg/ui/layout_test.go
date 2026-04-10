package ui

import "testing"

type stubElement struct {
	size       Size
	lastBounds Rect
}

func (s *stubElement) Measure(_ *Context, constraints Constraints) Size {
	return constraints.Clamp(s.size)
}

func (s *stubElement) Draw(_ *Context, bounds Rect) {
	s.lastBounds = bounds
}

func TestRectInsetClampsToZero(t *testing.T) {
	rect := Rect{X: 10, Y: 20, W: 15, H: 12}
	got := rect.Inset(Insets{Top: 4, Right: 20, Bottom: 10, Left: 6})
	if got.W != 0 {
		t.Fatalf("width = %v, want 0", got.W)
	}
	if got.H != 0 {
		t.Fatalf("height = %v, want 0", got.H)
	}
	if got.X != 16 || got.Y != 24 {
		t.Fatalf("origin = (%v,%v), want (16,24)", got.X, got.Y)
	}
}

func TestColumnDrawDistributesFlexRemainder(t *testing.T) {
	first := &stubElement{size: Size{W: 80, H: 20}}
	second := &stubElement{size: Size{W: 80, H: 10}}

	column := Column{
		Children: []Child{
			Fixed(first),
			Flex(second, 1),
		},
		Spacing: 6,
	}

	column.Draw(&Context{}, Rect{X: 5, Y: 7, W: 120, H: 70})

	if first.lastBounds != (Rect{X: 5, Y: 7, W: 120, H: 20}) {
		t.Fatalf("first bounds = %+v", first.lastBounds)
	}
	if second.lastBounds != (Rect{X: 5, Y: 33, W: 120, H: 44}) {
		t.Fatalf("second bounds = %+v", second.lastBounds)
	}
}

func TestRowDrawDistributesFlexRemainder(t *testing.T) {
	first := &stubElement{size: Size{W: 30, H: 18}}
	second := &stubElement{size: Size{W: 10, H: 18}}

	row := Row{
		Children: []Child{
			Fixed(first),
			Flex(second, 1),
		},
		Spacing: 8,
	}

	row.Draw(&Context{}, Rect{X: 2, Y: 4, W: 100, H: 24})

	if first.lastBounds != (Rect{X: 2, Y: 4, W: 30, H: 24}) {
		t.Fatalf("first bounds = %+v", first.lastBounds)
	}
	if second.lastBounds != (Rect{X: 40, Y: 4, W: 62, H: 24}) {
		t.Fatalf("second bounds = %+v", second.lastBounds)
	}
}
