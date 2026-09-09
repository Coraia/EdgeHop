//go:build darwin

// Package macclipboard reads and writes the macOS clipboard via pbcopy/pbpaste.
//
// GUI-launched macOS apps (Finder, Dock, LaunchAgent) inherit no LANG/LC_*
// environment. pbcopy/pbpaste then interpret text with the legacy MacRoman
// C-string encoding instead of UTF-8, which mangles every multi-byte character
// in both directions: CJK pastes from the clipboard turn into "????" and text
// piped into pbcopy is stored as double-encoded mojibake. All child processes
// therefore run with an explicit UTF-8 locale.
package macclipboard

import (
	"os"
	"os/exec"
	"slices"
	"strings"
)

// utf8LocaleEnv returns the process environment with LANG/LC_ALL pinned to a
// UTF-8 locale so pbcopy/pbpaste encode text as UTF-8 regardless of how this
// process was launched. en_US.UTF-8 is always present on macOS.
func utf8LocaleEnv() []string {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, "LANG=") ||
			strings.HasPrefix(kv, "LC_ALL=") ||
			strings.HasPrefix(kv, "LC_CTYPE=")
	})
	return append(env, "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
}

func command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = utf8LocaleEnv()
	return cmd
}

// Read returns the current clipboard text. ok=false when the clipboard is
// empty or unreadable.
func Read() (string, bool) {
	out, err := command("/usr/bin/pbpaste").Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

// Write replaces the clipboard with text.
func Write(text string) error {
	cmd := command("/usr/bin/pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
