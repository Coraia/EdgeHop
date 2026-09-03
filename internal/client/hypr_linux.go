//go:build linux

package client

import (
	"fmt"
	"math"
	"os/exec"
	"regexp"
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
	out, err := exec.Command("hyprctl", "monitors").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("hyprctl monitors: %w", err)
	}
	return parseMonitorSize(string(out))
}

// parseMonitorSize extracts the logical monitor size from "hyprctl monitors"
// output. It handles a scale factor (integer like 2 or fractional like 1.25).
func parseMonitorSize(out string) (int, int, error) {
	var phW, phH int
	scale := 1.0
	reRes := regexp.MustCompile(`^\s*(\d+)x(\d+)@`)
	reScale := regexp.MustCompile(`^\s*scale:\s*([0-9.]+)`)
	var err error
	for _, line := range strings.Split(out, "\n") {
		if m := reRes.FindStringSubmatch(line); m != nil {
			phW, err = strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			phH, err = strconv.Atoi(m[2])
			if err != nil {
				continue
			}
		}
		if m := reScale.FindStringSubmatch(line); m != nil {
			if f, ferr := strconv.ParseFloat(m[1], 64); ferr == nil && f > 0 {
				scale = f
			}
		}
	}
	if phW <= 0 || phH <= 0 {
		return 0, 0, fmt.Errorf("no monitor size found in hyprctl monitors")
	}
	return int(math.Round(float64(phW) / scale)), int(math.Round(float64(phH) / scale)), nil
}

// hyprCursorPos returns the current cursor position in Omarchy screen
// coordinates by asking Hyprland's IPC ("hyprctl cursorpos" -> "x, y").
// These coordinates are in the same logical space as hyprScreenSize.
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
