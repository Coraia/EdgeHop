//go:build darwin

package server

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation

#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdint.h>

// onEventGo is implemented in Go (see below). It returns 0 to consume the
// event (suppress it from reaching apps) or 1 to let it pass.
extern int onEventGo(int type, int64_t keycode, int64_t flags,
                     double dx, double dy, int64_t button,
                     int64_t scrollAxis1, int64_t scrollAxis2);

static CFMachPortRef g_tap = NULL;

static CGEventRef tapCallback(CGEventTapProxy proxy, CGEventType type,
                              CGEventRef event, void *refcon) {
    (void)proxy; (void)refcon;
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        // Re-enable the tap after a timeout; keep control flowing.
        if (g_tap != NULL) {
            CGEventTapEnable(g_tap, true);
        }
        return event;
    }
    int64_t keycode = 0, flags = 0, button = -1, s1 = 0, s2 = 0;
    double dx = 0, dy = 0;
    switch (type) {
        case kCGEventKeyDown:
        case kCGEventKeyUp:
        case kCGEventFlagsChanged:
            keycode = CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
            flags = (int64_t)CGEventGetFlags(event);
            break;
        case kCGEventMouseMoved:
        case kCGEventLeftMouseDragged:
        case kCGEventRightMouseDragged:
        case kCGEventOtherMouseDragged:
            dx = CGEventGetIntegerValueField(event, kCGMouseEventDeltaX);
            dy = CGEventGetIntegerValueField(event, kCGMouseEventDeltaY);
            button = CGEventGetIntegerValueField(event, kCGMouseEventButtonNumber);
            break;
        case kCGEventLeftMouseDown:
        case kCGEventLeftMouseUp:
        case kCGEventRightMouseDown:
        case kCGEventRightMouseUp:
        case kCGEventOtherMouseDown:
        case kCGEventOtherMouseUp:
            button = CGEventGetIntegerValueField(event, kCGMouseEventButtonNumber);
            break;
        case kCGEventScrollWheel:
            s1 = CGEventGetIntegerValueField(event, kCGScrollWheelEventDeltaAxis1);
            s2 = CGEventGetIntegerValueField(event, kCGScrollWheelEventDeltaAxis2);
            break;
        default:
            break;
    }
    int decision = onEventGo((int)type, keycode, flags, dx, dy, button, s1, s2);
    if (decision == 0) {
        return NULL;
    }
    return event;
}

// createTapAndRun creates the session-level event tap and blocks on the
// current thread's run loop. Returns -1 if the tap cannot be created
// (usually: Accessibility permission not granted).
static int createTapAndRun(void) {
    CGEventMask mask = 0;
    mask |= CGEventMaskBit(kCGEventKeyDown);
    mask |= CGEventMaskBit(kCGEventKeyUp);
    mask |= CGEventMaskBit(kCGEventFlagsChanged);
    mask |= CGEventMaskBit(kCGEventMouseMoved);
    mask |= CGEventMaskBit(kCGEventLeftMouseDragged);
    mask |= CGEventMaskBit(kCGEventRightMouseDragged);
    mask |= CGEventMaskBit(kCGEventOtherMouseDragged);
    mask |= CGEventMaskBit(kCGEventLeftMouseDown);
    mask |= CGEventMaskBit(kCGEventLeftMouseUp);
    mask |= CGEventMaskBit(kCGEventRightMouseDown);
    mask |= CGEventMaskBit(kCGEventRightMouseUp);
    mask |= CGEventMaskBit(kCGEventOtherMouseDown);
    mask |= CGEventMaskBit(kCGEventOtherMouseUp);
    mask |= CGEventMaskBit(kCGEventScrollWheel);

    CFMachPortRef tap = CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap,
                                         kCGEventTapOptionDefault, mask,
                                         tapCallback, NULL);
    if (tap == NULL) {
        return -1;
    }
    g_tap = tap;
    CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0);
    CFRunLoopAddSource(CFRunLoopGetCurrent(), src, kCFRunLoopCommonModes);
    CGEventTapEnable(tap, true);
    CFRelease(src);
    CFRelease(tap);
    CFRunLoopRun();
    return 0;
}

// mouseX / mouseY / screenW / screenH / warpMouse: display geometry helpers.
static double mouseX(void) {
    CGEventRef e = CGEventCreate(NULL);
    CGPoint p = CGEventGetLocation(e);
    CFRelease(e);
    return p.x;
}
static double mouseY(void) {
    CGEventRef e = CGEventCreate(NULL);
    CGPoint p = CGEventGetLocation(e);
    CFRelease(e);
    return p.y;
}
static int screenW(void) {
    CGRect b = CGDisplayBounds(CGMainDisplayID());
    return (int)b.size.width;
}
static int screenH(void) {
    CGRect b = CGDisplayBounds(CGMainDisplayID());
    return (int)b.size.height;
}
static void warpMouse(double x, double y) {
    CGEventRef e = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved,
                                           CGPointMake(x, y), kCGMouseButtonLeft);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
}
*/
import "C"

import (
	"errors"
)

// onEventGo is called from the C event tap callback. It returns 0 to consume
// the event, 1 to pass it through.
//
//export onEventGo
func onEventGo(ctype C.int, keycode C.int64_t, flags C.int64_t,
	dx, dy C.double, button C.int64_t, s1, s2 C.int64_t) C.int {
	if evHandler == nil {
		return 1 // pass through until engine is wired up
	}
	consume := evHandler(rawEvent{
		ctype:   int(ctype),
		keycode: int64(keycode),
		flags:   uint64(flags),
		dx:      float64(dx),
		dy:      float64(dy),
		button:  int64(button),
		s1:      int64(s1),
		s2:      int64(s2),
	})
	if consume {
		return 0
	}
	return 1
}

// evHandler is the single engine handler; wired at startup.
var evHandler func(ev rawEvent) bool

// startEventTap begins the CGEventTap and blocks the calling goroutine.
// It returns an error if the tap cannot be created (Accessibility missing).
func startEventTap(handler func(ev rawEvent) bool) error {
	evHandler = handler
	if rc := C.createTapAndRun(); rc != 0 {
		return errors.New("event tap creation failed: grant the app Accessibility permission in System Settings > Privacy & Security > Accessibility")
	}
	return nil
}

// initDisplay wires the platform display functions used by the engine.
func initDisplay() {
	screen = func() screenSize {
		return screenSize{W: float64(C.screenW()), H: float64(C.screenH())}
	}
	mousePos = func() (float64, float64) {
		return float64(C.mouseX()), float64(C.mouseY())
	}
	warpMouse = func(x, y float64) {
		C.warpMouse(C.double(x), C.double(y))
	}
}
