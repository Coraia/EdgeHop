//go:build darwin

package server

/*
#cgo LDFLAGS: -framework Carbon -framework CoreGraphics -framework CoreFoundation
#include <Carbon/Carbon.h>
#include <CoreGraphics/CoreGraphics.h>
#include <pthread.h>

// Carbon's secure-input query is not thread safe. Serialize our reads.
// Only the background poller uses this; never query inside a HID callback.
static pthread_mutex_t guardMutex = PTHREAD_MUTEX_INITIALIZER;

static int sessionBool(CFDictionaryRef session, CFStringRef key) {
    CFTypeRef value = CFDictionaryGetValue(session, key);
    if (value == NULL) return 0;
    if (CFGetTypeID(value) == CFBooleanGetTypeID()) {
        return CFBooleanGetValue(value);
    }
    if (CFGetTypeID(value) == CFNumberGetTypeID()) {
        int number = 0;
        if (CFNumberGetValue(value, kCFNumberIntType, &number)) return number != 0;
    }
    return 0;
}

static int inputBlockReason(void) {
    pthread_mutex_lock(&guardMutex);
    CFDictionaryRef session = CGSessionCopyCurrentDictionary();
    int reason = 0;
    if (session == NULL) {
        reason = 3;
    } else {
        if (sessionBool(session, CFSTR("CGSSessionScreenIsLocked"))) {
            reason = 1;
        } else if (!sessionBool(session, kCGSessionOnConsoleKey) ||
                   !sessionBool(session, kCGSessionLoginDoneKey)) {
            reason = 3;
        } else if (IsSecureEventInputEnabled()) {
            reason = 2;
        }
        CFRelease(session);
    }
    pthread_mutex_unlock(&guardMutex);
    return reason;
}
*/
import "C"

func platformInputBlockReason() string {
	switch C.inputBlockReason() {
	case 1:
		return "mac_locked"
	case 2:
		return "secure_input"
	case 3:
		return "session_unavailable"
	default:
		return ""
	}
}
