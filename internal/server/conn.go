package server

import (
	"bufio"
	"net"
	"sync"
	"time"
	"universal_control/internal/protocol"
)

// clientConn is a netConn backed by a TCP socket. Writes are serialized and
// flushed per message; a short write deadline prevents wedging on dead peers.
type clientConn struct {
	c  net.Conn
	w  *bufio.Writer
	mu sync.Mutex
}

func newClientConn(c net.Conn) *clientConn {
	return &clientConn{c: c, w: bufio.NewWriter(c)}
}

// Send writes one frame and flushes.
func (cc *clientConn) Send(typ byte, payload []byte) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if err := cc.c.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	if err := protocol.WriteFrame(cc.w, typ, payload); err != nil {
		return err
	}
	return cc.w.Flush()
}

// readLoop decodes frames from the connection until failure, then closes ch.
func readLoop(c net.Conn, ch chan<- protocol.Frame) {
	defer close(ch)
	r := bufio.NewReader(c)
	for {
		f, err := protocol.ReadFrame(r)
		if err != nil {
			return
		}
		select {
		case ch <- f:
		default: // reader too slow: drop oldest is complex; drop frame instead
		}
	}
}
