package server

import (
	"sync"
	"testing"
	"time"
	"universal_control/internal/protocol"
)

func TestDisconnectRestoresLocalPointerAssociation(t *testing.T) {
	restorePlatform := installTestPlatform()
	defer restorePlatform()

	cfg := DefaultConfig()
	e := newEngine(cfg)
	conn := &recordingConn{}
	e.setConn(conn)

	var associations []bool
	setMouseAssoc = func(associated bool) {
		associations = append(associations, associated)
	}

	e.enterRemote()
	e.dropConn(conn)

	if e.isRemote() {
		t.Fatal("engine remained remote after disconnect")
	}
	want := []bool{false, true}
	if len(associations) != len(want) {
		t.Fatalf("pointer association calls = %v, want %v", associations, want)
	}
	for i := range want {
		if associations[i] != want[i] {
			t.Fatalf("pointer association calls = %v, want %v", associations, want)
		}
	}
}

func TestLocalHotkeySwitchesOnlyAfterKeysAreReleased(t *testing.T) {
	restorePlatform := installTestPlatform()
	defer restorePlatform()

	cfg := DefaultConfig()
	cfg.SwitchKeys = []int{55, 56, 49}
	e := newEngine(cfg)
	e.setConn(&recordingConn{})

	e.consume(rawEvent{ctype: cgEventFlagsChanged, keycode: 55, flags: cgFlagCommand})
	e.consume(rawEvent{ctype: cgEventFlagsChanged, keycode: 56, flags: cgFlagCommand | cgFlagShift})
	if consumed := e.consume(rawEvent{ctype: cgEventKeyDown, keycode: 49}); !consumed {
		t.Fatal("hotkey trigger key was not consumed")
	}
	if e.isRemote() {
		t.Fatal("engine switched before local hotkey keys were released")
	}

	if consumed := e.consume(rawEvent{ctype: cgEventKeyUp, keycode: 49}); consumed {
		t.Fatal("local key release was consumed")
	}
	e.consume(rawEvent{ctype: cgEventFlagsChanged, keycode: 56, flags: cgFlagCommand})
	if e.isRemote() {
		t.Fatal("engine switched while Command was still held")
	}
	e.consume(rawEvent{ctype: cgEventFlagsChanged, keycode: 55, flags: 0})
	if !e.isRemote() {
		t.Fatal("engine did not switch after all local hotkey keys were released")
	}
}

func TestQueuedFramesStayWithTheirOriginalConnection(t *testing.T) {
	restorePlatform := installTestPlatform()
	defer restorePlatform()

	e := newEngine(DefaultConfig())
	oldConn := &recordingConn{blocked: make(chan struct{})}
	newConn := &recordingConn{}
	e.setConn(oldConn)
	go e.writer()

	e.send(protocol.MsgKey, protocol.EncodeKey(30, 1))
	deadline := time.After(time.Second)
	for oldConn.sendCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("writer did not start sending to old connection")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	e.send(protocol.MsgKey, protocol.EncodeKey(30, 0))
	e.setConn(newConn)
	close(oldConn.blocked)

	time.Sleep(20 * time.Millisecond)
	if got := newConn.sendCount(); got != 0 {
		t.Fatalf("new connection received %d frame(s) queued for old session", got)
	}
}

func TestInitialStatusReportsLocalMode(t *testing.T) {
	e := newEngine(DefaultConfig())
	var got Status
	e.setOnStatus(func(status Status) {
		got = status
	})

	e.updateStatus(func(status *Status) {
		status.ServerRunning = true
	})

	if got.Mode != "local" {
		t.Fatalf("initial mode = %q, want local", got.Mode)
	}
}

func TestCapsLockSendsACompletePulseForEveryToggle(t *testing.T) {
	restorePlatform := installTestPlatform()
	defer restorePlatform()

	e := newEngine(DefaultConfig())
	conn := &recordingConn{}
	e.setConn(conn)
	e.switchTo(ModeRemote)
	go e.writer()

	e.forwardRemote(rawEvent{ctype: cgEventFlagsChanged, keycode: 0x39, flags: cgFlagAlphaShift})
	e.forwardRemote(rawEvent{ctype: cgEventFlagsChanged, keycode: 0x39, flags: 0})

	frames := conn.waitForFrames(t, 4)
	wantPressed := []uint8{1, 0, 1, 0}
	for i, frame := range frames {
		code, pressed, err := protocol.DecodeKey(frame.payload)
		if err != nil {
			t.Fatalf("decode frame %d: %v", i, err)
		}
		if code != 58 || pressed != wantPressed[i] {
			t.Fatalf("frame %d = key %d pressed %d, want key 58 pressed %d", i, code, pressed, wantPressed[i])
		}
	}
}

func TestLeftAndRightShiftReleaseIndependently(t *testing.T) {
	restorePlatform := installTestPlatform()
	defer restorePlatform()

	e := newEngine(DefaultConfig())
	conn := &recordingConn{}
	e.setConn(conn)
	e.switchTo(ModeRemote)
	go e.writer()

	e.forwardRemote(rawEvent{ctype: cgEventFlagsChanged, keycode: 0x38, flags: cgFlagShift})
	e.forwardRemote(rawEvent{ctype: cgEventFlagsChanged, keycode: 0x3C, flags: cgFlagShift})
	e.forwardRemote(rawEvent{ctype: cgEventFlagsChanged, keycode: 0x38, flags: cgFlagShift})

	frames := conn.waitForFrames(t, 3)
	want := []struct {
		code    uint16
		pressed uint8
	}{
		{42, 1},
		{54, 1},
		{42, 0},
	}
	for i, frame := range frames {
		code, pressed, err := protocol.DecodeKey(frame.payload)
		if err != nil {
			t.Fatalf("decode frame %d: %v", i, err)
		}
		if code != want[i].code || pressed != want[i].pressed {
			t.Fatalf("frame %d = key %d pressed %d, want key %d pressed %d", i, code, pressed, want[i].code, want[i].pressed)
		}
	}
}

func TestEdgeSwitchWaitsForLocalMouseRelease(t *testing.T) {
	restorePlatform := installTestPlatform()
	defer restorePlatform()

	e := newEngine(DefaultConfig())
	e.scrW, e.scrH = 1920, 1080
	e.setConn(&recordingConn{})
	mousePos = func() (float64, float64) { return 1919, 540 }

	e.watchLocal(rawEvent{ctype: cgEventLeftMouseDown})
	e.watchLocal(rawEvent{ctype: cgEventLeftMouseDragged, dx: 8})
	e.breakFree()
	if e.isRemote() {
		t.Fatal("engine switched while the Mac mouse button was still held")
	}

	if consumed := e.watchLocal(rawEvent{ctype: cgEventLeftMouseUp}); consumed {
		t.Fatal("local mouse release was consumed")
	}
	if !e.isRemote() {
		t.Fatal("engine did not switch after the local mouse button was released")
	}
}

func TestScreenRefreshUpdatesEdgeGeometry(t *testing.T) {
	restorePlatform := installTestPlatform()
	defer restorePlatform()

	e := newEngine(DefaultConfig())
	e.scrW, e.scrH = 1920, 1080
	e.setConn(&recordingConn{})
	screen = func() screenSize { return screenSize{W: 2560, H: 1440} }
	mousePos = func() (float64, float64) { return 1919, 720 }

	e.refreshScreen()
	e.consume(rawEvent{ctype: cgEventMouseMoved, dx: 1})

	if e.sticky {
		t.Fatal("old display edge remained active after geometry changed")
	}
}

func TestConcurrentStickyBreakAndDisconnectRemainsLocal(t *testing.T) {
	restorePlatform := installTestPlatform()
	defer restorePlatform()

	for i := 0; i < 100; i++ {
		e := newEngine(DefaultConfig())
		conn := &recordingConn{}
		e.setConn(conn)
		e.sticky = true

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			e.breakFree()
		}()
		go func() {
			defer wg.Done()
			e.dropConn(conn)
		}()
		wg.Wait()

		if e.isRemote() {
			t.Fatalf("iteration %d remained remote after disconnect", i)
		}
	}
}

func TestFullSendQueueClosesSessionInsteadOfDroppingKeyRelease(t *testing.T) {
	e := newEngine(DefaultConfig())
	conn := &recordingConn{}
	e.setConn(conn)
	for i := 0; i < cap(e.sendQ); i++ {
		e.send(protocol.MsgMouseMove, protocol.EncodeMouseMove(1, 0))
	}

	e.send(protocol.MsgKey, protocol.EncodeKey(30, 0))

	if conn.closeCount() == 0 {
		t.Fatal("full queue dropped a key release without closing the session")
	}
}

func TestDisconnectClearsHeldInputState(t *testing.T) {
	e := newEngine(DefaultConfig())
	conn := &recordingConn{}
	e.setConn(conn)
	e.keysDown[55] = true
	e.buttonsDown[0] = true
	e.hotkeyLocked = true
	e.pendingRemote = true

	e.dropConn(conn)

	if len(e.keysDown) != 0 || len(e.buttonsDown) != 0 || e.hotkeyLocked || e.pendingRemote {
		t.Fatal("disconnect retained held input state")
	}
}

type recordingConn struct {
	mu      sync.Mutex
	sends   int
	closes  int
	blocked chan struct{}
	frames  []frameOut
}

func (c *recordingConn) Send(typ byte, payload []byte) error {
	c.mu.Lock()
	c.sends++
	c.frames = append(c.frames, frameOut{typ: typ, payload: append([]byte(nil), payload...)})
	c.mu.Unlock()
	if c.blocked != nil {
		<-c.blocked
	}
	return nil
}

func (c *recordingConn) Close() error {
	c.mu.Lock()
	c.closes++
	c.mu.Unlock()
	return nil
}

func (c *recordingConn) sendCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sends
}

func (c *recordingConn) closeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closes
}

func (c *recordingConn) waitForFrames(t *testing.T, count int) []frameOut {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		if len(c.frames) >= count {
			frames := append([]frameOut(nil), c.frames...)
			c.mu.Unlock()
			return frames
		}
		c.mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatalf("received %d frame(s), want %d", c.sendCount(), count)
		}
		time.Sleep(time.Millisecond)
	}
}

func installTestPlatform() func() {
	oldScreen := screen
	oldMousePos := mousePos
	oldWarpMouse := warpMouse
	oldHideCursor := hideCursor
	oldShowCursor := showCursor
	oldCursorVisible := cursorVisible
	oldWarpOffscreen := warpOffscreen
	oldSetMouseAssoc := setMouseAssoc

	screen = func() screenSize { return screenSize{W: 1920, H: 1080} }
	mousePos = func() (float64, float64) { return 0, 540 }
	warpMouse = func(float64, float64) {}
	hideCursor = func() {}
	showCursor = func() {}
	cursorVisible = func() int { return 1 }
	warpOffscreen = func(float64, float64) {}
	setMouseAssoc = func(bool) {}

	return func() {
		screen = oldScreen
		mousePos = oldMousePos
		warpMouse = oldWarpMouse
		hideCursor = oldHideCursor
		showCursor = oldShowCursor
		cursorVisible = oldCursorVisible
		warpOffscreen = oldWarpOffscreen
		setMouseAssoc = oldSetMouseAssoc
	}
}
