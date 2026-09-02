// uc-server runs on the Mac mini (the machine with the physical trackpad and
// keyboard). It captures global input via CGEventTap, forwards it to the
// uc-client on the Omarchy machine over TCP, and synchronizes the clipboard.
//
// Permissions required (System Settings > Privacy & Security):
//   - Accessibility: needed for the event tap and synthetic events.
package main

import (
	"flag"
	"log"
	"strconv"
	"strings"
	"time"
	"universal_control/internal/server"
)

func main() {
	var (
		listen       = flag.String("listen", "0.0.0.0:24800", "TCP listen address")
		connect      = flag.String("connect", "", "client address to auto-connect (e.g. 192.168.1.20:24800); empty = accept inbound only")
		edge         = flag.String("edge", "right", "which edge of the Mac screen touches the client: right or left")
		swKeysRaw    = flag.String("switch-keys", "", "comma-separated macOS keycodes for a hotkey that toggles remote mode (e.g. 55,56,49)")
		sensitivity  = flag.Float64("edge-sensitivity", 2.0, "edge margin in points that triggers switching")
		clipInterval = flag.Duration("clip-interval", 500*time.Millisecond, "clipboard poll interval")
	)
	flag.Parse()

	cfg := server.DefaultConfig()
	cfg.ListenAddr = *listen
	cfg.ClientAddr = *connect
	if *edge != server.EdgeRight && *edge != server.EdgeLeft {
		log.Fatalf("invalid -edge %q (want right|left)", *edge)
	}
	cfg.RemoteEdge = *edge
	cfg.EdgeSensitivity = *sensitivity
	cfg.ClipboardPollInterval = *clipInterval
	if *swKeysRaw != "" {
		for _, p := range strings.Split(*swKeysRaw, ",") {
			v, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				log.Fatalf("bad switch keycode %q", p)
			}
			cfg.SwitchKeys = append(cfg.SwitchKeys, v)
		}
	}

	log.Printf("uc-server starting: listen=%s connect=%q edge=%s", cfg.ListenAddr, cfg.ClientAddr, cfg.RemoteEdge)
	if err := server.Run(cfg); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}
