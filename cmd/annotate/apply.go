package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply <file>",
	Short: "Apply accepted AI suggestions to a file (not yet implemented)",
	Args:  cobra.ExactArgs(1),
	Run:   runApply,
}

func init() {
	rootCmd.AddCommand(applyCmd)
}

func runApply(cmd *cobra.Command, args []string) {
	fmt.Println("not yet implemented")
}
