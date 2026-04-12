package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Fetch and render metrics from the daemon (not yet implemented)",
	Run:   runReport,
}

func init() {
	rootCmd.AddCommand(reportCmd)
}

func runReport(cmd *cobra.Command, args []string) {
	fmt.Println("not yet implemented")
}
