package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempJSON(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestKeepSetFromFile(t *testing.T) {
	path := writeTempJSON(t, `{"name":"config","items":[{"item_id":"a","config":{}},{"item_id":"b","config":{}}]}`)
	keep, err := keepSetFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keep) != 2 || !keep["a"] || !keep["b"] {
		t.Fatalf("expected {a,b}, got %v", keep)
	}
}

func TestKeepSetFromFileEmptyItems(t *testing.T) {
	path := writeTempJSON(t, `{"name":"config","items":[]}`)
	keep, err := keepSetFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keep) != 0 {
		t.Fatalf("expected empty keep-set, got %v", keep)
	}
}

func TestKeepSetFromFileRejectsMissingItemID(t *testing.T) {
	path := writeTempJSON(t, `{"name":"config","items":[{"config":{}}]}`)
	if _, err := keepSetFromFile(path); err == nil {
		t.Fatal("a row without item_id must be rejected (it would churn every deploy)")
	}
}

// An empty keep-set must produce a nil query so DeleteData/GetDataTotal treat it
// as "match every row" — converging the collection to empty when the repo file
// declares no rows.
func TestForeignRowsQueryEmptyKeepMatchesAll(t *testing.T) {
	if q := foreignRowsQuery(map[string]bool{}); q != nil {
		t.Fatalf("empty keep-set must yield a nil (match-all) query, got %v", q)
	}
}

// A non-empty keep-set ANDs one NotEqualTo per kept id, so the query matches
// exactly the rows whose item_id is none of them.
func TestForeignRowsQueryExcludesKept(t *testing.T) {
	q := foreignRowsQuery(map[string]bool{"a": true, "b": true})
	if q == nil {
		t.Fatal("non-empty keep-set must yield a query")
	}
	if len(q.Filters) != 1 || len(q.Filters[0]) != 2 {
		t.Fatalf("expected one AND-group of two filters, got %v", q.Filters)
	}
	for _, f := range q.Filters[0] {
		if f.Operator != "!=" || f.Field != "item_id" {
			t.Fatalf("expected item_id != <id> filters, got %+v", f)
		}
	}
}
