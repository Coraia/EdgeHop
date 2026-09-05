//go:build linux

package client

import (
	"github.com/Coraia/EdgeHop/internal/protocol"
	"log"
	"os/exec"
	"strings"
	"time"
)

// readClipboard returns the current Wayland clipboard text via wl-paste.
func readClipboard() (string, bool) {
	ensureWaylandEnv()
	out, err := exec.Command("wl-paste", "--no-newline").Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

// writeClipboard sets the Wayland clipboard via wl-copy.
func writeClipboard(text string) {
	ensureWaylandEnv()
	cmd := exec.Command("wl-copy")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		log.Printf("warn: wl-copy failed: %v", err)
	}
}

// clipboardLoop polls the Wayland clipboard and forwards changes to the server.
func (c *Client) clipboardLoop() {
	t := time.NewTicker(c.cfg.ClipInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-c.stop:
			return
		}
		if !c.connected() {
			continue
		}
		cur, ok := readClipboard()
		if !ok || cur == "" {
			continue
		}
		if !c.clipboard.ShouldSend(cur) {
			continue
		}
		c.send(protocol.MsgClipboard, []byte(cur))
	}
}

// applyClipboard writes server clipboard content to the Wayland clipboard
// without echoing it back.
func (c *Client) applyClipboard(payload []byte) {
	if len(payload) == 0 {
		return
	}
	text := string(payload)
	c.clipboard.AppliedRemote(text)
	writeClipboard(text)
}
