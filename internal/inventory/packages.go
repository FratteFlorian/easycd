package inventory

import (
	"fmt"
	"io"
	"os/exec"
)

type packageManager struct {
	name    string
	install []string // args for install, package names appended
	remove  []string // args for remove, package names appended
}

func detectPackageManager() (*packageManager, error) {
	candidates := []packageManager{
		{name: "apt-get", install: []string{"apt-get", "install", "-y"}, remove: []string{"apt-get", "remove", "-y"}},
		{name: "dnf", install: []string{"dnf", "install", "-y"}, remove: []string{"dnf", "remove", "-y"}},
		{name: "yum", install: []string{"yum", "install", "-y"}, remove: []string{"yum", "remove", "-y"}},
		{name: "pacman", install: []string{"pacman", "-S", "--noconfirm"}, remove: []string{"pacman", "-R", "--noconfirm"}},
	}
	for _, pm := range candidates {
		if _, err := exec.LookPath(pm.name); err == nil {
			p := pm
			return &p, nil
		}
	}
	return nil, fmt.Errorf("no supported package manager found (tried apt-get, dnf, yum, pacman)")
}

func updatePackageIndex(pm *packageManager, log io.Writer) error {
	switch pm.name {
	case "apt-get":
		return runCmd(log, "apt-get", "update", "-qq")
	case "dnf", "yum":
		return runCmd(log, pm.name, "makecache", "-q")
	default:
		return nil // pacman updates index as part of -S
	}
}

func installPackages(pm *packageManager, pkgs []string, log io.Writer) error {
	if err := updatePackageIndex(pm, log); err != nil {
		fmt.Fprintf(log, "[simplecd] warning: package index update failed: %v\n", err)
	}
	args := append(pm.install, pkgs...)
	return runCmd(log, args[0], args[1:]...)
}

func removePackage(pm *packageManager, pkg string, log io.Writer) error {
	args := append(pm.remove, pkg)
	return runCmd(log, args[0], args[1:]...)
}

func runCmd(log io.Writer, name string, args ...string) error {
	fmt.Fprintf(log, "[simplecd] $ %s %v\n", name, args)
	cmd := exec.Command(name, args...)
	cmd.Env = append(cmd.Environ(), "DEBIAN_FRONTEND=noninteractive")
	cmd.Stdout = log
	cmd.Stderr = log
	return cmd.Run()
}
