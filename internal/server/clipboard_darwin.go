//go:build darwin

package server

import (
	"log"
	"os/exec"
	"strings"
	"time"
	"universal_control/internal/protocol"
)

// readClipboard returns the current text on the macOS clipboard.
// ok=false when the clipboard is empty or unreadable.
func readClipboard() (string, bool) {
	out, err := exec.Command("/usr/bin/pbpaste").Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

// writeClipboard replaces the macOS clipboard with text.
func writeClipboard(text string) {
	cmd := exec.Command("/usr/bin/pbcopy")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		log.Printf("warn: pbcopy failed: %v", err)
	}
}

// clipboardLoop polls the macOS clipboard and forwards new local changes.
func (e *engine) clipboardLoop() {
	t := time.NewTicker(e.cfg.ClipboardPollInterval)
	defer t.Stop()
	for range t.C {
		if !e.clientConnected() {
			continue
		}
		cur, ok := readClipboard()
		if !ok || cur == "" {
			continue
		}
		if !e.clipboard.ShouldSend(cur) {
			continue
		}
		e.send(protocol.MsgClipboard, []byte(cur))
	}
}

// applyClipboard writes client clipboard content onto the macOS clipboard and
// marks it as locally set so the poller does not echo it back.
func (e *engine) applyClipboard(payload []byte) {
	if len(payload) == 0 {
		return
	}
	text := string(payload)
	e.clipboard.AppliedRemote(text)
	writeClipboard(text)
}
