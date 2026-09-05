package client

import (
	"testing"
	"time"
)

func TestConfigRejectsInvalidRuntimeIntervals(t *testing.T) {
	valid := DefaultConfig()
	valid.ServerAddr = "192.0.2.10:24800"
	valid.PairingSecret = []byte("0123456789abcdef0123456789abcdef")

	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"zero clipboard interval", func(c *Config) { c.ClipInterval = 0 }},
		{"negative edge interval", func(c *Config) { c.EdgePollInterval = -time.Second }},
		{"zero edge margin", func(c *Config) { c.EdgeMargin = 0 }},
		{"invalid edge", func(c *Config) { c.Edge = "top" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.change(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate accepted invalid configuration")
			}
		})
	}
}
