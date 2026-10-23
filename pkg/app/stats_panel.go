package app

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/lkarlslund/jetkvm-desktop/pkg/ui"
)

const (
	statsPanelDefaultW = 320.0
	statsPanelMinW     = 240.0
	statsPanelMargin   = 16.0
	statsPanelFooterH  = 24.0
	statsPanelInset    = 16.0
	statsPanelGrip     = 6.0
	statsGraphNaturalH = 58.0
	statsGraphMinH     = 44.0
)

// statsPanelEdges is a bitmask of the panel edges a resize drag moves.
type statsPanelEdges uint8

const (
	statsEdgeLeft statsPanelEdges = 1 << iota
	statsEdgeRight
	statsEdgeTop
	statsEdgeBottom
)

type statsPanelDrag struct {
	edges      statsPanelEdges
	start      rect
	startPoint ui.Point
}

// statsPanelDefaultRect places the panel in the lower-right corner, above the status footer.
func statsPanelDefaultRect(screenW, screenH, w, h float64) rect {
	return rect{
		x: screenW - w - statsPanelMargin,
		y: screenH - h - statsPanelMargin - statsPanelFooterH,
		w: w,
		h: h,
	}
}

// clampStatsPanel keeps the panel at least its minimum size and fully inside the screen.
func clampStatsPanel(r rect, screenW, screenH, minW, minH float64) rect {
	r.w = min(max(r.w, minW), screenW)
	r.h = min(max(r.h, minH), screenH)
	r.x = min(max(r.x, 0), screenW-r.w)
	r.y = min(max(r.y, 0), screenH-r.h)
	return r
}

// dragStatsPanel moves the panel (no edges) or resizes the given edges by the pointer delta.
func dragStatsPanel(start rect, edges statsPanelEdges, dx, dy, minW, minH float64) rect {
	if edges == 0 {
		start.x += dx
		start.y += dy
		return start
	}
	r := start
	if edges&statsEdgeLeft != 0 {
		r.w = max(start.w-dx, minW)
		r.x = start.x + start.w - r.w
	}
	if edges&statsEdgeRight != 0 {
		r.w = max(start.w+dx, minW)
	}
	if edges&statsEdgeTop != 0 {
		r.h = max(start.h-dy, minH)
		r.y = start.y + start.h - r.h
	}
	if edges&statsEdgeBottom != 0 {
		r.h = max(start.h+dy, minH)
	}
	return r
}

// statsPanelHitEdges reports which resize edges a point is on, using a grip band inside the panel border.
func statsPanelHitEdges(r rect, x, y float64) statsPanelEdges {
	var edges statsPanelEdges
	if x < r.x+statsPanelGrip {
		edges |= statsEdgeLeft
	} else if x >= r.x+r.w-statsPanelGrip {
		edges |= statsEdgeRight
	}
	if y < r.y+statsPanelGrip {
		edges |= statsEdgeTop
	} else if y >= r.y+r.h-statsPanelGrip {
		edges |= statsEdgeBottom
	}
	return edges
}

func statsPanelCursor(edges statsPanelEdges) ebiten.CursorShapeType {
	switch edges {
	case statsEdgeLeft, statsEdgeRight:
		return ebiten.CursorShapeEWResize
	case statsEdgeTop, statsEdgeBottom:
		return ebiten.CursorShapeNSResize
	case statsEdgeLeft | statsEdgeTop, statsEdgeRight | statsEdgeBottom:
		return ebiten.CursorShapeNWSEResize
	case statsEdgeRight | statsEdgeTop, statsEdgeLeft | statsEdgeBottom:
		return ebiten.CursorShapeNESWResize
	default:
		return ebiten.CursorShapeMove
	}
}

// syncStatsPanelCursor shows move/resize cursors while hovering or dragging the stats panel.
func (a *App) syncStatsPanelCursor() {
	shape := ebiten.CursorShapeDefault
	if a.statsOpen && !a.launcherOpen && !a.relative && a.statsPanelRect.valid() {
		x, y := ebiten.CursorPosition()
		switch {
		case a.statsDragging:
			shape = statsPanelCursor(a.statsDrag.edges)
		case a.statsPanelRect.contains(x, y):
			shape = statsPanelCursor(statsPanelHitEdges(a.statsPanelRect, float64(x), float64(y)))
		}
	}
	if shape != a.cursorShape {
		ebiten.SetCursorShape(shape)
		a.cursorShape = shape
	}
}

// registerStatsPanelControls makes the whole panel a drag handle; presses near the border resize instead.
func (a *App) registerStatsPanelControls(runtime *ui.Runtime, panel rect, minW, minH float64) {
	if runtime == nil {
		return
	}
	runtime.Register(ui.Control{
		ID:      "stats_panel",
		Rect:    ui.Rect{X: panel.x, Y: panel.y, W: panel.w, H: panel.h},
		Enabled: true,
		OnPress: func(event ui.PointerEvent) {
			a.statsDragging = true
			a.statsDrag = statsPanelDrag{
				edges:      statsPanelHitEdges(panel, event.Point.X, event.Point.Y),
				start:      panel,
				startPoint: event.Point,
			}
		},
		OnDrag: func(event ui.PointerEvent) {
			if !a.statsDragging {
				return
			}
			a.statsPanel = dragStatsPanel(
				a.statsDrag.start,
				a.statsDrag.edges,
				event.Point.X-a.statsDrag.startPoint.X,
				event.Point.Y-a.statsDrag.startPoint.Y,
				minW, minH,
			)
		},
		OnRelease: func(ui.PointerEvent) {
			a.statsDragging = false
		},
	})
}
