//go:build linux

package client

import (
	"fmt"
	"os"
	"strings"
)

// ensureHyprEnv sets HYPRLAND_INSTANCE_SIGNATURE (and XDG_RUNTIME_DIR) from the
// running Hyprland instance socket when they are not already set. This lets a
// systemd-launched uc-client (started at boot, before login) pick up the
// Hyprland IPC connection once the user has logged in — without a restart — so
// hyprctl-based screen size / cursor edge detection keep working across the
// login boundary. Returns true when a usable instance is available.
func ensureHyprEnv() bool {
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" {
		return true
	}
	uid := os.Getuid()
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		os.Setenv("XDG_RUNTIME_DIR", fmt.Sprintf("/run/user/%d", uid))
	}
	dir := fmt.Sprintf("/run/user/%d/hypr", uid)
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return false
	}
	os.Setenv("HYPRLAND_INSTANCE_SIGNATURE", entries[0].Name())
	return true
}

// ensureWaylandEnv sets WAYLAND_DISPLAY (and XDG_RUNTIME_DIR) from the first
// available wayland socket when not already set, so wl-clipboard works for a
// systemd-launched uc-client that did not inherit the session environment.
func ensureWaylandEnv() {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return
	}
	uid := os.Getuid()
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		os.Setenv("XDG_RUNTIME_DIR", fmt.Sprintf("/run/user/%d", uid))
	}
	dir := fmt.Sprintf("/run/user/%d", uid)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "wayland-") {
			os.Setenv("WAYLAND_DISPLAY", e.Name())
			return
		}
	}
}
