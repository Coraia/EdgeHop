//go:build linux

package client

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// Pure-Go /dev/uinput driver. Lets us inject keyboard/mouse events from a
// Wayland session without cgo, ydotool or a daemon. Requires write access to
// /dev/uinput (add user to the "input" group + a udev rule).

// evdev event types.
const (
	evSyn  = 0x00
	evKey  = 0x01
	evRel  = 0x02
	evMsc  = 0x04
)

// Event codes.
const (
	relX     = 0x00
	relY     = 0x01
	relHwheel = 0x06
	relWheel = 0x08
	synReport = 0
)

// Mouse buttons (evdev).
const (
	btnLeft   = 0x110
	btnRight  = 0x111
	btnMiddle = 0x112
	keyMax    = 0x2ff
)

// uinput ioctl numbers.
const (
	uiSetEvbit   = 100
	uiSetKeybit  = 101
	uiSetRelbit  = 102
	uiDevSetup   = 103
	uiDevCreate  = 1
	uiDevDestroy = 2
)

const (
	iocNRSHIFT   = 0
	iocTYPESHIFT = 8
	iocSIZESHIFT = 16
	iocDIRSHIFT  = 30
	iocWrite     = 1
	ioctlTypeU   = 'U'
)

func ioc(dir, typ, nr, size uintptr) uintptr {
	return (dir << iocDIRSHIFT) | (size << iocSIZESHIFT) | (typ << iocTYPESHIFT) | (nr << iocNRSHIFT)
}

// inputID matches struct input_id (8 bytes).
type inputID struct {
	Bustype uint16
	Vendor  uint16
	Product uint16
	Version uint16
}

// uinputSetup matches struct uinput_setup (96 bytes on 64-bit).
type uinputSetup struct {
	ID           inputID
	FfEffectsMax uint64
	Name         [80]byte
}

// inputEvent matches struct input_event (24 bytes on 64-bit Linux).
type inputEvent struct {
	Sec   int64
	Usec  int64
	Type  uint16
	Code  uint16
	Value int32
}

// VirtualDevice is a combined virtual keyboard + mouse on /dev/uinput.
type VirtualDevice struct {
	f *os.File
}

// OpenVirtualDevice creates and configures the uinput device.
func OpenVirtualDevice(name string) (*VirtualDevice, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	d := &VirtualDevice{f: f}

	setBit := func(req uintptr, bit int) error {
		_, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), req, uintptr(bit))
		if e != 0 {
			return e
		}
		return nil
	}

	for _, ev := range []int{evKey, evRel, evSyn} {
		if err := setBit(ioc(iocWrite, ioctlTypeU, uiSetEvbit, unsafe.Sizeof(int(0))), ev); err != nil {
			d.Close()
			return nil, err
		}
	}
	// Enable every key code so any code from the server is accepted.
	for k := 0; k <= keyMax; k++ {
		if err := setBit(ioc(iocWrite, ioctlTypeU, uiSetKeybit, unsafe.Sizeof(int(0))), k); err != nil {
			d.Close()
			return nil, err
		}
	}
	for _, rel := range []int{relX, relY, relWheel, relHwheel} {
		if err := setBit(ioc(iocWrite, ioctlTypeU, uiSetRelbit, unsafe.Sizeof(int(0))), rel); err != nil {
			d.Close()
			return nil, err
		}
	}

	setup := uinputSetup{
		ID: inputID{
			Bustype: 0x03, // BUS_USB
			Vendor:  0x5555,
			Product: 0x0001,
			Version: 1,
		},
	}
	copy(setup.Name[:], name)

	sz := unsafe.Sizeof(setup)
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(),
		ioc(iocWrite, ioctlTypeU, uiDevSetup, sz), uintptr(unsafe.Pointer(&setup))); e != 0 {
		d.Close()
		return nil, e
	}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(),
		ioc(0, ioctlTypeU, uiDevCreate, 0), 0); e != 0 {
		d.Close()
		return nil, e
	}
	return d, nil
}

// Close destroys the device.
func (d *VirtualDevice) Close() error {
	if d.f == nil {
		return nil
	}
	syscall.Syscall(syscall.SYS_IOCTL, d.f.Fd(), ioc(0, ioctlTypeU, uiDevDestroy, 0), 0)
	err := d.f.Close()
	d.f = nil
	return err
}

// writeEvent emits a single input event.
func (d *VirtualDevice) writeEvent(typ, code uint16, value int32) error {
	if d.f == nil {
		return errors.New("uinput: device closed")
	}
	ev := inputEvent{Type: typ, Code: code, Value: value}
	_, err := d.f.Write((*(*[unsafe.Sizeof(inputEvent{})]byte)(unsafe.Pointer(&ev)))[:])
	return err
}

func (d *VirtualDevice) sync() error {
	return d.writeEvent(evSyn, synReport, 0)
}

// Key sends a key press (value=1) or release (value=0).
func (d *VirtualDevice) Key(code uint16, pressed bool) error {
	v := int32(0)
	if pressed {
		v = 1
	}
	if err := d.writeEvent(evKey, code, v); err != nil {
		return err
	}
	return d.sync()
}

// MouseMoveRel moves the cursor by (dx, dy).
func (d *VirtualDevice) MouseMoveRel(dx, dy int16) error {
	if dx != 0 {
		if err := d.writeEvent(evRel, relX, int32(dx)); err != nil {
			return err
		}
	}
	if dy != 0 {
		if err := d.writeEvent(evRel, relY, int32(dy)); err != nil {
			return err
		}
	}
	return d.sync()
}

// MouseButton presses (pressed=true) or releases a button.
func (d *VirtualDevice) MouseButton(btn uint8, pressed bool) error {
	var code uint16
	switch btn {
	case 1:
		code = btnLeft
	case 2:
		code = btnRight
	case 3:
		code = btnMiddle
	}
	return d.Key(code, pressed)
}

// MouseWheel scrolls by (dx, dy) wheel clicks.
func (d *VirtualDevice) MouseWheel(dx, dy int16) error {
	if dx != 0 {
		if err := d.writeEvent(evRel, relHwheel, int32(dx)); err != nil {
			return err
		}
	}
	if dy != 0 {
		if err := d.writeEvent(evRel, relWheel, int32(dy)); err != nil {
			return err
		}
	}
	return d.sync()
}
