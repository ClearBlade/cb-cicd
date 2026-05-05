package main

import (
	"fmt"
	"strings"

	cb "github.com/clearblade/Go-SDK"
)

// newClient creates a ClearBlade DevClient authenticated via email and password.
func newClient(platformURL, email, password string) (*cb.DevClient, error) {
	if platformURL == "" {
		return nil, fmt.Errorf("platform URL is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if password == "" {
		return nil, fmt.Errorf("password is required")
	}

	// SDK endpoints start with '/', so strip any trailing slash to avoid double-slash URLs.
	platformURL = strings.TrimRight(platformURL, "/")

	client := cb.NewDevClientWithAddrs(platformURL, "", email, password)
	if _, err := client.Authenticate(); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	return client, nil
}

// newClientWithToken creates a ClearBlade DevClient pre-authenticated via a dev token.
func newClientWithToken(platformURL, token string) (*cb.DevClient, error) {
	if platformURL == "" {
		return nil, fmt.Errorf("platform URL is required")
	}
	if token == "" {
		return nil, fmt.Errorf("dev token is required")
	}

	platformURL = strings.TrimRight(platformURL, "/")

	return cb.NewDevClientWithTokenAndAddrs(platformURL, "", token, ""), nil
}
