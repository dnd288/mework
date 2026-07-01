package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// OfflineAgentInfo describes a locally-registered offline agent.
type OfflineAgentInfo struct {
	Name        string `json:"name"`
	SocketPath  string `json:"socket_path"`
	Status      string `json:"status"`
	Workspace   string `json:"workspace"`
	Backend     string `json:"backend"`
}

// offlineRegistryPath returns the path to the local agent registry file.
func offlineRegistryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".mework", "offline-agents.json")
	}
	return filepath.Join(home, ".mework", "offline-agents.json")
}

// RegisterOfflineAgent writes the agent's info to the local registry so
// "mework agent list" and "mework agent send" can find it.
func RegisterOfflineAgent(info OfflineAgentInfo) error {
	mu.Lock()
	defer mu.Unlock()

	registry, err := loadOfflineRegistry()
	if err != nil {
		return err
	}
	registry[info.Name] = info
	return saveOfflineRegistry(registry)
}

// UnregisterOfflineAgent removes an agent from the local registry.
func UnregisterOfflineAgent(name string) error {
	mu.Lock()
	defer mu.Unlock()

	registry, err := loadOfflineRegistry()
	if err != nil {
		return err
	}
	delete(registry, name)
	return saveOfflineRegistry(registry)
}

// ListOfflineAgents returns all locally-registered offline agents.
func ListOfflineAgents() ([]OfflineAgentInfo, error) {
	mu.RLock()
	defer mu.RUnlock()

	registry, err := loadOfflineRegistry()
	if err != nil {
		return nil, err
	}
	agents := make([]OfflineAgentInfo, 0, len(registry))
	for _, info := range registry {
		agents = append(agents, info)
	}
	return agents, nil
}

// LookupOfflineAgent returns the info for a named offline agent, or nil.
func LookupOfflineAgent(name string) (*OfflineAgentInfo, error) {
	mu.RLock()
	defer mu.RUnlock()

	registry, err := loadOfflineRegistry()
	if err != nil {
		return nil, err
	}
	info, ok := registry[name]
	if !ok {
		return nil, nil
	}
	return &info, nil
}

// --- internal helpers ---

var (
	mu          sync.RWMutex
)

func loadOfflineRegistry() (map[string]OfflineAgentInfo, error) {
	path := offlineRegistryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]OfflineAgentInfo), nil
		}
		return nil, err
	}
	var registry map[string]OfflineAgentInfo
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("parse offline agent registry: %w", err)
	}
	return registry, nil
}

func saveOfflineRegistry(registry map[string]OfflineAgentInfo) error {
	path := offlineRegistryPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
