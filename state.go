package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	cb "github.com/clearblade/Go-SDK"
)

// The reconcile model needs exactly one piece of memory: the set of artifacts this
// tool has applied and not yet pruned (the "managed set"). Everything else is
// observable — desired state is the whitelist, actual state is the platform.
//
// That memory lives IN THE TARGET SYSTEM, in the `cicd_state` collection, so it
// cannot drift from the environment it describes the way an external marker (a git
// tag, a workflow input) can. Whitelist `cicd_state` as a collection_schema and the
// first reconcile creates its own state store: bootstrap needs no special case.
//
// Rows are append-only — one per successful reconcile, newest wins — so the
// collection doubles as an audit log of what was applied when.
const stateCollection = "cicd_state"

// managedSet is a set of "type:name" artifact keys.
type managedSet map[string]bool

func resourceKey(r SyncResource) string { return r.Type + ":" + r.Name }

// keyType splits a "type:name" key back apart.
func keyParts(key string) (string, string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", key
	}
	return parts[0], parts[1]
}

func setFromResources(resources []SyncResource) managedSet {
	s := make(managedSet, len(resources))
	for _, r := range resources {
		s[resourceKey(r)] = true
	}
	return s
}

// union returns a ∪ b.
func union(a, b managedSet) managedSet {
	out := make(managedSet, len(a)+len(b))
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
}

// subtract returns a − b, sorted for stable output.
func subtract(a, b managedSet) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// stateRow is the JSON payload stored per reconcile.
type stateRow struct {
	Managed   []string `json:"managed"`
	AppliedAt string   `json:"applied_at"`
	Tool      string   `json:"tool"`
}

// LoadManagedSet reads the newest cicd_state row. A missing collection or an empty
// collection is NOT an error: it means this system has never been reconciled (or
// the state store was just created by this very push), i.e. bootstrap. Prune is
// simply unavailable until a managed set has been recorded.
//
// Rows accumulate one per successful reconcile, so ask the platform for just the
// newest (applied_at is a real column precisely so it can be sorted server-side);
// the client-side newest-wins scan below stays as belt-and-braces for any rows
// written before the column existed.
func LoadManagedSet(client *cb.DevClient, systemKey string) (managedSet, bool, error) {
	q := cb.NewQuery()
	q.Order = []cb.Ordering{{OrderKey: "applied_at", SortOrder: false}} // newest first
	q.PageSize = 5
	q.PageNumber = 1
	resp, err := client.GetDataByNameWithSystemKey(systemKey, stateCollection, q)
	if err != nil {
		// Collection absent => bootstrap. Any other failure must not be silently
		// treated as "no state" or prune would think everything is unmanaged.
		if strings.Contains(strings.ToLower(err.Error()), "not found") ||
			strings.Contains(strings.ToLower(err.Error()), "doesn't exist") ||
			strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("could not read %s: %w", stateCollection, err)
	}

	data, _ := resp["DATA"].([]interface{})
	var newest *stateRow
	var newestAt string
	for _, item := range data {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		blob, _ := row["state"].(string)
		if blob == "" {
			continue
		}
		var sr stateRow
		if err := json.Unmarshal([]byte(blob), &sr); err != nil {
			continue
		}
		if newest == nil || sr.AppliedAt > newestAt {
			srCopy := sr
			newest = &srCopy
			newestAt = sr.AppliedAt
		}
	}

	if newest == nil {
		return nil, false, nil
	}
	s := make(managedSet, len(newest.Managed))
	for _, k := range newest.Managed {
		s[k] = true
	}
	return s, true, nil
}

// SaveManagedSet appends a new state row recording the current managed set.
func SaveManagedSet(client *cb.DevClient, systemKey string, managed managedSet) error {
	keys := subtract(managed, managedSet{}) // sorted slice of every key
	now := time.Now().UTC().Format(time.RFC3339)
	blob, err := json.Marshal(stateRow{
		Managed:   keys,
		AppliedAt: now,
		Tool:      "cb-cicd " + version,
	})
	if err != nil {
		return err
	}
	if _, err := client.CreateDataByName(systemKey, stateCollection, map[string]interface{}{
		"state":      string(blob),
		"applied_at": now,
	}); err != nil {
		return fmt.Errorf("could not write %s: %w", stateCollection, err)
	}
	return nil
}
