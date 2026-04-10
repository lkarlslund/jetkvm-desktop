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
