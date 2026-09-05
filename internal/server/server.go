package server

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"time"
	"universal_control/internal/protocol"
	"universal_control/internal/secureconn"
)

// Run starts the macOS server: the event tap, the engine, and the network
// accept/dial loop. It blocks until the process is terminated.
func Run(cfg Config) error {
	return RunWithStatus(cfg, nil)
}

// RunWithStatus is like Run but reports state changes through onStatus (used
// by the menu-bar app). The event tap is retried until it succeeds, so
// granting Accessibility later takes effect without a restart.
func RunWithStatus(cfg Config, onStatus func(Status)) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	auth, err := secureconn.New(cfg.PairingSecret)
	if err != nil {
		return err
	}
	initDisplay()
	e := newEngine(cfg)
	e.onEdgeUI = func(on bool, barLen float64) {
		setStickyOverlay(on, barLen, cfg.RemoteEdge == EdgeRight)
	}
	if onStatus != nil {
		e.setOnStatus(onStatus)
	}

	// Event tap on a dedicated goroutine with retry (never exits the process).
	// While Accessibility permission is missing we wait quietly instead of
	// retrying tap creation: each untrusted CGEventTapCreate attempt makes
	// macOS pop an authorization dialog, which would stack up dialogs.
	go func() {
		for {
			if !isAccessibilityTrusted() {
				e.reportError(errors.New("waiting for Accessibility permission (System Settings > Privacy & Security > Accessibility)"))
				time.Sleep(5 * time.Second)
				continue
			}
			e.updateStatus(func(s *Status) { s.LastError = "" })
			if err := startEventTap(e.consume); err != nil {
				e.reportError(err)
				time.Sleep(2 * time.Second)
				continue
			}
			return // startEventTap blocks on CFRunLoopRun until stopped
		}
	}()

	// Engine drives modes/clipboard.
	go e.run()

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return err
	}
	e.updateStatus(func(s *Status) {
		s.ServerRunning = true
		s.LastError = ""
	})
	log.Printf("uc-server listening on %s (remote edge: %s)", cfg.ListenAddr, cfg.RemoteEdge)
	log.Printf("grant Accessibility permission if prompted; set remote edge to %q if Omarchy sits on the left", cfg.RemoteEdge)

	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go func() {
			secure, err := auth.Accept(c)
			if err != nil {
				log.Printf("rejected connection from %s: %v", c.RemoteAddr(), err)
				return
			}
			e.handleConn(secure)
		}()
	}
}

// handleConn performs the handshake and then relays messages for one client.
func (e *engine) handleConn(c net.Conn) {
	cc := newClientConn(c)
	r := bufio.NewReader(c)

	// Handshake with timeout.
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	hello, err := protocol.ReadFrame(r)
	if err != nil || hello.Type != protocol.MsgHello || string(hello.Payload) != protocol.Version {
		log.Printf("handshake failed from %s", c.RemoteAddr())
		c.Close()
		return
	}
	scr, err := protocol.ReadFrame(r)
	if err != nil || scr.Type != protocol.MsgScreen {
		log.Printf("missing screen info from %s", c.RemoteAddr())
		c.Close()
		return
	}
	cliW, cliH, err := protocol.DecodeScreen(scr.Payload)
	if err != nil || cliW < 0 || cliH < 0 {
		if err == nil {
			err = fmt.Errorf("invalid screen size %dx%d", cliW, cliH)
		}
		log.Printf("bad screen payload from %s: %v", c.RemoteAddr(), err)
		c.Close()
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	e.setClientScreen(cliW, cliH)

	sc := screen()
	e.refreshScreen()
	if err := cc.Send(protocol.MsgHello, []byte(protocol.Version)); err != nil {
		c.Close()
		return
	}
	if err := cc.Send(protocol.MsgScreen, protocol.EncodeScreen(int32(sc.W), int32(sc.H))); err != nil {
		c.Close()
		return
	}

	log.Printf("client ready: %s (client screen %dx%d)", c.RemoteAddr(), cliW, cliH)
	e.setConn(cc)
	e.updateStatus(func(s *Status) { s.ClientConnected = true })
	// Re-sync control mode to the new client: if we are already in remote mode
	// (e.g. the client reconnected while we were controlling Omarchy), tell it
	// so its edge-watch / input path starts correctly.
	if e.isRemote() {
		_ = cc.Send(protocol.MsgSwitch, protocol.EncodeSwitch(protocol.Switch{
			Direction: protocol.SwitchRemote,
		}))
		log.Printf("re-synced remote mode to reconnected client")
	}
	e.recvLoop(r)

	e.dropConn(cc)
	log.Printf("client disconnected: %s", c.RemoteAddr())
}
