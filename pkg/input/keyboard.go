package input

import "sort"

type KeyEvent struct {
	HID   byte
	Press bool
}

type Keyboard struct {
	pressed map[Key]bool
}

func NewKeyboard() *Keyboard {
	return &Keyboard{
		pressed: map[Key]bool{},
	}
}

func (k *Keyboard) Update(keys []Key) []KeyEvent {
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	current := make(map[Key]bool, len(keys))
	events := make([]KeyEvent, 0, len(keys)+len(k.pressed))

	for _, key := range keys {
		current[key] = true
		if k.pressed[key] {
			continue
		}
		if hid, ok := KeyToHID(key); ok {
			events = append(events, KeyEvent{HID: hid, Press: true})
		}
	}

	for key := range k.pressed {
		if current[key] {
			continue
		}
		if hid, ok := KeyToHID(key); ok {
			events = append(events, KeyEvent{HID: hid, Press: false})
		}
	}

	k.pressed = current
	return events
}

func (k *Keyboard) ReleaseAll() []KeyEvent {
	keys := make([]Key, 0, len(k.pressed))
	for key := range k.pressed {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	events := make([]KeyEvent, 0, len(keys))
	for _, key := range keys {
		if hid, ok := KeyToHID(key); ok {
			events = append(events, KeyEvent{HID: hid, Press: false})
		}
	}
	k.pressed = map[Key]bool{}
	return events
}

func (k *Keyboard) Pressed() []Key {
	keys := make([]Key, 0, len(k.pressed))
	for key := range k.pressed {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
