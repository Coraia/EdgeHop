// Package protocol defines the wire protocol shared by uc-server (macOS)
// and uc-client (Linux). Messages are framed as:
//
//	[4-byte big-endian payload length][1-byte type][payload]
//
// All integers are network byte order (big-endian).
package protocol

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MaxFrameSize guards against unbounded memory usage from malicious/broken peers.
const MaxFrameSize = 16 << 20 // 16 MiB

// ErrFrameTooLarge is returned when a frame exceeds MaxFrameSize.
var ErrFrameTooLarge = errors.New("protocol: frame too large")

// Message types.
const (
	// MsgHello: client -> server handshake ("universal-control/1").
	MsgHello byte = 0x01
	// MsgScreen: server -> client screen geometry (w:int32, h:int32).
	MsgScreen byte = 0x02
	// MsgMouseMove: server -> client relative move (dx:int16, dy:int16).
	MsgMouseMove byte = 0x10
	// MsgMouseAbs: server -> client absolute position (x:int32, y:int32).
	MsgMouseAbs byte = 0x11
	// MsgMouseButton: server -> client (button:uint8, pressed:uint8).
	MsgMouseButton byte = 0x12
	// MsgMouseWheel: server -> client scroll (dx:int16, dy:int16).
	MsgMouseWheel byte = 0x13
	// MsgKey: server -> client (evdevKeycode:uint16, pressed:uint8).
	MsgKey byte = 0x20
	// MsgClipboard: either direction; payload is UTF-8 text.
	MsgClipboard byte = 0x30
	// MsgSwitch: control-mode switch request. Client -> server: "back".
	MsgSwitch byte = 0x40
)

// Mouse button constants used in MsgMouseButton.
const (
	ButtonLeft   uint8 = 1
	ButtonRight  uint8 = 2
	ButtonMiddle uint8 = 3
)

// Frame is a single decoded message.
type Frame struct {
	Type    byte
	Payload []byte
}

// WriteFrame encodes and writes one frame to w.
func WriteFrame(w io.Writer, typ byte, payload []byte) error {
	if len(payload) > MaxFrameSize {
		return ErrFrameTooLarge
	}
	hdr := make([]byte, 5)
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	hdr[4] = typ
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// ReadFrame reads one frame from r. A bufio.Reader should be used to avoid
// per-message syscalls on the hot path.
func ReadFrame(r *bufio.Reader) (Frame, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	ln := binary.BigEndian.Uint32(hdr[0:4])
	if ln > MaxFrameSize {
		return Frame{}, ErrFrameTooLarge
	}
	f := Frame{Type: hdr[4]}
	if ln > 0 {
		f.Payload = make([]byte, ln)
		if _, err := io.ReadFull(r, f.Payload); err != nil {
			return Frame{}, err
		}
	}
	return f, nil
}

// --- Payload builders/parsers ---------------------------------------------

// EncodeScreen builds a MsgScreen payload.
func EncodeScreen(w, h int32) []byte {
	p := make([]byte, 8)
	binary.BigEndian.PutUint32(p[0:4], uint32(w))
	binary.BigEndian.PutUint32(p[4:8], uint32(h))
	return p
}

// DecodeScreen parses a MsgScreen payload.
func DecodeScreen(p []byte) (w, h int32, err error) {
	if len(p) < 8 {
		return 0, 0, errors.New("protocol: short screen payload")
	}
	w = int32(binary.BigEndian.Uint32(p[0:4]))
	h = int32(binary.BigEndian.Uint32(p[4:8]))
	return w, h, nil
}

// EncodeMouseMove builds a MsgMouseMove payload.
func EncodeMouseMove(dx, dy int16) []byte {
	p := make([]byte, 4)
	binary.BigEndian.PutUint16(p[0:2], uint16(dx))
	binary.BigEndian.PutUint16(p[2:4], uint16(dy))
	return p
}

// DecodeMouseMove parses a MsgMouseMove payload.
func DecodeMouseMove(p []byte) (dx, dy int16, err error) {
	if len(p) < 4 {
		return 0, 0, errors.New("protocol: short mouse move payload")
	}
	dx = int16(binary.BigEndian.Uint16(p[0:2]))
	dy = int16(binary.BigEndian.Uint16(p[2:4]))
	return dx, dy, nil
}

// EncodeMouseAbs builds a MsgMouseAbs payload.
func EncodeMouseAbs(x, y int32) []byte {
	p := make([]byte, 8)
	binary.BigEndian.PutUint32(p[0:4], uint32(x))
	binary.BigEndian.PutUint32(p[4:8], uint32(y))
	return p
}

// DecodeMouseAbs parses a MsgMouseAbs payload.
func DecodeMouseAbs(p []byte) (x, y int32, err error) {
	if len(p) < 8 {
		return 0, 0, errors.New("protocol: short mouse abs payload")
	}
	x = int32(binary.BigEndian.Uint32(p[0:4]))
	y = int32(binary.BigEndian.Uint32(p[4:8]))
	return x, y, nil
}

// EncodeMouseButton builds a MsgMouseButton payload.
func EncodeMouseButton(button, pressed uint8) []byte {
	return []byte{button, pressed}
}

// DecodeMouseButton parses a MsgMouseButton payload.
func DecodeMouseButton(p []byte) (button, pressed uint8, err error) {
	if len(p) < 2 {
		return 0, 0, errors.New("protocol: short mouse button payload")
	}
	return p[0], p[1], nil
}

// EncodeMouseWheel builds a MsgMouseWheel payload.
func EncodeMouseWheel(dx, dy int16) []byte {
	p := make([]byte, 4)
	binary.BigEndian.PutUint16(p[0:2], uint16(dx))
	binary.BigEndian.PutUint16(p[2:4], uint16(dy))
	return p
}

// DecodeMouseWheel parses a MsgMouseWheel payload.
func DecodeMouseWheel(p []byte) (dx, dy int16, err error) {
	if len(p) < 4 {
		return 0, 0, errors.New("protocol: short mouse wheel payload")
	}
	dx = int16(binary.BigEndian.Uint16(p[0:2]))
	dy = int16(binary.BigEndian.Uint16(p[2:4]))
	return dx, dy, nil
}

// EncodeKey builds a MsgKey payload.
func EncodeKey(evcode uint16, pressed uint8) []byte {
	p := make([]byte, 3)
	binary.BigEndian.PutUint16(p[0:2], evcode)
	p[2] = pressed
	return p
}

// DecodeKey parses a MsgKey payload.
func DecodeKey(p []byte) (evcode uint16, pressed uint8, err error) {
	if len(p) < 3 {
		return 0, 0, errors.New("protocol: short key payload")
	}
	evcode = binary.BigEndian.Uint16(p[0:2])
	return evcode, p[2], nil
}

// String returns a human-readable description of a frame type.
func (f Frame) String() string {
	switch f.Type {
	case MsgHello:
		return fmt.Sprintf("Hello(%q)", string(f.Payload))
	case MsgScreen:
		w, h, _ := DecodeScreen(f.Payload)
		return fmt.Sprintf("Screen(%dx%d)", w, h)
	case MsgMouseMove:
		dx, dy, _ := DecodeMouseMove(f.Payload)
		return fmt.Sprintf("MouseMove(%d,%d)", dx, dy)
	case MsgMouseAbs:
		x, y, _ := DecodeMouseAbs(f.Payload)
		return fmt.Sprintf("MouseAbs(%d,%d)", x, y)
	case MsgMouseButton:
		b, p, _ := DecodeMouseButton(f.Payload)
		return fmt.Sprintf("MouseButton(btn=%d pressed=%d)", b, p)
	case MsgMouseWheel:
		dx, dy, _ := DecodeMouseWheel(f.Payload)
		return fmt.Sprintf("MouseWheel(%d,%d)", dx, dy)
	case MsgKey:
		c, p, _ := DecodeKey(f.Payload)
		return fmt.Sprintf("Key(code=%d pressed=%d)", c, p)
	case MsgClipboard:
		return fmt.Sprintf("Clipboard(%d bytes)", len(f.Payload))
	case MsgSwitch:
		return fmt.Sprintf("Switch(%q)", string(f.Payload))
	default:
		return fmt.Sprintf("Unknown(type=%d,len=%d)", f.Type, len(f.Payload))
	}
}
