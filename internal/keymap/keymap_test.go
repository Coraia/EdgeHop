package keymap

import "testing"

func TestCommonKeys(t *testing.T) {
	cases := []struct {
		mac  int
		ev   uint16
		name string
	}{
		{0x00, 30, "A"},
		{0x12, 2, "1"},
		{0x24, 28, "Return"},
		{0x31, 57, "Space"},
		{0x35, 1, "Escape"},
		{0x33, 14, "Backspace"},
		{0x37, 125, "Command"},
		{0x38, 42, "Shift"},
		{0x3B, 29, "Control"},
		{0x7E, 103, "Up"},
		{0x7B, 105, "Left"},
		{0x7A, 59, "F1"},
	}
	for _, c := range cases {
		got, ok := ToEvdev(c.mac)
		if !ok || got != c.ev {
			t.Errorf("%s: mac 0x%02X -> got %d (ok=%v), want %d", c.name, c.mac, got, ok, c.ev)
		}
	}
}

func TestFnKeyIsDropped(t *testing.T) {
	if _, ok := ToEvdev(0x3F); ok {
		t.Fatal("Fn key should be dropped")
	}
}

func TestUnknownKeyDropped(t *testing.T) {
	if _, ok := ToEvdev(0xFE); ok {
		t.Fatal("unknown keycode should be dropped")
	}
}

func TestRightCommandAndExtendedFunctionKeys(t *testing.T) {
	cases := []struct {
		mac  int
		ev   uint16
		name string
	}{
		{0x36, 126, "RightCommand"},
		{0x71, 185, "F15"},
		{0x6A, 186, "F16"},
		{0x40, 187, "F17"},
		{0x4F, 188, "F18"},
		{0x50, 189, "F19"},
		{0x5A, 190, "F20"},
	}
	for _, c := range cases {
		got, ok := ToEvdev(c.mac)
		if !ok || got != c.ev {
			t.Errorf("%s: got %d (ok=%v), want %d", c.name, got, ok, c.ev)
		}
	}
}
