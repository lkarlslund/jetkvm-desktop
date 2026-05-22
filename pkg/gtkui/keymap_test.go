package gtkui

import (
	"testing"

	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
)

func TestGDKKeyToInputKey(t *testing.T) {
	tests := []struct {
		keyval uint
		want   input.Key
	}{
		{0x061, input.KeyA},   // 'a'
		{0x07a, input.KeyZ},   // 'z'
		{0x041, input.KeyA},   // 'A'
		{0x05a, input.KeyZ},   // 'Z'
		{0x031, input.Key1},   // '1'
		{0x030, input.Key0},   // '0'
		{0xff0d, input.KeyEnter},
		{0xff1b, input.KeyEscape},
		{0xff08, input.KeyBackspace},
		{0xff09, input.KeyTab},
		{0x020, input.KeySpace},
		{0xffbe, input.KeyF1},
		{0xffc9, input.KeyF12},
		{0xff53, input.KeyRight},
		{0xff51, input.KeyLeft},
		{0xff54, input.KeyDown},
		{0xff52, input.KeyUp},
		{0xffe3, input.KeyControlLeft},
		{0xffe1, input.KeyShiftLeft},
		{0xffe9, input.KeyAltLeft},
		{0xffeb, input.KeyMetaLeft},
	}
	for _, tt := range tests {
		got, ok := gdkKeyToInputKey(tt.keyval)
		if !ok {
			t.Errorf("gdkKeyToInputKey(0x%x) returned ok=false, want %v", tt.keyval, tt.want)
			continue
		}
		if got != tt.want {
			t.Errorf("gdkKeyToInputKey(0x%x) = %v, want %v", tt.keyval, got, tt.want)
		}
	}
}

func TestGDKKeyToInputKey_Unknown(t *testing.T) {
	_, ok := gdkKeyToInputKey(0xdead)
	if ok {
		t.Error("expected ok=false for unknown keyval 0xdead")
	}
}
