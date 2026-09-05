//go:build linux

package client

import (
	"bufio"
	"github.com/Coraia/EdgeHop/internal/protocol"
	"net"
	"testing"
	"time"
)

func TestInputFramesAreInjectedOnlyWhileRemote(t *testing.T) {
	dev := &recordingInput{}
	c := &Client{dev: dev}
	keyDown := protocol.Frame{
		Type:    protocol.MsgKey,
		Payload: protocol.EncodeKey(30, 1),
	}

	c.handle(keyDown)
	if dev.keyEvents != 0 {
		t.Fatalf("local mode injected %d key event(s)", dev.keyEvents)
	}

	c.remote.Store(true)
	c.handle(keyDown)
	if dev.keyEvents != 1 {
		t.Fatalf("remote mode injected %d key event(s), want 1", dev.keyEvents)
	}

	c.remote.Store(false)
	c.handle(keyDown)
	if dev.keyEvents != 1 {
		t.Fatalf("late local frame injected a key event; total = %d", dev.keyEvents)
	}
}

func TestScreenGeometryRecoversWhenSessionAppears(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	c := &Client{
		dev:  &recordingInput{},
		conn: clientConn,
		w:    bufio.NewWriter(clientConn),
		screenSize: func() (int, int, error) {
			return 1536, 1728, nil
		},
	}

	refreshDone := make(chan struct{})
	go func() {
		c.refreshScreen()
		close(refreshDone)
	}()

	serverConn.SetReadDeadline(time.Now().Add(time.Second))
	frame, err := protocol.ReadFrame(bufio.NewReader(serverConn))
	if err != nil {
		t.Fatalf("read screen refresh: %v", err)
	}
	if frame.Type != protocol.MsgScreen {
		t.Fatalf("refresh type = %d, want MsgScreen", frame.Type)
	}
	w, h, err := protocol.DecodeScreen(frame.Payload)
	if err != nil {
		t.Fatalf("decode screen refresh: %v", err)
	}
	if w != 1536 || h != 1728 {
		t.Fatalf("screen refresh = %dx%d, want 1536x1728", w, h)
	}
	<-refreshDone
}

func TestClientAppliesServerSelectedEdge(t *testing.T) {
	c := &Client{}
	c.edge.Store(uint32(protocol.EdgeRight))

	c.handle(protocol.Frame{
		Type:    protocol.MsgClientEdge,
		Payload: protocol.EncodeClientEdge(protocol.EdgeLeft),
	})

	if got := c.currentClientEdge(); got != protocol.EdgeLeft {
		t.Fatalf("client edge = %d, want left", got)
	}
}

type recordingInput struct {
	keyEvents int
}

func (*recordingInput) Close() error                    { return nil }
func (*recordingInput) MouseMoveRel(int16, int16) error { return nil }
func (*recordingInput) MouseButton(uint8, bool) error   { return nil }
func (*recordingInput) MouseWheel(int16, int16) error   { return nil }
func (d *recordingInput) Key(uint16, bool) error {
	d.keyEvents++
	return nil
}
