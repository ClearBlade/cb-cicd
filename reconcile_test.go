package main

import (
	"testing"
)

// Every type ResourcePaths can deploy must be either auto-deletable or an
// explicit, deliberate "manual" — a type silently absent from both would make
// prune quietly skip it forever.
func TestPruneCoversEveryDeployableType(t *testing.T) {
	deployable := []string{
		"service", "library", "collection", "collection_schema", "collection_replace",
		"trigger", "timer",
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
	if _, err := parseReconcileFlags([]string{"stray arg"}); err == nil {
		t.Fatal("positional args must be rejected")
	}
}

func TestPruneFlagsRequireValidKeys(t *testing.T) {
	if _, err := parsePruneFlags([]string{}); err == nil {
		t.Fatal("prune with no keys must be rejected")
	}
	if _, err := parsePruneFlags([]string{"notakey"}); err == nil {
		t.Fatal("a key without type: must be rejected")
	}
	pf, err := parsePruneFlags([]string{"-dry-run", "service:dead", "timer:old"})
	if err != nil {
		t.Fatal(err)
	}
	if !pf.dryRun || len(pf.keys) != 2 || pf.keys[0] != "service:dead" {
		t.Fatalf("unexpected parse: %+v", pf)
	}
}

func TestKeyParts(t *testing.T) {
	typ, name := keyParts("collection_schema:device_import")
	if typ != "collection_schema" || name != "device_import" {
		t.Fatalf("keyParts got %q %q", typ, name)
	}
	if typ, _ := keyParts("notakey"); typ != "" {
		t.Fatalf("bare key should yield empty type, got %q", typ)
	}
}
