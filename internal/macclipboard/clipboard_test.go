//go:build darwin

package macclipboard

import (
	"slices"
	"strings"
	"testing"
)

func TestUTF8LocaleEnvPinsUTF8Locale(t *testing.T) {
	t.Setenv("LANG", "C")
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_CTYPE", "C")
	t.Setenv("PATH", "/usr/bin:/bin")

	env := utf8LocaleEnv()

	if !slices.Contains(env, "LANG=en_US.UTF-8") {
		t.Errorf("env missing LANG=en_US.UTF-8: %v", env)
	}
	if !slices.Contains(env, "LC_ALL=en_US.UTF-8") {
		t.Errorf("env missing LC_ALL=en_US.UTF-8: %v", env)
	}
	if !slices.Contains(env, "PATH=/usr/bin:/bin") {
		t.Errorf("env dropped unrelated variables: %v", env)
	}
	if slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "LC_CTYPE=") }) {
		t.Errorf("stale LC_CTYPE survived: %v", env)
	}
	for _, key := range []string{"LANG=", "LC_ALL="} {
		if n := slices.IndexFunc(env, func(kv string) bool { return strings.HasPrefix(kv, key) }); env[n] != key+"en_US.UTF-8" {
			t.Errorf("expected exactly one %sen_US.UTF-8, got: %v", key, env)
		}
	}
}

// Regression guard for the GUI-launch scenario: even when the process
// environment carries only empty locale variables (or none), the env handed to
// pbcopy/pbpaste must pin UTF-8.
func TestUTF8LocaleEnvWithEmptyLocale(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")

	env := utf8LocaleEnv()

	if !slices.Contains(env, "LC_ALL=en_US.UTF-8") || !slices.Contains(env, "LANG=en_US.UTF-8") {
		t.Errorf("env must pin UTF-8 locale even when unset: %v", env)
	}
	if slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "LC_CTYPE=") }) {
		t.Errorf("stale LC_CTYPE survived: %v", env)
	}
}
