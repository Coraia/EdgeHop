//go:build linux

package client

import (
	"testing"
	"unsafe"
)

// These sizes must match the kernel's struct uinput_setup across versions.
// The modern layout (>= 6.12) is 92 bytes; the pre-6.12 layout is 96 bytes.
func TestUinputSetupSizes(t *testing.T) {
	if got := unsafe.Sizeof(uinputSetup{}); got != 92 {
		t.Errorf("modern uinputSetup size = %d, want 92", got)
	}
	if got := unsafe.Sizeof(uinputSetupOld{}); got != 96 {
		t.Errorf("legacy uinputSetupOld size = %d, want 96", got)
	}
	// name must land at offset 8 (modern) so the kernel sees a non-empty name.
	var s uinputSetup
	copy(s.Name[:], "universal-control")
	if s.Name[0] != 'u' {
		t.Error("modern layout name offset wrong")
	}
}

// inputEvent must stay 24 bytes on 64-bit Linux.
func TestInputEventSize(t *testing.T) {
	if got := unsafe.Sizeof(inputEvent{}); got != 24 {
		t.Errorf("inputEvent size = %d, want 24", got)
	}
}
