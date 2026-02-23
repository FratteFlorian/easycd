package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const stateDir = "/var/lib/eacd"

func projectStateDir(project string) string {
	return filepath.Join(stateDir, project)
}

func inventoryPath(project string) string {
	return filepath.Join(projectStateDir(project), "inventory.json")
}

func globalStatePath() string {
	return filepath.Join(stateDir, ".global", "package-owners.json")
}

func loadStoredInventory(project string) (*storedInventory, error) {
	path := inventoryPath(project)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &storedInventory{}, nil
	}
	if err != nil {
		return nil, err
	}
	var inv storedInventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

func saveStoredInventory(project string, inv *storedInventory) error {
	dir := projectStateDir(project)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(inventoryPath(project), data, 0644)
}

func loadGlobalState() (*globalState, error) {
	path := globalStatePath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &globalState{PackageOwners: make(map[string][]string)}, nil
	}
	if err != nil {
		return nil, err
	}
	var gs globalState
	if err := json.Unmarshal(data, &gs); err != nil {
		return nil, err
	}
	if gs.PackageOwners == nil {
		gs.PackageOwners = make(map[string][]string)
	}
	return &gs, nil
}

func saveGlobalState(gs *globalState) error {
	dir := filepath.Join(stateDir, ".global")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(gs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(globalStatePath(), data, 0644)
}
