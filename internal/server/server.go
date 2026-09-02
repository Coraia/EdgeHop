package server

import (
	"log"
	"net"
	"time"
	"universal_control/internal/protocol"
)

// Run starts the macOS server: the event tap, the engine, and the network
// accept/dial loop. It blocks until the process is terminated.
func Run(cfg Config) error {
	initDisplay()
	e := newEngine(cfg)

	// Event tap on a dedicated goroutine (blocks on CFRunLoopRun).
	go func() {
		if err := startEventTap(e.consume); err != nil {
			log.Fatalf("fatal: %v", err)
		}
	}()

	// Engine consumes tap events and drives modes/clipboard.
	go e.run()

	// Optionally dial the client (if ClientAddr configured) with reconnects.
	if cfg.ClientAddr != "" {
		go e.dialLoop(cfg.ClientAddr)
	}

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return err
	}
	log.Printf("uc-server listening on %s (remote edge: %s)", cfg.ListenAddr, cfg.RemoteEdge)
	log.Printf("grant Accessibility permission if prompted; set remote edge to %q if Omarchy sits on the left", cfg.RemoteEdge)

	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go e.handleConn(c)
	}
}

// dialLoop keeps one client connection alive by dialing ClientAddr.
func (e *engine) dialLoop(addr string) {
	for {
		c, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			log.Printf("connect %s failed: %v (retrying in 3s)", addr, err)
			time.Sleep(3 * time.Second)
			continue
		}
		e.handleConn(c)
	}
}

// handleConn performs the handshake and then relays messages for one client.
func (e *engine) handleConn(c net.Conn) {
	cc := newClientConn(c)
	frameCh := make(chan protocol.Frame, 128)
	go readLoop(c, frameCh)

	// Handshake with timeout.
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	hello, ok := <-frameCh
	if !ok || hello.Type != protocol.MsgHello {
		log.Printf("handshake failed from %s", c.RemoteAddr())
		c.Close()
		return
	}
	scr, ok := <-frameCh
	if !ok || scr.Type != protocol.MsgScreen {
		log.Printf("missing screen info from %s", c.RemoteAddr())
		c.Close()
		return
	}
	cliW, cliH, err := protocol.DecodeScreen(scr.Payload)
	if err != nil {
		log.Printf("bad screen payload from %s: %v", c.RemoteAddr(), err)
		c.Close()
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	e.setClientScreen(cliW, cliH)

	sc := screen()
	if err := cc.Send(protocol.MsgHello, []byte("universal-control/1")); err != nil {
		c.Close()
		return
	}
	if err := cc.Send(protocol.MsgScreen, protocol.EncodeScreen(int32(sc.W), int32(sc.H))); err != nil {
		c.Close()
		return
	}

	log.Printf("client ready: %s (client screen %dx%d)", c.RemoteAddr(), cliW, cliH)
	e.setConn(cc)
	e.recvLoop(cc, frameCh)

	e.dropConn(cc)
	_ = c.Close()
	log.Printf("client disconnected: %s", c.RemoteAddr())
}
