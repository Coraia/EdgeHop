//go:build darwin

package server

import "universal_control/internal/keymap"

// CGEventType values (CoreGraphics/CGEventTypes.h).
const (
	cgEventLeftMouseDown     = 1
	cgEventLeftMouseUp       = 2
	cgEventRightMouseDown    = 3
	cgEventRightMouseUp      = 4
	cgEventMouseMoved        = 5
	cgEventLeftMouseDragged  = 6
	cgEventRightMouseDragged = 7
	cgEventOtherMouseDragged = 8
	cgEventKeyDown           = 10
	cgEventKeyUp             = 11
	cgEventFlagsChanged      = 12
	cgEventScrollWheel       = 22
	cgEventOtherMouseDown    = 25
	cgEventOtherMouseUp      = 26
)

// CGEvent flags (bit positions).
const (
	cgFlagAlphaShift  uint64 = 1 << 16 // CapsLock
	cgFlagShift       uint64 = 1 << 17
	cgFlagControl     uint64 = 1 << 18
	cgFlagAlternate   uint64 = 1 << 19 // Option
	cgFlagCommand     uint64 = 1 << 20
	cgFlagSecondaryFn uint64 = 1 << 22 // Fn
)

// flagForMacKey maps a macOS virtual keycode (modifiers) to the CGEvent flag
// that indicates it is currently pressed.
func flagForMacKey(keycode int) uint64 {
	switch keycode {
	case 0x36, 0x37: // RightCommand / Command
		return cgFlagCommand
	case 0x38, 0x3C: // Shift / RightShift
		return cgFlagShift
	case 0x3A, 0x3D: // Option / RightOption
		return cgFlagAlternate
	case 0x3B, 0x3E: // Control / RightControl
		return cgFlagControl
	case 0x39: // CapsLock
		return cgFlagAlphaShift
	case 0x3F: // Fn
		return cgFlagSecondaryFn
	}
	return 0
}

// keymapToEvdev wraps the shared key mapping table.
func keymapToEvdev(mac int) (uint16, bool) {
	return keymap.ToEvdev(mac)
}
