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
		ListenAddr:           "0.0.0.0:24800",
		RemoteEdge:           "right",
		EdgeSensitivity:      8.0,
		StickyDwellMin:       300 * time.Millisecond,
		StickyDwellMax:       1 * time.Second,
		StickyPush:           4.0,
		ClipboardPollInterval: 500 * time.Millisecond,
	}
}
