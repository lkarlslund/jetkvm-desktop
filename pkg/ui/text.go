package ui

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
	fontOnce   sync.Once
	fontSource *ebitentext.GoTextFaceSource
	fontErr    error
)

func face(size float64) *ebitentext.GoTextFace {
	fontOnce.Do(func() {
		fontSource, fontErr = ebitentext.NewGoTextFaceSource(bytes.NewReader(ibmPlexSansTTF))
	})
	if fontErr != nil || fontSource == nil {
		return nil
	}
	return &ebitentext.GoTextFace{
		Source: fontSource,
		Size:   size,
	}
}

func DrawText(dst *ebiten.Image, value string, x, y, size float64, clr color.Color) {
	fontFace := face(size)
	if fontFace == nil || value == "" {
		return
	}
	op := &ebitentext.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	ebitentext.Draw(dst, value, fontFace, op)
}

func MeasureText(value string, size float64) (float64, float64) {
	fontFace := face(size)
	if fontFace == nil || value == "" {
		return 0, 0
	}
	return ebitentext.Measure(value, fontFace, 0)
}

func DrawWrappedText(dst *ebiten.Image, value string, x, y, width, size float64, clr color.Color) float64 {
	lines := WrapText(value, width, size)
	if len(lines) == 0 {
		return 0
	}
	lineHeight := WrappedLineHeight(size)
	for i, line := range lines {
		DrawText(dst, line, x, y+(float64(i)*lineHeight), size, clr)
	}
	return float64(len(lines)) * lineHeight
}

func WrappedTextHeight(value string, width, size float64) float64 {
	lines := WrapText(value, width, size)
	if len(lines) == 0 {
		return 0
	}
	return float64(len(lines)) * WrappedLineHeight(size)
}

func WrappedLineHeight(size float64) float64 {
	return size + 5
}

func WrapText(value string, width, size float64) []string {
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
				if TextFits(word, width, size) {
					current = word
					continue
				}
				chunks := breakWord(word, width, size)
				lines = append(lines, chunks[:len(chunks)-1]...)
				current = chunks[len(chunks)-1]
				continue
			}
			candidate := current + " " + word
			if TextFits(candidate, width, size) {
				current = candidate
				continue
			}
			lines = append(lines, current)
			if TextFits(word, width, size) {
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

func TextFits(value string, width, size float64) bool {
	w, _ := MeasureText(value, size)
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
		if current == "" || TextFits(candidate, width, size) {
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
