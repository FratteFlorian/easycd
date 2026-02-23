package inventory

import "github.com/flo-mic/simplecd/internal/api"

// storedInventory is persisted per project at /var/lib/simplecd/<project>/inventory.json
type storedInventory struct {
	Packages []string              `json:"packages"`
	Services []api.InventoryService `json:"services"`
	Users    []api.InventoryUser   `json:"users"`
}

// globalState is persisted at /var/lib/simplecd/.global/package-owners.json
// It maps package names to the set of projects that declare them.
type globalState struct {
	PackageOwners map[string][]string `json:"package_owners"` // pkg → []projectName
}
