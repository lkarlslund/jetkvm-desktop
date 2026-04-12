package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"golang.design/x/clipboard"
)

const (
	textInputRepeatDelay    = 24
	textInputRepeatInterval = 3
	textFieldHorizontalPad  = 12.0
)

type TextInputState struct {
	FieldID  string
	Anchor   int
	Caret    int
	Dragging bool
}

type TextInputBinding struct {
	ID           string
	Value        string
	DisplayValue string
	TextSize     float64
}

type TextInputResult struct {
	Value   string
	Changed bool
	Handled bool
}

var textClipboardReady = clipboard.Init() == nil

func (s *TextInputState) ClearFocus() {
	s.Dragging = false
	s.FieldID = ""
}

func (s *TextInputState) Sync(binding *TextInputBinding) {
	if binding == nil {
		s.ClearFocus()
		return
	}
	if s.FieldID != binding.ID {
		caret := utf8.RuneCountInString(binding.Value)
		s.FieldID = binding.ID
		s.Anchor = caret
		s.Caret = caret
		s.Dragging = false
		return
	}
	caret := utf8.RuneCountInString(binding.Value)
	if s.Anchor > caret {
		s.Anchor = caret
	}
	if s.Caret > caret {
		s.Caret = caret
	}
}

func (s *TextInputState) Bind(field TextField, binding *TextInputBinding) TextField {
	if binding != nil && binding.ID == field.ID {
		field.Value = binding.Value
		field.DisplayValue = binding.DisplayValue
	}
	if binding == nil || binding.ID != field.ID || s.FieldID != field.ID {
		field.Focused = false
		field.CaretIndex = 0
		field.SelectionStart = 0
		field.SelectionEnd = 0
		return field
	}
	field.Focused = true
	field.CaretIndex = s.Caret
	field.SelectionStart = s.SelectionStart()
	field.SelectionEnd = s.SelectionEnd()
	return field
}

func (s *TextInputState) SelectionStart() int {
	if s.Anchor < s.Caret {
		return s.Anchor
	}
	return s.Caret
}

func (s *TextInputState) SelectionEnd() int {
	if s.Anchor > s.Caret {
		return s.Anchor
	}
	return s.Caret
}

func (s *TextInputState) HasSelection() bool {
	return s.Anchor != s.Caret
}

func (s *TextInputState) BeginPointer(binding TextInputBinding, field Rect, cursorX float64, shift bool) {
	s.Sync(&binding)
	caret := s.caretIndexForX(binding, field, cursorX)
	if shift {
		s.Caret = caret
	} else {
		s.Anchor = caret
		s.Caret = caret
	}
	s.Dragging = true
}

func (s *TextInputState) UpdateDrag(binding TextInputBinding, field Rect, cursorX float64, mouseDown bool) {
	if !s.Dragging {
		return
	}
	if !mouseDown {
		s.Dragging = false
		return
	}
	s.Caret = s.caretIndexForX(binding, field, cursorX)
}

func (s *TextInputState) HandleInput(binding TextInputBinding) TextInputResult {
	s.Sync(&binding)
	if s.FieldID == "" {
		return TextInputResult{Value: binding.Value}
	}
	if shortcutPressed() {
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeyA):
			runes := []rune(binding.Value)
			s.Anchor = 0
			s.Caret = len(runes)
			return TextInputResult{Value: binding.Value, Handled: true}
		case inpututil.IsKeyJustPressed(ebiten.KeyC):
			if !s.HasSelection() {
				return TextInputResult{Value: binding.Value}
			}
			_ = writeClipboardText(s.selectedText(binding.Value))
			return TextInputResult{Value: binding.Value, Handled: true}
		case inpututil.IsKeyJustPressed(ebiten.KeyV):
			text, err := readClipboardText()
			if err != nil {
				return TextInputResult{Value: binding.Value}
			}
			next := s.replaceSelection(binding.Value, sanitizeSingleLineText(text))
			return TextInputResult{Value: next, Changed: next != binding.Value, Handled: true}
		}
	}

	shift := shiftPressed()
	runes := []rune(binding.Value)
	switch {
	case repeatingKeyPressed(ebiten.KeyLeft):
		if s.HasSelection() && !shift {
			s.Caret = s.SelectionStart()
			s.Anchor = s.Caret
			return TextInputResult{Value: binding.Value, Handled: true}
		}
		if s.Caret > 0 {
			s.Caret--
		}
		if !shift {
			s.Anchor = s.Caret
		}
		return TextInputResult{Value: binding.Value, Handled: true}
	case repeatingKeyPressed(ebiten.KeyRight):
		if s.HasSelection() && !shift {
			s.Caret = s.SelectionEnd()
			s.Anchor = s.Caret
			return TextInputResult{Value: binding.Value, Handled: true}
		}
		if s.Caret < len(runes) {
			s.Caret++
		}
		if !shift {
			s.Anchor = s.Caret
		}
		return TextInputResult{Value: binding.Value, Handled: true}
	case repeatingKeyPressed(ebiten.KeyHome):
		s.Caret = 0
		if !shift {
			s.Anchor = s.Caret
		}
		return TextInputResult{Value: binding.Value, Handled: true}
	case repeatingKeyPressed(ebiten.KeyEnd):
		s.Caret = len(runes)
		if !shift {
			s.Anchor = s.Caret
		}
		return TextInputResult{Value: binding.Value, Handled: true}
	case repeatingKeyPressed(ebiten.KeyBackspace):
		if s.HasSelection() {
			next := s.deleteSelection(binding.Value)
			return TextInputResult{Value: next, Changed: next != binding.Value, Handled: true}
		}
		if s.Caret == 0 || len(runes) == 0 {
			return TextInputResult{Value: binding.Value}
		}
		caret := s.Caret
		next := string(append(runes[:caret-1], runes[caret:]...))
		s.Caret--
		s.Anchor = s.Caret
		return TextInputResult{Value: next, Changed: true, Handled: true}
	case repeatingKeyPressed(ebiten.KeyDelete):
		if s.HasSelection() {
			next := s.deleteSelection(binding.Value)
			return TextInputResult{Value: next, Changed: next != binding.Value, Handled: true}
		}
		if s.Caret >= len(runes) {
			return TextInputResult{Value: binding.Value}
		}
		caret := s.Caret
		next := string(append(runes[:caret], runes[caret+1:]...))
		return TextInputResult{Value: next, Changed: true, Handled: true}
	}

	inserted := false
	value := binding.Value
	for _, r := range ebiten.AppendInputChars(nil) {
		if r < 32 || r == 127 {
			continue
		}
		value = s.replaceSelection(value, string(r))
		inserted = true
	}
	return TextInputResult{Value: value, Changed: inserted, Handled: inserted}
}

func (s *TextInputState) caretIndexForX(binding TextInputBinding, field Rect, cursorX float64) int {
	display := binding.displayText()
	runes := []rune(display)
	if len(runes) == 0 {
		return 0
	}
	textX := field.X + textFieldHorizontalPad
	if cursorX <= textX {
		return 0
	}
	advances := PrefixAdvances(display, binding.effectiveTextSize())
	for i := 1; i <= len(runes); i++ {
		lastWidth := advances[i-1]
		width := advances[i]
		threshold := textX + lastWidth + (width-lastWidth)/2
		if cursorX < threshold {
			return i - 1
		}
	}
	return len(runes)
}

func (s *TextInputState) replaceSelection(value, insert string) string {
	insertRunes := []rune(insert)
	if !s.HasSelection() {
		runes := []rune(value)
		caret := s.Caret
		next := append(append([]rune{}, runes[:caret]...), append(insertRunes, runes[caret:]...)...)
		s.Caret = caret + len(insertRunes)
		s.Anchor = s.Caret
		return string(next)
	}
	start := s.SelectionStart()
	end := s.SelectionEnd()
	runes := []rune(value)
	next := append(append([]rune{}, runes[:start]...), append(insertRunes, runes[end:]...)...)
	s.Caret = start + len(insertRunes)
	s.Anchor = s.Caret
	return string(next)
}

func (s *TextInputState) deleteSelection(value string) string {
	start := s.SelectionStart()
	end := s.SelectionEnd()
	runes := []rune(value)
	next := string(append(append([]rune{}, runes[:start]...), runes[end:]...))
	s.Caret = start
	s.Anchor = s.Caret
	return next
}

func (s *TextInputState) selectedText(value string) string {
	start := s.SelectionStart()
	end := s.SelectionEnd()
	runes := []rune(value)
	if start < 0 || end > len(runes) || start >= end {
		return ""
	}
	return string(runes[start:end])
}

func (b TextInputBinding) effectiveTextSize() float64 {
	if b.TextSize <= 0 {
		return 13
	}
	return b.TextSize
}

func (b TextInputBinding) displayText() string {
	if b.DisplayValue != "" {
		return b.DisplayValue
	}
	return b.Value
}

func repeatingKeyPressed(key ebiten.Key) bool {
	duration := inpututil.KeyPressDuration(key)
	if duration == 1 {
		return true
	}
	if duration < textInputRepeatDelay {
		return false
	}
	return (duration-textInputRepeatDelay)%textInputRepeatInterval == 0
}

func shortcutPressed() bool {
	return ebiten.IsKeyPressed(ebiten.KeyControlLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyControlRight) ||
		ebiten.IsKeyPressed(ebiten.KeyMetaLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyMetaRight)
}

func shiftPressed() bool {
	return ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
}

func sanitizeSingleLineText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return value
}

func readClipboardText() (string, error) {
	if !textClipboardReady {
		return "", fmt.Errorf("system clipboard is not available on this platform/session")
	}
	data := clipboard.Read(clipboard.FmtText)
	if len(data) == 0 {
		return "", nil
	}
	return string(data), nil
}

func writeClipboardText(value string) error {
	if !textClipboardReady {
		return fmt.Errorf("system clipboard is not available on this platform/session")
	}
	clipboard.Write(clipboard.FmtText, []byte(value))
	return nil
}
