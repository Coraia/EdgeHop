package server

import (
	"testing"
	"time"
)

func TestConfigRejectsNonPositiveClipboardInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PairingSecret = []byte("0123456789abcdef0123456789abcdef")

	for _, interval := range []time.Duration{0, -time.Second} {
		cfg.ClipboardPollInterval = interval
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate accepted clipboard interval %s", interval)
		}
	}
}
