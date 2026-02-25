package inventory

import (
	"fmt"
	"io"

	"github.com/flo-mic/eacd/internal/api"
)

// Reconcile brings the system state in line with the desired inventory.
// It installs/removes packages, manages services, and ensures users exist.
// State is persisted so subsequent deployments can diff correctly.
func Reconcile(project string, desired *api.Inventory, log io.Writer) error {
	stored, err := loadStoredInventory(project)
	if err != nil {
		return fmt.Errorf("loading stored inventory: %w", err)
	}
	gs, err := loadGlobalState()
	if err != nil {
		return fmt.Errorf("loading global state: %w", err)
	}

	pm, err := detectPackageManager()
	if err != nil {
		return fmt.Errorf("detecting package manager: %w", err)
	}

	// --- Packages ---
	toAdd, toRemove := diffStrings(desired.Packages, stored.Packages)

	if len(toAdd) > 0 {
		fmt.Fprintf(log, "[eacd] Installing packages: %v\n", toAdd)
		if err := installPackages(pm, toAdd, log); err != nil {
			return fmt.Errorf("installing packages: %w", err)
		}
	}
	// Update ownership only after successful install
	updateOwnership(gs, project, desired.Packages, stored.Packages)
	for _, pkg := range toRemove {
		owners := gs.PackageOwners[pkg]
		if len(owners) > 0 {
			fmt.Fprintf(log, "[eacd] Skipping removal of %s (still needed by: %v)\n", pkg, owners)
			continue
		}
		fmt.Fprintf(log, "[eacd] Removing package: %s\n", pkg)
		if err := removePackage(pm, pkg, log); err != nil {
			// Non-fatal: log and continue
			fmt.Fprintf(log, "[eacd] WARNING: could not remove %s: %v\n", pkg, err)
		}
		delete(gs.PackageOwners, pkg)
	}

	// --- Services ---
	for _, svc := range desired.Services {
		if err := reconcileService(svc, log); err != nil {
			return fmt.Errorf("reconciling service %s: %w", svc.Name, err)
		}
	}

	// --- Users ---
	for _, u := range desired.Users {
		if err := ensureUser(u, log); err != nil {
			return fmt.Errorf("ensuring user %s: %w", u.Name, err)
		}
	}

	// Persist new state
	stored.Packages = desired.Packages
	stored.Services = desired.Services
	stored.Users = desired.Users

	if err := saveStoredInventory(project, stored); err != nil {
		return fmt.Errorf("saving inventory state: %w", err)
	}
	if err := saveGlobalState(gs); err != nil {
		return fmt.Errorf("saving global state: %w", err)
	}

	return nil
}

// diffStrings returns elements in desired but not stored (toAdd)
// and elements in stored but not desired (toRemove).
func diffStrings(desired, stored []string) (toAdd, toRemove []string) {
	desiredSet := toSet(desired)
	storedSet := toSet(stored)

	for pkg := range desiredSet {
		if !storedSet[pkg] {
			toAdd = append(toAdd, pkg)
		}
	}
	for pkg := range storedSet {
		if !desiredSet[pkg] {
			toRemove = append(toRemove, pkg)
		}
	}
	return
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

// updateOwnership updates the global package-owners map.
// It adds the project as owner for all packages in newPkgs,
// and removes the project as owner for packages that are in oldPkgs but not newPkgs.
func updateOwnership(gs *globalState, project string, newPkgs, oldPkgs []string) {
	newSet := toSet(newPkgs)
	oldSet := toSet(oldPkgs)

	// Add ownership for new packages
	for pkg := range newSet {
		owners := gs.PackageOwners[pkg]
		if !containsStr(owners, project) {
			gs.PackageOwners[pkg] = append(owners, project)
		}
	}

	// Remove ownership for dropped packages
	for pkg := range oldSet {
		if !newSet[pkg] {
			gs.PackageOwners[pkg] = removeStr(gs.PackageOwners[pkg], project)
			if len(gs.PackageOwners[pkg]) == 0 {
				delete(gs.PackageOwners, pkg)
			}
		}
	}
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func removeStr(ss []string, s string) []string {
	out := ss[:0]
	for _, v := range ss {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}
