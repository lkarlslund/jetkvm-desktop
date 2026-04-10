package input

import (
	"strings"

	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/hidrpc"
)

const (
	modCtrlLeft  = 0x01
	modShiftLeft = 0x02
	modAltRight  = 0x40
)

type textKey struct {
	hid      byte
	modifier byte
}

var textKeyMap = map[rune]textKey{
	'a': {hid: 4}, 'b': {hid: 5}, 'c': {hid: 6}, 'd': {hid: 7}, 'e': {hid: 8},
	'f': {hid: 9}, 'g': {hid: 10}, 'h': {hid: 11}, 'i': {hid: 12}, 'j': {hid: 13},
	'k': {hid: 14}, 'l': {hid: 15}, 'm': {hid: 16}, 'n': {hid: 17}, 'o': {hid: 18},
	'p': {hid: 19}, 'q': {hid: 20}, 'r': {hid: 21}, 's': {hid: 22}, 't': {hid: 23},
	'u': {hid: 24}, 'v': {hid: 25}, 'w': {hid: 26}, 'x': {hid: 27}, 'y': {hid: 28},
	'z': {hid: 29},
	'A': {hid: 4, modifier: modShiftLeft}, 'B': {hid: 5, modifier: modShiftLeft},
	'C': {hid: 6, modifier: modShiftLeft}, 'D': {hid: 7, modifier: modShiftLeft},
	'E': {hid: 8, modifier: modShiftLeft}, 'F': {hid: 9, modifier: modShiftLeft},
	'G': {hid: 10, modifier: modShiftLeft}, 'H': {hid: 11, modifier: modShiftLeft},
	'I': {hid: 12, modifier: modShiftLeft}, 'J': {hid: 13, modifier: modShiftLeft},
	'K': {hid: 14, modifier: modShiftLeft}, 'L': {hid: 15, modifier: modShiftLeft},
	'M': {hid: 16, modifier: modShiftLeft}, 'N': {hid: 17, modifier: modShiftLeft},
	'O': {hid: 18, modifier: modShiftLeft}, 'P': {hid: 19, modifier: modShiftLeft},
	'Q': {hid: 20, modifier: modShiftLeft}, 'R': {hid: 21, modifier: modShiftLeft},
	'S': {hid: 22, modifier: modShiftLeft}, 'T': {hid: 23, modifier: modShiftLeft},
	'U': {hid: 24, modifier: modShiftLeft}, 'V': {hid: 25, modifier: modShiftLeft},
	'W': {hid: 26, modifier: modShiftLeft}, 'X': {hid: 27, modifier: modShiftLeft},
	'Y': {hid: 28, modifier: modShiftLeft}, 'Z': {hid: 29, modifier: modShiftLeft},
	'1': {hid: 30}, '2': {hid: 31}, '3': {hid: 32}, '4': {hid: 33}, '5': {hid: 34},
	'6': {hid: 35}, '7': {hid: 36}, '8': {hid: 37}, '9': {hid: 38}, '0': {hid: 39},
	'!': {hid: 30, modifier: modShiftLeft}, '@': {hid: 31, modifier: modShiftLeft},
	'#': {hid: 32, modifier: modShiftLeft}, '$': {hid: 33, modifier: modShiftLeft},
	'%': {hid: 34, modifier: modShiftLeft}, '^': {hid: 35, modifier: modShiftLeft},
	'&': {hid: 36, modifier: modShiftLeft}, '*': {hid: 37, modifier: modShiftLeft},
	'(': {hid: 38, modifier: modShiftLeft}, ')': {hid: 39, modifier: modShiftLeft},
	'\n': {hid: 40}, '\r': {hid: 40}, '\t': {hid: 43}, ' ': {hid: 44},
	'-': {hid: 45}, '_': {hid: 45, modifier: modShiftLeft},
	'=': {hid: 46}, '+': {hid: 46, modifier: modShiftLeft},
	'[': {hid: 47}, '{': {hid: 47, modifier: modShiftLeft},
	']': {hid: 48}, '}': {hid: 48, modifier: modShiftLeft},
	'\\': {hid: 49}, '|': {hid: 49, modifier: modShiftLeft},
	';': {hid: 51}, ':': {hid: 51, modifier: modShiftLeft},
	'\'': {hid: 52}, '"': {hid: 52, modifier: modShiftLeft},
	'`': {hid: 53}, '~': {hid: 53, modifier: modShiftLeft},
	',': {hid: 54}, '<': {hid: 54, modifier: modShiftLeft},
	'.': {hid: 55}, '>': {hid: 55, modifier: modShiftLeft},
	'/': {hid: 56}, '?': {hid: 56, modifier: modShiftLeft},
}

func BuildPasteMacro(layout, text string, delay uint16) ([]hidrpc.KeyboardMacroStep, []rune) {
	_ = NormalizeKeyboardLayoutCode(layout)
	steps := make([]hidrpc.KeyboardMacroStep, 0, len(text)*2)
	invalidMap := map[rune]bool{}
	invalid := make([]rune, 0)

	for _, r := range text {
		mapping, ok := textKeyMap[r]
		if !ok {
			if !invalidMap[r] {
				invalidMap[r] = true
				invalid = append(invalid, r)
			}
			continue
		}
		var press hidrpc.KeyboardMacroStep
		press.Modifier = mapping.modifier
		press.Keys[0] = mapping.hid
		press.Delay = 20
		steps = append(steps, press)
		steps = append(steps, hidrpc.KeyboardMacroStep{Delay: delay})
	}

	return steps, invalid
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
