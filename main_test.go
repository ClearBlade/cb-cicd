package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRunFlagsRejectsPositionalArgs(t *testing.T) {
	// A path with a space, unquoted, shatters into a flag value + a positional
	// token ("APIs.postman_collection.json"). Go's flag package stops parsing
	// there, silently dropping every -file after it — which truncated a real
	// sync's scope to nothing. This must be a hard error, not a silent no-op.
	args := []string{
		"-file", "api-spec/ClearBlade", "APIs.postman_collection.json",
		"-file", "code/services/api_createMqttDevice/api_createMqttDevice.js",
	}
	_, err := parseRunFlags("run", args)
	if err == nil {
		t.Fatal("expected an error for positional arguments, got nil")
	}
	if !strings.Contains(err.Error(), "-files-from") {
		t.Fatalf("error should point at the -files-from remedy, got: %s", err)
	}
}

func TestParseRunFlagsFilesFrom(t *testing.T) {
	list := filepath.Join(t.TempDir(), "changed.txt")
	content := "api-spec/ClearBlade APIs.postman_collection.json\r\n" + // CRLF + space in name
		"\n" + // blank line skipped
		"roles/Mueller MQTT Device.json\n" +
		"code/services/api_createMqttDevice/api_createMqttDevice.js\n"
	if err := os.WriteFile(list, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	rf, err := parseRunFlags("run", []string{"-files-from", list})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	want := []string{
		"api-spec/ClearBlade APIs.postman_collection.json",
		"roles/Mueller MQTT Device.json",
		"code/services/api_createMqttDevice/api_createMqttDevice.js",
	}
	if len(rf.files) != len(want) {
		t.Fatalf("got %d files %v, want %d", len(rf.files), rf.files, len(want))
	}
	for i, w := range want {
		if rf.files[i] != w {
			t.Errorf("files[%d] = %q, want %q", i, rf.files[i], w)
		}
	}
}

func TestParseRunFlagsCombinesFileAndFilesFrom(t *testing.T) {
	list := filepath.Join(t.TempDir(), "changed.txt")
	if err := os.WriteFile(list, []byte("roles/IO Admin.json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rf, err := parseRunFlags("run", []string{"-file", "data/assets.json", "-files-from", list})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(rf.files) != 2 || rf.files[0] != "data/assets.json" || rf.files[1] != "roles/IO Admin.json" {
		t.Fatalf("got files %v, want [data/assets.json, roles/IO Admin.json]", rf.files)
	}
}

func TestParseRunFlagsFilesFromMissingFile(t *testing.T) {
	_, err := parseRunFlags("run", []string{"-files-from", filepath.Join(t.TempDir(), "missing.txt")})
	if err == nil {
		t.Fatal("expected an error for a missing -files-from file, got nil")
	}
}
