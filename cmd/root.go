package cmd

import (
	"cdp/internal/utility"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "cdp",
	Short:         "Chrome DevTools Protocol CLI",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.PersistentFlags().BoolVarP(&utility.Verbose, "verbose", "v", false, "Enable debug output")
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		switch {
		case utility.IsUserError(err):
			os.Exit(1)
		case utility.IsRuntimeError(err):
			os.Exit(2)
		default:
			os.Exit(1)
		}
	}
}
