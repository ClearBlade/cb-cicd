package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSetFromResourcesAndKeys(t *testing.T) {
	s := setFromResources([]SyncResource{
		{Name: "api_foo", Type: "service"},
		{Name: "device_import", Type: "collection_schema"},
	})
	if !s["service:api_foo"] || !s["collection_schema:device_import"] {
		t.Fatalf("unexpected set contents: %v", s)
	}
	typ, name := keyParts("collection_schema:device_import")
	if typ != "collection_schema" || name != "device_import" {
		t.Fatalf("keyParts got %q %q", typ, name)
	}
}

func TestUnionAndSubtract(t *testing.T) {
	prev := managedSet{"service:a": true, "service:b": true, "timer:t": true}
	curr := managedSet{"service:a": true, "service:c": true}

	managed := union(prev, curr)
	for _, k := range []string{"service:a", "service:b", "service:c", "timer:t"} {
		if !managed[k] {
			t.Fatalf("union missing %s", k)
		}
	}

	// Stale = managed − current: b and t were applied before and are no longer
	// whitelisted; a is still whitelisted; c is new.
	stale := subtract(managed, curr)
	want := []string{"service:b", "timer:t"}
	if !reflect.DeepEqual(stale, want) {
		t.Fatalf("subtract = %v, want %v", stale, want)
	}

	// Bootstrap: no previous state means nothing is stale, whatever is whitelisted.
	if got := subtract(union(nil, curr), curr); len(got) != 0 {
		t.Fatalf("bootstrap subtract should be empty, got %v", got)
	}
}

func TestStateRowRoundTrip(t *testing.T) {
	managed := managedSet{"service:z": true, "service:a": true}
	keys := subtract(managed, managedSet{})
	blob, err := json.Marshal(stateRow{Managed: keys, AppliedAt: "2026-07-14T00:00:00Z", Tool: "test"})
	if err != nil {
		t.Fatal(err)
	}
	var sr stateRow
	if err := json.Unmarshal(blob, &sr); err != nil {
		t.Fatal(err)
	}
	// Sorted, complete.
	if !reflect.DeepEqual(sr.Managed, []string{"service:a", "service:z"}) {
		t.Fatalf("round-trip managed = %v", sr.Managed)
	}
}

// Every type ResourcePaths can deploy must be either auto-deletable or an
// explicit, deliberate "manual" — a type silently absent from both would make
// prune quietly skip it forever.
func TestPruneCoversEveryDeployableType(t *testing.T) {
	deployable := []string{
		"service", "library", "collection", "collection_schema", "trigger", "timer",
		"webhook", "deployment", "role", "user", "secret", "edge", "device", "plugin",
		"service_cache", "external_database", "bucket_set", "bucket_set_files",
		"file_store", "portal", "adaptor", "device_schema", "user_schema", "edge_schema",
	}
	// Deliberately manual: schema singletons are not deletable artifacts;
	// bucket_set_files are files inside a bucket set; users/devices/edges are
	// per-environment principals, not repo artifacts; file_store has no delete API
	// in the pinned SDK.
	manual := map[string]bool{
		"device_schema": true, "user_schema": true, "edge_schema": true,
		"bucket_set_files": true, "user": true, "device": true, "edge": true,
		"file_store": true,
	}

	for _, typ := range deployable {
		// Every deployable type must resolve a path (sanity that this list mirrors paths.go).
		if _, err := ResourcePaths(SyncResource{Name: "x", Type: typ}); err != nil {
			t.Fatalf("type %q not deployable per ResourcePaths: %v", typ, err)
		}
		_, deletable := deletableTypes[typ]
		if !deletable && !manual[typ] {
			t.Errorf("type %q is neither auto-deletable nor explicitly manual — prune would silently skip it", typ)
		}
		if deletable && manual[typ] {
			t.Errorf("type %q is both deletable and manual — pick one", typ)
		}
	}
}

func TestPrunePlanMarksManualTypes(t *testing.T) {
	plan := PrunePlan([]string{"service:dead_svc", "user_schema:*", "bucket_set_files:ia_files"})
	if plan[0].Action != "deletable" {
		t.Fatalf("service should be deletable, got %s", plan[0].Action)
	}
	for _, o := range plan[1:] {
		if o.Action != "manual" {
			t.Fatalf("%s should be manual, got %s", o.Key, o.Action)
		}
	}
}

func TestReconcileFlagsRejectPositional(t *testing.T) {
	if _, err := parseReconcileFlags([]string{"-prune", "stray arg"}); err == nil {
		t.Fatal("positional args must be rejected")
	}
}
