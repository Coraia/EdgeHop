// uc-client runs on the Omarchy machine (Arch + Hyprland, Wayland). It
// connects to uc-server on the Mac mini, injects received keyboard/mouse
// events through /dev/uinput, and syncs the clipboard via wl-clipboard.
//
// Requirements on Omarchy:
//   - /dev/uinput access: `sudo usermod -aG input $USER` + udev rule
//   - wl-clipboard (pacman -S wl-clipboard)
//   - hyprland-utils for `hyprctl` (edge detection)
package main

import (
	"flag"
	"log"
	"time"
	"universal_control/internal/client"
)

func main() {
	var (
		server    = flag.String("server", "", "Mac mini address, e.g. 192.168.1.10:24800 (required)")
		device    = flag.String("device", "universal-control", "uinput device name")
		edgeMargin = flag.Float64("edge-margin", 2.0, "left-edge distance (px) that returns control to the Mac")
		clipInt   = flag.Duration("clip-interval", 500*time.Millisecond, "clipboard poll interval")
		edgePoll  = flag.Duration("edge-poll", 40*time.Millisecond, "cursor edge poll interval")
	)
	flag.Parse()

	if *server == "" {
		log.Fatal("missing -server (Mac mini address)")
	}

	cfg := client.DefaultConfig()
	cfg.ServerAddr = *server
	cfg.DeviceName = *device
	cfg.EdgeMargin = *edgeMargin
	cfg.ClipInterval = *clipInt
	cfg.EdgePollInterval = *edgePoll

	log.Printf("uc-client starting: server=%s device=%q", cfg.ServerAddr, cfg.DeviceName)
	c, err := client.New(cfg)
	if err != nil {
		log.Fatalf("fatal: %v (ensure /dev/uinput is writable: user in 'input' group + udev rule)", err)
	}
	defer c.Close()
	if err := c.Run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}
