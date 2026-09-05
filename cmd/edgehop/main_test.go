package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveRemoteEdgePreservesOtherConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	initial := fileConfig{
		Listen:     "127.0.0.1:24800",
		Edge:       "right",
		SwitchKeys: []int{55, 56, 49},
	}
	data, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := saveRemoteEdge(path, "left"); err != nil {
		t.Fatalf("saveRemoteEdge: %v", err)
	}

	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got fileConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Edge != "left" || got.Listen != initial.Listen {
		t.Fatalf("config = %+v, want edge=left and listen preserved", got)
	}
	if len(got.SwitchKeys) != len(initial.SwitchKeys) {
		t.Fatalf("switch keys = %v, want %v", got.SwitchKeys, initial.SwitchKeys)
	}

	if err := saveRemoteEdge(path, "top"); err == nil {
		t.Fatal("saveRemoteEdge accepted invalid edge")
	}
}

func TestMigrateLegacyDataPreservesPairingAndConfig(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, "universal-control")
	edgeHopDir := filepath.Join(root, "edgehop")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := []byte("legacy-pairing-secret")
	if err := os.WriteFile(filepath.Join(legacyDir, "pairing.key"), secret, 0o600); err != nil {
		t.Fatal(err)
	}
	config := []byte("{\"edge\":\"left\"}\n")
	if err := os.WriteFile(filepath.Join(legacyDir, "config.json"), config, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacyData(legacyDir, edgeHopDir); err != nil {
		t.Fatalf("migrateLegacyData: %v", err)
	}

	gotSecret, err := os.ReadFile(filepath.Join(edgeHopDir, "pairing.key"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotSecret, secret) {
		t.Fatalf("pairing secret changed: got %q want %q", gotSecret, secret)
	}
	gotConfig, err := os.ReadFile(filepath.Join(edgeHopDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotConfig, config) {
		t.Fatalf("config changed: got %q want %q", gotConfig, config)
	}
}

func TestMigrateLegacyLoginItemPreservesAutoStart(t *testing.T) {
	root := t.TempDir()
	legacyPath := filepath.Join(root, legacyAppLabel+".plist")
	edgeHopPath := filepath.Join(root, appLabel+".plist")
	if err := os.WriteFile(legacyPath, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacyLoginItem(legacyPath, edgeHopPath, "/Applications/EdgeHop.app/Contents/MacOS/edgehop"); err != nil {
		t.Fatalf("migrateLegacyLoginItem: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy login item still exists: %v", err)
	}
	data, err := os.ReadFile(edgeHopPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(appLabel)) || !bytes.Contains(data, []byte("/Applications/EdgeHop.app/Contents/MacOS/edgehop")) {
		t.Fatalf("migrated login item has wrong contents: %s", data)
	}
}
