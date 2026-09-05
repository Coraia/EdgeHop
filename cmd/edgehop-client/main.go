// edgehop-client runs on Omarchy (Arch + Hyprland, Wayland). It connects to
// EdgeHop on macOS, injects keyboard/mouse events through /dev/uinput, and
// syncs the clipboard via wl-clipboard.
//
// Requirements on Omarchy:
//   - /dev/uinput access: `sudo usermod -aG input $USER` + udev rule
//   - wl-clipboard (pacman -S wl-clipboard)
//   - hyprland-utils for `hyprctl` (edge detection)
package main

import (
	"flag"
	"github.com/Coraia/EdgeHop/internal/client"
	"github.com/Coraia/EdgeHop/internal/secureconn"
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	var (
		server      = flag.String("server", "", "Mac mini address, e.g. 192.168.1.10:24800 (required)")
		pairingFile = flag.String("pairing-file", filepath.Join(os.Getenv("HOME"), ".config", "edgehop", "pairing.key"), "shared pairing key file")
		device      = flag.String("device", "edgehop", "uinput device name")
		edge        = flag.String("edge", "right", "which edge of the Omarchy screen faces the Mac: \"right\" if Omarchy is on the left of the PBP display, \"left\" if Omarchy is on the right")
		edgeMargin  = flag.Float64("edge-margin", 8.0, "edge distance (px) that returns control to the Mac")
		clipInt     = flag.Duration("clip-interval", 500*time.Millisecond, "clipboard poll interval")
		edgePoll    = flag.Duration("edge-poll", 40*time.Millisecond, "cursor edge poll interval")
	)
	flag.Parse()

	if *server == "" {
		log.Fatal("missing -server (Mac mini address)")
	}

	cfg := client.DefaultConfig()
	secret, _, err := secureconn.LoadSecret(*pairingFile)
	if err != nil {
		log.Fatalf("pairing key: %v", err)
	}
	cfg.ServerAddr = *server
	cfg.PairingSecret = secret
	cfg.DeviceName = *device
	cfg.Edge = *edge
	cfg.EdgeMargin = *edgeMargin
	cfg.ClipInterval = *clipInt
	cfg.EdgePollInterval = *edgePoll

	log.Printf("EdgeHop client starting: server=%s device=%q", cfg.ServerAddr, cfg.DeviceName)
	c, err := client.New(cfg)
	if err != nil {
		log.Fatalf("fatal: %v (ensure /dev/uinput is writable: user in 'input' group + udev rule)", err)
	}
	defer c.Close()
	if err := c.Run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}
