package input

import (
	"sort"
	"time"
)

type KeyEvent struct {
	HID   byte
	Press bool
}

type Keyboard struct {
	pressed       map[Key]bool
	nextKeepAlive time.Time
}

func NewKeyboard() *Keyboard {
	return &Keyboard{
		pressed: map[Key]bool{},
	}
}

const (
	keyKeepAliveInterval = 50 * time.Millisecond
)

func (k *Keyboard) Update(keys []Key, now time.Time) []KeyEvent {
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	current := make(map[Key]bool, len(keys))
	events := make([]KeyEvent, 0, len(keys)+len(k.pressed))
	newPress := false

	for _, key := range keys {
		current[key] = true
		if k.pressed[key] {
			continue
		}
		newPress = true
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
	switch {
	case len(current) == 0:
		k.nextKeepAlive = time.Time{}
	case newPress || k.nextKeepAlive.IsZero():
		k.nextKeepAlive = now.Add(keyKeepAliveInterval)
	}
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
	k.nextKeepAlive = time.Time{}
	return events
}

func (k *Keyboard) KeepAlive(now time.Time) bool {
	if k.nextKeepAlive.IsZero() || now.Before(k.nextKeepAlive) || len(k.pressed) == 0 {
		return false
	}
	for !k.nextKeepAlive.After(now) {
		k.nextKeepAlive = k.nextKeepAlive.Add(keyKeepAliveInterval)
	}
	return true
}

func (k *Keyboard) Pressed() []Key {
	keys := make([]Key, 0, len(k.pressed))
	for key := range k.pressed {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
