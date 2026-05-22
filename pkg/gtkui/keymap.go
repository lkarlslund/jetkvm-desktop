package gtkui

import "github.com/lkarlslund/jetkvm-desktop/pkg/input"

// GDK key values (from gdk/gdkkeysyms.h). Only the subset we need.
const (
	gdkA            = 0x061
	gdkZ            = 0x07a
	gdk0            = 0x030
	gdk9            = 0x039
	gdkReturn       = 0xff0d
	gdkEscape       = 0xff1b
	gdkBackSpace    = 0xff08
	gdkTab          = 0xff09
	gdkSpace        = 0x020
	gdkMinus        = 0x02d
	gdkEqual        = 0x03d
	gdkBracketLeft  = 0x05b
	gdkBracketRight = 0x05d
	gdkBackslash    = 0x05c
	gdkSemicolon    = 0x03b
	gdkApostrophe   = 0x027
	gdkGraveAccent  = 0x060
	gdkComma        = 0x02c
	gdkPeriod       = 0x02e
	gdkSlash        = 0x02f
	gdkCapsLock     = 0xffe5
	gdkF1           = 0xffbe
	gdkF12          = 0xffc9
	gdkF13          = 0xffca
	gdkF24          = 0xffd5
	gdkPrint        = 0xff61
	gdkScrollLock   = 0xff14
	gdkPause        = 0xff13
	gdkMenu         = 0xff67
	gdkInsert       = 0xff63
	gdkHome         = 0xff50
	gdkPageUp       = 0xff55
	gdkDelete       = 0xffff
	gdkEnd          = 0xff57
	gdkPageDown     = 0xff56
	gdkRight        = 0xff53
	gdkLeft         = 0xff51
	gdkDown         = 0xff54
	gdkUp           = 0xff52
	gdkNumLock      = 0xff7f
	gdkKPDivide     = 0xffaf
	gdkKPMultiply   = 0xffaa
	gdkKPSubtract   = 0xffad
	gdkKPAdd        = 0xffab
	gdkKPEnter      = 0xff8d
	gdkKP1          = 0xffb1
	gdkKP0          = 0xffb0
	gdkKPDecimal    = 0xffae
	gdkKPEqual      = 0xffbd
	gdkControlL     = 0xffe3
	gdkShiftL       = 0xffe1
	gdkAltL         = 0xffe9
	gdkSuperL       = 0xffeb
	gdkControlR     = 0xffe4
	gdkShiftR       = 0xffe2
	gdkAltR         = 0xffea
	gdkSuperR       = 0xffec
)

func gdkKeyToInputKey(keyval uint) (input.Key, bool) {
	// Lowercase letters a-z
	if keyval >= gdkA && keyval <= gdkZ {
		return input.KeyA + input.Key(keyval-gdkA), true
	}
	// Uppercase A-Z map to same keys
	if keyval >= 0x041 && keyval <= 0x05a {
		return input.KeyA + input.Key(keyval-0x041), true
	}
	// Digits 0-9
	if keyval >= gdk0 && keyval <= gdk9 {
		// input.Key order: Key1..Key9, Key0
		if keyval == gdk0 {
			return input.Key0, true
		}
		return input.Key1 + input.Key(keyval-gdk0-1), true
	}
	// F1-F12
	if keyval >= gdkF1 && keyval <= gdkF12 {
		return input.KeyF1 + input.Key(keyval-gdkF1), true
	}
	// F13-F24
	if keyval >= gdkF13 && keyval <= gdkF24 {
		return input.KeyF13 + input.Key(keyval-gdkF13), true
	}
	// Numpad 0-9
	if keyval >= gdkKP0 && keyval <= gdkKP0+9 {
		// input.Key order: KeyNumpad1..KeyNumpad9, KeyNumpad0
		if keyval == gdkKP0 {
			return input.KeyNumpad0, true
		}
		return input.KeyNumpad1 + input.Key(keyval-gdkKP1), true
	}

	switch keyval {
	case gdkReturn:
		return input.KeyEnter, true
	case gdkEscape:
		return input.KeyEscape, true
	case gdkBackSpace:
		return input.KeyBackspace, true
	case gdkTab:
		return input.KeyTab, true
	case gdkSpace:
		return input.KeySpace, true
	case gdkMinus:
		return input.KeyMinus, true
	case gdkEqual:
		return input.KeyEqual, true
	case gdkBracketLeft:
		return input.KeyLeftBracket, true
	case gdkBracketRight:
		return input.KeyRightBracket, true
	case gdkBackslash:
		return input.KeyBackslash, true
	case gdkSemicolon:
		return input.KeySemicolon, true
	case gdkApostrophe:
		return input.KeyApostrophe, true
	case gdkGraveAccent:
		return input.KeyGraveAccent, true
	case gdkComma:
		return input.KeyComma, true
	case gdkPeriod:
		return input.KeyPeriod, true
	case gdkSlash:
		return input.KeySlash, true
	case gdkCapsLock:
		return input.KeyCapsLock, true
	case gdkPrint:
		return input.KeyPrintScreen, true
	case gdkScrollLock:
		return input.KeyScrollLock, true
	case gdkPause:
		return input.KeyPause, true
	case gdkMenu:
		return input.KeyContextMenu, true
	case gdkInsert:
		return input.KeyInsert, true
	case gdkHome:
		return input.KeyHome, true
	case gdkPageUp:
		return input.KeyPageUp, true
	case gdkDelete:
		return input.KeyDelete, true
	case gdkEnd:
		return input.KeyEnd, true
	case gdkPageDown:
		return input.KeyPageDown, true
	case gdkRight:
		return input.KeyRight, true
	case gdkLeft:
		return input.KeyLeft, true
	case gdkDown:
		return input.KeyDown, true
	case gdkUp:
		return input.KeyUp, true
	case gdkNumLock:
		return input.KeyNumLock, true
	case gdkKPDivide:
		return input.KeyNumpadDivide, true
	case gdkKPMultiply:
		return input.KeyNumpadMultiply, true
	case gdkKPSubtract:
		return input.KeyNumpadSubtract, true
	case gdkKPAdd:
		return input.KeyNumpadAdd, true
	case gdkKPEnter:
		return input.KeyNumpadEnter, true
	case gdkKPDecimal:
		return input.KeyNumpadDecimal, true
	case gdkKPEqual:
		return input.KeyNumpadEqual, true
	case gdkControlL:
		return input.KeyControlLeft, true
	case gdkShiftL:
		return input.KeyShiftLeft, true
	case gdkAltL:
		return input.KeyAltLeft, true
	case gdkSuperL:
		return input.KeyMetaLeft, true
	case gdkControlR:
		return input.KeyControlRight, true
	case gdkShiftR:
		return input.KeyShiftRight, true
	case gdkAltR:
		return input.KeyAltRight, true
	case gdkSuperR:
		return input.KeyMetaRight, true
	}
	return input.KeyUnknown, false
}
