package client

import (
	"errors"
	"fmt"
	"github.com/Coraia/EdgeHop/internal/secureconn"
	"time"
)

// Config configures the Linux client.
type Config struct {
	// ServerAddr is the Mac mini address, e.g. "192.168.1.10:24800".
	ServerAddr string

	// PairingSecret authenticates and encrypts the server connection.
	PairingSecret []byte

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
		DeviceName:       "edgehop",
		Edge:             "right",
		EdgeMargin:       8.0,
		ClipInterval:     500 * time.Millisecond,
		EdgePollInterval: 40 * time.Millisecond,
	}
}

// Validate checks configuration before opening uinput or starting goroutines.
func (c Config) Validate() error {
	if c.ServerAddr == "" {
		return errors.New("client: server address is required")
	}
	if err := secureconn.ValidateSecret(c.PairingSecret); err != nil {
		return fmt.Errorf("client: %w", err)
	}
	if c.DeviceName == "" {
		return errors.New("client: device name is required")
	}
	if c.Edge != "left" && c.Edge != "right" {
		return fmt.Errorf("client: invalid edge %q", c.Edge)
	}
	if c.EdgeMargin <= 0 {
		return errors.New("client: edge margin must be positive")
	}
	if c.ClipInterval <= 0 {
		return errors.New("client: clipboard interval must be positive")
	}
	if c.EdgePollInterval <= 0 {
		return errors.New("client: edge poll interval must be positive")
	}
	return nil
}
