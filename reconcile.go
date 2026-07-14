package main

import (
	"flag"
	"fmt"
	"os"

	cb "github.com/clearblade/Go-SDK"
)

// reconcile pushes the ENTIRE whitelist every run and reports (optionally deletes)
// whatever this tool previously applied that is no longer whitelisted.
//
// This replaces diff-scoped syncing. A diff sync must remember what it last
// applied, and that memory — a git tag, a workflow event payload — is a claim
// rather than an observation, so it eventually lies: a failed sync strands its
// range behind a green checkmark, a whitelist-only change matches no files and
// deploys nothing, a new system never bootstraps. Reconcile has none of those
// states: desired = whitelist + repo content, and the platform upsert makes
// actual converge on desired. Pushing an unchanged artifact is a no-op, so
// converging repeatedly is safe and a failed run is repaired by the next one.

type reconcileFlags struct {
	devToken   string
	email      string
	password   string
	systemKey  string
	url        string
	configPath string
	prune      bool
	dryRun     bool
}

func parseReconcileFlags(args []string) (reconcileFlags, error) {
	fs := flag.NewFlagSet("reconcile", flag.ContinueOnError)

	devToken := fs.String("dev-token", "", "ClearBlade developer token")
	email := fs.String("email", "", "ClearBlade developer email")
	password := fs.String("password", "", "ClearBlade developer password")
	systemKey := fs.String("system-key", "", "ClearBlade system key")
	url := fs.String("url", "", "ClearBlade platform URL")
	configPath := fs.String("config", defaultConfigPath, "path to cicd-config.json")
	prune := fs.Bool("prune", false, "delete previously managed artifacts that are no longer whitelisted")
	dryRun := fs.Bool("dry-run", false, "show what would be pushed and pruned without changing anything")

	if err := fs.Parse(args); err != nil {
		return reconcileFlags{}, err
	}
	if fs.NArg() > 0 {
		return reconcileFlags{}, fmt.Errorf("unexpected positional argument(s) %q", fs.Args())
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

	return reconcileFlags{
		devToken:   *devToken,
		email:      *email,
		password:   *password,
		systemKey:  *systemKey,
		url:        *url,
		configPath: *configPath,
		prune:      *prune,
		dryRun:     *dryRun,
	}, nil
}

func runReconcile(rf reconcileFlags) error {
	cfg, err := LoadConfig(rf.configPath)
	if err != nil {
		return err
	}
	if rf.systemKey == "" || rf.url == "" {
		return fmt.Errorf("system-key and url are required (set via flags or CICD_SYSTEM_KEY / CICD_URL)")
	}

	var client *cb.DevClient
	switch {
	case rf.devToken != "":
		client, err = newClientWithToken(rf.url, rf.devToken)
	case rf.email != "" && rf.password != "":
		client, err = newClient(rf.url, rf.email, rf.password)
	default:
		return fmt.Errorf("authentication required: provide -dev-token (or CICD_DEV_TOKEN) or both -email and -password")
	}
	if err != nil {
		return err
	}

	// Read the previously managed set BEFORE pushing, so a push failure leaves the
	// recorded state describing what was actually last applied.
	previous, hasState, err := LoadManagedSet(client, rf.systemKey)
	if err != nil {
		return err
	}
	if !hasState {
		fmt.Printf("No %s state found — first reconcile of this system (bootstrap). Prune is unavailable until a managed set has been recorded.\n", stateCollection)
	}

	// Push the whole whitelist.
	fmt.Printf("Reconciling all %d whitelisted resources.\n", len(cfg.SyncResources))
	for _, r := range cfg.SyncResources {
		fmt.Printf("  → %s (%s)\n", r.Name, r.Type)
	}

	systemDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not determine working directory: %w", err)
	}
	tempDir, err := BuildTempDir(systemDir, cfg.SyncResources)
	if err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("could not build temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := PushTempDir(tempDir, rf.systemKey, client, rf.dryRun); err != nil {
		return err
	}

	// Compute the prune set. managed = everything we have ever applied and not yet
	// pruned; stale = managed − currently whitelisted. The state collection itself
	// is never pruned: deleting it would destroy the managed-set memory and turn
	// every later reconcile into a bootstrap.
	current := setFromResources(cfg.SyncResources)
	managed := union(previous, current)
	stale := make([]string, 0)
	for _, key := range subtract(managed, current) {
		if _, name := keyParts(key); name == stateCollection {
			continue
		}
		stale = append(stale, key)
	}

	if len(stale) == 0 {
		fmt.Println("Prune: nothing stale — every managed artifact is still whitelisted.")
	} else {
		fmt.Printf("Prune: %d managed artifact(s) are no longer whitelisted:\n", len(stale))
		for _, o := range PrunePlan(stale) {
			suffix := ""
			if o.Detail != "" {
				suffix = " — " + o.Detail
			}
			fmt.Printf("  ✂ %s [%s]%s\n", o.Key, o.Action, suffix)
		}
	}

	if rf.dryRun {
		fmt.Println("Dry run: no state written, nothing pruned.")
		return nil
	}

	if len(stale) > 0 && rf.prune {
		deleted, outcomes := ExecutePrune(client, rf.systemKey, stale)
		for _, o := range outcomes {
			suffix := ""
			if o.Detail != "" {
				suffix = " — " + o.Detail
			}
			fmt.Printf("  ✂ %s [%s]%s\n", o.Key, o.Action, suffix)
		}
		for _, key := range deleted {
			delete(managed, key)
		}
	} else if len(stale) > 0 {
		fmt.Println("Pass -prune to delete them. They remain in the managed set until pruned.")
	}

	if err := SaveManagedSet(client, rf.systemKey, managed); err != nil {
		// The push succeeded; only the bookkeeping failed. Say so precisely.
		return fmt.Errorf("push succeeded but recording the managed set failed (next run will still converge): %w", err)
	}
	return nil
}
