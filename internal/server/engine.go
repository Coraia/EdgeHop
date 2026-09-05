package server

import (
	"bufio"
	"log"
	"sync"
	"sync/atomic"
	"time"
	"universal_control/internal/clipsync"
	"universal_control/internal/protocol"
)

// Mode is the current control state.
type Mode int32

const (
	// ModeLocal: input applies to the Mac itself.
	ModeLocal Mode = iota
	// ModeRemote: input is forwarded to the client (Omarchy).
	ModeRemote
)

// Edge values.
const (
	EdgeRight = "right"
	EdgeLeft  = "left"
)

// engine is the macOS-side state machine. It owns:
//   - the current control mode,
//   - the connected client writer,
//   - edge/hotkey switch decisions,
//   - the clipboard poller.
//
// The cgo event tap calls consume() synchronously: the switch decision must be
// made inline (cheap checks only) and returned to the tap so it knows whether
// to suppress the event. Network sends are enqueued to sendQ and drained by a
// separate writer goroutine, so a slow client never blocks the tap.
type engine struct {
	cfg     Config
	mode    atomic.Int32
	remote  atomic.Bool // fast path: whether we are currently forwarding
	stateMu sync.Mutex

	mu   sync.Mutex
	conn netConn // current client connection writer (nil if disconnected)
	scrW int32   // Mac screen size (cached)
	scrH int32
	cliW int32 // client screen size (from handshake)
	cliH int32

	swKeys      map[int]bool // hotkey codes for manual toggle
	keysDown    map[int]bool // mac keycodes currently down
	buttonsDown map[int]bool // Mac mouse buttons currently down

	// hotkeyLocked prevents the switch-hotkey from firing again until all of
	// its keys are released (see hotkeyDown).
	hotkeyLocked bool
	// pendingRemote delays a local-to-remote switch until the local machine
	// has received key-up events for the hotkey that requested the switch.
	pendingRemote bool

	// edgeArmed gates edge re-entry after returning to local: the cursor must
	// first move away from the shared edge before an edge crossing will switch
	// to remote again. Prevents the "warp to edge -> immediate re-enter" loop.
	edgeArmed bool

	// sticky tracks the "sticky edge" state: the cursor has reached the shared
	// edge but has not yet broken free. While sticky, the UI shows an
	// Apple-style glowing edge line whose halo narrows as the cursor presses
	// closer to the seam, and the break-free dwell interpolates from
	// StickyDwellMax (light touch) down to StickyDwellMin (firm press).
	sticky     bool
	stickyAt   time.Time // when sticky was entered
	stickyPush float64   // accumulated outward push while sticky
	// onEdgeUI reports sticky-state changes to the overlay (true=edge line
	// shown, false=hidden) together with the current halo width in points.
	// Nil disables the visual layer.
	onEdgeUI func(on bool, barLen float64)

	clipboard clipsync.Tracker

	statusMu sync.Mutex
	status   Status
	onStatus func(Status)

	sendQ chan frameOut // buffered frames to client writer
}

// Sticky bar geometry: the halo WIDTH of the edge line when the cursor enters
// the zone, and the width when pressed hard against the seam (the line spans
// the full screen height; the halo narrows as the cursor approaches).
const (
	stickyBarMax = 14.0
	stickyBarMin = 6.0
)

// Status is a snapshot of server state, pushed to the tray/UI via the status
// callback.
type Status struct {
	ServerRunning   bool
	ClientConnected bool
	Mode            string // "local" | "remote"
	LastError       string
}

type netConn interface {
	Send(typ byte, payload []byte) error
	Close() error
}

type frameOut struct {
	typ     byte
	payload []byte
	conn    netConn
}

// rawEvent is a normalized event from the cgo tap.
type rawEvent struct {
	ctype   int
	keycode int64
	flags   uint64
	dx, dy  float64
	button  int64
	s1, s2  int64
}

// screenSize describes the current macOS display geometry.
type screenSize struct {
	W, H float64
}

// screen returns the current main display size. Overridable in tests.
var screen func() screenSize

// mousePos returns the current cursor position. Overridable in tests.
var mousePos func() (x, y float64)

// warpMouse moves the cursor. Overridable in tests.
var warpMouse func(x, y float64)

// hideCursor / showCursor hide and show the local (Mac) cursor. While in
// remote mode the Mac cursor is hidden so it does not keep visibly tracking
// the touchpad (it would otherwise leave a confusing "trail" on the Mac screen
// while control is on Omarchy). Overridable in tests.
var hideCursor func()
var showCursor func()

// cursorVisible returns 1 if the system cursor is visible, 0 if hidden, -1 if
// unknown. Used for diagnostics only (best-effort).
var cursorVisible func() int

// warpOffscreen moves the local cursor outside the visible screen (used while
// remote is active so the Mac cursor is never visible even if the hide API is
// unavailable). Overridable in tests.
var warpOffscreen func(x, y float64)

// setMouseAssoc disassociates (false) or re-associates (true) the physical
// mouse with the cursor position. While disassociated the cursor never tracks
// the mouse, no matter the event tap level. Overridable in tests.
var setMouseAssoc func(assoc bool)

// remoteEdgeIsRight reports whether the client sits on the Mac's right edge.
func (e *engine) remoteEdgeIsRight() bool {
	return e.cfg.RemoteEdge == EdgeRight
}

// newEngine builds an engine from config and wires platform functions.
func newEngine(cfg Config) *engine {
	e := &engine{
		cfg:         cfg,
		sendQ:       make(chan frameOut, 256),
		swKeys:      make(map[int]bool, len(cfg.SwitchKeys)),
		keysDown:    make(map[int]bool),
		buttonsDown: make(map[int]bool),
		status:      Status{Mode: modeName(ModeLocal)},
	}
	for _, k := range cfg.SwitchKeys {
		e.swKeys[k] = true
	}
	e.remote.Store(false)
	e.edgeArmed = true // initial state: first edge crossing may enter remote
	return e
}

// isRemote reports current mode.
func (e *engine) isRemote() bool { return e.remote.Load() }

// setOnStatus registers a status callback (e.g. the tray app). It is invoked
// from various goroutines; the callback must be quick and thread-safe.
func (e *engine) setOnStatus(f func(Status)) {
	e.statusMu.Lock()
	e.onStatus = f
	e.statusMu.Unlock()
}

// updateStatus mutates the status snapshot and notifies the callback.
func (e *engine) updateStatus(mut func(*Status)) {
	e.statusMu.Lock()
	mut(&e.status)
	s := e.status
	f := e.onStatus
	e.statusMu.Unlock()
	if f != nil {
		f(s)
	}
}

// setConn installs a new client connection.
func (e *engine) setConn(c netConn) {
	e.mu.Lock()
	old := e.conn
	e.conn = c
	e.mu.Unlock()
	if old != nil && old != c {
		_ = old.Close()
	}
}

// dropConn clears the connection only if it is the current one.
func (e *engine) dropConn(c netConn) {
	e.mu.Lock()
	if e.conn == c {
		e.conn = nil
	}
	gone := e.conn == nil
	e.mu.Unlock()
	_ = c.Close()
	if gone {
		// Lost the client: force back to local control.
		e.stateMu.Lock()
		wasRemote := e.isRemote()
		e.switchTo(ModeLocal)
		e.sticky = false
		e.pendingRemote = false
		e.hotkeyLocked = false
		clear(e.keysDown)
		clear(e.buttonsDown)
		if e.onEdgeUI != nil {
			e.onEdgeUI(false, 0)
		}
		if wasRemote {
			if setMouseAssoc != nil {
				setMouseAssoc(true)
			}
			if showCursor != nil {
				showCursor()
			}
		}
		e.stateMu.Unlock()
		e.updateStatus(func(s *Status) { s.ClientConnected = false })
		log.Printf("control returned to local (client gone)")
	}
}

// setClientScreen records the client's display size from the handshake.
func (e *engine) setClientScreen(w, h int32) {
	e.mu.Lock()
	e.cliW, e.cliH = w, h
	e.mu.Unlock()
}

func (e *engine) refreshScreen() {
	s := screen()
	w, h := int32(s.W), int32(s.H)
	if w <= 0 || h <= 0 {
		return
	}
	e.mu.Lock()
	changed := e.scrW != w || e.scrH != h
	e.scrW, e.scrH = w, h
	e.mu.Unlock()
	if changed {
		log.Printf("mac screen updated: %dx%d", w, h)
		e.send(protocol.MsgScreen, protocol.EncodeScreen(w, h))
	}
}

// clientConnected reports whether a client is attached.
func (e *engine) clientConnected() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.conn != nil
}

// switchTo changes mode and logs it.
func (e *engine) switchTo(m Mode) {
	old := Mode(e.mode.Swap(int32(m)))
	e.remote.Store(m == ModeRemote)
	if old == m {
		return
	}
	log.Printf("mode: %v -> %v", modeName(old), modeName(m))
	e.updateStatus(func(s *Status) { s.Mode = modeName(m) })
}

// reportError records a non-fatal error for the status callback.
func (e *engine) reportError(err error) {
	log.Printf("error: %v", err)
	e.updateStatus(func(s *Status) { s.LastError = err.Error() })
}

func modeName(m Mode) string {
	if m == ModeRemote {
		return "remote"
	}
	return "local"
}

// run starts background services. It is called once after the tap is up.
func (e *engine) run() {
	go e.writer()

	e.refreshScreen()
	go e.screenLoop()

	go e.clipboardLoop()
	go e.stickyDwellLoop()
}

func (e *engine) screenLoop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for range t.C {
		e.refreshScreen()
	}
}

// stickyDwellLoop drives the sticky-edge visual and timing: it refreshes the
// light-bar height while the cursor is stuck at the seam, and breaks free
// (switching to remote) once the cursor has rested there for the dwell that
// matches how close it is to the edge. It is harmless while not sticky.
func (e *engine) stickyDwellLoop() {
	t := time.NewTicker(40 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		e.stateMu.Lock()
		if e.isRemote() || !e.sticky {
			e.stateMu.Unlock()
			continue
		}
		x, _ := mousePos()
		c := e.edgeCloseness(x)
		// Keep the light bar glued to the cursor, shrinking as it presses closer.
		if e.onEdgeUI != nil {
			e.onEdgeUI(true, stickyBarLen(c))
		}
		if time.Since(e.stickyAt) >= e.stickyDwell(c) {
			e.breakFreeLocked()
		}
		e.stateMu.Unlock()
	}
}

// edgeCloseness returns how close the cursor is to the shared edge, from 0
// (just entered the sticky zone) to 1 (pressed against the seam).
func (e *engine) edgeCloseness(x float64) float64 {
	z := e.cfg.EdgeSensitivity
	if z <= 0 {
		return 1
	}
	var c float64
	if e.remoteEdgeIsRight() {
		c = (x - (e.scrWf() - z)) / z
	} else {
		c = (z - x) / z
	}
	if c < 0 {
		c = 0
	}
	if c > 1 {
		c = 1
	}
	return c
}

// stickyDwell is the break-free dwell for a given closeness: a light touch at
// the zone edge needs StickyDwellMax; pressing against the seam needs only
// StickyDwellMin.
func (e *engine) stickyDwell(c float64) time.Duration {
	span := e.cfg.StickyDwellMax - e.cfg.StickyDwellMin
	return e.cfg.StickyDwellMax - time.Duration(float64(span)*c)
}

// stickyBarLen is the light-bar height (px) for a given closeness.
func stickyBarLen(c float64) float64 {
	return stickyBarMax - (stickyBarMax-stickyBarMin)*c
}

// consume is the tap callback adapter. It MUST return the final decision:
// true = consume (suppress locally), false = pass through to the Mac.
func (e *engine) consume(ev rawEvent) bool {
	if !e.clientConnected() {
		return false
	}
	if e.isRemote() {
		e.forwardRemote(ev)
		return true
	}
	return e.watchLocal(ev)
}

// watchLocal: input applies to the Mac; watch for edge/hotkey switches.
// Returns true if the event was consumed by a switch.
func (e *engine) watchLocal(ev rawEvent) bool {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	// Track held keys for hotkey detection.
	switch ev.ctype {
	case cgEventKeyDown:
		e.keysDown[int(ev.keycode)] = true
	case cgEventKeyUp:
		delete(e.keysDown, int(ev.keycode))
	case cgEventFlagsChanged:
		if int(ev.keycode) != 0x39 {
			down := e.modifierDown(int(ev.keycode), ev.flags)
			if down {
				e.keysDown[int(ev.keycode)] = true
			} else {
				delete(e.keysDown, int(ev.keycode))
			}
		}
	case cgEventLeftMouseDown, cgEventRightMouseDown, cgEventOtherMouseDown:
		e.buttonsDown[int(ev.button)] = true
	case cgEventLeftMouseUp, cgEventRightMouseUp, cgEventOtherMouseUp:
		delete(e.buttonsDown, int(ev.button))
	}
	e.afterKeysUpdate()
	if e.finishPendingRemote() {
		return false
	}

	// Edge switch on mouse move: reaching the shared edge enters the "sticky"
	// state (cursor held at the seam with visual feedback); the switch to
	// remote only happens after breaking free (resting StickyDwell, or pushing
	// outward by StickyPush).
	if ev.ctype == cgEventMouseMoved || ev.ctype == cgEventLeftMouseDragged ||
		ev.ctype == cgEventRightMouseDragged || ev.ctype == cgEventOtherMouseDragged {
		if x, _ := mousePos(); e.atRemoteEdge(x) {
			if !e.edgeArmed {
				// Just returned to local with the cursor warped onto the seam;
				// require it to leave the edge before it can go sticky again.
			} else if !e.sticky {
				e.sticky = true
				e.stickyAt = time.Now()
				e.stickyPush = 0
				if e.onEdgeUI != nil {
					e.onEdgeUI(true, stickyBarLen(e.edgeCloseness(x)))
				}
			} else {
				// Already sticky: accumulate outward push. Pushing "out" means
				// toward the remote machine, i.e. dx < 0 for a left edge.
				if e.remoteEdgeIsRight() {
					if ev.dx > 0 {
						e.stickyPush += ev.dx
					}
				} else if ev.dx < 0 {
					e.stickyPush -= ev.dx
				}
				if e.stickyPush >= e.cfg.StickyPush {
					return e.breakFreeLocked()
				}
			}
		} else {
			// Cursor moved away from the shared edge: leave sticky and arm it
			// so a later crossing can re-enter remote (prevents immediate
			// re-entry after returning to local with the cursor on the seam).
			if e.sticky {
				e.sticky = false
				if e.onEdgeUI != nil {
					e.onEdgeUI(false, 0)
				}
			}
			e.edgeArmed = true
		}
	}

	// Hotkey toggle.
	if e.hotkeyDown() {
		e.hotkeyLocked = true
		e.pendingRemote = true
		return true
	}
	return false
}

// breakFree leaves the sticky state and switches control to the remote machine.
func (e *engine) breakFree() bool {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	return e.breakFreeLocked()
}

func (e *engine) breakFreeLocked() bool {
	if !e.sticky {
		return false
	}
	e.sticky = false
	if e.onEdgeUI != nil {
		e.onEdgeUI(false, 0)
	}
	if e.localInputHeld() {
		e.pendingRemote = true
		return false
	}
	e.enterRemoteLocked()
	return true
}

func (e *engine) localInputHeld() bool {
	return len(e.keysDown) > 0 || len(e.buttonsDown) > 0
}

func (e *engine) finishPendingRemote() bool {
	if !e.pendingRemote || e.localInputHeld() {
		return false
	}
	e.pendingRemote = false
	e.enterRemoteLocked()
	return true
}

// forwardRemote forwards input to the client.
func (e *engine) forwardRemote(ev rawEvent) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	// Hotkey while remote returns control to the Mac. We check it only on
	// press events (not on key-up), and only after recording the press, so that
	// the key-up that follows an enter-remote/leave-remote does not bounce
	// control back. When the hotkey fires, the press is consumed (NOT forwarded)
	// so Omarchy's own binding for the same combo never triggers.
	switch ev.ctype {
	case cgEventKeyDown:
		e.keysDown[int(ev.keycode)] = true
		if e.hotkeyDown() {
			e.hotkeyLocked = true
			e.leaveRemoteLocked(-1)
			return
		}
		e.afterKeysUpdate()
		if code, ok := keymapToEvdev(int(ev.keycode)); ok {
			e.send(protocol.MsgKey, protocol.EncodeKey(code, 1))
		}
	case cgEventKeyUp:
		delete(e.keysDown, int(ev.keycode))
		e.afterKeysUpdate()
		if code, ok := keymapToEvdev(int(ev.keycode)); ok {
			e.send(protocol.MsgKey, protocol.EncodeKey(code, 0))
		}
	case cgEventFlagsChanged:
		if int(ev.keycode) == 0x39 {
			if code, ok := keymapToEvdev(int(ev.keycode)); ok {
				e.send(protocol.MsgKey, protocol.EncodeKey(code, 1))
				e.send(protocol.MsgKey, protocol.EncodeKey(code, 0))
			}
			return
		}
		down := e.modifierDown(int(ev.keycode), ev.flags)
		if down {
			e.keysDown[int(ev.keycode)] = true
			if e.hotkeyDown() {
				e.hotkeyLocked = true
				e.leaveRemoteLocked(-1)
				return
			}
		} else {
			delete(e.keysDown, int(ev.keycode))
		}
		e.afterKeysUpdate()
		if code, ok := keymapToEvdev(int(ev.keycode)); ok {
			p := byte(0)
			if down {
				p = 1
			}
			e.send(protocol.MsgKey, protocol.EncodeKey(code, p))
		}
	case cgEventMouseMoved, cgEventLeftMouseDragged, cgEventRightMouseDragged, cgEventOtherMouseDragged:
		e.send(protocol.MsgMouseMove, protocol.EncodeMouseMove(int16(ev.dx), int16(ev.dy)))
	case cgEventLeftMouseDown, cgEventLeftMouseUp:
		e.sendMouseButton(protocol.ButtonLeft, u8(ev.ctype == cgEventLeftMouseDown))
	case cgEventRightMouseDown, cgEventRightMouseUp:
		e.sendMouseButton(protocol.ButtonRight, u8(ev.ctype == cgEventRightMouseDown))
	case cgEventOtherMouseDown, cgEventOtherMouseUp:
		e.sendMouseButton(protocol.ButtonMiddle, u8(ev.ctype == cgEventOtherMouseDown))
	case cgEventScrollWheel:
		e.send(protocol.MsgMouseWheel, protocol.EncodeMouseWheel(int16(ev.s2), int16(ev.s1)))
	}
}

func (e *engine) modifierDown(keycode int, flags uint64) bool {
	switch keycode {
	case 0x36, 0x37, 0x38, 0x3C, 0x3A, 0x3D, 0x3B, 0x3E:
		return !e.keysDown[keycode]
	default:
		return flags&flagForMacKey(keycode) != 0
	}
}

func (e *engine) sendMouseButton(btn, pressed uint8) {
	e.send(protocol.MsgMouseButton, protocol.EncodeMouseButton(btn, pressed))
}

func u8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// enterRemote switches control to the client and parks the cursor.
func (e *engine) enterRemote() {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	e.enterRemoteLocked()
}

func (e *engine) enterRemoteLocked() {
	if e.isRemote() {
		return
	}
	e.switchTo(ModeRemote)
	e.edgeArmed = false // will warp off-screen; require the cursor to leave the edge before re-entering
	_, y := mousePos()
	// Tell the client where the Mac cursor was on the seam so it can land at
	// the same Y (proportionally mapped) instead of a fixed park point.
	e.send(protocol.MsgSwitch, protocol.EncodeSwitch(protocol.Switch{
		Direction: protocol.SwitchRemote,
		Y:         int32(y),
		HasY:      true,
	}))
	// Stop the physical mouse from driving the Mac cursor and hide it.
	if setMouseAssoc != nil {
		setMouseAssoc(false)
	}
	if hideCursor != nil {
		hideCursor()
	}
	// Park the Mac cursor off-screen (clamped to the edge on macOS, which is
	// fine — it will not track the touchpad while disassociated).
	if warpOffscreen != nil {
		if e.remoteEdgeIsRight() {
			warpOffscreen(e.scrWf()+1000, clampY(y))
		} else {
			warpOffscreen(-1000, clampY(y))
		}
	}
	log.Printf("remote control active (mouse disassociated, cursor hidden)")
	if cursorVisible != nil {
		log.Printf("cursor visibility after hide: %d (1=visible 0=hidden -1=unknown)", cursorVisible())
	}
}

// leaveRemote returns control to the Mac and warps the cursor to the seam.
// edgeY is the client-side Y the cursor was at on the remote edge (from the
// "back:Y" switch request), or -1 to use the current Mac cursor Y.
func (e *engine) leaveRemote(edgeY float64) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	e.leaveRemoteLocked(edgeY)
}

func (e *engine) leaveRemoteLocked(edgeY float64) {
	if !e.isRemote() {
		return
	}
	e.switchTo(ModeLocal)
	e.send(protocol.MsgSwitch, protocol.EncodeSwitch(protocol.Switch{
		Direction: protocol.SwitchBack,
	}))
	e.edgeArmed = false // cursor will be warped onto the edge; require it to leave first
	// Re-associate the mouse with the cursor and show it again.
	if setMouseAssoc != nil {
		setMouseAssoc(true)
	}
	if showCursor != nil {
		showCursor()
	}
	var y float64
	if edgeY >= 0 {
		y = e.mapClientYToMac(edgeY)
	} else {
		_, y = mousePos()
	}
	y = clampY(y)
	if e.remoteEdgeIsRight() {
		warpMouse(e.scrWf()-1, y)
	} else {
		warpMouse(0, y)
	}
}

// handleSwitchRequest processes a validated client request to return control.
func (e *engine) handleSwitchRequest(payload []byte) {
	message, err := protocol.DecodeSwitch(payload)
	if err != nil || message.Direction != protocol.SwitchBack {
		return
	}
	edgeY := -1.0
	if message.HasY {
		edgeY = float64(message.Y)
	}
	e.leaveRemote(edgeY)
}

// mapClientYToMac maps a client-screen Y back to the Mac's screen Y.
func (e *engine) mapClientYToMac(y float64) float64 {
	e.mu.Lock()
	cliH, scrH := e.cliH, e.scrH
	e.mu.Unlock()
	if cliH > 0 && scrH > 0 {
		return y * float64(scrH) / float64(cliH)
	}
	return y
}

func (e *engine) scrWf() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return float64(e.scrW)
}

func (e *engine) atRemoteEdge(x float64) bool {
	if e.remoteEdgeIsRight() {
		return x >= e.scrWf()-e.cfg.EdgeSensitivity
	}
	return x <= e.cfg.EdgeSensitivity
}

// hotkeyDown reports whether all switch-hotkey keys are currently held.
// hotkeyLocked disables the hotkey after it has fired once, until every hotkey
// key is physically released, so holding the combo (key auto-repeat) does not
// bounce control between local and remote repeatedly.
func (e *engine) hotkeyDown() bool {
	if e.hotkeyLocked || len(e.swKeys) == 0 {
		return false
	}
	for k := range e.swKeys {
		if !e.keysDown[k] {
			return false
		}
	}
	return true
}

// afterKeysUpdate is called whenever keysDown changes. It unlocks the hotkey
// once every hotkey key has been released.
func (e *engine) afterKeysUpdate() {
	if !e.hotkeyLocked {
		return
	}
	for k := range e.swKeys {
		if e.keysDown[k] {
			return // a hotkey key is still held; keep locked
		}
	}
	e.hotkeyLocked = false
}

// send enqueues a frame for the writer goroutine (non-blocking drop on full).
func (e *engine) send(typ byte, payload []byte) {
	e.mu.Lock()
	c := e.conn
	e.mu.Unlock()
	if c == nil {
		return
	}
	select {
	case e.sendQ <- frameOut{typ: typ, payload: payload, conn: c}:
	default:
		log.Printf("warn: send queue full, closing client connection")
		_ = c.Close()
	}
}

// writer drains sendQ to the current client connection.
func (e *engine) writer() {
	for f := range e.sendQ {
		if err := f.conn.Send(f.typ, f.payload); err != nil {
			log.Printf("warn: send failed: %v", err)
			e.dropConn(f.conn)
		}
	}
}

// recvLoop handles client->server messages on the connection's read side.
// It runs on the connection goroutine; the channel closes on disconnect.
func (e *engine) recvLoop(r *bufio.Reader) {
	for {
		f, err := protocol.ReadFrame(r)
		if err != nil {
			return
		}
		switch f.Type {
		case protocol.MsgClipboard:
			e.applyClipboard(f.Payload)
		case protocol.MsgSwitch:
			e.handleSwitchRequest(f.Payload)
		case protocol.MsgScreen:
			w, h, err := protocol.DecodeScreen(f.Payload)
			if err == nil && w > 0 && h > 0 {
				e.setClientScreen(w, h)
				log.Printf("client screen updated: %dx%d", w, h)
			}
		}
	}
}

func clampY(y float64) float64 {
	h := float64(0)
	if s := screen(); s.H > 0 {
		h = s.H
	}
	if y < 0 {
		return 0
	}
	if y > h-1 {
		return h - 1
	}
	return y
}
