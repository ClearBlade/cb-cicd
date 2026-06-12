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
