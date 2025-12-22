package cmd

import (
	"cdp/internal"
	"fmt"

	"github.com/spf13/cobra"
)

var chromiumCmd = &cobra.Command{
	Use:   "chromium",
	Short: "Manage Chromium installation",
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Download and install Chromium for Testing",
	RunE:  runInstall,
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove installed Chromium",
	RunE:  runUninstall,
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade to latest Chromium version",
	RunE:  runUpgrade,
}

var (
	installChannel string
	installPath    string
	uninstallVer   string
	uninstallPath  string
	upgradeChannel string
	upgradePath    string
	upgradeClean   bool
)

func init() {
	installCmd.Flags().StringVar(&installChannel, "channel", "Stable", "Release channel (Stable|Beta|Dev|Canary)")
	installCmd.Flags().StringVar(&installPath, "path", "", "Custom install location")
	uninstallCmd.Flags().StringVar(&uninstallVer, "version", "", "Specific version to remove (default: all)")
	uninstallCmd.Flags().StringVar(&uninstallPath, "path", "", "Custom install location")
	upgradeCmd.Flags().StringVar(&upgradeChannel, "channel", "Stable", "Release channel (Stable|Beta|Dev|Canary)")
	upgradeCmd.Flags().StringVar(&upgradePath, "path", "", "Custom install location")
	upgradeCmd.Flags().BoolVar(&upgradeClean, "clean", false, "Remove old versions after upgrade")
	chromiumCmd.AddCommand(installCmd, uninstallCmd, upgradeCmd)
	rootCmd.AddCommand(chromiumCmd)
}

func runInstall(_ *cobra.Command, _ []string) error {
	bin, err := internal.Install(installChannel, installPath)
	if err != nil {
		return err
	}
	fmt.Println(bin)
	return nil
}

func runUninstall(_ *cobra.Command, _ []string) error {
	err := internal.Uninstall(uninstallVer, uninstallPath)
	if err != nil {
		return err
	}
	if uninstallVer != "" {
		fmt.Println("removed", uninstallVer)
	} else {
		fmt.Println("removed all")
	}
	return nil
}

func runUpgrade(_ *cobra.Command, _ []string) error {
	bin, err := internal.Upgrade(upgradeChannel, upgradePath, upgradeClean)
	if err != nil {
		return err
	}
	if bin == "" {
		fmt.Println("already up to date")
	} else {
		fmt.Println(bin)
	}
	return nil
}
