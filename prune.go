package main

import (
	"fmt"

	cb "github.com/clearblade/Go-SDK"
)

// The platform's zip upload UPSERTS: artifacts absent from the zip are left
// untouched, so removing an artifact from the repo never removes it from the
// platform (deleted services and timers have kept running in production for
// weeks). Prune closes that gap: anything in the managed set that is no longer
// whitelisted is deleted — but only when the operator passes -prune. By default
// the prune set is only reported, because deleting a live artifact is rare and
// high-stakes enough to deserve a human decision.

// pruneOutcome describes what happened (or would happen) to one stale artifact.
type pruneOutcome struct {
	Key    string
	Action string // "deleted", "failed", "manual"
	Detail string
}

// deletableTypes maps whitelist types to their platform delete call. Types absent
// here are reported as "manual": schema singletons cannot be deleted as artifacts,
// and bucket_set_files are individual files inside a bucket set — pruning those
// automatically risks more than it saves.
var deletableTypes = map[string]func(client *cb.DevClient, systemKey, name string) error{
	"service": func(c *cb.DevClient, sk, n string) error { return c.DeleteService(sk, n) },
	"library": func(c *cb.DevClient, sk, n string) error { return c.DeleteLibrary(sk, n) },
	"timer":   func(c *cb.DevClient, sk, n string) error { return c.DeleteTimer(sk, n) },
	"trigger": func(c *cb.DevClient, sk, n string) error { return c.DeleteTrigger(sk, n) },
	"webhook": func(c *cb.DevClient, sk, n string) error { return c.DeleteWebhook(sk, n) },
	"secret":  func(c *cb.DevClient, sk, n string) error { _, err := c.DeleteSecret(sk, n); return err },
	"plugin":  func(c *cb.DevClient, sk, n string) error { _, err := c.DeletePlugin(sk, n); return err },
	"portal":  func(c *cb.DevClient, sk, n string) error { return c.DeletePortal(sk, n) },
	"adaptor": func(c *cb.DevClient, sk, n string) error { return c.DeleteAdaptor(sk, n) },
	"deployment": func(c *cb.DevClient, sk, n string) error {
		return c.DeleteDeploymentByName(sk, n)
	},
	"bucket_set": func(c *cb.DevClient, sk, n string) error { return c.DeleteBucketSet(sk, n) },
	"service_cache": func(c *cb.DevClient, sk, n string) error {
		return c.DeleteServiceCacheMeta(sk, n)
	},
	"external_database": func(c *cb.DevClient, sk, n string) error {
		return c.DeleteExternalDBConnection(sk, n)
	},
	"collection":        deleteCollectionByName,
	"collection_schema": deleteCollectionByName,
	"role":              deleteRoleByName,
}

// deleteCollectionByName resolves the collection's ID (DeleteCollection takes an
// ID, not a name) and deletes it — rows included, which is what pruning a
// collection means. The prune set is printed before anything is deleted, and
// deletion only happens under -prune.
func deleteCollectionByName(client *cb.DevClient, systemKey, name string) error {
	cols, err := client.GetAllCollections(systemKey)
	if err != nil {
		return fmt.Errorf("could not list collections: %w", err)
	}
	for _, c := range cols {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if m["name"] == name {
			id, _ := m["collectionID"].(string)
			if id == "" {
				return fmt.Errorf("collection %q has no collectionID", name)
			}
			return client.DeleteCollection(id)
		}
	}
	return fmt.Errorf("collection %q not found on platform (already gone?)", name)
}

// deleteRoleByName resolves the role's ID (DeleteRole takes an ID).
func deleteRoleByName(client *cb.DevClient, systemKey, name string) error {
	roles, err := client.GetAllRoles(systemKey)
	if err != nil {
		return fmt.Errorf("could not list roles: %w", err)
	}
	for _, r := range roles {
		m, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		if m["Name"] == name {
			id, _ := m["ID"].(string)
			if id == "" {
				return fmt.Errorf("role %q has no ID", name)
			}
			return client.DeleteRole(systemKey, id)
		}
	}
	return fmt.Errorf("role %q not found on platform (already gone?)", name)
}

// PrunePlan reports each stale key and whether it can be auto-deleted.
func PrunePlan(staleKeys []string) []pruneOutcome {
	out := make([]pruneOutcome, 0, len(staleKeys))
	for _, key := range staleKeys {
		typ, _ := keyParts(key)
		if _, ok := deletableTypes[typ]; ok {
			out = append(out, pruneOutcome{Key: key, Action: "deletable"})
		} else {
			out = append(out, pruneOutcome{Key: key, Action: "manual",
				Detail: fmt.Sprintf("type %q has no automatic delete; remove it by hand if intended", typ)})
		}
	}
	return out
}

// ExecutePrune deletes every deletable stale artifact. It returns the keys that
// were successfully deleted (so the caller can drop them from the managed set) —
// a failed delete stays managed, so the next -prune retries it instead of the
// artifact silently falling off the books.
func ExecutePrune(client *cb.DevClient, systemKey string, staleKeys []string) (deleted []string, outcomes []pruneOutcome) {
	for _, key := range staleKeys {
		typ, name := keyParts(key)
		del, ok := deletableTypes[typ]
		if !ok {
			outcomes = append(outcomes, pruneOutcome{Key: key, Action: "manual",
				Detail: fmt.Sprintf("type %q has no automatic delete", typ)})
			continue
		}
		if err := del(client, systemKey, name); err != nil {
			outcomes = append(outcomes, pruneOutcome{Key: key, Action: "failed", Detail: err.Error()})
			continue
		}
		outcomes = append(outcomes, pruneOutcome{Key: key, Action: "deleted"})
		deleted = append(deleted, key)
	}
	return deleted, outcomes
}
