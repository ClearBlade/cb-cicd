package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// BuildTempDir creates a temporary directory mirroring the cb-cli project
// structure and populates it with only the given resources copied from systemDir.
// The caller is responsible for os.RemoveAll when done.
func BuildTempDir(systemDir string, resources []SyncResource) (string, error) {
	tempDir, err := os.MkdirTemp("", "cb-cicd-*")
	if err != nil {
		return "", fmt.Errorf("could not create temp dir: %w", err)
	}

	for _, r := range resources {
		paths, err := ResourcePaths(r)
		if err != nil {
			return tempDir, fmt.Errorf("resource %q (%s): %w", r.Name, r.Type, err)
		}
		for _, rp := range paths {
			src := filepath.Join(systemDir, filepath.FromSlash(rp.Path))
			dst := filepath.Join(tempDir, filepath.FromSlash(rp.Path))
			if rp.IsDir {
				if err := copyDir(src, dst); err != nil {
					return tempDir, fmt.Errorf("resource %q: %w", r.Name, err)
				}
			} else {
				if err := copyFile(src, dst); err != nil {
					if rp.Optional && os.IsNotExist(err) {
						continue
					}
					return tempDir, fmt.Errorf("resource %q: %w", r.Name, err)
				}
				if r.Type == "collection_schema" {
					if err := stripCollectionItems(dst); err != nil {
						return tempDir, fmt.Errorf("resource %q: strip items: %w", r.Name, err)
					}
				}
			}
		}
	}

	return tempDir, nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// stripCollectionItems removes the "items" key from a collection JSON file so
// that only schema (columns, indexes) is uploaded — row data is left untouched
// on the platform. This mirrors cblib's copyCollectionSchemaToZip behavior.
func stripCollectionItems(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	delete(m, "items")
	out, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}
