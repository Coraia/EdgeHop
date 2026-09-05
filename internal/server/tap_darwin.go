//go:build darwin

package server

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework AppKit -framework ApplicationServices

#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>
#include <ApplicationServices/ApplicationServices.h>
#include <objc/runtime.h>
#include <objc/message.h>
#include <dlfcn.h>
#include <stdint.h>

// onEventGo is implemented in Go (see below). It returns 0 to consume the
// event (suppress it from reaching apps) or 1 to let it pass.
extern int onEventGo(int type, int64_t keycode, int64_t flags,
                     double dx, double dy, int64_t button,
                     int64_t scrollAxis1, int64_t scrollAxis2);

// Sticky-edge overlay (overlay_darwin.m).
void uc_overlay_set_sticky(int on, int rightEdge, double cx, double cy, double barLen);
void uc_overlay_teardown(void);

// Swipe gesture type: NSEventTypeSwipe == 31. Not exposed as a named CGEvent
// constant, but delivered to session-level event taps.
#define kUCSwipeEventType 31

static CFMachPortRef g_tap = NULL;
static CFMachPortRef g_swipe_tap = NULL;
static int g_swipe_tap_ok = 0;

static int createSwipeTap(void);

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

// createTapAndRun creates the HID-level event tap and blocks on the
// current thread's run loop. HID level is used so that consuming an event
// prevents the cursor from moving / the key from reaching apps (session level
// sees events only after the window server has already applied them, so the
// Mac cursor would keep tracking the touchpad during remote control).
// Returns -1 if the tap cannot be created (Accessibility permission missing).
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

    CFMachPortRef tap = CGEventTapCreate(kCGHIDEventTap, kCGHeadInsertEventTap,
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
    // Second, a session-level tap that only watches trackpad swipe gestures
    // (three-finger swipes -> Mission Control / Space switching). While remote
    // control is active we consume them so the Mac does not react to gestures
    // that should belong to Omarchy. This tap is best-effort: if it fails the
    // HID tap above still controls the mouse/keyboard.
    //
    // Known limitation (docs/known-issues.md Issue #1): system-bound gestures
    // such as three-finger-up -> Mission Control are recognized by WindowServer
    // on a separate event path and never reach this tap, so they cannot be
    // consumed here. Kept as best-effort for non-system swipes.
    g_swipe_tap_ok = (createSwipeTap() == 0);
    if (!g_swipe_tap_ok) {
        fprintf(stderr, "[uc] warning: swipe-gesture tap not installed; "
                        "trackpad gestures will reach the Mac while remote control is active\n");
    }
    CFRunLoopRun();
    return 0;
}

// isTrusted reports whether this process has been granted Accessibility
// permission. Used to gate tap creation: calling CGEventTapCreate while
// untrusted makes macOS pop an authorization dialog, so we must not retry
// blindly.
static int isTrusted(void) {
    return AXIsProcessTrusted() ? 1 : 0;
}

// Swipe-gesture tap callback. onEventGo returns 0 to consume (Mac must not
// react to the gesture), 1 to let the system handle it.
static CGEventRef swipeTapCallback(CGEventTapProxy proxy, CGEventType type,
                                   CGEventRef event, void *refcon) {
    (void)proxy; (void)refcon;
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        if (g_swipe_tap != NULL) {
            CGEventTapEnable(g_swipe_tap, true);
        }
        return event;
    }
    int64_t dx = CGEventGetIntegerValueField(event, kCGScrollWheelEventDeltaAxis2);
    int64_t dy = CGEventGetIntegerValueField(event, kCGScrollWheelEventDeltaAxis1);
    int decision = onEventGo(kUCSwipeEventType, 0, 0, (double)dx, (double)dy,
                             -1, dx, dy);
    if (decision == 0) {
        return NULL; // consumed: gesture stays on the active machine
    }
    return event;
}

static int createSwipeTap(void) {
    CGEventMask mask = (CGEventMask)1 << kUCSwipeEventType;
    CFMachPortRef tap = CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap,
                                         kCGEventTapOptionDefault, mask,
                                         swipeTapCallback, NULL);
    if (tap == NULL) {
        return -1;
    }
    g_swipe_tap = tap;
    CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0);
    CFRunLoopAddSource(CFRunLoopGetCurrent(), src, kCFRunLoopCommonModes);
    CGEventTapEnable(tap, true);
    CFRelease(src);
    CFRelease(tap);
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
static void screenSize(int *w, int *h) {
    CGRect b = CGDisplayBounds(CGMainDisplayID());
    *w = (int)b.size.width;
    *h = (int)b.size.height;
}
static void warpMouse(double x, double y) {
    CGEventRef e = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved,
                                           CGPointMake(x, y), kCGMouseButtonLeft);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
}
// warpOffscreen moves the cursor to a point outside the visible screen so it
// cannot be seen while remote control is active. CGWarpMouseCursorPosition
// sets the position directly (no HID event is posted).
static void warpOffscreen(double x, double y) {
    CGWarpMouseCursorPosition(CGPointMake(x, y));
}
// setMouseAssociatation disassociates/associates the physical mouse from the
// cursor position. While disassociated, moving the mouse does NOT move the
// cursor at all (the OS guarantees it, regardless of event tap level), which
// is exactly what remote mode needs so the Mac cursor never tracks the
// touchpad. Input events are still delivered to taps while disassociated.
// NOTE: this API only takes effect while the calling application is in the
// FOREGROUND, so it is ineffective for a background menu-bar app. It is kept
// here as a harmless best-effort; cursor hiding is what actually works.
static void setMouseAssociation(int associated) {
    CGAssociateMouseAndMouseCursorPosition(associated ? true : false);
}

// Cursor control (background process):
// macOS only honors cursor show/hide requests from the frontmost application.
// A menu-bar (accessory) app is never frontmost, so NSCursor.hide() and
// CGDisplayHideCursor() are normally ignored. To let this background app hide
// the cursor while the user is controlling the remote machine, we opt into
// background cursor control via the private CoreGraphics SPI
// CGSMainConnectionID + CGSSetConnectionProperty("SetsCursorInBackground"),
// resolved at runtime with dlsym (stable across many macOS releases, used by
// popular cursor utilities).

typedef int32_t (*UCMainConnectionIDFn)(void);
typedef int32_t (*UCSetConnectionPropertyFn)(int32_t, int32_t, CFStringRef, CFTypeRef);

// enableBackgroundCursorControl returns 0 on success.
static int enableBackgroundCursorControl(void) {
    void *cg = dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", RTLD_NOW);
    if (cg == NULL) {
        return -1;
    }
    UCMainConnectionIDFn mainConn = (UCMainConnectionIDFn)dlsym(cg, "CGSMainConnectionID");
    UCSetConnectionPropertyFn setProp =
        (UCSetConnectionPropertyFn)dlsym(cg, "CGSSetConnectionProperty");
    if (mainConn == NULL || setProp == NULL) {
        return -2;
    }
    int32_t conn = mainConn();
    int32_t status = setProp(conn, conn, CFSTR("SetsCursorInBackground"), kCFBooleanTrue);
    return status;
}

// hideCursor / showCursor via AppKit NSCursor (persistent until unhide).
static void hideCursorNSCursor(void) {
    id cls = (id)objc_getClass("NSCursor");
    if (cls != NULL) {
        ((void (*)(id, SEL))objc_msgSend)(cls, sel_registerName("hide"));
    }
    // Belt and suspenders: also try the CoreGraphics hide (refcounted too).
    CGDisplayHideCursor(kCGDirectMainDisplay);
}
static void showCursorNSCursor(void) {
    id cls = (id)objc_getClass("NSCursor");
    if (cls != NULL) {
        ((void (*)(id, SEL))objc_msgSend)(cls, sel_registerName("unhide"));
    }
    CGDisplayShowCursor(kCGDirectMainDisplay);
}
static void hideCursor(void) {
    hideCursorNSCursor();
}
static void showCursor(void) {
    showCursorNSCursor();
}

// cursorVisible reports the global cursor visibility via the private
// CGCursorIsVisible symbol (marked unavailable in the SDK but present at
// runtime). Returns -1 if the symbol cannot be resolved.
static int cursorVisible(void) {
    void *cg = dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", RTLD_NOW);
    if (cg == NULL) {
        return -1;
    }
    typedef int32_t (*CursorIsVisibleFn)(void);
    CursorIsVisibleFn fn = (CursorIsVisibleFn)dlsym(cg, "CGCursorIsVisible");
    if (fn == NULL) {
        return -1;
    }
    return fn() != 0 ? 1 : 0;
}
*/
import "C"

import (
	"errors"
	"log"
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
// The caller must gate calls on isAccessibilityTrusted(): creating a tap while
// untrusted makes macOS pop an authorization dialog on every attempt.
func startEventTap(handler func(ev rawEvent) bool) error {
	evHandler = handler
	if rc := C.createTapAndRun(); rc != 0 {
		return errors.New("event tap creation failed: grant the app Accessibility permission in System Settings > Privacy & Security > Accessibility")
	}
	return nil
}

// isAccessibilityTrusted reports whether the app has Accessibility permission.
// When false, creating event taps triggers repeated system authorization dialogs,
// so the startup loop waits quietly until the user grants it.
func isAccessibilityTrusted() bool {
	return C.isTrusted() == 1
}

// initDisplay wires the platform display functions used by the engine.
func initDisplay() {
	screen = func() screenSize {
		var w, h C.int
		C.screenSize(&w, &h)
		return screenSize{W: float64(w), H: float64(h)}
	}
	mousePos = func() (float64, float64) {
		return float64(C.mouseX()), float64(C.mouseY())
	}
	warpMouse = func(x, y float64) {
		C.warpMouse(C.double(x), C.double(y))
	}
	warpOffscreen = func(x, y float64) {
		C.warpOffscreen(C.double(x), C.double(y))
	}
	setMouseAssoc = func(assoc bool) {
		C.setMouseAssociation(0)
		if assoc {
			C.setMouseAssociation(1)
		}
	}
	hideCursor = func() { C.hideCursor() }
	showCursor = func() { C.showCursor() }
	cursorVisible = func() int { return int(C.cursorVisible()) }
	// Opt into background cursor control so hideCursor() actually takes effect
	// while this menu-bar app is not the frontmost process.
	if rc := C.enableBackgroundCursorControl(); rc != 0 {
		log.Printf("warning: SetsCursorInBackground SPI failed (%d); cursor hiding may not work", int(rc))
	} else {
		log.Printf("background cursor control enabled (SetsCursorInBackground)")
	}
}

// setStickyOverlay shows or hides the sticky-edge light bar at the current
// cursor position. barLen is the current bar height in points. It is safe to
// call from any goroutine; the ObjC overlay dispatches the actual AppKit work
// to the main queue.
func setStickyOverlay(on bool, barLen float64, rightEdge bool) {
	x, y := mousePos()
	onI := 0
	if on {
		onI = 1
	}
	rightI := 0
	if rightEdge {
		rightI = 1
	}
	C.uc_overlay_set_sticky(C.int(onI), C.int(rightI), C.double(x), C.double(y), C.double(barLen))
}
