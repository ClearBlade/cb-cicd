package main

import "testing"

func TestMatchResourcesSpaceNamedFile(t *testing.T) {
	whitelist := []SyncResource{
		{Name: "Mueller MQTT Device", Type: "role"},
		{Name: "api_createMqttDevice", Type: "service"},
		{Name: "node_settings", Type: "collection_schema"},
	}
	changed := []string{
		"roles/Mueller MQTT Device.json", // space in name must match exactly
		"code/services/api_createMqttDevice/api_createMqttDevice.js",
	}

	matched := MatchResources(changed, whitelist)
	if len(matched) != 2 {
		t.Fatalf("got %d matches %v, want 2", len(matched), matched)
	}
	if matched[0].Name != "Mueller MQTT Device" || matched[1].Name != "api_createMqttDevice" {
		t.Fatalf("unexpected matches: %v", matched)
	}
}

func TestMatchResourcesNoMatches(t *testing.T) {
	whitelist := []SyncResource{{Name: "node_settings", Type: "collection_schema"}}
	matched := MatchResources([]string{"docs/readme.md"}, whitelist)
	if len(matched) != 0 {
		t.Fatalf("got %v, want no matches", matched)
	}
}

func TestWhitelistFileChanged(t *testing.T) {
	cfg := "./cicd-config.json"
	if !WhitelistFileChanged([]string{"webhooks/x.json", "cicd-config.json"}, cfg) {
		t.Fatal("expected true when cicd-config.json is among the changed files")
	}
	if WhitelistFileChanged([]string{"webhooks/x.json", "code/services/y/y.js"}, cfg) {
		t.Fatal("expected false when cicd-config.json is not among the changed files")
	}
	// basename match: config given with a path prefix still matches a bare changed entry.
	if !WhitelistFileChanged([]string{"cicd-config.json"}, "/repo/mueller_iot_core/cicd-config.json") {
		t.Fatal("expected basename match regardless of configPath prefix")
	}
}
