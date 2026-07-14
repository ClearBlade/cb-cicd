package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	cb "github.com/clearblade/Go-SDK"
)

// The platform's zip upload UPSERTS: artifacts absent from the zip are left
// untouched, so removing an artifact from the repo never removes it from the
// platform (deleted services and timers have kept running in production for
// weeks). `cb-cicd prune <type:name>...` closes that gap — statelessly and
// deliberately: the operator names what to delete, because a whitelist removal
// arrives as a reviewed PR diff (git already knows exactly what was removed),
// and deleting a live artifact is rare and high-stakes enough to deserve a
// human decision. CI never prunes.

// keyParts splits a "type:name" artifact key.
func keyParts(key string) (string, string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", key
	}
	return parts[0], parts[1]
}

// pruneOutcome describes what happened (or would happen) to one artifact.
type pruneOutcome struct {
	Key    string
	Action string // "deletable", "deleted", "failed", "manual"
	Detail string
}

// deletableTypes maps whitelist types to their platform delete call. Types absent
// here are reported as "manual": schema singletons cannot be deleted as artifacts,
// bucket_set_files are individual files inside a bucket set, and user/device/edge
// are per-environment principals — pruning those automatically risks more than it
// saves.
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
// collection means.
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

// PrunePlan reports each key and whether it can be auto-deleted.
func PrunePlan(keys []string) []pruneOutcome {
	out := make([]pruneOutcome, 0, len(keys))
	for _, key := range keys {
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

// ExecutePrune deletes every deletable named artifact and reports each outcome.
func ExecutePrune(client *cb.DevClient, systemKey string, keys []string) []pruneOutcome {
	var outcomes []pruneOutcome
	for _, key := range keys {
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
	}
	return outcomes
}

type pruneFlags struct {
	devToken   string
	email      string
	password   string
	systemKey  string
	url        string
	configPath string
	dryRun     bool
	keys       []string
}

// parsePruneFlags accepts positional args: the type:name keys to delete.
func parsePruneFlags(args []string) (pruneFlags, error) {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)

	devToken := fs.String("dev-token", "", "ClearBlade developer token")
	email := fs.String("email", "", "ClearBlade developer email")
	password := fs.String("password", "", "ClearBlade developer password")
	systemKey := fs.String("system-key", "", "ClearBlade system key")
	url := fs.String("url", "", "ClearBlade platform URL")
	configPath := fs.String("config", defaultConfigPath, "path to cicd-config.json (guards against pruning a still-whitelisted artifact)")
	dryRun := fs.Bool("dry-run", false, "show what would be deleted without deleting")

	if err := fs.Parse(args); err != nil {
		return pruneFlags{}, err
	}
	keys := fs.Args()
	if len(keys) == 0 {
		return pruneFlags{}, fmt.Errorf("nothing to prune: pass one or more type:name keys, e.g. `cb-cicd prune service:autoCurveGeneration timer:autoCurveGenerationTimer`")
	}
	for _, k := range keys {
		if typ, _ := keyParts(k); typ == "" {
			return pruneFlags{}, fmt.Errorf("invalid key %q: expected type:name (e.g. service:autoCurveGeneration)", k)
		}
	}

	if *devToken == "" {
		*devToken = os.Getenv("CICD_DEV_TOKEN")
	}
	if *email == "" {
		*email = os.Getenv("CICD_EMAIL")
	}
	if *password == "" {
		*password = os.Getenv("CICD_PASSWORD")
	}
	if *systemKey == "" {
		*systemKey = os.Getenv("CICD_SYSTEM_KEY")
	}
	if *url == "" {
		*url = os.Getenv("CICD_URL")
	}

	return pruneFlags{
		devToken:   *devToken,
		email:      *email,
		password:   *password,
		systemKey:  *systemKey,
		url:        *url,
		configPath: *configPath,
		dryRun:     *dryRun,
		keys:       keys,
	}, nil
}

func runPrune(pf pruneFlags) error {
	// Refuse to prune anything the whitelist still claims: a still-whitelisted
	// artifact would be recreated by the next reconcile anyway, so asking for its
	// deletion is almost certainly a mistake (a typo, or the whitelist edit was
	// never committed). Skipped when no config is readable.
	if cfg, err := LoadConfig(pf.configPath); err == nil {
		whitelisted := make(map[string]bool, len(cfg.SyncResources))
		for _, r := range cfg.SyncResources {
			whitelisted[r.Type+":"+r.Name] = true
		}
		for _, k := range pf.keys {
			if whitelisted[k] {
				return fmt.Errorf("refusing to prune %s: it is still whitelisted in %s — remove the whitelist entry first (the next reconcile would just recreate it)", k, pf.configPath)
			}
		}
	}

	fmt.Printf("Prune plan for %d artifact(s):\n", len(pf.keys))
	for _, o := range PrunePlan(pf.keys) {
		suffix := ""
		if o.Detail != "" {
			suffix = " — " + o.Detail
		}
		fmt.Printf("  ✂ %s [%s]%s\n", o.Key, o.Action, suffix)
	}
	if pf.dryRun {
		fmt.Println("Dry run: nothing deleted.")
		return nil
	}

	if pf.systemKey == "" || pf.url == "" {
		return fmt.Errorf("system-key and url are required (set via flags or CICD_SYSTEM_KEY / CICD_URL)")
	}

	var client *cb.DevClient
	var err error
	switch {
	case pf.devToken != "":
		client, err = newClientWithToken(pf.url, pf.devToken)
	case pf.email != "" && pf.password != "":
		client, err = newClient(pf.url, pf.email, pf.password)
	default:
		return fmt.Errorf("authentication required: provide -dev-token (or CICD_DEV_TOKEN) or both -email and -password")
	}
	if err != nil {
		return err
	}

	failed := 0
	for _, o := range ExecutePrune(client, pf.systemKey, pf.keys) {
		suffix := ""
		if o.Detail != "" {
			suffix = " — " + o.Detail
		}
		fmt.Printf("  ✂ %s [%s]%s\n", o.Key, o.Action, suffix)
		if o.Action == "failed" {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d prune(s) failed", failed)
	}
	return nil
}
