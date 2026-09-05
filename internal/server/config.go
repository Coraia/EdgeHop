package server

import (
	"errors"
	"fmt"
	"time"
	"universal_control/internal/secureconn"
)

// Config configures the macOS server.
type Config struct {
	// ListenAddr is the TCP listen address, e.g. "0.0.0.0:24800".
	ListenAddr string

	// PairingSecret authenticates and encrypts the client connection.
	PairingSecret []byte

	// RemoteEdge is which edge of the Mac screen connects to the client.
	// "right" means the Omarchy screen sits to the right (PBP layout with
	// Mac on the left), "left" means it sits to the left.
	RemoteEdge string

	// SwitchKeys is an optional hotkey (list of macOS virtual keycodes that
	// must be held together) to toggle remote control without reaching an
	// edge. Empty disables the hotkey.
	SwitchKeys []int

	// EdgeSensitivity is the margin (in points) from the screen edge that
	// triggers the sticky state. Larger values make the light bar appear
	// earlier and give the cursor more room to "stick".
	EdgeSensitivity float64

	// StickyDwellMin is the dwell required to break free when the cursor is
	// pressed hard against the seam (closeness = 1).
	StickyDwellMin time.Duration

	// StickyDwellMax is the dwell required when the cursor only just entered
	// the sticky zone (closeness = 0). Dwell interpolates between these two
	// values as the cursor approaches the edge, so a light touch needs longer
	// before switching (anti-accidental) and a firm press switches sooner.
	StickyDwellMax time.Duration

	// StickyPush is the outward distance (points) the user must push past the
	// edge to break free immediately (instead of waiting for the dwell).
	StickyPush float64

	// ClipboardPollInterval is how often the server polls the macOS
	// clipboard for changes.
	ClipboardPollInterval time.Duration
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		ListenAddr:            "0.0.0.0:24800",
		RemoteEdge:            "right",
		SwitchKeys:            []int{55, 56, 49},
		EdgeSensitivity:       8.0,
		StickyDwellMin:        300 * time.Millisecond,
		StickyDwellMax:        1 * time.Second,
		StickyPush:            4.0,
		ClipboardPollInterval: 500 * time.Millisecond,
	}
}

// Validate checks configuration before any goroutines or event taps start.
func (c Config) Validate() error {
	if c.ListenAddr == "" {
		return errors.New("server: listen address is required")
	}
	if c.RemoteEdge != EdgeLeft && c.RemoteEdge != EdgeRight {
		return fmt.Errorf("server: invalid edge %q", c.RemoteEdge)
	}
	if err := secureconn.ValidateSecret(c.PairingSecret); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	if c.EdgeSensitivity <= 0 {
		return errors.New("server: edge sensitivity must be positive")
	}
	if c.StickyDwellMin <= 0 || c.StickyDwellMax < c.StickyDwellMin {
		return errors.New("server: sticky dwell range is invalid")
	}
	if c.StickyPush <= 0 {
		return errors.New("server: sticky push must be positive")
	}
	if c.ClipboardPollInterval <= 0 {
		return errors.New("server: clipboard interval must be positive")
	}
	return nil
}
