package client

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	monitorResolution = regexp.MustCompile(`^\s*(\d+)x(\d+)@`)
	monitorScale      = regexp.MustCompile(`^\s*scale:\s*([0-9.]+)`)
)

// parseMonitorSize extracts the logical size of the first monitor from
// "hyprctl monitors" output.
func parseMonitorSize(out string) (int, int, error) {
	var phW, phH int
	scale := 1.0
	var err error
	for _, line := range strings.Split(out, "\n") {
		if phW == 0 {
			if m := monitorResolution.FindStringSubmatch(line); m != nil {
				phW, err = strconv.Atoi(m[1])
				if err != nil {
					phW = 0
					continue
				}
				phH, err = strconv.Atoi(m[2])
				if err != nil {
					phW, phH = 0, 0
					continue
				}
			}
			continue
		}
		if m := monitorScale.FindStringSubmatch(line); m != nil {
			if value, parseErr := strconv.ParseFloat(m[1], 64); parseErr == nil && value > 0 {
				scale = value
			}
			break
		}
		if monitorResolution.MatchString(line) {
			break
		}
	}
	if phW <= 0 || phH <= 0 {
		return 0, 0, fmt.Errorf("no monitor size found in hyprctl monitors")
	}
	return int(math.Round(float64(phW) / scale)), int(math.Round(float64(phH) / scale)), nil
}
