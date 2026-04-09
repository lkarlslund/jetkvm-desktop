package input

import "testing"

func TestBuildPasteMacro(t *testing.T) {
	steps, invalid := BuildPasteMacro("en_US", "A!\n", 35)
	if len(invalid) != 0 {
		t.Fatalf("unexpected invalid runes: %v", invalid)
	}
	if len(steps) != 6 {
		t.Fatalf("expected 6 macro steps, got %d", len(steps))
	}
	if steps[0].Modifier != 0x02 || steps[0].Keys[0] != 4 || steps[0].Delay != 20 {
		t.Fatalf("unexpected first step: %+v", steps[0])
	}
	if steps[1].Modifier != 0 || steps[1].Keys[0] != 0 || steps[1].Delay != 35 {
		t.Fatalf("unexpected reset step: %+v", steps[1])
	}
	if steps[2].Modifier != 0x02 || steps[2].Keys[0] != 30 {
		t.Fatalf("unexpected punctuation step: %+v", steps[2])
	}
	if steps[4].Modifier != 0 || steps[4].Keys[0] != 40 {
		t.Fatalf("unexpected enter step: %+v", steps[4])
	}
}

func TestBuildPasteMacroReportsInvalidRunes(t *testing.T) {
	_, invalid := BuildPasteMacro("en_US", "ok€é", 20)
	if len(invalid) != 2 || invalid[0] != '€' || invalid[1] != 'é' {
		t.Fatalf("unexpected invalid runes: %v", invalid)
	}
}
