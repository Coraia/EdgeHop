//go:build linux

package client

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"universal_control/internal/protocol"
)

// Client runs on the Omarchy machine. It connects to uc-server on the Mac,
// injects received input through uinput, and watches its own left edge to hand
// control back.
type Client struct {
	cfg    Config
	dev    *VirtualDevice
	remote atomic.Bool

	scrW, scrH float64 // client screen size (set during handshake)
	macW, macH float64 // server (Mac) screen size (from handshake)

	mu   sync.Mutex
	conn net.Conn
	w    *bufio.Writer

	clpMu    sync.Mutex
	lastSet  string
	lastSent string

	stop     chan struct{}
	stopOnce sync.Once
}

// New creates a client. The uinput device is opened immediately.
func New(cfg Config) (*Client, error) {
	dev, err := OpenVirtualDevice(cfg.DeviceName)
	if err != nil {
		return nil, err
	}
	c := &Client{
		cfg:  cfg,
		dev:  dev,
		stop: make(chan struct{}),
	}
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
	conn, err := net.DialTimeout("tcp", c.cfg.ServerAddr, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	c.mu.Lock()
	c.conn = conn
	c.w = bufio.NewWriter(conn)
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		c.leaveRemote()
	}()

	// Handshake: Hello + our screen size, then read the server's screen size.
	if err := protocol.WriteFrame(c.w, protocol.MsgHello, []byte("universal-control/1")); err != nil {
		return err
	}
	cw, ch, err := hyprScreenSize()
	if err != nil {
		cw, ch = 0, 0
		log.Printf("warn: screen detection failed: %v", err)
	}
	c.mu.Lock()
	c.scrW, c.scrH = float64(cw), float64(ch)
	c.mu.Unlock()
	if err := protocol.WriteFrame(c.w, protocol.MsgScreen, protocol.EncodeScreen(int32(cw), int32(ch))); err != nil {
		return err
	}
	if err := c.w.Flush(); err != nil {
		return err
	}

	r := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	hello, err := protocol.ReadFrame(r)
	if err != nil || hello.Type != protocol.MsgHello {
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
	c.macW, c.macH = float64(sw), float64(sh)
	c.mu.Unlock()
	log.Printf("connected to uc-server at %s (server screen %dx%d)", c.cfg.ServerAddr, sw, sh)

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
	case protocol.MsgSwitch:
		c.handleSwitch(string(f.Payload))
	}
}

// handleSwitch processes a control-mode switch request. Payload is "remote"
// (or "remote:<mac-edge-y>") to take control, and "back" (or "back:<y>") to
// return it.
func (c *Client) handleSwitch(s string) {
	if s == "remote" || strings.HasPrefix(s, "remote:") {
		edgeY := -1.0
		if i := strings.IndexByte(s, ':'); i >= 0 {
			if v, err := strconv.ParseFloat(s[i+1:], 64); err == nil {
				edgeY = v
			}
		}
		c.enterRemote(edgeY)
		return
	}
	if s == "back" || strings.HasPrefix(s, "back:") {
		c.leaveRemote()
		return
	}
	log.Printf("warn: unknown switch payload %q", s)
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
	if c.scrH > 0 && c.macH > 0 {
		return y * float64(c.scrH) / float64(c.macH)
	}
	return y
}

// parkX returns the X coordinate to park the virtual cursor at when entering
// remote: just inside the shared edge so the cursor stays visible and near the
// PBP boundary without tripping the return detector.
func (c *Client) parkX() float64 {
	const inset = 24.0
	if c.cfg.Edge == "left" {
		return inset
	}
	return c.scrW - inset
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
	c.releaseAll()
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

// sharedEdgeX returns the X coordinate of the edge shared with the Mac.
func (c *Client) sharedEdgeX() float64 {
	if c.cfg.Edge == "left" {
		return 0
	}
	return c.scrW
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
		atEdge := x <= c.cfg.EdgeMargin
		if c.cfg.Edge == "right" {
			atEdge = x >= c.scrW-c.cfg.EdgeMargin
		}
		if atEdge {
			if inZoneSince.IsZero() {
				inZoneSince = time.Now()
				continue
			}
			if time.Since(inZoneSince) >= dwell {
				log.Printf("edge dwell: x=%.0f (scrW=%.0f margin=%.0f edge=%s)", x, c.scrW, c.cfg.EdgeMargin, c.cfg.Edge)
				// Tell the server where we crossed so the Mac cursor lands at
				// the same Y (proportionally) instead of a fixed park point.
				_, y, errY := hyprCursorPos()
				if errY != nil {
					c.send(protocol.MsgSwitch, []byte("back"))
				} else {
					c.send(protocol.MsgSwitch, []byte(fmt.Sprintf("back:%d", int(y))))
				}
				c.leaveRemote()
				return
			}
		} else {
			inZoneSince = time.Time{}
		}
	}
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
		if err := c.dev.MouseMoveRel(dx, dy); err != nil {
			return
		}
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
