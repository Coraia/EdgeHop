package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ensureHyprEnv sets HYPRLAND_INSTANCE_SIGNATURE (and XDG_RUNTIME_DIR) from the
// running Hyprland instance socket when they are not already set. This lets a
// systemd-launched EdgeHop client (started at boot, before login) pick up the
// Hyprland IPC connection once the user has logged in — without a restart — so
// hyprctl-based screen size / cursor edge detection keep working across the
// login boundary. Returns true when a usable instance is available.
func ensureHyprEnv() bool {
	runtimeDir := ensureRuntimeDir()
	if signature := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"); signature != "" &&
		isSocket(filepath.Join(runtimeDir, "hypr", signature, ".socket.sock")) {
		return true
	}
	os.Unsetenv("HYPRLAND_INSTANCE_SIGNATURE")
	dir := filepath.Join(runtimeDir, "hypr")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if isSocket(filepath.Join(dir, entry.Name(), ".socket.sock")) {
			os.Setenv("HYPRLAND_INSTANCE_SIGNATURE", entry.Name())
			return true
		}
	}
	return false
}

// ensureWaylandEnv sets WAYLAND_DISPLAY (and XDG_RUNTIME_DIR) from the first
// available wayland socket when not already set, so wl-clipboard works for a
// systemd-launched EdgeHop client that did not inherit the session environment.
func ensureWaylandEnv() {
	runtimeDir := ensureRuntimeDir()
	if display := os.Getenv("WAYLAND_DISPLAY"); display != "" &&
		isSocket(filepath.Join(runtimeDir, display)) {
		return
	}
	os.Unsetenv("WAYLAND_DISPLAY")
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "wayland-") &&
			isSocket(filepath.Join(runtimeDir, e.Name())) {
			os.Setenv("WAYLAND_DISPLAY", e.Name())
			return
		}
	}
}

func ensureRuntimeDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	dir := fmt.Sprintf("/run/user/%d", os.Getuid())
	os.Setenv("XDG_RUNTIME_DIR", dir)
	return dir
}

func isSocket(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}
