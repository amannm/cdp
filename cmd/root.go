package cmd

import (
	"cdp/internal"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "cdp",
	Short:         "Chrome DevTools Protocol CLI",
	Long:          "A context-efficient tool for LLM agents to interact with the web via CDP.",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&internal.Verbose, "verbose", "v", false, "Enable debug output")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		switch {
		case internal.IsUserError(err):
			os.Exit(1)
		case internal.IsRuntimeError(err):
			os.Exit(2)
		default:
			os.Exit(1)
		}
	}
}
