package main

import "fmt"

// ResourcePath represents a single path for a resource on disk.
// IsDir=true means the path is a directory that should be matched by prefix
// and copied recursively. IsDir=false means it is an exact file path.
type ResourcePath struct {
	Path  string
	IsDir bool
}

// ResourcePaths returns the disk path(s) for the given sync resource.
// All paths use forward slashes and are relative to the system directory root,
// matching cblib's syspath conventions exactly.
func ResourcePaths(r SyncResource) ([]ResourcePath, error) {
	n := r.Name
	switch r.Type {
	// Code — services and libraries live in a named subdirectory
	case "service":
		return []ResourcePath{{Path: fmt.Sprintf("code/services/%s", n), IsDir: true}}, nil
	case "library":
		return []ResourcePath{{Path: fmt.Sprintf("code/libraries/%s", n), IsDir: true}}, nil

	// Both types share the same file on disk; BuildTempDir strips "items" for collection_schema.
	case "collection", "collection_schema":
		return []ResourcePath{{Path: fmt.Sprintf("data/%s.json", n)}}, nil

	// Simple single-file resources
	case "trigger":
		return []ResourcePath{{Path: fmt.Sprintf("triggers/%s.json", n)}}, nil
	case "timer":
		return []ResourcePath{{Path: fmt.Sprintf("timers/%s.json", n)}}, nil
	case "webhook":
		return []ResourcePath{{Path: fmt.Sprintf("webhooks/%s.json", n)}}, nil
	case "deployment":
		return []ResourcePath{{Path: fmt.Sprintf("deployments/%s.json", n)}}, nil
	case "role":
		return []ResourcePath{{Path: fmt.Sprintf("roles/%s.json", n)}}, nil
	case "user":
		return []ResourcePath{{Path: fmt.Sprintf("users/%s.json", n)}}, nil
	case "secret":
		return []ResourcePath{{Path: fmt.Sprintf("secrets/%s.json", n)}}, nil
	case "edge":
		return []ResourcePath{{Path: fmt.Sprintf("edges/%s.json", n)}}, nil
	case "device":
		return []ResourcePath{{Path: fmt.Sprintf("devices/%s.json", n)}}, nil
	case "plugin":
		return []ResourcePath{{Path: fmt.Sprintf("plugins/%s.json", n)}}, nil

	// Hyphenated directory names — confirmed from cblib syspath regexes
	case "service_cache":
		return []ResourcePath{{Path: fmt.Sprintf("shared-caches/%s.json", n)}}, nil
	case "external_database":
		return []ResourcePath{{Path: fmt.Sprintf("external-databases/%s.json", n)}}, nil
	case "bucket_set":
		return []ResourcePath{{Path: fmt.Sprintf("bucket-sets/%s.json", n)}}, nil
	case "bucket_set_files":
		return []ResourcePath{{Path: fmt.Sprintf("bucket-set-files/%s", n), IsDir: true}}, nil
	case "file_store":
		return []ResourcePath{{Path: fmt.Sprintf("file-stores/%s.json", n)}}, nil

	// Directory resources
	case "portal":
		return []ResourcePath{{Path: fmt.Sprintf("portals/%s", n), IsDir: true}}, nil
	case "adaptor":
		return []ResourcePath{{Path: fmt.Sprintf("adapters/%s", n), IsDir: true}}, nil

	// Schema singletons — name field is ignored, path is fixed
	case "device_schema":
		return []ResourcePath{{Path: "devices/schema.json"}}, nil
	case "user_schema":
		return []ResourcePath{{Path: "users/schema.json"}}, nil
	case "edge_schema":
		return []ResourcePath{{Path: "edges/schema.json"}}, nil

	default:
		return nil, fmt.Errorf("unknown resource type %q", r.Type)
	}
}
