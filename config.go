package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type SyncResource struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	PushRows     bool     `json:"push_rows"`
	PushRoles    bool     `json:"push_roles"`
	UpsertKey    string   `json:"upsert_key"`
	SelectedRows []string `json:"selected_rows"`
}

type FileStoreConfig struct {
	Name        string `json:"name"`
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
}

type Config struct {
	SyncResources []SyncResource    `json:"sync_resources"`
	FileStores    []FileStoreConfig `json:"file_stores"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read config file %q: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("could not parse config file %q: %w", path, err)
	}

	if len(cfg.SyncResources) == 0 {
		return nil, fmt.Errorf("config has no sync_resources entries")
	}

	return &cfg, nil
}
