package hotkeys

import (
	"testing"

	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
)

func TestWindowManagerDisabledDoesNotConsumeKeys(t *testing.T) {
	manager := NewManager()

	result := manager.Update([]input.Key{input.KeyControlLeft, input.KeyAltLeft, input.KeyGraveAccent})
	if result.Consumed {
		t.Fatal("disabled manager should not consume keys")
	}
	if len(result.Actions) != 0 {
		t.Fatalf("disabled manager actions = %+v, want none", result.Actions)
	}
}

func TestWindowManagerMatchesNextTaskSwitcherOncePerHold(t *testing.T) {
	manager := NewManager()
	manager.SetEnabled(true)

	keys := []input.Key{input.KeyControlLeft, input.KeyAltLeft, input.KeyGraveAccent}
	first := manager.Update(keys)
	if !first.Consumed {
		t.Fatal("expected experimental chord to be consumed")
	}
	if len(first.Actions) != 1 || first.Actions[0] != ActionRemoteTaskSwitcherNext {
		t.Fatalf("first actions = %+v, want next task switcher", first.Actions)
	}

	second := manager.Update(keys)
	if !second.Consumed {
		t.Fatal("held chord should remain consumed")
	}
	if len(second.Actions) != 0 {
		t.Fatalf("second actions = %+v, want none while held", second.Actions)
	}

	manager.Update(nil)
	third := manager.Update(keys)
	if len(third.Actions) != 1 || third.Actions[0] != ActionRemoteTaskSwitcherNext {
		t.Fatalf("third actions = %+v, want next task switcher after reset", third.Actions)
	}
}

func TestWindowManagerMatchesPreviousTaskSwitcher(t *testing.T) {
	manager := NewManager()
	manager.SetEnabled(true)

	result := manager.Update([]input.Key{
		input.KeyControlRight,
		input.KeyAltRight,
		input.KeyShiftLeft,
		input.KeyGraveAccent,
	})
	if !result.Consumed {
		t.Fatal("expected previous task switcher chord to be consumed")
	}
	if len(result.Actions) != 1 || result.Actions[0] != ActionRemoteTaskSwitcherPrev {
		t.Fatalf("actions = %+v, want previous task switcher", result.Actions)
	}
}

func TestMacroStepsForNextTaskSwitcher(t *testing.T) {
	steps, err := MacroSteps(ActionRemoteTaskSwitcherNext)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 4 {
		t.Fatalf("step count = %d, want 4", len(steps))
	}
	if steps[0].Modifier != 0x04 {
		t.Fatalf("first modifier = %08b, want left alt", steps[0].Modifier)
	}
	if steps[1].Modifier != 0x04 || steps[1].Keys[0] != 43 {
		t.Fatalf("second step = %+v, want alt+tab", steps[1])
	}
	if steps[3].Modifier != 0 || steps[3].Keys[0] != 0 {
		t.Fatalf("final step = %+v, want release all", steps[3])
	}
}
