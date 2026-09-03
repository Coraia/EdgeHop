package server

import (
	"log"
	"sync"
	"sync/atomic"
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
	cfg    Config
	mode   atomic.Int32
	remote atomic.Bool // fast path: whether we are currently forwarding

	mu    sync.Mutex
	conn  netConn // current client connection writer (nil if disconnected)
	scrW  int32   // Mac screen size (cached)
	scrH  int32
	cliW  int32 // client screen size (from handshake)
	cliH  int32

	swKeys   map[int]bool // hotkey codes for manual toggle
	keysDown map[int]bool // mac keycodes currently down

	clpMu    sync.Mutex
	lastSet  string // clipboard content last written locally (from client)
	lastSent string // clipboard content last forwarded to the client

	statusMu sync.Mutex
	status   Status
	onStatus func(Status)

	sendQ chan frameOut // buffered frames to client writer
}

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
}

type frameOut struct {
	typ     byte
	payload []byte
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

// remoteEdgeIsRight reports whether the client sits on the Mac's right edge.
func (e *engine) remoteEdgeIsRight() bool {
	return e.cfg.RemoteEdge == EdgeRight
}

// newEngine builds an engine from config and wires platform functions.
func newEngine(cfg Config) *engine {
	e := &engine{
		cfg:      cfg,
		sendQ:    make(chan frameOut, 4096),
		swKeys:   make(map[int]bool, len(cfg.SwitchKeys)),
		keysDown: make(map[int]bool),
	}
	for _, k := range cfg.SwitchKeys {
		e.swKeys[k] = true
	}
	e.remote.Store(false)
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
	e.conn = c
	e.mu.Unlock()
}

// dropConn clears the connection only if it is the current one.
func (e *engine) dropConn(c netConn) {
	e.mu.Lock()
	if e.conn == c {
		e.conn = nil
	}
	gone := e.conn == nil
	e.mu.Unlock()
	if gone {
		// Lost the client: force back to local control.
		e.switchTo(ModeLocal)
		if showCursor != nil {
			showCursor()
		}
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

	s := screen()
	e.scrW, e.scrH = int32(s.W), int32(s.H)
	log.Printf("mac screen: %dx%d", e.scrW, e.scrH)

	go e.clipboardLoop()
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
	// Track held keys for hotkey detection.
	switch ev.ctype {
	case cgEventKeyDown:
		e.keysDown[int(ev.keycode)] = true
	case cgEventKeyUp:
		delete(e.keysDown, int(ev.keycode))
	case cgEventFlagsChanged:
		down := ev.flags&flagForMacKey(int(ev.keycode)) != 0
		if down {
			e.keysDown[int(ev.keycode)] = true
		} else {
			delete(e.keysDown, int(ev.keycode))
		}
	}

	// Edge switch on mouse move.
	if ev.ctype == cgEventMouseMoved || ev.ctype == cgEventLeftMouseDragged ||
		ev.ctype == cgEventRightMouseDragged || ev.ctype == cgEventOtherMouseDragged {
		if x, _ := mousePos(); e.atRemoteEdge(x) {
			e.enterRemote()
			return true
		}
	}

	// Hotkey toggle.
	if e.hotkeyDown() {
		e.enterRemote()
		return true
	}
	return false
}

// forwardRemote forwards input to the client.
func (e *engine) forwardRemote(ev rawEvent) {
	switch ev.ctype {
	case cgEventKeyDown, cgEventKeyUp:
		e.keysDown[int(ev.keycode)] = ev.ctype == cgEventKeyDown
		if code, ok := keymapToEvdev(int(ev.keycode)); ok {
			p := byte(0)
			if ev.ctype == cgEventKeyDown {
				p = 1
			}
			e.send(protocol.MsgKey, protocol.EncodeKey(code, p))
		}
	case cgEventFlagsChanged:
		down := ev.flags&flagForMacKey(int(ev.keycode)) != 0
		if down {
			e.keysDown[int(ev.keycode)] = true
		} else {
			delete(e.keysDown, int(ev.keycode))
		}
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

	// Return path: hotkey pressed while remote -> back to local.
	if e.hotkeyDown() {
		e.leaveRemote()
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
	if e.isRemote() {
		return
	}
	e.switchTo(ModeRemote)
	e.send(protocol.MsgSwitch, []byte("remote"))
	if hideCursor != nil {
		hideCursor()
	}
	if e.remoteEdgeIsRight() {
		_, y := mousePos()
		warpMouse(e.scrWf()-1, clampY(y))
	}
}

// leaveRemote returns control to the Mac and warps the cursor to the edge.
func (e *engine) leaveRemote() {
	if !e.isRemote() {
		return
	}
	e.switchTo(ModeLocal)
	e.send(protocol.MsgSwitch, []byte("back"))
	if showCursor != nil {
		showCursor()
	}
	_, y := mousePos()
	if e.remoteEdgeIsRight() {
		warpMouse(e.scrWf()-1, clampY(y))
	} else {
		warpMouse(0, clampY(y))
	}
}

// handleSwitchRequest processes a client request to return control.
func (e *engine) handleSwitchRequest(payload []byte) {
	if string(payload) == "back" {
		e.leaveRemote()
	}
}

func (e *engine) scrWf() float64 { return float64(e.scrW) }

func (e *engine) atRemoteEdge(x float64) bool {
	if e.remoteEdgeIsRight() {
		return x >= e.scrWf()-e.cfg.EdgeSensitivity
	}
	return x <= e.cfg.EdgeSensitivity
}

func (e *engine) hotkeyDown() bool {
	if len(e.swKeys) == 0 {
		return false
	}
	for k := range e.swKeys {
		if !e.keysDown[k] {
			return false
		}
	}
	return true
}

// send enqueues a frame for the writer goroutine (non-blocking drop on full).
func (e *engine) send(typ byte, payload []byte) {
	select {
	case e.sendQ <- frameOut{typ: typ, payload: payload}:
	default:
		log.Printf("warn: send queue full, dropping frame type=%d", typ)
	}
}

// writer drains sendQ to the current client connection.
func (e *engine) writer() {
	for f := range e.sendQ {
		e.mu.Lock()
		c := e.conn
		e.mu.Unlock()
		if c == nil {
			continue
		}
		if err := c.Send(f.typ, f.payload); err != nil {
			log.Printf("warn: send failed: %v", err)
		}
	}
}

// recvLoop handles client->server messages on the connection's read side.
// It runs on the connection goroutine; the channel closes on disconnect.
func (e *engine) recvLoop(c netConn, frameCh <-chan protocol.Frame) {
	for f := range frameCh {
		switch f.Type {
		case protocol.MsgClipboard:
			e.applyClipboard(f.Payload)
		case protocol.MsgSwitch:
			e.handleSwitchRequest(f.Payload)
		}
	}
	e.dropConn(c)
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
