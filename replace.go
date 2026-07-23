package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	cb "github.com/clearblade/Go-SDK"
)

// collection_replace is a "collection" whose ROWS are repo-authoritative. After
// the normal zip upsert, any platform row whose item_id is absent from the repo
// file is deleted, converging the collection's rowset EXACTLY onto the repo.
//
// Plain "collection" only ever upserts (adds/overwrites by item_id, never
// deletes), so a row written out-of-band — e.g. an admin UI that
// DELETE-all-then-POSTs a row with a fresh random item_id on every save —
// accumulates forever alongside the repo row. Readers that take "the first row"
// then serve a nondeterministic one. collection_replace is for the case where
// the repo is the single source of truth for the rows and out-of-band writes
// must not survive a deploy.
//
// This is the ONE place CI deletes rows, and it stays true to the reconcile
// contract that "deletion is never inferred from remembered state": the deletion
// is DECLARED, not guessed. `collection_replace` in cicd-config.json is a
// reviewed, committed statement that this collection's rows are repo-owned.
// Nothing is remembered; every run converges from the repo file + live rows.
//
// Ordering matters: converge runs AFTER the upload, so the repo rows are already
// present when the foreign rows are deleted — the authoritative rows never leave
// the collection, so there is no window where a reader sees it empty.

// convergeReplaceCollections deletes platform rows absent from the repo file for
// every collection_replace resource in `resources`. tempDir holds the exact
// uploaded files (post env-layer merge), so its item_ids are the authoritative
// keep-set.
func convergeReplaceCollections(tempDir, systemKey string, resources []SyncResource, client *cb.DevClient, isDryRun bool) error {
	for _, r := range resources {
		if r.Type != "collection_replace" {
			continue
		}
		if err := convergeReplaceCollection(tempDir, systemKey, r.Name, client, isDryRun); err != nil {
			return fmt.Errorf("collection_replace %q: %w", r.Name, err)
		}
	}
	return nil
}

func convergeReplaceCollection(tempDir, systemKey, name string, client *cb.DevClient, isDryRun bool) error {
	keep, err := keepSetFromFile(filepath.Join(tempDir, "data", name+".json"))
	if err != nil {
		return err
	}

	colID, err := collectionIDByName(client, systemKey, name)
	if err != nil {
		return err
	}

	// A query matching every row NOT in the keep-set: item_id != k1 AND != k2 ...
	// A nil query matches everything — correct when the repo declares zero rows.
	foreign := foreignRowsQuery(keep)

	count, err := countMatching(client, colID, foreign)
	if err != nil {
		return fmt.Errorf("count foreign rows: %w", err)
	}

	if count == 0 {
		fmt.Printf("collection_replace %q: rows already converged (no foreign rows).\n", name)
		return nil
	}

	if isDryRun {
		fmt.Printf("collection_replace %q: would delete %d row(s) not in the repo file.\n", name, count)
		return nil
	}

	if err := client.DeleteData(colID, foreign); err != nil {
		return fmt.Errorf("delete foreign rows: %w", err)
	}
	fmt.Printf("collection_replace %q: deleted %d row(s) not in the repo file.\n", name, count)
	return nil
}

// keepSetFromFile reads the item_ids of every row in a collection file. A row
// without an item_id is a hard error: collection_replace matches rows by item_id,
// and a repo row lacking one could never be told apart from a foreign row — it
// would be deleted right after being upserted, churning every deploy.
func keepSetFromFile(path string) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read collection file %s: %w", path, err)
	}
	var file struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	keep := make(map[string]bool, len(file.Items))
	for i, it := range file.Items {
		id, _ := it["item_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("row %d has no item_id — collection_replace rows must each carry an item_id so the repo row is distinguishable from a foreign one", i)
		}
		keep[id] = true
	}
	return keep, nil
}

// foreignRowsQuery builds a query selecting rows whose item_id is none of the
// keep-set. Returns nil (matches all rows) when the keep-set is empty, so an
// empty repo file converges the collection to empty.
func foreignRowsQuery(keep map[string]bool) *cb.Query {
	if len(keep) == 0 {
		return nil
	}
	q := cb.NewQuery()
	for id := range keep {
		q.NotEqualTo("item_id", id)
	}
	return q
}

// countMatching returns how many rows match the query (nil = all rows).
func countMatching(client *cb.DevClient, colID string, query *cb.Query) (int, error) {
	resp, err := client.GetDataTotal(colID, query)
	if err != nil {
		return 0, err
	}
	c, ok := resp["count"].(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected count response %v", resp)
	}
	return int(c), nil
}
