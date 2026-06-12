package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	cb "github.com/clearblade/Go-SDK"
)

const defaultConfigPath = "./cicd-config.json"

var version = "dev"

type runFlags struct {
	files      []string
	devToken   string
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

// parseRunFlags parses run/test flags. Any positional argument left after flag
// parsing is rejected: Go's flag package stops at the first non-flag token, so a
// leftover positional means an unquoted path with a space shattered the argument
// list — silently dropping every -file after it and truncating the sync scope.
func parseRunFlags(name string, args []string) (runFlags, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)

	var files multiFlag
	devToken := fs.String("dev-token", "", "ClearBlade developer token")
	email := fs.String("email", "", "ClearBlade developer email")
	password := fs.String("password", "", "ClearBlade developer password")
	systemKey := fs.String("system-key", "", "ClearBlade system key")
	url := fs.String("url", "", "ClearBlade platform URL")
	all := fs.Bool("all", false, "sync all whitelisted resources, ignoring -file/-files-from")
	configPath := fs.String("config", defaultConfigPath, "path to cicd-config.json")
	filesFrom := fs.String("files-from", "", "file containing newline-delimited changed file paths")
	fs.Var(&files, "file", "changed file path (repeatable)")

	if err := fs.Parse(args); err != nil {
		return runFlags{}, err
	}

	if fs.NArg() > 0 {
		return runFlags{}, fmt.Errorf(
			"unexpected positional argument(s) %q — a file path containing a space was split by the shell; pass paths via -files-from (newline-delimited file) or quote each -file value",
			fs.Args())
	}

	if *filesFrom != "" {
		fromFile, err := readFilesFrom(*filesFrom)
		if err != nil {
			return runFlags{}, err
		}
		files = append(files, fromFile...)
	}

	// Env var fallbacks for secrets.
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

	return runFlags{
		files:      files,
		devToken:   *devToken,
		email:      *email,
		password:   *password,
		systemKey:  *systemKey,
		url:        *url,
		all:        *all,
		configPath: *configPath,
	}, nil
}

// readFilesFrom reads a newline-delimited list of file paths, tolerating CRLF
// line endings and skipping blank lines. Paths may contain spaces.
func readFilesFrom(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read -files-from list: %w", err)
	}
	var files []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

const helpText = `usage: cb-cicd <run|test|help> [flags]

Subcommands:
  run   sync matched resources to the ClearBlade platform
  test  dry run — shows what would be synced without pushing
  help  show this help message

Flags:
  -dev-token  <string>   ClearBlade developer token    (env: CICD_DEV_TOKEN)
  -email      <string>   ClearBlade developer email    (env: CICD_EMAIL)
  -password   <string>   ClearBlade developer password (env: CICD_PASSWORD)
  -system-key <string>   ClearBlade system key         (env: CICD_SYSTEM_KEY)
  -url        <string>   ClearBlade platform URL       (env: CICD_URL)
  -config     <string>   path to cicd-config.json      (default: ./cicd-config.json)
  -all                   sync all whitelisted resources; required if no -file flags are provided
  -file       <path>     changed file path             (repeatable)
  -files-from <path>     file containing newline-delimited changed file paths
                         (safe for paths with spaces; combined with -file)

Authentication: provide either -dev-token or both -email and -password.
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

	rf, err := parseRunFlags(subcommand, os.Args[2:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "flag error: %s\n", err)
		os.Exit(1)
	}

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

	// Determine which resources are in scope.
	var resources []SyncResource
	if rf.all {
		resources = cfg.SyncResources
		fmt.Printf("Syncing all %d whitelisted resources.\n", len(resources))
	} else if len(rf.files) == 0 {
		fmt.Println("No files specified and -all not set; nothing to sync.")
		return nil
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
