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
	"universal_control/internal/protocol"
)

// Client runs on the Omarchy machine. It connects to uc-server on the Mac,
// injects received input through uinput, and watches its own left edge to hand
// control back.
type Client struct {
	cfg    Config
	dev    *VirtualDevice
	remote atomic.Bool

	mu   sync.Mutex
	conn net.Conn
	w    *bufio.Writer

	clpMu    sync.Mutex
	lastSet  string
	lastSent string

	stop   chan struct{}
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
	if err := protocol.WriteFrame(c.w, protocol.MsgScreen, protocol.EncodeScreen(0, 0)); err != nil {
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
		switch string(f.Payload) {
		case "remote":
			c.enterRemote()
		case "back":
			c.leaveRemote()
		}
	}
}

// enterRemote starts injecting and watches the left edge for a return.
func (c *Client) enterRemote() {
	if c.remote.Swap(true) {
		return
	}
	log.Printf("remote control: Mac -> Omarchy")
	// Park the virtual cursor at the left edge (keeping its current Y) so it
	// lines up with the Mac's right edge in PBP, then begin edge watching.
	go func() {
		_, y, err := hyprCursorPos()
		if err != nil {
			return
		}
		c.moveCursorAbs(0, y)
	}()
	go c.edgeWatch()
}

// leaveRemote stops edge watching.
func (c *Client) leaveRemote() {
	if !c.remote.Swap(false) {
		return
	}
	log.Printf("remote control: returned to Mac")
}

// edgeWatch polls the cursor and asks the server for control back at the left
// edge. It self-terminates when remote mode ends.
func (c *Client) edgeWatch() {
	t := time.NewTicker(c.cfg.EdgePollInterval)
	defer t.Stop()
	for range t.C {
		if !c.remote.Load() || !c.connected() {
			return
		}
		x, _, err := hyprCursorPos()
		if err != nil {
			continue
		}
		if x <= c.cfg.EdgeMargin {
			c.send(protocol.MsgSwitch, []byte("back"))
			c.leaveRemote()
			return
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
