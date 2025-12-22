package cmd

import (
	"cdp/internal"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Manage browser instances",
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new Chromium instance with remote debugging",
	RunE:  runStart,
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running Chromium instance",
	RunE:  runStop,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List running Chromium instances",
	RunE:  runList,
}

var (
	startName        string
	startPort        int
	startHeadless    bool
	startUserDataDir string
	stopName         string
	stopAll          bool
)

func init() {
	startCmd.Flags().StringVar(&startName, "name", "", "Instance identifier")
	startCmd.Flags().IntVar(&startPort, "port", 0, "Remote debugging port (0 = auto)")
	startCmd.Flags().BoolVar(&startHeadless, "headless", false, "Run in headless mode")
	startCmd.Flags().StringVar(&startUserDataDir, "user-data-dir", "", "Profile directory")
	stopCmd.Flags().StringVar(&stopName, "name", "", "Instance name to stop")
	stopCmd.Flags().BoolVar(&stopAll, "all", false, "Stop all instances")
	browserCmd.AddCommand(startCmd, stopCmd, listCmd)
	rootCmd.AddCommand(browserCmd)
}

func runStart(_ *cobra.Command, _ []string) error {
	opts := internal.StartOptions{
		Name:        startName,
		Port:        startPort,
		Headless:    startHeadless,
		UserDataDir: startUserDataDir,
	}
	inst, err := internal.StartBrowser(opts)
	if err != nil {
		return err
	}
	out, err := json.Marshal(inst)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func runStop(_ *cobra.Command, _ []string) error {
	if !stopAll && stopName == "" {
		return internal.ErrUser("--name or --all required")
	}
	if stopAll {
		return internal.StopAllInstances()
	}
	return internal.StopInstance(stopName)
}

func runList(_ *cobra.Command, _ []string) error {
	instances, cleanupErrs, err := internal.ListInstances()
	if err != nil {
		return err
	}
	out, err := json.Marshal(instances)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	if cleanupErrs > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to cleanup %d stale instance(s)\n", cleanupErrs)
	}
	return nil
}
