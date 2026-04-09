package input

import "fmt"

type Key int

const (
	KeyUnknown Key = iota
	KeyA
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	Key0
	KeyEnter
	KeyEscape
	KeyBackspace
	KeyTab
	KeySpace
	KeyMinus
	KeyEqual
	KeyLeftBracket
	KeyRightBracket
	KeyBackslash
	KeySemicolon
	KeyApostrophe
	KeyGraveAccent
	KeyComma
	KeyPeriod
	KeySlash
	KeyCapsLock
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyPrintScreen
	KeyScrollLock
	KeyPause
	KeyInsert
	KeyHome
	KeyPageUp
	KeyDelete
	KeyEnd
	KeyPageDown
	KeyRight
	KeyLeft
	KeyDown
	KeyUp
	KeyNumLock
	KeyNumpadDivide
	KeyNumpadMultiply
	KeyNumpadSubtract
	KeyNumpadAdd
	KeyNumpadEnter
	KeyNumpad1
	KeyNumpad2
	KeyNumpad3
	KeyNumpad4
	KeyNumpad5
	KeyNumpad6
	KeyNumpad7
	KeyNumpad8
	KeyNumpad9
	KeyNumpad0
	KeyNumpadDecimal
	KeyControlLeft
	KeyShiftLeft
	KeyAltLeft
	KeyMetaLeft
	KeyControlRight
	KeyShiftRight
	KeyAltRight
	KeyMetaRight
)

func KeyToHID(key Key) (byte, bool) {
	switch key {
	case KeyA:
		return 4, true
	case KeyB:
		return 5, true
	case KeyC:
		return 6, true
	case KeyD:
		return 7, true
	case KeyE:
		return 8, true
	case KeyF:
		return 9, true
	case KeyG:
		return 10, true
	case KeyH:
		return 11, true
	case KeyI:
		return 12, true
	case KeyJ:
		return 13, true
	case KeyK:
		return 14, true
	case KeyL:
		return 15, true
	case KeyM:
		return 16, true
	case KeyN:
		return 17, true
	case KeyO:
		return 18, true
	case KeyP:
		return 19, true
	case KeyQ:
		return 20, true
	case KeyR:
		return 21, true
	case KeyS:
		return 22, true
	case KeyT:
		return 23, true
	case KeyU:
		return 24, true
	case KeyV:
		return 25, true
	case KeyW:
		return 26, true
	case KeyX:
		return 27, true
	case KeyY:
		return 28, true
	case KeyZ:
		return 29, true
	case Key1:
		return 30, true
	case Key2:
		return 31, true
	case Key3:
		return 32, true
	case Key4:
		return 33, true
	case Key5:
		return 34, true
	case Key6:
		return 35, true
	case Key7:
		return 36, true
	case Key8:
		return 37, true
	case Key9:
		return 38, true
	case Key0:
		return 39, true
	case KeyEnter:
		return 40, true
	case KeyEscape:
		return 41, true
	case KeyBackspace:
		return 42, true
	case KeyTab:
		return 43, true
	case KeySpace:
		return 44, true
	case KeyMinus:
		return 45, true
	case KeyEqual:
		return 46, true
	case KeyLeftBracket:
		return 47, true
	case KeyRightBracket:
		return 48, true
	case KeyBackslash:
		return 49, true
	case KeySemicolon:
		return 51, true
	case KeyApostrophe:
		return 52, true
	case KeyGraveAccent:
		return 53, true
	case KeyComma:
		return 54, true
	case KeyPeriod:
		return 55, true
	case KeySlash:
		return 56, true
	case KeyCapsLock:
		return 57, true
	case KeyF1:
		return 58, true
	case KeyF2:
		return 59, true
	case KeyF3:
		return 60, true
	case KeyF4:
		return 61, true
	case KeyF5:
		return 62, true
	case KeyF6:
		return 63, true
	case KeyF7:
		return 64, true
	case KeyF8:
		return 65, true
	case KeyF9:
		return 66, true
	case KeyF10:
		return 67, true
	case KeyF11:
		return 68, true
	case KeyF12:
		return 69, true
	case KeyPrintScreen:
		return 70, true
	case KeyScrollLock:
		return 71, true
	case KeyPause:
		return 72, true
	case KeyInsert:
		return 73, true
	case KeyHome:
		return 74, true
	case KeyPageUp:
		return 75, true
	case KeyDelete:
		return 76, true
	case KeyEnd:
		return 77, true
	case KeyPageDown:
		return 78, true
	case KeyRight:
		return 79, true
	case KeyLeft:
		return 80, true
	case KeyDown:
		return 81, true
	case KeyUp:
		return 82, true
	case KeyNumLock:
		return 83, true
	case KeyNumpadDivide:
		return 84, true
	case KeyNumpadMultiply:
		return 85, true
	case KeyNumpadSubtract:
		return 86, true
	case KeyNumpadAdd:
		return 87, true
	case KeyNumpadEnter:
		return 88, true
	case KeyNumpad1:
		return 89, true
	case KeyNumpad2:
		return 90, true
	case KeyNumpad3:
		return 91, true
	case KeyNumpad4:
		return 92, true
	case KeyNumpad5:
		return 93, true
	case KeyNumpad6:
		return 94, true
	case KeyNumpad7:
		return 95, true
	case KeyNumpad8:
		return 96, true
	case KeyNumpad9:
		return 97, true
	case KeyNumpad0:
		return 98, true
	case KeyNumpadDecimal:
		return 99, true
	case KeyControlLeft:
		return 224, true
	case KeyShiftLeft:
		return 225, true
	case KeyAltLeft:
		return 226, true
	case KeyMetaLeft:
		return 227, true
	case KeyControlRight:
		return 228, true
	case KeyShiftRight:
		return 229, true
	case KeyAltRight:
		return 230, true
	case KeyMetaRight:
		return 231, true
	default:
		return 0, false
	}
}

func (k Key) String() string {
	switch k {
	case KeyA:
		return "A"
	case KeyB:
		return "B"
	case KeyC:
		return "C"
	case KeyD:
		return "D"
	case KeyE:
		return "E"
	case KeyF:
		return "F"
	case KeyG:
		return "G"
	case KeyH:
		return "H"
	case KeyI:
		return "I"
	case KeyJ:
		return "J"
	case KeyK:
		return "K"
	case KeyL:
		return "L"
	case KeyM:
		return "M"
	case KeyN:
		return "N"
	case KeyO:
		return "O"
	case KeyP:
		return "P"
	case KeyQ:
		return "Q"
	case KeyR:
		return "R"
	case KeyS:
		return "S"
	case KeyT:
		return "T"
	case KeyU:
		return "U"
	case KeyV:
		return "V"
	case KeyW:
		return "W"
	case KeyX:
		return "X"
	case KeyY:
		return "Y"
	case KeyZ:
		return "Z"
	case Key1:
		return "1"
	case Key2:
		return "2"
	case Key3:
		return "3"
	case Key4:
		return "4"
	case Key5:
		return "5"
	case Key6:
		return "6"
	case Key7:
		return "7"
	case Key8:
		return "8"
	case Key9:
		return "9"
	case Key0:
		return "0"
	case KeyEnter:
		return "Enter"
	case KeyEscape:
		return "Esc"
	case KeyBackspace:
		return "Backspace"
	case KeyTab:
		return "Tab"
	case KeySpace:
		return "Space"
	case KeyMinus:
		return "-"
	case KeyEqual:
		return "="
	case KeyLeftBracket:
		return "["
	case KeyRightBracket:
		return "]"
	case KeyBackslash:
		return "\\"
	case KeySemicolon:
		return ";"
	case KeyApostrophe:
		return "'"
	case KeyGraveAccent:
		return "`"
	case KeyComma:
		return ","
	case KeyPeriod:
		return "."
	case KeySlash:
		return "/"
	case KeyCapsLock:
		return "Caps Lock"
	case KeyF1:
		return "F1"
	case KeyF2:
		return "F2"
	case KeyF3:
		return "F3"
	case KeyF4:
		return "F4"
	case KeyF5:
		return "F5"
	case KeyF6:
		return "F6"
	case KeyF7:
		return "F7"
	case KeyF8:
		return "F8"
	case KeyF9:
		return "F9"
	case KeyF10:
		return "F10"
	case KeyF11:
		return "F11"
	case KeyF12:
		return "F12"
	case KeyPrintScreen:
		return "Print Screen"
	case KeyScrollLock:
		return "Scroll Lock"
	case KeyPause:
		return "Pause"
	case KeyInsert:
		return "Insert"
	case KeyHome:
		return "Home"
	case KeyPageUp:
		return "Page Up"
	case KeyDelete:
		return "Delete"
	case KeyEnd:
		return "End"
	case KeyPageDown:
		return "Page Down"
	case KeyRight:
		return "Right"
	case KeyLeft:
		return "Left"
	case KeyDown:
		return "Down"
	case KeyUp:
		return "Up"
	case KeyNumLock:
		return "Num Lock"
	case KeyNumpadDivide:
		return "Num /"
	case KeyNumpadMultiply:
		return "Num *"
	case KeyNumpadSubtract:
		return "Num -"
	case KeyNumpadAdd:
		return "Num +"
	case KeyNumpadEnter:
		return "Num Enter"
	case KeyNumpad1:
		return "Num 1"
	case KeyNumpad2:
		return "Num 2"
	case KeyNumpad3:
		return "Num 3"
	case KeyNumpad4:
		return "Num 4"
	case KeyNumpad5:
		return "Num 5"
	case KeyNumpad6:
		return "Num 6"
	case KeyNumpad7:
		return "Num 7"
	case KeyNumpad8:
		return "Num 8"
	case KeyNumpad9:
		return "Num 9"
	case KeyNumpad0:
		return "Num 0"
	case KeyNumpadDecimal:
		return "Num ."
	case KeyControlLeft:
		return "Left Ctrl"
	case KeyShiftLeft:
		return "Left Shift"
	case KeyAltLeft:
		return "Left Alt"
	case KeyMetaLeft:
		return "Left Meta"
	case KeyControlRight:
		return "Right Ctrl"
	case KeyShiftRight:
		return "Right Shift"
	case KeyAltRight:
		return "Right Alt"
	case KeyMetaRight:
		return "Right Meta"
	default:
		return fmt.Sprintf("Key(%d)", k)
	}
}
