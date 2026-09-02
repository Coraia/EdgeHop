package client

import "time"

// Config configures the Linux client.
type Config struct {
	// ServerAddr is the Mac mini address, e.g. "192.168.1.10:24800".
	ServerAddr string

	// DeviceName is the uinput device name shown in Hyprland.
	DeviceName string

	// Edge is which edge of the client (Omarchy) screen connects to the Mac:
	// "right" when Omarchy sits on the left of the PBP display (its right edge
	// faces the Mac), "left" when Omarchy sits on the right.
	Edge string

	// EdgeMargin is the edge distance (px) that triggers a switch back to
	// the Mac.
	EdgeMargin float64

	// ClipInterval is the Wayland clipboard poll interval.
	ClipInterval time.Duration

	// EdgePollInterval is how often the client checks the cursor position
	// while under remote control.
	EdgePollInterval time.Duration
}

// DefaultConfig returns sensible defaults (Omarchy on the left of a PBP
// display, Mac on the right).
func DefaultConfig() Config {
	return Config{
		DeviceName:       "universal-control",
		Edge:             "right",
		EdgeMargin:       2.0,
		ClipInterval:     500 * time.Millisecond,
		EdgePollInterval: 40 * time.Millisecond,
	}
}
