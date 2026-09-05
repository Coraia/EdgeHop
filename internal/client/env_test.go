package client

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureHyprEnvReplacesStaleSignature(t *testing.T) {
	runtimeDir := shortTempDir(t)
	socketPath := filepath.Join(runtimeDir, "hypr", "fresh", ".socket.sock")
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "stale")
	if !ensureHyprEnv() {
		t.Fatal("fresh Hyprland socket was not discovered")
	}
	if got := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"); got != "fresh" {
		t.Fatalf("signature = %q, want fresh", got)
	}
}

func TestEnsureWaylandEnvReplacesStaleDisplay(t *testing.T) {
	runtimeDir := shortTempDir(t)
	socketPath := filepath.Join(runtimeDir, "wayland-1")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-stale")
	ensureWaylandEnv()
	if got := os.Getenv("WAYLAND_DISPLAY"); got != "wayland-1" {
		t.Fatalf("display = %q, want wayland-1", got)
	}
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "uc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
