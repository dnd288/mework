package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type identity struct {
	RunnerID string `json:"runner_id"`
	Secret   string `json:"secret"`
}

func IdentityPath() string {
	return filepath.Join(MeworkDir(), "identity.json")
}

func SaveIdentity(runnerID, secret string) error {
	if err := ensureDir(MeworkDir()); err != nil {
		return err
	}
	data, err := json.MarshalIndent(identity{RunnerID: runnerID, Secret: secret}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(IdentityPath(), data, 0o600)
}

func LoadIdentity() (string, string, error) {
	data, err := os.ReadFile(IdentityPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil
		}
		return "", "", err
	}

	var id identity
	if err := json.Unmarshal(data, &id); err != nil {
		return "", "", err
	}
	return id.RunnerID, id.Secret, nil
}

func RemoveIdentity() error {
	err := os.Remove(IdentityPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
