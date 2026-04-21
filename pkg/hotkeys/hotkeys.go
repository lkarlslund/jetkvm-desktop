package hotkeys

import (
	"fmt"

	"github.com/lkarlslund/jetkvm-desktop/pkg/input"
	"github.com/lkarlslund/jetkvm-desktop/pkg/protocol/hidrpc"
)

type Action uint8

const (
	ActionUnknown Action = iota
	ActionRemoteTaskSwitcherNext
	ActionRemoteTaskSwitcherPrev
)

func (a Action) String() string {
	switch a {
	case ActionRemoteTaskSwitcherNext:
		return "remote_task_switcher_next"
	case ActionRemoteTaskSwitcherPrev:
		return "remote_task_switcher_prev"
	default:
		return "unknown"
	}
}

type Scope uint8

const (
	ScopeUnknown Scope = iota
	ScopeWindow
)

func (s Scope) String() string {
	switch s {
	case ScopeWindow:
		return "window"
	default:
		return "unknown"
	}
}

type Capability struct {
	Backend            string
	Scope              Scope
	GlobalRegistration bool
	LocalSuppression   bool
}

type Registration struct {
	Action      Action
	Trigger     string
	Description string
}

type UpdateResult struct {
	Actions  []Action
	Consumed bool
}

type Manager interface {
	Enabled() bool
	SetEnabled(enabled bool)
	Capability() Capability
	Registrations() []Registration
	Reset()
	Update(keys []input.Key) UpdateResult
}

type windowManager struct {
	enabled    bool
	capability Capability
	fired      map[Action]bool
}

func NewManager() Manager {
	return &windowManager{
		capability: Capability{
			Backend:            "ebiten",
			Scope:              ScopeWindow,
			GlobalRegistration: false,
			LocalSuppression:   true,
		},
		fired: make(map[Action]bool),
	}
}

func (m *windowManager) Enabled() bool {
	return m.enabled
}

func (m *windowManager) SetEnabled(enabled bool) {
	m.enabled = enabled
	if !enabled {
		m.Reset()
	}
}

func (m *windowManager) Capability() Capability {
	return m.capability
}

func (m *windowManager) Registrations() []Registration {
	return []Registration{
		{
			Action:      ActionRemoteTaskSwitcherNext,
			Trigger:     "Ctrl+Alt+`",
			Description: "Send remote Alt+Tab",
		},
		{
			Action:      ActionRemoteTaskSwitcherPrev,
			Trigger:     "Ctrl+Alt+Shift+`",
			Description: "Send remote Shift+Alt+Tab",
		},
	}
}

func (m *windowManager) Reset() {
	clear(m.fired)
}

func (m *windowManager) Update(keys []input.Key) UpdateResult {
	if !m.enabled {
		m.Reset()
		return UpdateResult{}
	}

	actions := matchActions(keys)
	if len(actions) == 0 {
		m.Reset()
		return UpdateResult{}
	}

	result := UpdateResult{Consumed: true}
	for _, action := range actions {
		if m.fired[action] {
			continue
		}
		m.fired[action] = true
		result.Actions = append(result.Actions, action)
	}
	return result
}

func matchActions(keys []input.Key) []Action {
	switch {
	case matchesPreviousTaskSwitcher(keys):
		return []Action{ActionRemoteTaskSwitcherPrev}
	case matchesNextTaskSwitcher(keys):
		return []Action{ActionRemoteTaskSwitcherNext}
	default:
		return nil
	}
}

func matchesNextTaskSwitcher(keys []input.Key) bool {
	return len(keys) == 3 &&
		hasKey(keys, input.KeyGraveAccent) &&
		hasEither(keys, input.KeyControlLeft, input.KeyControlRight) &&
		hasEither(keys, input.KeyAltLeft, input.KeyAltRight)
}

func matchesPreviousTaskSwitcher(keys []input.Key) bool {
	return len(keys) == 4 &&
		hasKey(keys, input.KeyGraveAccent) &&
		hasEither(keys, input.KeyControlLeft, input.KeyControlRight) &&
		hasEither(keys, input.KeyAltLeft, input.KeyAltRight) &&
		hasEither(keys, input.KeyShiftLeft, input.KeyShiftRight)
}

func hasKey(keys []input.Key, want input.Key) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

func hasEither(keys []input.Key, left, right input.Key) bool {
	return hasKey(keys, left) || hasKey(keys, right)
}

func MacroSteps(action Action) ([]hidrpc.KeyboardMacroStep, error) {
	switch action {
	case ActionRemoteTaskSwitcherNext:
		return []hidrpc.KeyboardMacroStep{
			mustStep([]input.Key{input.KeyAltLeft}, 20),
			mustStep([]input.Key{input.KeyAltLeft, input.KeyTab}, 20),
			mustStep([]input.Key{input.KeyAltLeft}, 20),
			{Delay: 0},
		}, nil
	case ActionRemoteTaskSwitcherPrev:
		return []hidrpc.KeyboardMacroStep{
			mustStep([]input.Key{input.KeyAltLeft, input.KeyShiftLeft}, 20),
			mustStep([]input.Key{input.KeyAltLeft, input.KeyShiftLeft, input.KeyTab}, 20),
			mustStep([]input.Key{input.KeyAltLeft, input.KeyShiftLeft}, 20),
			{Delay: 0},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported hotkey action %q", action.String())
	}
}

func mustStep(keys []input.Key, delay uint16) hidrpc.KeyboardMacroStep {
	step, err := macroStep(keys, delay)
	if err != nil {
		panic(err)
	}
	return step
}

func macroStep(keys []input.Key, delay uint16) (hidrpc.KeyboardMacroStep, error) {
	var step hidrpc.KeyboardMacroStep
	step.Delay = delay
	keyIndex := 0
	for _, key := range keys {
		hid, ok := input.KeyToHID(key)
		if !ok {
			return hidrpc.KeyboardMacroStep{}, fmt.Errorf("no HID code for %v", key)
		}
		if modifierBit, ok := hidModifierBit(hid); ok {
			step.Modifier |= modifierBit
			continue
		}
		if keyIndex >= len(step.Keys) {
			return hidrpc.KeyboardMacroStep{}, fmt.Errorf("too many HID keys in hotkey step")
		}
		step.Keys[keyIndex] = hid
		keyIndex++
	}
	return step, nil
}

func hidModifierBit(hid byte) (byte, bool) {
	if hid < 224 || hid > 231 {
		return 0, false
	}
	return 1 << (hid - 224), true
}
