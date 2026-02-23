package cmd

import (
	"os"

	"github.com/flo-mic/simplecd/internal/api"
	"gopkg.in/yaml.v3"
)

// inventoryFile mirrors the .simplecd/inventory.yaml structure.
type inventoryFile struct {
	Packages []string        `yaml:"packages"`
	Services []serviceSpec   `yaml:"services"`
	Users    []userSpec      `yaml:"users"`
}

type serviceSpec struct {
	Name    string `yaml:"name"`
	Enabled bool   `yaml:"enabled"`
	State   string `yaml:"state"`
}

type userSpec struct {
	Name   string   `yaml:"name"`
	Home   string   `yaml:"home"`
	Shell  string   `yaml:"shell"`
	Groups []string `yaml:"groups"`
}

// loadInventory reads .simplecd/inventory.yaml and returns an api.Inventory.
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

	inv := &api.Inventory{
		Packages: f.Packages,
	}
	for _, s := range f.Services {
		inv.Services = append(inv.Services, api.InventoryService{
			Name:    s.Name,
			Enabled: s.Enabled,
			State:   s.State,
		})
	}
	for _, u := range f.Users {
		inv.Users = append(inv.Users, api.InventoryUser{
			Name:   u.Name,
			Home:   u.Home,
			Shell:  u.Shell,
			Groups: u.Groups,
		})
	}
	return inv, nil
}
