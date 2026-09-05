// edgehop-server runs on the Mac with the physical trackpad and keyboard. It
// captures global input via CGEventTap, forwards it to the Linux client, and
// synchronizes the clipboard.
//
// Permissions required (System Settings > Privacy & Security):
//   - Accessibility: needed for the event tap and synthetic events.
package main

import (
	"flag"
	"github.com/Coraia/EdgeHop/internal/secureconn"
	"github.com/Coraia/EdgeHop/internal/server"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	var (
		listen       = flag.String("listen", "0.0.0.0:24800", "TCP listen address")
		pairingFile  = flag.String("pairing-file", filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "edgehop", "pairing.key"), "shared pairing key file")
		edge         = flag.String("edge", "right", "which edge of the Mac screen touches the client: right or left")
		swKeysRaw    = flag.String("switch-keys", "", "comma-separated macOS keycodes for a hotkey that toggles remote mode (e.g. 55,56,49)")
		sensitivity  = flag.Float64("edge-sensitivity", 2.0, "edge margin in points that triggers switching")
		clipInterval = flag.Duration("clip-interval", 500*time.Millisecond, "clipboard poll interval")
	)
	flag.Parse()

	cfg := server.DefaultConfig()
	cfg.ListenAddr = *listen
	secret, _, err := secureconn.LoadOrCreateSecret(*pairingFile)
	if err != nil {
		log.Fatalf("pairing key: %v", err)
	}
	cfg.PairingSecret = secret
	cfg.RemoteEdge = *edge
	cfg.EdgeSensitivity = *sensitivity
	cfg.ClipboardPollInterval = *clipInterval
	if *swKeysRaw != "" {
		cfg.SwitchKeys = nil
		for _, p := range strings.Split(*swKeysRaw, ",") {
			v, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				log.Fatalf("bad switch keycode %q", p)
			}
			cfg.SwitchKeys = append(cfg.SwitchKeys, v)
		}
	}

	log.Printf("EdgeHop server starting: listen=%s edge=%s", cfg.ListenAddr, cfg.RemoteEdge)
	if err := server.Run(cfg); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}
