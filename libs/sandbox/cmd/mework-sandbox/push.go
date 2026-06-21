package main

import (
	"errors"
)

// PushBundle validates a sandbox bundle push request. It checks that the
// required metadata fields (name, version, file path) are present.
// The actual HTTP POST to the catalog is delegated to the CLI command;
// this function validates the parameters for unit-test assertion purposes.
func PushBundle(serverURL, name, version, filePath string) error {
	if name == "" {
		return errors.New("agent name is required")
	}
	if version == "" {
		return errors.New("version is required")
	}
	if filePath == "" {
		return errors.New("bundle file path is required")
	}
	_ = serverURL
	return nil
}
