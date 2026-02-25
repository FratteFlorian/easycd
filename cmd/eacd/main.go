package main

import (
	"fmt"
	"os"

	"github.com/flo-mic/eacd/internal/cmd"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "deploy":
		if err := cmd.Deploy(os.Args[2:], os.Stdout, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "rollback":
		if err := cmd.Rollback(os.Args[2:], os.Stdout, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "init":
		if err := cmd.Init(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "install-daemon":
		if err := cmd.InstallDaemon(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: eacd <command>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  init [--reinit]                             Initialize (or reinitialize) .eacd/ configuration")
	fmt.Fprintln(os.Stderr, "  deploy                                      Deploy the project to the configured server")
	fmt.Fprintln(os.Stderr, "  rollback                                    Restore the previous deployment snapshot")
	fmt.Fprintln(os.Stderr, "  install-daemon --host <ip> [--user <user>]  Install eacdd on any Linux host via SSH")
}
