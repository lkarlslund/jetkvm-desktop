package hidrpc

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	Version byte = 0x01

	TypeHandshake           byte = 0x01
	TypeKeyboardReport      byte = 0x02
	TypePointerReport       byte = 0x03
	TypeWheelReport         byte = 0x04
	TypeKeypressReport      byte = 0x05
	TypeMouseReport         byte = 0x06
	TypeKeyboardMacro       byte = 0x07
	TypeCancelKeyboardMacro byte = 0x08
	TypeKeyboardLEDState    byte = 0x32
	TypeKeysDownState       byte = 0x33
	TypeKeyboardMacroState  byte = 0x34
	TypeKeypressKeepAlive   byte = 0x09
)

const HidKeyBufferSize = 6

var ErrUnsupportedMessage = errors.New("unsupported HID-RPC message")

type Message interface {
	Type() byte
	MarshalBinary() ([]byte, error)
}

type Handshake struct {
	Version byte
}

func (m Handshake) Type() byte { return TypeHandshake }
func (m Handshake) MarshalBinary() ([]byte, error) {
	return []byte{TypeHandshake, m.Version}, nil
}

type Keypress struct {
	Key   byte
	Press bool
}

func (m Keypress) Type() byte { return TypeKeypressReport }
func (m Keypress) MarshalBinary() ([]byte, error) {
	pressed := byte(0)
	if m.Press {
		pressed = 1
	}
	return []byte{TypeKeypressReport, m.Key, pressed}, nil
}

type KeyboardReport struct {
	Modifier byte
	Keys     []byte
}

func (m KeyboardReport) Type() byte { return TypeKeyboardReport }
func (m KeyboardReport) MarshalBinary() ([]byte, error) {
	out := make([]byte, 0, 2+len(m.Keys))
	out = append(out, TypeKeyboardReport, m.Modifier)
	out = append(out, m.Keys...)
	return out, nil
}

type Pointer struct {
	X       int32
	Y       int32
	Buttons byte
}

func (m Pointer) Type() byte { return TypePointerReport }
func (m Pointer) MarshalBinary() ([]byte, error) {
	out := make([]byte, 10)
	out[0] = TypePointerReport
	binary.BigEndian.PutUint32(out[1:5], uint32(m.X))
	binary.BigEndian.PutUint32(out[5:9], uint32(m.Y))
	out[9] = m.Buttons
	return out, nil
}

type Mouse struct {
	DX      int8
	DY      int8
	Buttons byte
}

func (m Mouse) Type() byte { return TypeMouseReport }
func (m Mouse) MarshalBinary() ([]byte, error) {
	return []byte{TypeMouseReport, byte(m.DX), byte(m.DY), m.Buttons}, nil
}

type Wheel struct {
	Delta int8
}

func (m Wheel) Type() byte { return TypeWheelReport }
func (m Wheel) MarshalBinary() ([]byte, error) {
	return []byte{TypeWheelReport, byte(m.Delta)}, nil
}

type KeyboardMacroStep struct {
	Modifier byte
	Keys     [HidKeyBufferSize]byte
	Delay    uint16
}

type KeyboardMacroReport struct {
	IsPaste bool
	Steps   []KeyboardMacroStep
}

func (m KeyboardMacroReport) Type() byte { return TypeKeyboardMacro }
func (m KeyboardMacroReport) MarshalBinary() ([]byte, error) {
	out := make([]byte, 0, 6+len(m.Steps)*(1+HidKeyBufferSize+2))
	out = append(out, TypeKeyboardMacro)
	if m.IsPaste {
		out = append(out, 1)
	} else {
		out = append(out, 0)
	}
	out = binary.BigEndian.AppendUint32(out, uint32(len(m.Steps)))
	for _, step := range m.Steps {
		out = append(out, step.Modifier)
		out = append(out, step.Keys[:]...)
		out = binary.BigEndian.AppendUint16(out, step.Delay)
	}
	return out, nil
}

type CancelKeyboardMacro struct{}

func (m CancelKeyboardMacro) Type() byte { return TypeCancelKeyboardMacro }
func (m CancelKeyboardMacro) MarshalBinary() ([]byte, error) {
	return []byte{TypeCancelKeyboardMacro}, nil
}

type KeypressKeepAlive struct{}

func (m KeypressKeepAlive) Type() byte { return TypeKeypressKeepAlive }
func (m KeypressKeepAlive) MarshalBinary() ([]byte, error) {
	return []byte{TypeKeypressKeepAlive}, nil
}

type KeyboardLEDState struct {
	Mask byte
}

func (m KeyboardLEDState) Type() byte { return TypeKeyboardLEDState }
func (m KeyboardLEDState) MarshalBinary() ([]byte, error) {
	return []byte{TypeKeyboardLEDState, m.Mask}, nil
}

type KeysDownState struct {
	Modifier byte
	Keys     []byte
}

func (m KeysDownState) Type() byte { return TypeKeysDownState }
func (m KeysDownState) MarshalBinary() ([]byte, error) {
	out := make([]byte, 0, 2+len(m.Keys))
	out = append(out, TypeKeysDownState, m.Modifier)
	out = append(out, m.Keys...)
	return out, nil
}

type KeyboardMacroState struct {
	State   bool
	IsPaste bool
}

func (m KeyboardMacroState) Type() byte { return TypeKeyboardMacroState }
func (m KeyboardMacroState) MarshalBinary() ([]byte, error) {
	state := byte(0)
	if m.State {
		state = 1
	}
	isPaste := byte(0)
	if m.IsPaste {
		isPaste = 1
	}
	return []byte{TypeKeyboardMacroState, state, isPaste}, nil
}

func Decode(data []byte) (Message, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty HID-RPC message")
	}

	switch data[0] {
	case TypeHandshake:
		if len(data) != 2 {
			return nil, fmt.Errorf("handshake length %d", len(data))
		}
		return Handshake{Version: data[1]}, nil
	case TypeKeypressReport:
		if len(data) != 3 {
			return nil, fmt.Errorf("keypress length %d", len(data))
		}
		return Keypress{Key: data[1], Press: data[2] == 1}, nil
	case TypeKeyboardReport:
		if len(data) < 2 {
			return nil, fmt.Errorf("keyboard report length %d", len(data))
		}
		return KeyboardReport{Modifier: data[1], Keys: append([]byte(nil), data[2:]...)}, nil
	case TypePointerReport:
		if len(data) != 10 {
			return nil, fmt.Errorf("pointer length %d", len(data))
		}
		return Pointer{
			X:       int32(binary.BigEndian.Uint32(data[1:5])),
			Y:       int32(binary.BigEndian.Uint32(data[5:9])),
			Buttons: data[9],
		}, nil
	case TypeMouseReport:
		if len(data) != 4 {
			return nil, fmt.Errorf("mouse length %d", len(data))
		}
		return Mouse{DX: int8(data[1]), DY: int8(data[2]), Buttons: data[3]}, nil
	case TypeKeyboardMacro:
		if len(data) < 6 {
			return nil, fmt.Errorf("keyboard macro length %d", len(data))
		}
		stepCount := int(binary.BigEndian.Uint32(data[2:6]))
		expected := 6 + stepCount*(1+HidKeyBufferSize+2)
		if len(data) != expected {
			return nil, fmt.Errorf("keyboard macro length %d expected %d", len(data), expected)
		}
		steps := make([]KeyboardMacroStep, 0, stepCount)
		offset := 6
		for i := 0; i < stepCount; i++ {
			var keys [HidKeyBufferSize]byte
			copy(keys[:], data[offset+1:offset+1+HidKeyBufferSize])
			steps = append(steps, KeyboardMacroStep{
				Modifier: data[offset],
				Keys:     keys,
				Delay:    binary.BigEndian.Uint16(data[offset+1+HidKeyBufferSize : offset+1+HidKeyBufferSize+2]),
			})
			offset += 1 + HidKeyBufferSize + 2
		}
		return KeyboardMacroReport{IsPaste: data[1] == 1, Steps: steps}, nil
	case TypeCancelKeyboardMacro:
		if len(data) != 1 {
			return nil, fmt.Errorf("cancel keyboard macro length %d", len(data))
		}
		return CancelKeyboardMacro{}, nil
	case TypeWheelReport:
		if len(data) != 2 {
			return nil, fmt.Errorf("wheel length %d", len(data))
		}
		return Wheel{Delta: int8(data[1])}, nil
	case TypeKeyboardLEDState:
		if len(data) != 2 {
			return nil, fmt.Errorf("keyboard LED length %d", len(data))
		}
		return KeyboardLEDState{Mask: data[1]}, nil
	case TypeKeysDownState:
		if len(data) < 2 {
			return nil, fmt.Errorf("keys down length %d", len(data))
		}
		return KeysDownState{Modifier: data[1], Keys: append([]byte(nil), data[2:]...)}, nil
	case TypeKeyboardMacroState:
		if len(data) != 3 {
			return nil, fmt.Errorf("keyboard macro state length %d", len(data))
		}
		return KeyboardMacroState{State: data[1] == 1, IsPaste: data[2] == 1}, nil
	case TypeKeypressKeepAlive:
		if len(data) != 1 {
			return nil, fmt.Errorf("keypress keepalive length %d", len(data))
		}
		return KeypressKeepAlive{}, nil
	default:
		return nil, ErrUnsupportedMessage
	}
}
