package app

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/lkarlslund/jetkvm-desktop/pkg/ui"
)

func (a *App) newUIContext(screen *ebiten.Image, register func(chromeButton)) *ui.Context {
	return &ui.Context{
		Screen:          screen,
		Theme:           ui.DefaultTheme(),
		MeasureText:     ui.MeasureText,
		MeasureWrapped:  ui.WrappedTextHeight,
		DrawText:        ui.DrawText,
		DrawWrappedText: ui.DrawWrappedText,
		RegisterHitTarget: func(hit ui.HitTarget) {
			register(chromeButton{
				id:      hit.ID,
				enabled: hit.Enabled,
				rect: rect{
					x: hit.Rect.X,
					y: hit.Rect.Y,
					w: hit.Rect.W,
					h: hit.Rect.H,
				},
			})
		},
	}
}

func (a *App) drawUIRoot(screen *ebiten.Image, register func(chromeButton), root ui.Element) {
	if root == nil {
		return
	}
	ctx := a.newUIContext(screen, register)
	bounds := screen.Bounds()
	root.Draw(ctx, ui.Rect{W: float64(bounds.Dx()), H: float64(bounds.Dy())})
}
