package cmd

import (
	"os"

	"github.com/flo-mic/eacd/internal/api"
	"gopkg.in/yaml.v3"
)

// inventoryFile mirrors the .eacd/inventory.yaml structure.
// gopkg.in/yaml.v3 maps lowercase YAML keys to unexported field names by default,
// so api.InventoryService and api.InventoryUser unmarshal correctly without extra tags.
type inventoryFile struct {
	Packages []string               `yaml:"packages"`
	Services []api.InventoryService `yaml:"services"`
	Users    []api.InventoryUser    `yaml:"users"`
}

// loadInventory reads .eacd/inventory.yaml and returns an api.Inventory.
// Returns nil, nil if the file does not exist.
func loadInventory(path string) (*api.Inventory, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var f inventoryFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}

	if len(f.Packages) == 0 && len(f.Services) == 0 && len(f.Users) == 0 {
		return nil, nil
	}

	return &api.Inventory{
		Packages: f.Packages,
		Services: f.Services,
		Users:    f.Users,
	}, nil
}
