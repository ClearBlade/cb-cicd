package main

import "strings"

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
