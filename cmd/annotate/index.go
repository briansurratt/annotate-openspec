package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Index management commands",
}

var indexRebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Trigger an index rebuild on the daemon (not yet implemented)",
	Run:   runIndexRebuild,
}

func init() {
	indexCmd.AddCommand(indexRebuildCmd)
	rootCmd.AddCommand(indexCmd)
}

func runIndexRebuild(cmd *cobra.Command, args []string) {
	fmt.Println("not yet implemented")
}
