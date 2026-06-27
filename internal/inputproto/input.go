package inputproto

import (
	"encoding/binary"
	"fmt"
)

// Wire format for remote input (viewer -> agent over DataChannel).
// All coordinates are normalized u16: 0 = min, 65535 = max.

const (
	MsgMouseMove byte = 0x01
	MsgMouseDown byte = 0x02
	MsgMouseUp   byte = 0x03
	MsgKeyDown   byte = 0x04
	MsgKeyUp     byte = 0x05
	MsgWheel     byte = 0x06
)

const MouseLeft = 0
const MouseRight = 1
const MouseMiddle = 2

func EncodeMouseMove(x, y uint16) []byte {
	b := make([]byte, 5)
	b[0] = MsgMouseMove
	binary.LittleEndian.PutUint16(b[1:], x)
	binary.LittleEndian.PutUint16(b[3:], y)
	return b
}

func EncodeMouseButton(btn byte, down bool, x, y uint16) []byte {
	b := make([]byte, 6)
	if down {
		b[0] = MsgMouseDown
	} else {
		b[0] = MsgMouseUp
	}
	b[1] = btn
	binary.LittleEndian.PutUint16(b[2:], x)
	binary.LittleEndian.PutUint16(b[4:], y)
	return b
}

func EncodeKey(down bool, vk uint16) []byte {
	b := make([]byte, 3)
	if down {
		b[0] = MsgKeyDown
	} else {
		b[0] = MsgKeyUp
	}
	binary.LittleEndian.PutUint16(b[1:], vk)
	return b
}

func EncodeWheel(delta int16, x, y uint16) []byte {
	b := make([]byte, 7)
	b[0] = MsgWheel
	binary.LittleEndian.PutUint16(b[1:], x)
	binary.LittleEndian.PutUint16(b[3:], y)
	binary.LittleEndian.PutUint16(b[5:], uint16(delta))
	return b
}

type Event struct {
	Kind   byte
	Button byte
	X      uint16
	Y      uint16
	VK     uint16
	Delta  int16
}

func Decode(data []byte) (Event, error) {
	if len(data) < 1 {
		return Event{}, fmt.Errorf("empty input message")
	}
	ev := Event{Kind: data[0]}
	switch ev.Kind {
	case MsgMouseMove:
		if len(data) < 5 {
			return Event{}, fmt.Errorf("mouse move too short")
		}
		ev.X = binary.LittleEndian.Uint16(data[1:3])
		ev.Y = binary.LittleEndian.Uint16(data[3:5])
	case MsgMouseDown, MsgMouseUp:
		if len(data) < 6 {
			return Event{}, fmt.Errorf("mouse button too short")
		}
		ev.Button = data[1]
		ev.X = binary.LittleEndian.Uint16(data[2:4])
		ev.Y = binary.LittleEndian.Uint16(data[4:6])
	case MsgKeyDown, MsgKeyUp:
		if len(data) < 3 {
			return Event{}, fmt.Errorf("key event too short")
		}
		ev.VK = binary.LittleEndian.Uint16(data[1:3])
	case MsgWheel:
		if len(data) < 7 {
			return Event{}, fmt.Errorf("wheel too short")
		}
		ev.X = binary.LittleEndian.Uint16(data[1:3])
		ev.Y = binary.LittleEndian.Uint16(data[3:5])
		ev.Delta = int16(binary.LittleEndian.Uint16(data[5:7]))
	default:
		return Event{}, fmt.Errorf("unknown input kind: %d", ev.Kind)
	}
	return ev, nil
}
