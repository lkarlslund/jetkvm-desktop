package app

import (
	"bytes"
	_ "embed"
	"image/color"
	"strings"
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

func drawWrappedText(dst *ebiten.Image, value string, x, y, width, size float64, clr color.Color) float64 {
	lines := wrapText(value, width, size)
	if len(lines) == 0 {
		return 0
	}
	lineHeight := size + 5
	for i, line := range lines {
		drawText(dst, line, x, y+(float64(i)*lineHeight), size, clr)
	}
	return float64(len(lines)) * lineHeight
}

func wrapText(value string, width, size float64) []string {
	if value == "" {
		return nil
	}
	if width <= 0 {
		return []string{value}
	}
	paragraphs := strings.Split(value, "\n")
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := ""
		for _, word := range words {
			if current == "" {
				if textFits(word, width, size) {
					current = word
					continue
				}
				chunks := breakWord(word, width, size)
				lines = append(lines, chunks[:len(chunks)-1]...)
				current = chunks[len(chunks)-1]
				continue
			}
			candidate := current + " " + word
			if textFits(candidate, width, size) {
				current = candidate
				continue
			}
			lines = append(lines, current)
			if textFits(word, width, size) {
				current = word
				continue
			}
			chunks := breakWord(word, width, size)
			lines = append(lines, chunks[:len(chunks)-1]...)
			current = chunks[len(chunks)-1]
		}
		if current != "" {
			lines = append(lines, current)
		}
	}
	return lines
}

func textFits(value string, width, size float64) bool {
	w, _ := measureText(value, size)
	return w <= width
}

func breakWord(word string, width, size float64) []string {
	if word == "" {
		return nil
	}
	runes := []rune(word)
	chunks := make([]string, 0, len(runes))
	current := ""
	for _, r := range runes {
		candidate := current + string(r)
		if current == "" || textFits(candidate, width, size) {
			current = candidate
			continue
		}
		chunks = append(chunks, current)
		current = string(r)
	}
	if current != "" {
		chunks = append(chunks, current)
	}
	return chunks
}
