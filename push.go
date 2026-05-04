package main

import (
	"errors"
	"fmt"
	"strconv"

	cb "github.com/clearblade/Go-SDK"
	"github.com/clearblade/cblib/fs"
	"github.com/clearblade/cblib/models/systemUpload/dryRun"
)

// noopPrompter satisfies fs.SecretPrompter. External database passwords are not
// needed in a CI/CD context — secrets come from env vars / pipeline config.
type noopPrompter struct{}

func (noopPrompter) PromptForSecret(_ string) string { return "" }

// PushTempDir zips everything in tempDir and uploads it to the platform.
// With dryRun=true it calls UploadToSystemDryRun and prints the diff without committing.
func PushTempDir(tempDir, systemKey string, client *cb.DevClient, isDryRun bool) error {
	version, err := getUploadVersion(systemKey, client)
	if err != nil {
		return err
	}
	if version < 5 {
		return fmt.Errorf("platform upload version %d is too old; version 5+ is required for zip-based push", version)
	}

	opts := fs.NewZipOptions(nil)
	opts.AllAssets = true

	fmt.Println("Building zip from temp dir...")
	zipBytes, err := fs.GetSystemZipBytes(tempDir, noopPrompter{}, opts)
	if err != nil {
		return fmt.Errorf("could not build zip: %w", err)
	}

	if isDryRun {
		return doDryRun(systemKey, zipBytes, client)
	}
	return doPush(systemKey, zipBytes, client)
}

func getUploadVersion(systemKey string, client *cb.DevClient) (int, error) {
	resp, err := client.GetSystemUploadVersion(systemKey)
	if err != nil {
		return 0, fmt.Errorf("could not get platform upload version: %w", err)
	}
	respMap, ok := resp.(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("unexpected version response type %T: %v", resp, resp)
	}
	versionStr, ok := respMap["version"].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected version field type %T", respMap["version"])
	}
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		return 0, fmt.Errorf("could not parse upload version %q: %w", versionStr, err)
	}
	return version, nil
}

func doDryRun(systemKey string, zipBytes []byte, client *cb.DevClient) error {
	fmt.Println("Running dry-run upload...")
	result, err := client.UploadToSystemDryRun(systemKey, zipBytes)
	if err != nil {
		return fmt.Errorf("dry-run failed: %w", err)
	}

	dr, err := dryRun.New(result)
	if err != nil {
		return fmt.Errorf("could not parse dry-run result: %w", err)
	}

	if dr.HasErrors() {
		return errors.New(dr.String())
	}

	if !dr.HasChanges() {
		fmt.Println("Nothing to sync.")
		return nil
	}

	fmt.Print(dr.String())
	return nil
}

func doPush(systemKey string, zipBytes []byte, client *cb.DevClient) error {
	fmt.Println("Uploading to platform...")
	result, err := client.UploadToSystem(systemKey, zipBytes)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	if err := result.Error(); err != nil {
		return fmt.Errorf("platform rejected upload: %w", err)
	}
	fmt.Println("Push complete.")
	return nil
}
