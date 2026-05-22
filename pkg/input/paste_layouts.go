package input

// HID usage codes (USB HID Usage Tables, Keyboard/Keypad Page 0x07).
const (
	hidA = 0x04
	hidB = 0x05
	hidC = 0x06
	hidD = 0x07
	hidE = 0x08
	hidF = 0x09
	hidG = 0x0a
	hidH = 0x0b
	hidI = 0x0c
	hidJ = 0x0d
	hidK = 0x0e
	hidL = 0x0f
	hidM = 0x10
	hidN = 0x11
	hidO = 0x12
	hidP = 0x13
	hidQ = 0x14
	hidR = 0x15
	hidS = 0x16
	hidT = 0x17
	hidU = 0x18
	hidV = 0x19
	hidW = 0x1a
	hidX = 0x1b
	hidY = 0x1c
	hidZ = 0x1d

	hidDigit1 = 0x1e
	hidDigit2 = 0x1f
	hidDigit3 = 0x20
	hidDigit4 = 0x21
	hidDigit5 = 0x22
	hidDigit6 = 0x23
	hidDigit7 = 0x24
	hidDigit8 = 0x25
	hidDigit9 = 0x26
	hidDigit0 = 0x27

	hidEnter        = 0x28
	hidEscape       = 0x29
	hidBackspace    = 0x2a
	hidTab          = 0x2b
	hidSpace        = 0x2c
	hidMinus        = 0x2d
	hidEqual        = 0x2e
	hidBracketLeft  = 0x2f
	hidBracketRight = 0x30
	hidBackslash    = 0x31
	hidSemicolon    = 0x33
	hidQuote        = 0x34
	hidBackquote    = 0x35
	hidComma        = 0x36
	hidPeriod       = 0x37
	hidSlash        = 0x38

	hidIntlBackslash = 0x64
)

// pasteChar describes how to type one character on a given keyboard layout.
type pasteChar struct {
	HID      byte
	Modifier byte
	DeadKey  bool
	// Accent is an optional dead-key prefix that needs to be pressed before
	// the actual key (e.g. Portuguese Á = press ´ dead-key, then A).
	Accent *pasteChar
}

// pasteLayout is the per-language char→keystroke mapping used by the
// paste-text feature. Characters not present are reported as invalid.
type pasteLayout map[rune]pasteChar

// dead-key references shared by accented letters.
var (
	ptAcute = &pasteChar{HID: hidBracketRight}
	ptGrave = &pasteChar{HID: hidBracketRight, Modifier: modShiftLeft}
	ptTrema = &pasteChar{HID: hidBracketLeft, Modifier: modAltRight}
	ptTilde = &pasteChar{HID: hidBackslash}
	ptHat   = &pasteChar{HID: hidBackslash, Modifier: modShiftLeft}
)

// pasteLayouts maps keyboard layout codes (as normalized by
// NormalizeKeyboardLayoutCode) to char maps.
var pasteLayouts = map[string]pasteLayout{
	"en-US": enUSPasteChars,
	"en-UK": enUKPasteChars,
	"pt-PT": ptPTPasteChars,
	"de-DE": deDEPasteChars,
	"es-ES": esESPasteChars,
	"fr-FR": frFRPasteChars,
	"it-IT": itITPasteChars,
}

// lookupPasteLayout returns the char map for the given layout code, falling
// back to en-US if the layout is unknown or empty.
func lookupPasteLayout(code string) pasteLayout {
	code = NormalizeKeyboardLayoutCode(code)
	if layout, ok := pasteLayouts[code]; ok {
		return layout
	}
	return enUSPasteChars
}

// enUSPasteChars is the en-US (US-International ANSI) paste map.
var enUSPasteChars = pasteLayout{
	'a': {HID: hidA}, 'b': {HID: hidB}, 'c': {HID: hidC}, 'd': {HID: hidD}, 'e': {HID: hidE},
	'f': {HID: hidF}, 'g': {HID: hidG}, 'h': {HID: hidH}, 'i': {HID: hidI}, 'j': {HID: hidJ},
	'k': {HID: hidK}, 'l': {HID: hidL}, 'm': {HID: hidM}, 'n': {HID: hidN}, 'o': {HID: hidO},
	'p': {HID: hidP}, 'q': {HID: hidQ}, 'r': {HID: hidR}, 's': {HID: hidS}, 't': {HID: hidT},
	'u': {HID: hidU}, 'v': {HID: hidV}, 'w': {HID: hidW}, 'x': {HID: hidX}, 'y': {HID: hidY},
	'z': {HID: hidZ},
	'A': {HID: hidA, Modifier: modShiftLeft}, 'B': {HID: hidB, Modifier: modShiftLeft},
	'C': {HID: hidC, Modifier: modShiftLeft}, 'D': {HID: hidD, Modifier: modShiftLeft},
	'E': {HID: hidE, Modifier: modShiftLeft}, 'F': {HID: hidF, Modifier: modShiftLeft},
	'G': {HID: hidG, Modifier: modShiftLeft}, 'H': {HID: hidH, Modifier: modShiftLeft},
	'I': {HID: hidI, Modifier: modShiftLeft}, 'J': {HID: hidJ, Modifier: modShiftLeft},
	'K': {HID: hidK, Modifier: modShiftLeft}, 'L': {HID: hidL, Modifier: modShiftLeft},
	'M': {HID: hidM, Modifier: modShiftLeft}, 'N': {HID: hidN, Modifier: modShiftLeft},
	'O': {HID: hidO, Modifier: modShiftLeft}, 'P': {HID: hidP, Modifier: modShiftLeft},
	'Q': {HID: hidQ, Modifier: modShiftLeft}, 'R': {HID: hidR, Modifier: modShiftLeft},
	'S': {HID: hidS, Modifier: modShiftLeft}, 'T': {HID: hidT, Modifier: modShiftLeft},
	'U': {HID: hidU, Modifier: modShiftLeft}, 'V': {HID: hidV, Modifier: modShiftLeft},
	'W': {HID: hidW, Modifier: modShiftLeft}, 'X': {HID: hidX, Modifier: modShiftLeft},
	'Y': {HID: hidY, Modifier: modShiftLeft}, 'Z': {HID: hidZ, Modifier: modShiftLeft},
	'1': {HID: hidDigit1}, '2': {HID: hidDigit2}, '3': {HID: hidDigit3},
	'4': {HID: hidDigit4}, '5': {HID: hidDigit5}, '6': {HID: hidDigit6},
	'7': {HID: hidDigit7}, '8': {HID: hidDigit8}, '9': {HID: hidDigit9}, '0': {HID: hidDigit0},
	'!': {HID: hidDigit1, Modifier: modShiftLeft}, '@': {HID: hidDigit2, Modifier: modShiftLeft},
	'#': {HID: hidDigit3, Modifier: modShiftLeft}, '$': {HID: hidDigit4, Modifier: modShiftLeft},
	'%': {HID: hidDigit5, Modifier: modShiftLeft}, '^': {HID: hidDigit6, Modifier: modShiftLeft},
	'&': {HID: hidDigit7, Modifier: modShiftLeft}, '*': {HID: hidDigit8, Modifier: modShiftLeft},
	'(': {HID: hidDigit9, Modifier: modShiftLeft}, ')': {HID: hidDigit0, Modifier: modShiftLeft},
	'\n': {HID: hidEnter}, '\r': {HID: hidEnter}, '\t': {HID: hidTab}, ' ': {HID: hidSpace},
	'-': {HID: hidMinus}, '_': {HID: hidMinus, Modifier: modShiftLeft},
	'=': {HID: hidEqual}, '+': {HID: hidEqual, Modifier: modShiftLeft},
	'[': {HID: hidBracketLeft}, '{': {HID: hidBracketLeft, Modifier: modShiftLeft},
	']': {HID: hidBracketRight}, '}': {HID: hidBracketRight, Modifier: modShiftLeft},
	'\\': {HID: hidBackslash}, '|': {HID: hidBackslash, Modifier: modShiftLeft},
	';': {HID: hidSemicolon}, ':': {HID: hidSemicolon, Modifier: modShiftLeft},
	'\'': {HID: hidQuote}, '"': {HID: hidQuote, Modifier: modShiftLeft},
	'`': {HID: hidBackquote}, '~': {HID: hidBackquote, Modifier: modShiftLeft},
	',': {HID: hidComma}, '<': {HID: hidComma, Modifier: modShiftLeft},
	'.': {HID: hidPeriod}, '>': {HID: hidPeriod, Modifier: modShiftLeft},
	'/': {HID: hidSlash}, '?': {HID: hidSlash, Modifier: modShiftLeft},
}

// ptPTPasteChars is the Portuguese (pt-PT) paste map (ABNT-like layout with
// dead keys for accents). Ported from JetKVM web app's pt_PT layout.
var ptPTPasteChars = pasteLayout{
	// Uppercase letters
	'A': {HID: hidA, Modifier: modShiftLeft},
	'Á': {HID: hidA, Modifier: modShiftLeft, Accent: ptAcute},
	'À': {HID: hidA, Modifier: modShiftLeft, Accent: ptGrave},
	'Ä': {HID: hidA, Modifier: modShiftLeft, Accent: ptTrema},
	'Ã': {HID: hidA, Modifier: modShiftLeft, Accent: ptTilde},
	'Â': {HID: hidA, Modifier: modShiftLeft, Accent: ptHat},
	'B': {HID: hidB, Modifier: modShiftLeft},
	'C': {HID: hidC, Modifier: modShiftLeft},
	'D': {HID: hidD, Modifier: modShiftLeft},
	'E': {HID: hidE, Modifier: modShiftLeft},
	'É': {HID: hidE, Modifier: modShiftLeft, Accent: ptAcute},
	'È': {HID: hidE, Modifier: modShiftLeft, Accent: ptGrave},
	'Ë': {HID: hidE, Modifier: modShiftLeft, Accent: ptTrema},
	'Ê': {HID: hidE, Modifier: modShiftLeft, Accent: ptHat},
	'F': {HID: hidF, Modifier: modShiftLeft},
	'G': {HID: hidG, Modifier: modShiftLeft},
	'H': {HID: hidH, Modifier: modShiftLeft},
	'I': {HID: hidI, Modifier: modShiftLeft},
	'Í': {HID: hidI, Modifier: modShiftLeft, Accent: ptAcute},
	'Ì': {HID: hidI, Modifier: modShiftLeft, Accent: ptGrave},
	'Ï': {HID: hidI, Modifier: modShiftLeft, Accent: ptTrema},
	'Î': {HID: hidI, Modifier: modShiftLeft, Accent: ptHat},
	'J': {HID: hidJ, Modifier: modShiftLeft},
	'K': {HID: hidK, Modifier: modShiftLeft},
	'L': {HID: hidL, Modifier: modShiftLeft},
	'M': {HID: hidM, Modifier: modShiftLeft},
	'N': {HID: hidN, Modifier: modShiftLeft},
	'Ñ': {HID: hidN, Modifier: modShiftLeft, Accent: ptTilde},
	'O': {HID: hidO, Modifier: modShiftLeft},
	'Ó': {HID: hidO, Modifier: modShiftLeft, Accent: ptAcute},
	'Ò': {HID: hidO, Modifier: modShiftLeft, Accent: ptGrave},
	'Ö': {HID: hidO, Modifier: modShiftLeft, Accent: ptTrema},
	'Õ': {HID: hidO, Modifier: modShiftLeft, Accent: ptTilde},
	'Ô': {HID: hidO, Modifier: modShiftLeft, Accent: ptHat},
	'P': {HID: hidP, Modifier: modShiftLeft},
	'Q': {HID: hidQ, Modifier: modShiftLeft},
	'R': {HID: hidR, Modifier: modShiftLeft},
	'S': {HID: hidS, Modifier: modShiftLeft},
	'T': {HID: hidT, Modifier: modShiftLeft},
	'U': {HID: hidU, Modifier: modShiftLeft},
	'Ú': {HID: hidU, Modifier: modShiftLeft, Accent: ptAcute},
	'Ù': {HID: hidU, Modifier: modShiftLeft, Accent: ptGrave},
	'Ü': {HID: hidU, Modifier: modShiftLeft, Accent: ptTrema},
	'Û': {HID: hidU, Modifier: modShiftLeft, Accent: ptHat},
	'V': {HID: hidV, Modifier: modShiftLeft},
	'W': {HID: hidW, Modifier: modShiftLeft},
	'X': {HID: hidX, Modifier: modShiftLeft},
	'Y': {HID: hidY, Modifier: modShiftLeft},
	'Ý': {HID: hidY, Modifier: modShiftLeft, Accent: ptAcute},
	'Z': {HID: hidZ, Modifier: modShiftLeft},

	// Lowercase letters
	'a': {HID: hidA},
	'á': {HID: hidA, Accent: ptAcute},
	'à': {HID: hidA, Accent: ptGrave},
	'ä': {HID: hidA, Accent: ptTrema},
	'ã': {HID: hidA, Accent: ptTilde},
	'â': {HID: hidA, Accent: ptHat},
	'b': {HID: hidB},
	'c': {HID: hidC},
	'd': {HID: hidD},
	'e': {HID: hidE},
	'é': {HID: hidE, Accent: ptAcute},
	'è': {HID: hidE, Accent: ptGrave},
	'ë': {HID: hidE, Accent: ptTrema},
	'ê': {HID: hidE, Accent: ptHat},
	'€': {HID: hidE, Modifier: modAltRight},
	'f': {HID: hidF},
	'g': {HID: hidG},
	'h': {HID: hidH},
	'i': {HID: hidI},
	'í': {HID: hidI, Accent: ptAcute},
	'ì': {HID: hidI, Accent: ptGrave},
	'ï': {HID: hidI, Accent: ptTrema},
	'î': {HID: hidI, Accent: ptHat},
	'j': {HID: hidJ},
	'k': {HID: hidK},
	'l': {HID: hidL},
	'm': {HID: hidM},
	'n': {HID: hidN},
	'ñ': {HID: hidN, Accent: ptTilde},
	'o': {HID: hidO},
	'ó': {HID: hidO, Accent: ptAcute},
	'ò': {HID: hidO, Accent: ptGrave},
	'ö': {HID: hidO, Accent: ptTrema},
	'õ': {HID: hidO, Accent: ptTilde},
	'ô': {HID: hidO, Accent: ptHat},
	'p': {HID: hidP},
	'q': {HID: hidQ},
	'r': {HID: hidR},
	's': {HID: hidS},
	't': {HID: hidT},
	'u': {HID: hidU},
	'ú': {HID: hidU, Accent: ptAcute},
	'ù': {HID: hidU, Accent: ptGrave},
	'ü': {HID: hidU, Accent: ptTrema},
	'û': {HID: hidU, Accent: ptHat},
	'v': {HID: hidV},
	'w': {HID: hidW},
	'x': {HID: hidX},
	'y': {HID: hidY},
	'ý': {HID: hidY, Accent: ptAcute},
	'ÿ': {HID: hidY, Accent: ptTrema},
	'z': {HID: hidZ},

	// SC 29 → Backquote: \ |
	'\\': {HID: hidBackquote},
	'|':  {HID: hidBackquote, Modifier: modShiftLeft},

	// Number row
	'1':  {HID: hidDigit1},
	'!':  {HID: hidDigit1, Modifier: modShiftLeft},
	'2':  {HID: hidDigit2},
	'"':  {HID: hidDigit2, Modifier: modShiftLeft},
	'@':  {HID: hidDigit2, Modifier: modAltRight},
	'3':  {HID: hidDigit3},
	'#':  {HID: hidDigit3, Modifier: modShiftLeft},
	'£':  {HID: hidDigit3, Modifier: modAltRight},
	'4':  {HID: hidDigit4},
	'$':  {HID: hidDigit4, Modifier: modShiftLeft},
	'§':  {HID: hidDigit4, Modifier: modAltRight},
	'5':  {HID: hidDigit5},
	'%':  {HID: hidDigit5, Modifier: modShiftLeft},
	'6':  {HID: hidDigit6},
	'&':  {HID: hidDigit6, Modifier: modShiftLeft},
	'7':  {HID: hidDigit7},
	'/':  {HID: hidDigit7, Modifier: modShiftLeft},
	'{':  {HID: hidDigit7, Modifier: modAltRight},
	'8':  {HID: hidDigit8},
	'(':  {HID: hidDigit8, Modifier: modShiftLeft},
	'[':  {HID: hidDigit8, Modifier: modAltRight},
	'9':  {HID: hidDigit9},
	')':  {HID: hidDigit9, Modifier: modShiftLeft},
	']':  {HID: hidDigit9, Modifier: modAltRight},
	'0':  {HID: hidDigit0},
	'=':  {HID: hidDigit0, Modifier: modShiftLeft},
	'}':  {HID: hidDigit0, Modifier: modAltRight},

	// SC 0C → Minus: ' ?
	'\'': {HID: hidMinus},
	'?':  {HID: hidMinus, Modifier: modShiftLeft},

	// SC 0D → Equal: « »
	'«': {HID: hidEqual},
	'»': {HID: hidEqual, Modifier: modShiftLeft},

	// SC 1A → BracketLeft: + * ¨(dead)
	'+': {HID: hidBracketLeft},
	'*': {HID: hidBracketLeft, Modifier: modShiftLeft},
	'¨': {HID: hidBracketLeft, Modifier: modAltRight, DeadKey: true},

	// SC 1B → BracketRight: ´(dead) `(dead)
	'´': {HID: hidBracketRight, DeadKey: true},
	'`': {HID: hidBracketRight, Modifier: modShiftLeft, DeadKey: true},

	// SC 27 → Semicolon: ç Ç
	'ç': {HID: hidSemicolon},
	'Ç': {HID: hidSemicolon, Modifier: modShiftLeft},

	// SC 28 → Quote: º ª
	'º': {HID: hidQuote},
	'ª': {HID: hidQuote, Modifier: modShiftLeft},

	// SC 2B → Backslash: ~(dead) ^(dead)
	'~': {HID: hidBackslash, DeadKey: true},
	'^': {HID: hidBackslash, Modifier: modShiftLeft, DeadKey: true},

	// SC 33-35: Comma, Period, Slash
	',': {HID: hidComma},
	';': {HID: hidComma, Modifier: modShiftLeft},
	'.': {HID: hidPeriod},
	':': {HID: hidPeriod, Modifier: modShiftLeft},
	'-': {HID: hidSlash},
	'_': {HID: hidSlash, Modifier: modShiftLeft},

	// SC 56 → IntlBackslash: < >
	'<': {HID: hidIntlBackslash},
	'>': {HID: hidIntlBackslash, Modifier: modShiftLeft},

	' ':  {HID: hidSpace},
	'\n': {HID: hidEnter},
	'\r': {HID: hidEnter},
	'\t': {HID: hidTab},
}
