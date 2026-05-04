package main

import (
	"flag"
	"fmt"
	"os"
)

const defaultConfigPath = "./cicd-config.json"

var version = "dev"

type runFlags struct {
	files      []string
	email      string
	password   string
	systemKey  string
	url        string
	all        bool
	configPath string
}

// multiFlag allows -file to be specified multiple times.
type multiFlag []string

func (m *multiFlag) String() string     { return fmt.Sprintf("%v", *m) }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

const helpText = `usage: cb-cicd <run|test|help> [flags]

Subcommands:
  run   sync matched resources to the ClearBlade platform
  test  dry run — shows what would be synced without pushing
  help  show this help message

Flags:
  -email      <string>   ClearBlade developer email    (env: CICD_EMAIL)
  -password   <string>   ClearBlade developer password (env: CICD_PASSWORD)
  -system-key <string>   ClearBlade system key         (env: CICD_SYSTEM_KEY)
  -url        <string>   ClearBlade platform URL       (env: CICD_URL)
  -config     <string>   path to cicd-config.json      (default: ./cicd-config.json)
  -all                   sync all whitelisted resources (ignores -file)
  -file       <path>     changed file path             (repeatable)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, helpText)
		os.Exit(1)
	}

	subcommand := os.Args[1]
	if subcommand == "help" || subcommand == "--help" || subcommand == "-h" {
		fmt.Print(helpText)
		return
	}

	if subcommand == "version" || subcommand == "--version" || subcommand == "-v" {
		fmt.Println(version)
		return
	}

	if subcommand != "run" && subcommand != "test" {
		fmt.Fprintf(os.Stderr, "unknown subcommand %q; expected 'run', 'test', 'version', or 'help'\n", subcommand)
		os.Exit(1)
	}

	fs := flag.NewFlagSet(subcommand, flag.ExitOnError)

	var files multiFlag
	email := fs.String("email", "", "ClearBlade developer email")
	password := fs.String("password", "", "ClearBlade developer password")
	systemKey := fs.String("system-key", "", "ClearBlade system key")
	url := fs.String("url", "", "ClearBlade platform URL")
	all := fs.Bool("all", false, "sync all whitelisted resources, ignoring -file flags")
	configPath := fs.String("config", defaultConfigPath, "path to cicd-config.json")
	fs.Var(&files, "file", "changed file path (repeatable)")

	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "flag error: %s\n", err)
		os.Exit(1)
	}

	// Env var fallbacks for secrets.
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

	rf := runFlags{
		files:      files,
		email:      *email,
		password:   *password,
		systemKey:  *systemKey,
		url:        *url,
		all:        *all,
		configPath: *configPath,
	}

	var err error
	switch subcommand {
	case "run":
		err = runSync(rf, false)
	case "test":
		err = runSync(rf, true)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func runSync(rf runFlags, isDryRun bool) error {
	cfg, err := LoadConfig(rf.configPath)
	if err != nil {
		return err
	}

	if rf.email == "" || rf.password == "" || rf.systemKey == "" || rf.url == "" {
		return fmt.Errorf("email, password, system-key, and url are all required (set via flags or CICD_EMAIL / CICD_PASSWORD / CICD_SYSTEM_KEY / CICD_URL)")
	}

	client, err := newClient(rf.url, rf.email, rf.password)
	if err != nil {
		return err
	}

	// Determine which resources are in scope.
	var resources []SyncResource
	if rf.all || len(rf.files) == 0 {
		resources = cfg.SyncResources
		fmt.Printf("Syncing all %d whitelisted resources.\n", len(resources))
	} else {
		resources = MatchResources(rf.files, cfg.SyncResources)
		if len(resources) == 0 {
			fmt.Println("No whitelisted resources matched the provided files; nothing to sync.")
			return nil
		}
		fmt.Printf("Matched %d resource(s) from %d changed file(s).\n", len(resources), len(rf.files))
	}

	// Log what will be synced.
	for _, r := range resources {
		fmt.Printf("  → %s (%s)\n", r.Name, r.Type)
	}

	// Build temp dir containing only the scoped resources.
	systemDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not determine working directory: %w", err)
	}

	tempDir, err := BuildTempDir(systemDir, resources)
	if err != nil {
		// Attempt cleanup even on partial build failure.
		os.RemoveAll(tempDir)
		return fmt.Errorf("could not build temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	return PushTempDir(tempDir, rf.systemKey, client, isDryRun)
}
