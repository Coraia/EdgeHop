// Package keymap maps macOS virtual keycodes (Carbon kVK_*) to Linux evdev
// key codes (KEY_*), which are what /dev/uinput consumes.
//
// References:
//   - macOS keycodes: Carbon HIToolbox Events.h (kVK_*)
//   - evdev codes:    linux/input-event-codes.h (KEY_*)
package keymap

// macToEvdev maps Apple virtual keycode -> Linux evdev keycode.
// A value of 0 means "unknown, drop".
var macToEvdev = map[int]uint16{
	// Letters
	0x00: 30, // A
	0x01: 31, // S
	0x02: 32, // D
	0x03: 33, // F
	0x04: 35, // H
	0x05: 34, // G
	0x06: 44, // Z
	0x07: 45, // X
	0x08: 46, // C
	0x09: 47, // V
	0x0B: 48, // B
	0x0C: 16, // Q
	0x0D: 17, // W
	0x0E: 18, // E
	0x0F: 19, // R
	0x10: 21, // Y
	0x11: 20, // T
	0x1F: 24, // O
	0x20: 22, // U
	0x22: 23, // I
	0x23: 25, // P
	0x25: 38, // L
	0x26: 36, // J
	0x28: 37, // K
	0x2D: 49, // N
	0x2E: 50, // M

	// Digits (top row)
	0x12: 2,  // 1
	0x13: 3,  // 2
	0x14: 4,  // 3
	0x15: 5,  // 4
	0x17: 6,  // 5
	0x16: 7,  // 6
	0x1A: 8,  // 7
	0x1C: 9,  // 8
	0x19: 10, // 9
	0x1D: 11, // 0

	// Punctuation
	0x1B: 12, // -
	0x18: 13, // =
	0x21: 26, // [
	0x1E: 27, // ]
	0x2A: 43, // \
	0x29: 39, // ;
	0x27: 40, // '
	0x32: 41, // `
	0x2B: 51, // ,
	0x2F: 52, // .
	0x2C: 53, // /

	// Editing / navigation
	0x24: 28,  // Return -> ENTER
	0x30: 15,  // Tab
	0x31: 57,  // Space
	0x33: 14,  // Delete (backspace)
	0x35: 1,   // Escape
	0x73: 102, // Home
	0x77: 107, // End
	0x74: 104, // PageUp
	0x79: 109, // PageDown
	0x75: 111, // ForwardDelete -> DELETE
	0x7B: 105, // Left
	0x7C: 106, // Right
	0x7D: 108, // Down
	0x7E: 103, // Up

	// Modifiers
	0x38: 42,  // Shift        -> LEFTSHIFT
	0x3C: 54,  // RightShift   -> RIGHTSHIFT
	0x3B: 29,  // Control      -> LEFTCTRL
	0x3E: 97,  // RightControl -> RIGHTCTRL
	0x3A: 56,  // Option       -> LEFTALT
	0x3D: 100, // RightOption  -> RIGHTALT
	0x37: 125, // Command      -> LEFTMETA
	0x36: 126, // RightCommand -> RIGHTMETA
	0x39: 58,  // CapsLock
	0x3F: 0,   // Fn (no direct evdev equivalent; dropped)

	// Function keys
	0x7A: 59,  // F1
	0x78: 60,  // F2
	0x63: 61,  // F3
	0x76: 62,  // F4
	0x60: 63,  // F5
	0x61: 64,  // F6
	0x62: 65,  // F7
	0x64: 66,  // F8
	0x65: 67,  // F9
	0x6D: 68,  // F10
	0x67: 87,  // F11
	0x6F: 88,  // F12
	0x69: 183, // F13 -> PRINTSCREEN
	0x6B: 184, // F14 -> SCROLLLOCK
	0x71: 185, // F15
	0x6A: 186, // F16
	0x40: 187, // F17
	0x4F: 188, // F18
	0x50: 189, // F19
	0x5A: 190, // F20

	// Keypad
	0x41: 83,  // KeypadDecimal  -> KPDOT
	0x43: 55,  // KeypadMultiply -> KPASTERISK
	0x45: 78,  // KeypadPlus     -> KPPLUS
	0x47: 69,  // KeypadClear    -> NUMLOCK
	0x4B: 98,  // KeypadDivide   -> KPSLASH
	0x4C: 96,  // KeypadEnter    -> KPENTER
	0x4E: 74,  // KeypadMinus    -> KPMINUS
	0x51: 117, // KeypadEquals   -> KPEQUAL
	0x52: 82,  // Keypad0        -> KP0
	0x53: 79,  // Keypad1        -> KP1
	0x54: 80,  // Keypad2        -> KP2
	0x55: 81,  // Keypad3        -> KP3
	0x56: 75,  // Keypad4        -> KP4
	0x57: 76,  // Keypad5        -> KP5
	0x58: 77,  // Keypad6        -> KP6
	0x59: 71,  // Keypad7        -> KP7
	0x5B: 72,  // Keypad8        -> KP8
	0x5C: 73,  // Keypad9        -> KP9

	// Media
	0x48: 115, // VolumeUp
	0x49: 114, // VolumeDown
	0x4A: 113, // Mute
}

// ToEvdev maps a macOS virtual keycode to a Linux evdev key code.
// ok is false when the key has no sensible Linux counterpart.
func ToEvdev(macKeycode int) (uint16, bool) {
	code, ok := macToEvdev[macKeycode]
	return code, ok && code != 0
}
