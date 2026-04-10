package input

import "strings"

type KeyboardLayout struct {
	Code  string
	Label string
}

var supportedKeyboardLayouts = []KeyboardLayout{
	{Code: "en-US", Label: "US"},
	{Code: "en-UK", Label: "UK"},
	{Code: "da-DK", Label: "Danish"},
	{Code: "de-DE", Label: "German"},
	{Code: "fr-FR", Label: "French"},
	{Code: "es-ES", Label: "Spanish"},
	{Code: "it-IT", Label: "Italian"},
	{Code: "ja-JP", Label: "Japanese"},
}

func SupportedKeyboardLayouts() []KeyboardLayout {
	out := make([]KeyboardLayout, len(supportedKeyboardLayouts))
	copy(out, supportedKeyboardLayouts)
	return out
}

func NormalizeKeyboardLayoutCode(layout string) string {
	layout = strings.TrimSpace(layout)
	layout = strings.ReplaceAll(layout, "_", "-")
	switch layout {
	case "", "en-US", "en-UK", "da-DK", "de-DE", "fr-FR", "es-ES", "it-IT", "ja-JP":
		return layout
	default:
		return "en-US"
	}
}
