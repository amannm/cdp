package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Error types for exit code differentiation
type UserError struct{ Err error }
type RuntimeError struct{ Err error }

func (e UserError) Error() string    { return e.Err.Error() }
func (e RuntimeError) Error() string { return e.Err.Error() }
func (e UserError) Unwrap() error    { return e.Err }
func (e RuntimeError) Unwrap() error { return e.Err }

// Constructors
func ErrUser(format string, args ...interface{}) error {
	return UserError{Err: fmt.Errorf(format, args...)}
}
func ErrRuntime(format string, args ...interface{}) error {
	return RuntimeError{Err: fmt.Errorf(format, args...)}
}

var Verbose bool

var rootCmd = &cobra.Command{
	Use:           "cdp",
	Short:         "Chrome DevTools Protocol CLI",
	Long:          "A context-efficient tool for LLM agents to interact with the web via CDP.",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "Enable debug output")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		var userErr UserError
		var runtimeErr RuntimeError
		switch {
		case errors.As(err, &userErr):
			os.Exit(1)
		case errors.As(err, &runtimeErr):
			os.Exit(2)
		default:
			os.Exit(1) // Default to user error
		}
	}
}
