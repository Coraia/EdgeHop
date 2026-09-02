package server

import "time"

// Config configures the macOS server.
type Config struct {
	// ListenAddr is the TCP listen address, e.g. "0.0.0.0:24800".
	ListenAddr string

	// ClientAddr is the Omarchy machine address to auto-connect to, e.g.
	// "192.168.1.20:24800". If empty, the server waits for an incoming
	// connection (either direction is supported).
	ClientAddr string

	// RemoteEdge is which edge of the Mac screen connects to the client.
	// "right" means the Omarchy screen sits to the right (PBP layout with
	// Mac on the left), "left" means it sits to the left.
	RemoteEdge string

	// SwitchKeys is an optional hotkey (list of macOS virtual keycodes that
	// must be held together) to toggle remote control without reaching an
	// edge. Empty disables the hotkey.
	SwitchKeys []int

	// EdgeSensitivity is the margin (in points) from the screen edge that
	// triggers a switch. Larger values make switching easier.
	EdgeSensitivity float64

	// ClipboardPollInterval is how often the server polls the macOS
	// clipboard for changes.
	ClipboardPollInterval time.Duration
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		ListenAddr:           "0.0.0.0:24800",
		RemoteEdge:           "right",
		EdgeSensitivity:      2.0,
		ClipboardPollInterval: 500 * time.Millisecond,
	}
}
