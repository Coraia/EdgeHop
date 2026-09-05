//go:build linux

package client

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// hyprScreenSize returns the LOGICAL size of the first monitor via "hyprctl
// monitors". Hyprland reports the physical resolution (e.g. "1920x2160") and a
// "scale: 1.25" factor, while "hyprctl cursorpos" reports coordinates in the
// LOGICAL (scaled) space: on a 1.25-scaled monitor the cursor maxes out at
// x=1535 (=1920/1.25-1) even though the panel is 1920 physical pixels wide.
//
// Any cursor-edge math (parking, return detection) MUST use the logical size,
// otherwise the cursor never reaches the physical width and edge detection
// silently never fires.
func hyprScreenSize() (w, h int, err error) {
	if !ensureHyprEnv() {
		return 0, 0, fmt.Errorf("no Hyprland instance available")
	}
	out, err := exec.Command("hyprctl", "monitors").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("hyprctl monitors: %w", err)
	}
	return parseMonitorSize(string(out))
}

// hyprCursorPos returns the current cursor position in Omarchy screen
// coordinates by asking Hyprland's IPC ("hyprctl cursorpos" -> "x, y").
// These coordinates are in the same logical space as hyprScreenSize.
func hyprCursorPos() (x, y float64, err error) {
	if !ensureHyprEnv() {
		return 0, 0, fmt.Errorf("no Hyprland instance available")
	}
	out, err := exec.Command("hyprctl", "cursorpos").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("hyprctl cursorpos: %w", err)
	}
	s := strings.TrimSpace(string(out))
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected cursorpos output %q", s)
	}
	x, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, err
	}
	y, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}
