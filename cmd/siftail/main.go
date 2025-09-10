package main

import (
	"fmt"
	"os"

	"github.com/germanoeich/siftail/internal/cli"
)

func main() {
	// Parse command-line arguments
	config, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "siftail: %v\n", err)
		os.Exit(1)
	}

	// Handle help and version (already handled in ParseArgs, but check if we should exit)
	if config.ShowHelp || config.ShowVersion {
		return
	}

	// Validate configuration
	if err := cli.ValidateConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "siftail: %v\n", err)
		os.Exit(1)
	}

	// Run the application
	if err := cli.Run(config); err != nil {
		fmt.Fprintf(os.Stderr, "siftail: %v\n", err)
		os.Exit(1)
	}
}
