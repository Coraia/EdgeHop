// Package clipsync tracks clipboard ownership and suppresses only real echoes.
package clipsync

import "sync"

// Tracker decides whether an observed local clipboard value is a new change.
type Tracker struct {
	mu         sync.Mutex
	lastLocal  string
	remoteEcho string
}

// AppliedRemote records a value that is about to be written locally.
func (t *Tracker) AppliedRemote(text string) {
	t.mu.Lock()
	t.remoteEcho = text
	t.mu.Unlock()
}

// ShouldSend reports whether text is a new local change. A remote value is
// suppressed exactly once, so copying the same text later still synchronizes.
func (t *Tracker) ShouldSend(text string) bool {
	if text == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if text == t.remoteEcho {
		t.remoteEcho = ""
		t.lastLocal = text
		return false
	}
	t.remoteEcho = ""
	if text == t.lastLocal {
		return false
	}
	t.lastLocal = text
	return true
}
