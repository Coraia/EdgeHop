//go:build linux

package client

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// hyprScreenSize returns the size of the first monitor via "hyprctl
// monitors", e.g. "Monitor HDMI-A-1 (ID 0): 1920x2160@60.00 at 0x0".
func hyprScreenSize() (w, h int, err error) {
	out, err := exec.Command("hyprctl", "monitors").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("hyprctl monitors: %w", err)
	}
	re := regexp.MustCompile(`(\d+)x(\d+)`)
	for _, line := range strings.Split(string(out), "\n") {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		w, err = strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		h, err = strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		return w, h, nil
	}
	return 0, 0, fmt.Errorf("no monitor size found in hyprctl monitors")
}

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
