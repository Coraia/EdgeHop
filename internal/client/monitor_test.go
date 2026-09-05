package client

import "testing"

func TestParseMonitorSizeLogicalScale(t *testing.T) {
	// 1.25-scaled 1920x2160 panel: logical cursor space is 1536x1728.
	out := `Monitor HDMI-A-1 (ID 1):
	1920x2160@60.00000 at 0x0
	description: test
	scale: 1.25
	transform: 0
`
	w, h, err := parseMonitorSize(out)
	if err != nil {
		t.Fatalf("parseMonitorSize: %v", err)
	}
	if w != 1536 || h != 1728 {
		t.Fatalf("logical size = %dx%d, want 1536x1728", w, h)
	}
}

func TestParseMonitorSizeIntegerScale(t *testing.T) {
	out := `Monitor DP-1 (ID 0):
	3840x2160@60.00 at 0x0
	scale: 2.00
`
	w, h, err := parseMonitorSize(out)
	if err != nil {
		t.Fatalf("parseMonitorSize: %v", err)
	}
	if w != 1920 || h != 1080 {
		t.Fatalf("logical size = %dx%d, want 1920x1080", w, h)
	}
}

func TestParseMonitorSizeNoScale(t *testing.T) {
	out := `Monitor HDMI-A-1 (ID 1):
	1920x1080@60.00 at 0x0
	scale: 1.00
`
	w, h, err := parseMonitorSize(out)
	if err != nil {
		t.Fatalf("parseMonitorSize: %v", err)
	}
	if w != 1920 || h != 1080 {
		t.Fatalf("logical size = %dx%d, want 1920x1080", w, h)
	}
}

func TestParseMonitorSizeUsesFirstMonitor(t *testing.T) {
	out := `Monitor HDMI-A-1 (ID 1):
	1920x2160@60.00 at 0x0
	scale: 1.25
Monitor DP-1 (ID 2):
	3840x2160@60.00 at 1536x0
	scale: 2.00
`
	w, h, err := parseMonitorSize(out)
	if err != nil {
		t.Fatalf("parseMonitorSize: %v", err)
	}
	if w != 1536 || h != 1728 {
		t.Fatalf("first logical monitor = %dx%d, want 1536x1728", w, h)
	}
}
