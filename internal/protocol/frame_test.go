package protocol

import (
	"bufio"
	"bytes"
	"testing"
)

func TestRoundTripAllMessages(t *testing.T) {
	cases := []struct {
		typ     byte
		payload []byte
	}{
		{MsgHello, []byte(Version)},
		{MsgScreen, EncodeScreen(1920, 1080)},
		{MsgClientEdge, EncodeClientEdge(EdgeRight)},
		{MsgMouseMove, EncodeMouseMove(-12, 34)},
		{MsgMouseAbs, EncodeMouseAbs(1880, 540)},
		{MsgMouseButton, EncodeMouseButton(ButtonLeft, 1)},
		{MsgMouseWheel, EncodeMouseWheel(0, -3)},
		{MsgKey, EncodeKey(30, 1)}, // KEY_A
		{MsgClipboard, []byte("你好, world 中文 ✓")},
		{MsgSwitch, EncodeSwitch(Switch{Direction: SwitchBack})},
	}
	var buf bytes.Buffer
	for _, c := range cases {
		if err := WriteFrame(&buf, c.typ, c.payload); err != nil {
			t.Fatalf("write %d: %v", c.typ, err)
		}
	}
	r := bufio.NewReader(&buf)
	for _, c := range cases {
		f, err := ReadFrame(r)
		if err != nil {
			t.Fatalf("read %d: %v", c.typ, err)
		}
		if f.Type != c.typ {
			t.Fatalf("type mismatch: got %d want %d", f.Type, c.typ)
		}
		if !bytes.Equal(f.Payload, c.payload) {
			t.Fatalf("payload mismatch for type %d: got %v want %v", c.typ, f.Payload, c.payload)
		}
	}
	if _, err := ReadFrame(r); err == nil {
		t.Fatal("expected EOF after all frames")
	}
}

func TestClientEdgeRoundTripAndValidation(t *testing.T) {
	for _, want := range []Edge{EdgeLeft, EdgeRight} {
		got, err := DecodeClientEdge(EncodeClientEdge(want))
		if err != nil {
			t.Fatalf("DecodeClientEdge(%d): %v", want, err)
		}
		if got != want {
			t.Fatalf("client edge = %d, want %d", got, want)
		}
	}

	for _, payload := range [][]byte{nil, {0}, {3}, {1, 2}} {
		if _, err := DecodeClientEdge(payload); err == nil {
			t.Fatalf("DecodeClientEdge(%v) accepted malformed payload", payload)
		}
	}
}

func TestDecodeHelpers(t *testing.T) {
	w, h, err := DecodeScreen(EncodeScreen(3840, 2160))
	if err != nil || w != 3840 || h != 2160 {
		t.Fatalf("screen decode: %d x %d err=%v", w, h, err)
	}
	dx, dy, err := DecodeMouseMove(EncodeMouseMove(-32768, 32767))
	if err != nil || dx != -32768 || dy != 32767 {
		t.Fatalf("mousemove decode: %d,%d err=%v", dx, dy, err)
	}
	x, y, err := DecodeMouseAbs(EncodeMouseAbs(1, -1))
	if err != nil || x != 1 || y != -1 {
		t.Fatalf("mouseabs decode: %d,%d err=%v", x, y, err)
	}
	b, p, err := DecodeMouseButton(EncodeMouseButton(ButtonRight, 0))
	if err != nil || b != 2 || p != 0 {
		t.Fatalf("mousebutton decode: %d,%d err=%v", b, p, err)
	}
	code, pressed, err := DecodeKey(EncodeKey(125, 1))
	if err != nil || code != 125 || pressed != 1 {
		t.Fatalf("key decode: %d,%d err=%v", code, pressed, err)
	}
}

func TestFrameTooLargeRejected(t *testing.T) {
	var buf bytes.Buffer
	hdr := []byte{0x7f, 0xff, 0xff, 0xff, MsgClipboard}
	buf.Write(hdr)
	buf.Write(make([]byte, 1024))
	r := bufio.NewReader(&buf)
	if _, err := ReadFrame(r); err != ErrFrameTooLarge {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

func TestTruncatedFrame(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("abc")
	hdr := []byte{0, 0, 0, byte(len(payload)), MsgClipboard}
	buf.Write(hdr)
	buf.Write(payload[:2]) // cut short
	r := bufio.NewReader(&buf)
	if _, err := ReadFrame(r); err == nil {
		t.Fatal("expected error on truncated frame")
	}
}

func TestSwitchRoundTripAndValidation(t *testing.T) {
	want := Switch{Direction: SwitchRemote, Y: 540, HasY: true}
	got, err := DecodeSwitch(EncodeSwitch(want))
	if err != nil {
		t.Fatalf("DecodeSwitch: %v", err)
	}
	if got != want {
		t.Fatalf("switch = %+v, want %+v", got, want)
	}

	if _, err := DecodeSwitch([]byte("remote:garbage")); err == nil {
		t.Fatal("DecodeSwitch accepted a malformed payload")
	}
}
