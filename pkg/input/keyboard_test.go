package input

import (
	"testing"
)

func TestKeyboardUpdatePressAndRelease(t *testing.T) {
	k := NewKeyboard()

	events := k.Update([]Key{KeyA, KeyShiftLeft})
	if len(events) != 2 {
		t.Fatalf("expected 2 press events, got %d", len(events))
	}
	if events[0] != (KeyEvent{HID: 4, Press: true}) {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1] != (KeyEvent{HID: 225, Press: true}) {
		t.Fatalf("unexpected second event: %+v", events[1])
	}

	events = k.Update([]Key{KeyShiftLeft})
	if len(events) != 1 {
		t.Fatalf("expected 1 release event, got %d", len(events))
	}
	if events[0] != (KeyEvent{HID: 4, Press: false}) {
		t.Fatalf("unexpected release event: %+v", events[0])
	}
}

func TestKeyboardReleaseAll(t *testing.T) {
	k := NewKeyboard()
	_ = k.Update([]Key{KeyB, KeyControlRight})

	events := k.ReleaseAll()
	if len(events) != 2 {
		t.Fatalf("expected 2 release events, got %d", len(events))
	}
	if events[0].Press || events[1].Press {
		t.Fatalf("expected release events only, got %+v", events)
	}

	events = k.ReleaseAll()
	if len(events) != 0 {
		t.Fatalf("expected no events after repeated release all, got %+v", events)
	}
}

func TestKeyboardIgnoresUnknownKeys(t *testing.T) {
	k := NewKeyboard()

	events := k.Update([]Key{KeyUnknown})
	if len(events) != 0 {
		t.Fatalf("expected unknown key to be ignored, got %+v", events)
	}
}

func TestKeyboardSortsPressesDeterministically(t *testing.T) {
	k := NewKeyboard()

	events := k.Update([]Key{KeyShiftRight, KeyA, KeyControlLeft})
	if len(events) != 3 {
		t.Fatalf("expected 3 press events, got %d", len(events))
	}
	expected := []KeyEvent{
		{HID: 4, Press: true},
		{HID: 224, Press: true},
		{HID: 229, Press: true},
	}
	for i := range expected {
		if events[i] != expected[i] {
			t.Fatalf("unexpected event %d: got %+v want %+v", i, events[i], expected[i])
		}
	}
}

func TestKeyboardReleaseAllCoversModifiersAndKeypad(t *testing.T) {
	k := NewKeyboard()
	_ = k.Update([]Key{KeyControlLeft, KeyAltRight, KeyNumpadEnter, KeySlash})

	events := k.ReleaseAll()
	if len(events) != 4 {
		t.Fatalf("expected 4 release events, got %+v", events)
	}
	for _, event := range events {
		if event.Press {
			t.Fatalf("expected release event, got %+v", event)
		}
	}
}

func TestKeyboardPressedReturnsSortedKeys(t *testing.T) {
	k := NewKeyboard()
	_ = k.Update([]Key{KeyShiftRight, KeyA, KeyControlLeft})

	pressed := k.Pressed()
	expected := []Key{KeyA, KeyControlLeft, KeyShiftRight}
	if len(pressed) != len(expected) {
		t.Fatalf("unexpected pressed key count: got %v want %v", pressed, expected)
	}
	for i := range expected {
		if pressed[i] != expected[i] {
			t.Fatalf("unexpected pressed key at %d: got %v want %v", i, pressed[i], expected[i])
		}
	}
}

func TestKeyString(t *testing.T) {
	if got := KeyControlLeft.String(); got != "Left Ctrl" {
		t.Fatalf("unexpected string for control key: %q", got)
	}
	if got := KeyA.String(); got != "A" {
		t.Fatalf("unexpected string for letter key: %q", got)
	}
}
