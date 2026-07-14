package main

import (
	"flag"
	"fmt"
	"os"

	cb "github.com/clearblade/Go-SDK"
)

// reconcile pushes the ENTIRE whitelist every run: desired state = whitelist +
// repo content, and the platform upsert converges the system onto it.
//
// This replaces diff-scoped syncing. A diff sync must remember what it last
// applied, and that memory — a git tag, a workflow event payload — is a claim
// rather than an observation, so it eventually lies: a failed sync strands its
// range behind a green checkmark, a whitelist-only change matches no files and
// deploys nothing, a new system never bootstraps. Reconcile has none of those
// states: pushing an unchanged artifact is a no-op, so converging repeatedly is
// safe and a failed run is repaired by the next one.
//
// Reconcile is deliberately STATELESS. Deletion — the one thing convergence
// cannot infer — is not guessed at from recorded state: a whitelist removal
// arrives as a reviewed PR diff, so the operator names the artifact explicitly
// via `cb-cicd prune <type:name>`. CI never deletes anything.

type reconcileFlags struct {
	devToken   string
	email      string
	password   string
	systemKey  string
	url        string
	configPath string
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
	dryRun := fs.Bool("dry-run", false, "show what would change without pushing (the platform's semantic diff)")

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

	return PushTempDir(tempDir, rf.systemKey, client, rf.dryRun)
}
