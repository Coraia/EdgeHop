//go:build linux

package client

import (
	"bufio"
	"errors"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Coraia/EdgeHop/internal/clipsync"
	"github.com/Coraia/EdgeHop/internal/protocol"
	"github.com/Coraia/EdgeHop/internal/secureconn"
)

// Client runs on the Omarchy machine. It connects to EdgeHop on the Mac,
// injects received input through uinput, and watches its own left edge to hand
// control back.
type Client struct {
	cfg        Config
	dev        inputDevice
	auth       *secureconn.Authenticator
	screenSize func() (int, int, error)
	remote     atomic.Bool
	edge       atomic.Uint32
	inputMu    sync.Mutex

	scrW, scrH float64 // client screen size (set during handshake)
	macW, macH float64 // server (Mac) screen size (from handshake)

	mu   sync.Mutex
	conn net.Conn
	w    *bufio.Writer

	clipboard clipsync.Tracker

	stop     chan struct{}
	stopOnce sync.Once
}

type inputDevice interface {
	Close() error
	Key(code uint16, pressed bool) error
	MouseMoveRel(dx, dy int16) error
	MouseButton(button uint8, pressed bool) error
	MouseWheel(dx, dy int16) error
}

// New creates a client. The uinput device is opened immediately.
func New(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	auth, err := secureconn.New(cfg.PairingSecret)
	if err != nil {
		return nil, err
	}
	dev, err := OpenVirtualDevice(cfg.DeviceName)
	if err != nil {
		return nil, err
	}
	c := &Client{
		cfg:        cfg,
		dev:        dev,
		auth:       auth,
		screenSize: hyprScreenSize,
		stop:       make(chan struct{}),
	}
	edge, err := protocol.ParseEdge(cfg.Edge)
	if err != nil {
		edge = protocol.EdgeRight
	}
	c.edge.Store(uint32(edge))
	return c, nil
}

// Close tears down the client.
func (c *Client) Close() error {
	c.stopOnce.Do(func() { close(c.stop) })
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()
	return c.dev.Close()
}

// Run connects to the server and processes messages until stopped.
func (c *Client) Run() error {
	go c.clipboardLoop()
	go c.screenLoop()
	for {
		select {
		case <-c.stop:
			return nil
		default:
		}
		err := c.runOnce()
		if err != nil {
			log.Printf("connection error: %v (reconnecting in 2s)", err)
		}
		select {
		case <-c.stop:
			return nil
		case <-time.After(2 * time.Second):
		}
	}
}

func (c *Client) connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}

func (c *Client) send(typ byte, payload []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if err := protocol.WriteFrame(c.w, typ, payload); err != nil {
		log.Printf("warn: send failed: %v", err)
		return
	}
	if err := c.w.Flush(); err != nil {
		log.Printf("warn: flush failed: %v", err)
	}
}

// runOnce dials, handshakes and serves one connection.
func (c *Client) runOnce() error {
	raw, err := net.DialTimeout("tcp", c.cfg.ServerAddr, 5*time.Second)
	if err != nil {
		return err
	}
	conn, err := c.auth.Connect(raw)
	if err != nil {
		return err
	}
	defer conn.Close()

	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		c.leaveRemote()
	}()

	// Handshake: Hello + our screen size, then read the server's screen size.
	w := bufio.NewWriter(conn)
	if err := protocol.WriteFrame(w, protocol.MsgHello, []byte(protocol.Version)); err != nil {
		return err
	}
	cw, ch, err := c.screenSize()
	if err != nil {
		cw, ch = 0, 0
		log.Printf("warn: screen detection failed: %v", err)
	}
	c.mu.Lock()
	c.scrW, c.scrH = float64(cw), float64(ch)
	c.mu.Unlock()
	if err := protocol.WriteFrame(w, protocol.MsgScreen, protocol.EncodeScreen(int32(cw), int32(ch))); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}

	r := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	hello, err := protocol.ReadFrame(r)
	if err != nil || hello.Type != protocol.MsgHello || string(hello.Payload) != protocol.Version {
		return errors.New("handshake: bad hello from server")
	}
	scr, err := protocol.ReadFrame(r)
	if err != nil || scr.Type != protocol.MsgScreen {
		return errors.New("handshake: missing screen from server")
	}
	sw, sh, err := protocol.DecodeScreen(scr.Payload)
	if err != nil {
		return err
	}
	_ = conn.SetReadDeadline(time.Time{})
	c.mu.Lock()
	c.conn = conn
	c.w = w
	c.macW, c.macH = float64(sw), float64(sh)
	c.mu.Unlock()
	log.Printf("connected to EdgeHop at %s (server screen %dx%d)", c.cfg.ServerAddr, sw, sh)

	for {
		f, err := protocol.ReadFrame(r)
		if err != nil {
			return err
		}
		c.handle(f)
	}
}

// handle dispatches one frame from the server.
func (c *Client) handle(f protocol.Frame) {
	if isInputFrame(f.Type) {
		c.inputMu.Lock()
		defer c.inputMu.Unlock()
		if !c.remote.Load() {
			return
		}
	}
	switch f.Type {
	case protocol.MsgMouseMove:
		dx, dy, err := protocol.DecodeMouseMove(f.Payload)
		if err == nil {
			if e := c.dev.MouseMoveRel(dx, dy); e != nil {
				log.Printf("warn: inject move: %v", e)
			}
		}
	case protocol.MsgMouseButton:
		btn, pressed, err := protocol.DecodeMouseButton(f.Payload)
		if err == nil {
			if e := c.dev.MouseButton(btn, pressed == 1); e != nil {
				log.Printf("warn: inject button: %v", e)
			}
		}
	case protocol.MsgMouseWheel:
		dx, dy, err := protocol.DecodeMouseWheel(f.Payload)
		if err == nil {
			if e := c.dev.MouseWheel(dx, dy); e != nil {
				log.Printf("warn: inject wheel: %v", e)
			}
		}
	case protocol.MsgKey:
		code, pressed, err := protocol.DecodeKey(f.Payload)
		if err == nil {
			if e := c.dev.Key(code, pressed == 1); e != nil {
				log.Printf("warn: inject key: %v", e)
			}
		}
	case protocol.MsgClipboard:
		c.applyClipboard(f.Payload)
	case protocol.MsgClientEdge:
		edge, err := protocol.DecodeClientEdge(f.Payload)
		if err == nil {
			c.edge.Store(uint32(edge))
			log.Printf("device layout updated: shared client edge=%s", edge)
		}
	case protocol.MsgSwitch:
		message, err := protocol.DecodeSwitch(f.Payload)
		if err == nil {
			c.handleSwitch(message)
		}
	case protocol.MsgScreen:
		w, h, err := protocol.DecodeScreen(f.Payload)
		if err == nil && w > 0 && h > 0 {
			c.mu.Lock()
			c.macW, c.macH = float64(w), float64(h)
			c.mu.Unlock()
		}
	}
}

func isInputFrame(typ byte) bool {
	switch typ {
	case protocol.MsgMouseMove, protocol.MsgMouseButton, protocol.MsgMouseWheel, protocol.MsgKey:
		return true
	default:
		return false
	}
}

// handleSwitch processes a validated control-mode switch request.
func (c *Client) handleSwitch(message protocol.Switch) {
	if message.Direction == protocol.SwitchRemote {
		edgeY := -1.0
		if message.HasY {
			edgeY = float64(message.Y)
		}
		c.enterRemote(edgeY)
		return
	}
	if message.Direction == protocol.SwitchBack {
		c.leaveRemote()
	}
}

// enterRemote starts injecting and watches the shared edge for a return.
// edgeY is the Mac-side Y where the cursor crossed (proportionally mapped onto
// this screen), or -1 to keep the current Y.
func (c *Client) enterRemote(edgeY float64) {
	if c.remote.Swap(true) {
		return
	}
	log.Printf("remote control: Mac -> Omarchy")
	// Park the virtual cursor just inside the shared edge. When the Mac told us
	// where its cursor crossed (edgeY), land at the same Y (proportionally
	// mapped) so the crossing feels like a seamless slide; otherwise keep the
	// current Y. Then begin edge watching.
	go func() {
		x, y, err := hyprCursorPos()
		if err != nil {
			log.Printf("warn: park: cursorpos failed: %v", err)
			return
		}
		if edgeY >= 0 {
			y = c.mapMacY(edgeY)
		}
		c.moveCursorAbs(c.parkX(), y)
		log.Printf("parked cursor at x=%.0f y=%.0f (was x=%.0f)", c.parkX(), y, x)
	}()
	go c.edgeWatch()
}

// mapMacY maps a Mac-side Y onto this screen's height by proportion.
func (c *Client) mapMacY(y float64) float64 {
	_, scrH, _, macH := c.screenGeometry()
	if scrH > 0 && macH > 0 {
		return y * scrH / macH
	}
	return y
}

// parkX returns the X coordinate to park the virtual cursor at when entering
// remote: just inside the shared edge so the cursor stays visible and near the
// PBP boundary without tripping the return detector.
func (c *Client) parkX() float64 {
	const inset = 24.0
	if c.currentClientEdge() == protocol.EdgeLeft {
		return inset
	}
	scrW, _, _, _ := c.screenGeometry()
	return scrW - inset
}

// leaveRemote stops edge watching.
func (c *Client) leaveRemote() {
	if !c.remote.Swap(false) {
		return
	}
	log.Printf("remote control: returned to Mac")
	// Release every key and mouse button that may still be held down on the
	// client. Without this, a hotkey (e.g. Cmd+Shift+Space) pressed to return
	// could leave modifiers stuck on Omarchy.
	c.inputMu.Lock()
	c.releaseAll()
	c.inputMu.Unlock()
}

// releaseAll sends key-up for every possible keycode and releases every mouse
// button on the virtual device. Sending a key-up for a key that is not pressed
// is harmless on uinput, so this is a reliable way to clear any stuck state.
func (c *Client) releaseAll() {
	if c.dev == nil {
		return
	}
	for code := 0; code < 256; code++ {
		if err := c.dev.Key(uint16(code), false); err != nil {
			log.Printf("warn: release key %d: %v", code, err)
		}
	}
	for _, b := range []uint8{protocol.ButtonLeft, protocol.ButtonRight, protocol.ButtonMiddle} {
		if err := c.dev.MouseButton(b, false); err != nil {
			log.Printf("warn: release button %d: %v", b, err)
		}
	}
}

// edgeWatch polls the cursor and asks the server for control back when the
// cursor dwells at the shared edge. It self-terminates when remote mode ends.
//
// A dwell is required (not a single poll) so that simply passing through the
// edge zone — e.g. the cursor being parked near the edge when remote control
// starts — does not bounce control back immediately.
func (c *Client) edgeWatch() {
	// Give enterRemote's parking a moment to move the cursor inside the shared
	// edge, so a cursor already sitting at the edge (e.g. from the previous
	// remote session) does not immediately bounce control back.
	time.Sleep(300 * time.Millisecond)
	const dwell = 400 * time.Millisecond
	t := time.NewTicker(c.cfg.EdgePollInterval)
	defer t.Stop()
	var inZoneSince time.Time
	for range t.C {
		if !c.remote.Load() || !c.connected() {
			return
		}
		x, _, err := hyprCursorPos()
		if err != nil {
			continue
		}
		edge := c.currentClientEdge()
		atEdge := x <= c.cfg.EdgeMargin
		scrW, _, _, _ := c.screenGeometry()
		if edge == protocol.EdgeRight {
			atEdge = x >= scrW-c.cfg.EdgeMargin
		}
		if atEdge {
			if inZoneSince.IsZero() {
				inZoneSince = time.Now()
				continue
			}
			if time.Since(inZoneSince) >= dwell {
				log.Printf("edge dwell: x=%.0f (scrW=%.0f margin=%.0f edge=%s)", x, scrW, c.cfg.EdgeMargin, edge)
				// Tell the server where we crossed so the Mac cursor lands at
				// the same Y (proportionally) instead of a fixed park point.
				_, y, errY := hyprCursorPos()
				if errY != nil {
					c.send(protocol.MsgSwitch, protocol.EncodeSwitch(protocol.Switch{
						Direction: protocol.SwitchBack,
					}))
				} else {
					c.send(protocol.MsgSwitch, protocol.EncodeSwitch(protocol.Switch{
						Direction: protocol.SwitchBack,
						Y:         int32(y),
						HasY:      true,
					}))
				}
				c.leaveRemote()
				return
			}
		} else {
			inZoneSince = time.Time{}
		}
	}
}

func (c *Client) screenLoop() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if c.connected() {
				c.refreshScreen()
			}
		case <-c.stop:
			return
		}
	}
}

func (c *Client) refreshScreen() {
	w, h, err := c.screenSize()
	if err != nil || w <= 0 || h <= 0 {
		return
	}
	c.mu.Lock()
	changed := c.scrW != float64(w) || c.scrH != float64(h)
	c.scrW, c.scrH = float64(w), float64(h)
	c.mu.Unlock()
	if changed {
		c.send(protocol.MsgScreen, protocol.EncodeScreen(int32(w), int32(h)))
		log.Printf("screen geometry updated: %dx%d", w, h)
	}
}

func (c *Client) currentClientEdge() protocol.Edge {
	edge := protocol.Edge(c.edge.Load())
	if edge != protocol.EdgeLeft && edge != protocol.EdgeRight {
		return protocol.EdgeRight
	}
	return edge
}

func (c *Client) screenGeometry() (scrW, scrH, macW, macH float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scrW, c.scrH, c.macW, c.macH
}

// moveCursorAbs nudges the virtual cursor to (x, y) using a feedback loop,
// because uinput relative moves are subject to pointer acceleration.
func (c *Client) moveCursorAbs(x, y float64) {
	for i := 0; i < 40; i++ {
		cx, cy, err := hyprCursorPos()
		if err != nil {
			return
		}
		dx := int16(clamp(float64(x)-cx, -200, 200))
		dy := int16(clamp(float64(y)-cy, -200, 200))
		if dx == 0 && dy == 0 {
			return
		}
		c.inputMu.Lock()
		if !c.remote.Load() {
			c.inputMu.Unlock()
			return
		}
		if err := c.dev.MouseMoveRel(dx, dy); err != nil {
			c.inputMu.Unlock()
			return
		}
		c.inputMu.Unlock()
		time.Sleep(15 * time.Millisecond)
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
