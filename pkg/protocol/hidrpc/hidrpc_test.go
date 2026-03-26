package hidrpc

import "testing"

func TestRoundTrip(t *testing.T) {
	tests := []Message{
		Handshake{Version: Version},
		Keypress{Key: 4, Press: true},
		KeyboardReport{Modifier: 2, Keys: []byte{4, 5}},
		Pointer{X: 100, Y: 200, Buttons: 1},
		Mouse{DX: -2, DY: 4, Buttons: 1},
		Wheel{Delta: -1},
		KeyboardLEDState{Mask: 3},
		KeysDownState{Modifier: 2, Keys: []byte{4, 0, 0}},
		KeypressKeepAlive{},
	}

	for _, tt := range tests {
		data, err := tt.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal %T: %v", tt, err)
		}
		got, err := Decode(data)
		if err != nil {
			t.Fatalf("decode %T: %v", tt, err)
		}
		if got.Type() != tt.Type() {
			t.Fatalf("message type mismatch for %T", tt)
		}
	}
}
