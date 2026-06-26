package main

import (
	"path/filepath"
	"strings"
)

// MatchResources returns the subset of whitelist entries that have at least one
// corresponding path matching a changed file. Directory resources match if any
// changed file has that directory as a prefix.
func MatchResources(changedFiles []string, whitelist []SyncResource) []SyncResource {
	var matched []SyncResource
	for _, r := range whitelist {
		paths, err := ResourcePaths(r)
		if err != nil {
			continue
		}
		if anyPathMatches(paths, changedFiles) {
			matched = append(matched, r)
		}
	}
	return matched
}

// WhitelistFileChanged reports whether the cicd-config (whitelist) file itself is
// among the changed files. Changed-file paths are relative to the system dir
// (e.g. "cicd-config.json"), so we match on the config's basename.
func WhitelistFileChanged(changedFiles []string, configPath string) bool {
	base := filepath.Base(configPath)
	for _, f := range changedFiles {
		if filepath.Base(f) == base {
			return true
		}
	}
	return false
}

func anyPathMatches(paths []ResourcePath, changedFiles []string) bool {
	for _, rp := range paths {
		for _, f := range changedFiles {
			if rp.IsDir {
				if strings.HasPrefix(f, rp.Path+"/") {
					return true
				}
			} else {
				if f == rp.Path {
					return true
				}
			}
		}
	}
	return false
}
