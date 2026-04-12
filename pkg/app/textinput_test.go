package app

import (
	"testing"

	"github.com/lkarlslund/jetkvm-desktop/pkg/ui"
)

func TestCurrentTextBindingDefaultsToLauncherInput(t *testing.T) {
	app := &App{
		launcherOpen:  true,
		launcherMode:  launcherModeBrowse,
		launcherInput: "jetkvm.local",
	}
	binding := app.currentTextBinding()
	if binding == nil {
		t.Fatal("expected launcher browse binding")
	}
	if binding.ID != "launcher_focus_input" {
		t.Fatalf("unexpected binding id %q", binding.ID)
	}
	if binding.Value != &app.launcherInput {
		t.Fatal("launcher input binding did not point at launcherInput")
	}
}

func TestTextInputStateBindCopiesValueWithoutFocus(t *testing.T) {
	state := ui.TextInputState{}
	binding := ui.TextInputBinding{
		ID:       "launcher_focus_input",
		Value:    "192.168.1.50",
		TextSize: 15,
	}
	field := state.Bind(ui.TextField{ID: "launcher_focus_input"}, &binding)
	if field.Value != "192.168.1.50" {
		t.Fatalf("expected unfocused field to still receive bound value, got %q", field.Value)
	}
	if field.Focused {
		t.Fatal("field should not be focused when state field id is empty")
	}
}

func TestTextInputStateSyncAndBind(t *testing.T) {
	state := ui.TextInputState{}
	binding := ui.TextInputBinding{
		ID:       "launcher_focus_input",
		Value:    "hello",
		TextSize: 15,
	}
	state.Sync(&binding)
	if state.FieldID != "launcher_focus_input" || state.Caret != 5 || state.Anchor != 5 {
		t.Fatalf("unexpected state after Sync: %+v", state)
	}
	field := state.Bind(ui.TextField{ID: "launcher_focus_input"}, &binding)
	if !field.Focused || field.CaretIndex != 5 {
		t.Fatalf("unexpected field binding: %+v", field)
	}
	if field.Value != "hello" {
		t.Fatalf("expected field value to be bound from text input state, got %q", field.Value)
	}
}

func TestTextInputStateBeginPointer(t *testing.T) {
	state := ui.TextInputState{}
	binding := ui.TextInputBinding{
		ID:       "launcher_focus_input",
		Value:    "abcdef",
		TextSize: 13,
	}
	state.BeginPointer(binding, ui.Rect{X: 10, Y: 10, W: 200, H: 38}, 10, false)
	if state.FieldID != "launcher_focus_input" || state.Caret != 0 || state.Anchor != 0 {
		t.Fatalf("unexpected state after BeginPointer: %+v", state)
	}
}

func TestIsTextFieldAction(t *testing.T) {
	for _, id := range []string{
		"launcher_focus_input",
		"launcher_focus_password",
		"network_focus_hostname",
		"mqtt_focus_broker",
	} {
		if !isTextFieldAction(id) {
			t.Fatalf("expected %q to be recognized as a text field action", id)
		}
	}
	if isTextFieldAction("reconnect") {
		t.Fatal("non-text action recognized as text field")
	}
	if isTextFieldAction("fake_focus_button") {
		t.Fatal("stringly _focus_ action recognized as text field")
	}
}
