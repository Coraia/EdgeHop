// overlay_darwin.m — Sticky-edge visual feedback for the macOS server.
//
// Draws a thin, vertically-graduated white "light bar" at the shared screen
// edge that follows the cursor's Y position. Its height starts at ~10px when
// the cursor enters the sticky zone and shrinks as the cursor presses closer
// to the seam, signalling the impending break-free. The window is a
// borderless, transparent, click-through overlay at a very high level, so it
// never intercepts input and appears above everything else only while the
// sticky state is active.
//
// Exposed as plain C functions called from Go (see tap_darwin.go). All AppKit
// work is dispatched to the main queue because it must run on the main thread.
#include <Cocoa/Cocoa.h>
#include <dispatch/dispatch.h>

// ---- StickyOverlayView ----
@interface StickyOverlayView : NSView
@property(nonatomic) int sticky;
@property(nonatomic) int rightEdge;
@property(nonatomic) double cx, cy;
@property(nonatomic) double barLen;
@end

@implementation StickyOverlayView
- (BOOL)isFlipped {
    return YES; // top-left origin, matching Quartz cursor coordinates
}
- (void)drawRect:(NSRect)dirtyRect {
    (void)dirtyRect;
    if (!self.sticky) {
        return;
    }
    // Apple-style edge border: a thin, full-height glowing line at the shared
    // (left) edge. A ~2px bright core with a soft light halo that fades out
    // into the screen. The halo narrows (barLen: 14 -> 6) and the core stays
    // bright as the cursor presses closer to the seam, signalling "about to
    // break through". Drawn as stacked translucent strips (reliable under
    // cgo's non-ARC compilation, unlike NSGradient which renders nothing).
    double H = self.bounds.size.height;      // full screen height
    double totalW = MAX(6.0, self.barLen);   // halo width (14 -> 6)
    double coreW = 2.0;                       // bright core line
    double edgeX = self.rightEdge ? self.bounds.size.width - totalW : 0;
    int segs = 48;
    for (int i = 0; i <= segs; i++) {
        double x0 = totalW * (double)i / (double)segs;
        double xc = x0 + totalW / (double)segs / 2.0; // segment center
        double t = xc / totalW;                        // 0..1 across the halo
        double alpha;
        if (xc <= coreW) {
            alpha = 0.95; // bright core line
        } else {
            alpha = 0.95 * (1.0 - (xc - coreW) / (totalW - coreW)); // soft fade
        }
        if (alpha < 0.03) {
            continue;
        }
        // white core, faint blue tint toward the halo edge (visible over white UI)
        double cr = 1.0 - t * 0.30;
        double cg = 1.0 - t * 0.20;
        double cb = 1.0;
        double segW = totalW / (double)segs + 0.5;
        double drawX = self.rightEdge ? edgeX + totalW - x0 - segW : edgeX + x0;
        NSRect seg = NSMakeRect(drawX, 0, segW, H);
        [[NSColor colorWithCalibratedRed:cr green:cg blue:cb alpha:alpha] setFill];
        NSRectFill(seg);
    }
}
@end

static NSWindow *gWin = nil;
static StickyOverlayView *gView = nil;

static void ensureWindow(void) {
    if (gWin) {
        return;
    }
    NSScreen *screen = [NSScreen mainScreen];
    NSRect f = [screen frame]; // origin (0,0) for the main screen
    gWin = [[NSWindow alloc]
        initWithContentRect:f
                  styleMask:NSWindowStyleMaskBorderless
                    backing:NSBackingStoreBuffered
                      defer:NO];
    [gWin setOpaque:NO];
    [gWin setBackgroundColor:[NSColor clearColor]];
    [gWin setIgnoresMouseEvents:YES];
    [gWin setHasShadow:NO];
    [gWin setLevel:NSScreenSaverWindowLevel];
    [gWin setCollectionBehavior:
        (NSWindowCollectionBehaviorCanJoinAllSpaces |
         NSWindowCollectionBehaviorStationary |
         NSWindowCollectionBehaviorIgnoresCycle |
         NSWindowCollectionBehaviorFullScreenAuxiliary)];
    gView = [[StickyOverlayView alloc]
        initWithFrame:NSMakeRect(0, 0, f.size.width, f.size.height)];
    [gWin setContentView:gView];
    [gWin setFrame:f display:YES];
    [gWin orderFrontRegardless];
}

// uc_overlay_set_sticky shows (on=1) or hides (on=0) the sticky feedback.
// cx, cy are global Quartz coordinates (top-left origin); barLen is the
// current bar height in points.
void uc_overlay_set_sticky(int on, int rightEdge, double cx, double cy, double barLen) {
    dispatch_async(dispatch_get_main_queue(), ^{
        ensureWindow();
        gView.sticky = on;
        gView.rightEdge = rightEdge;
        gView.cx = cx;
        gView.cy = cy;
        gView.barLen = barLen;
        [gView setNeedsDisplay:YES];
        // Force a synchronous redraw on the spot so the bar appears even if
        // the app's run loop does not drive a normal display pass.
        [gView displayIfNeeded];
    });
}

// uc_overlay_teardown removes the overlay window.
void uc_overlay_teardown(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (gWin) {
            [gWin orderOut:nil];
            gWin = nil;
            gView = nil;
        }
    });
}
