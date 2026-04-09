package app

import (
	"bytes"
	_ "embed"
	"image/color"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	ebitentext "github.com/hajimehoshi/ebiten/v2/text/v2"
)

//go:embed assets/fonts/IBMPlexSans.ttf
var ibmPlexSansTTF []byte

var (
	uiFontOnce   sync.Once
	uiFontSource *ebitentext.GoTextFaceSource
	uiFontErr    error
)

func uiFace(size float64) *ebitentext.GoTextFace {
	uiFontOnce.Do(func() {
		uiFontSource, uiFontErr = ebitentext.NewGoTextFaceSource(bytes.NewReader(ibmPlexSansTTF))
	})
	if uiFontErr != nil || uiFontSource == nil {
		return nil
	}
	return &ebitentext.GoTextFace{
		Source: uiFontSource,
		Size:   size,
	}
}

func drawText(dst *ebiten.Image, value string, x, y, size float64, clr color.Color) {
	face := uiFace(size)
	if face == nil || value == "" {
		return
	}
	op := &ebitentext.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	ebitentext.Draw(dst, value, face, op)
}

func measureText(value string, size float64) (float64, float64) {
	face := uiFace(size)
	if face == nil || value == "" {
		return 0, 0
	}
	return ebitentext.Measure(value, face, 0)
}
