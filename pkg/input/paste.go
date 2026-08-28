package input

import (
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/hidrpc"
)

const (
	modCtrlLeft  = 0x01
	modShiftLeft = 0x02
	modAltRight  = 0x40
)

// BuildPasteMacro converts the given text into a sequence of keyboard macro
// steps for the given keyboard layout. Characters that are not supported by
// the layout are skipped and reported in `invalid`.
//
// Each character produces a press step (held for 20ms) followed by a release
// step (held for `delay` ms). Accented characters automatically emit the dead
// key sequence first. Stand-alone dead keys (e.g. ~ on a Portuguese layout)
// are followed by a space to commit the character.
func BuildPasteMacro(layout, text string, delay uint16) ([]hidrpc.KeyboardMacroStep, []rune) {
	// macOS (and some other sources) provide text in NFD form, where 'á' is
	// stored as 'a' + combining acute. Our layout tables use NFC, so normalize
	// the input before lookup so we don't drop accented characters as invalid.
	text = norm.NFC.String(text)
	chars := lookupPasteLayout(layout)
	steps := make([]hidrpc.KeyboardMacroStep, 0, len(text)*4)
	invalidMap := map[rune]bool{}
	invalid := make([]rune, 0)

	for _, r := range text {
		entry, ok := chars[r]
		if !ok {
			if !invalidMap[r] {
				invalidMap[r] = true
				invalid = append(invalid, r)
			}
			continue
		}

		if entry.Accent != nil {
			steps = appendPress(steps, entry.Accent.HID, entry.Accent.Modifier, delay)
		}

		steps = appendPress(steps, entry.HID, entry.Modifier, delay)

		// Stand-alone dead keys need a following space to actually emit the
		// accent glyph rather than waiting for the next letter to combine.
		if entry.DeadKey && entry.Accent == nil {
			steps = appendPress(steps, hidSpace, 0, delay)
		}
	}

	return steps, invalid
}

func appendPress(steps []hidrpc.KeyboardMacroStep, key, modifier byte, delay uint16) []hidrpc.KeyboardMacroStep {
	var press hidrpc.KeyboardMacroStep
	press.Modifier = modifier
	press.Keys[0] = key
	press.Delay = 20
	steps = append(steps, press)
	steps = append(steps, hidrpc.KeyboardMacroStep{Delay: delay})
	return steps
}

func InvalidRunesString(invalid []rune) string {
	if len(invalid) == 0 {
		return ""
	}
	parts := make([]string, 0, len(invalid))
	for _, r := range invalid {
		parts = append(parts, string(r))
	}
	return strings.Join(parts, ", ")
}
