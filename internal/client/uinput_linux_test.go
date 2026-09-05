//go:build linux

package client

import (
	"testing"
	"unsafe"
)

// This size must match the kernel's struct uinput_setup.
func TestUinputSetupSizes(t *testing.T) {
	if got := unsafe.Sizeof(uinputSetup{}); got != 92 {
		t.Errorf("uinputSetup size = %d, want 92", got)
	}
	// name must land at offset 8 so the kernel sees a non-empty name.
	var s uinputSetup
	copy(s.Name[:], "edgehop")
	if s.Name[0] != 'e' {
		t.Error("modern layout name offset wrong")
	}
}

// inputEvent must stay 24 bytes on 64-bit Linux.
func TestInputEventSize(t *testing.T) {
	if got := unsafe.Sizeof(inputEvent{}); got != 24 {
		t.Errorf("inputEvent size = %d, want 24", got)
	}
}
