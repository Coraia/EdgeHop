//go:build linux

package client

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// hyprCursorPos returns the current cursor position in Omarchy screen
// coordinates by asking Hyprland's IPC ("hyprctl cursorpos" -> "x, y").
func hyprCursorPos() (x, y float64, err error) {
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
