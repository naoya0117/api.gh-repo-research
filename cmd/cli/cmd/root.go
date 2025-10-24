package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "shuron-cli",
	Short: "CLI tool for thesis project repository analysis",
	Long: `A CLI tool for managing GitHub repositories for the thesis project.
This tool provides functionality for collecting repositories and performing setup operations.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(collectCmd)
	rootCmd.AddCommand(setupCmd)
}
