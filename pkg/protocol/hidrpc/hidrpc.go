package hidrpc

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	Version byte = 0x01

	TypeHandshake         byte = 0x01
	TypeKeyboardReport    byte = 0x02
	TypePointerReport     byte = 0x03
	TypeWheelReport       byte = 0x04
	TypeKeypressReport    byte = 0x05
	TypeMouseReport       byte = 0x06
	TypeKeyboardLEDState  byte = 0x32
	TypeKeysDownState     byte = 0x33
	TypeKeypressKeepAlive byte = 0x09
)

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
	X       uint16
	Y       uint16
	Buttons byte
}

func (m Pointer) Type() byte { return TypePointerReport }
func (m Pointer) MarshalBinary() ([]byte, error) {
	out := make([]byte, 6)
	out[0] = TypePointerReport
	binary.BigEndian.PutUint16(out[1:3], m.X)
	binary.BigEndian.PutUint16(out[3:5], m.Y)
	out[5] = m.Buttons
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
		if len(data) != 6 {
			return nil, fmt.Errorf("pointer length %d", len(data))
		}
		return Pointer{
			X:       binary.BigEndian.Uint16(data[1:3]),
			Y:       binary.BigEndian.Uint16(data[3:5]),
			Buttons: data[5],
		}, nil
	case TypeMouseReport:
		if len(data) != 4 {
			return nil, fmt.Errorf("mouse length %d", len(data))
		}
		return Mouse{DX: int8(data[1]), DY: int8(data[2]), Buttons: data[3]}, nil
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
	case TypeKeypressKeepAlive:
		if len(data) != 1 {
			return nil, fmt.Errorf("keypress keepalive length %d", len(data))
		}
		return KeypressKeepAlive{}, nil
	default:
		return nil, ErrUnsupportedMessage
	}
}
