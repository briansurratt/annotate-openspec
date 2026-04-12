// Package main is the entry point for the annotate CLI binary.
// It wires the cobra root command and delegates to subcommands.
package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var cfgPath string

// rootCmd is the top-level annotate command. All subcommands attach to it.
var rootCmd = &cobra.Command{
	Use:          "annotate",
	Short:        "Annotate — AI-powered note annotation daemon and CLI",
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", defaultConfigPath(), "path to config file")
}

// defaultConfigPath returns the default config file location (~/.annotate/config.yaml).
func defaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".annotate", "config.yaml")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
