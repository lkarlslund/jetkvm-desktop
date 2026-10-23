package app

import "testing"

func TestStatsPanelDefaultRectSitsInLowerRightAboveFooter(t *testing.T) {
	got := statsPanelDefaultRect(1280, 800, 320, 400)
	want := rect{x: 1280 - 320 - statsPanelMargin, y: 800 - 400 - statsPanelMargin - statsPanelFooterH, w: 320, h: 400}
	if got != want {
		t.Fatalf("default rect = %+v, want %+v", got, want)
	}
	if got.w >= 1280/2 {
		t.Fatalf("default width %.0f should not span the screen", got.w)
	}
}

func TestClampStatsPanel(t *testing.T) {
	tests := []struct {
		name string
		in   rect
		want rect
	}{
		{"inside stays put", rect{x: 100, y: 100, w: 300, h: 300}, rect{x: 100, y: 100, w: 300, h: 300}},
		{"off the right and bottom", rect{x: 900, y: 700, w: 300, h: 300}, rect{x: 700, y: 500, w: 300, h: 300}},
		{"off the left and top", rect{x: -50, y: -20, w: 300, h: 300}, rect{x: 0, y: 0, w: 300, h: 300}},
		{"below minimum size", rect{x: 10, y: 10, w: 50, h: 50}, rect{x: 10, y: 10, w: 240, h: 200}},
		{"larger than screen", rect{x: 10, y: 10, w: 2000, h: 2000}, rect{x: 0, y: 0, w: 1000, h: 800}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampStatsPanel(tt.in, 1000, 800, 240, 200); got != tt.want {
				t.Fatalf("clamp = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDragStatsPanel(t *testing.T) {
	start := rect{x: 500, y: 400, w: 300, h: 300}
	tests := []struct {
		name   string
		edges  statsPanelEdges
		dx, dy float64
		want   rect
	}{
		{"move", 0, -40, 25, rect{x: 460, y: 425, w: 300, h: 300}},
		{"grow from right", statsEdgeRight, 50, 0, rect{x: 500, y: 400, w: 350, h: 300}},
		{"grow from top-left keeps bottom-right fixed", statsEdgeLeft | statsEdgeTop, -60, -40, rect{x: 440, y: 360, w: 360, h: 340}},
		{"shrink from left stops at minimum", statsEdgeLeft, 200, 0, rect{x: 560, y: 400, w: 240, h: 300}},
		{"shrink from bottom stops at minimum", statsEdgeBottom, 0, -200, rect{x: 500, y: 400, w: 300, h: 200}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dragStatsPanel(start, tt.edges, tt.dx, tt.dy, 240, 200); got != tt.want {
				t.Fatalf("drag = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStatsPanelHitEdges(t *testing.T) {
	r := rect{x: 100, y: 100, w: 300, h: 200}
	tests := []struct {
		name string
		x, y float64
		want statsPanelEdges
	}{
		{"center moves", 250, 200, 0},
		{"left border", 101, 200, statsEdgeLeft},
		{"right border", 398, 200, statsEdgeRight},
		{"top border", 250, 102, statsEdgeTop},
		{"bottom border", 250, 298, statsEdgeBottom},
		{"top-left corner", 101, 101, statsEdgeLeft | statsEdgeTop},
		{"bottom-right corner", 399, 299, statsEdgeRight | statsEdgeBottom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statsPanelHitEdges(r, tt.x, tt.y); got != tt.want {
				t.Fatalf("edges = %b, want %b", got, tt.want)
			}
		})
	}
}
