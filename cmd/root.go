package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cdp",
	Short: "Chrome DevTools Protocol CLI",
	Long:  "A context-efficient tool for LLM agents to interact with the web via CDP.",
}

func Execute() error {
	return rootCmd.Execute()
}
