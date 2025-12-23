package cmd

import (
	"cdp/internal/install"
	"fmt"

	"github.com/spf13/cobra"
)

var chromeCmd = &cobra.Command{
	Use:   "chrome",
	Short: "Manage Chrome installation",
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Download and install Chrome",
	RunE:  runInstall,
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove installed Chrome",
	RunE:  runUninstall,
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade to latest Chrome version",
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
	installCmd.Flags().StringVarP(&installChannel, "channel", "c", "Stable", "Release channel (Stable|Beta|Dev|Canary)")
	installCmd.Flags().StringVarP(&installPath, "path", "p", "", "Custom install location")
	uninstallCmd.Flags().StringVarP(&uninstallVer, "version", "v", "", "Specific version to remove (default: all)")
	uninstallCmd.Flags().StringVarP(&uninstallPath, "path", "p", "", "Custom install location")
	upgradeCmd.Flags().StringVarP(&upgradeChannel, "channel", "c", "Stable", "Release channel (Stable|Beta|Dev|Canary)")
	upgradeCmd.Flags().StringVarP(&upgradePath, "path", "p", "", "Custom install location")
	upgradeCmd.Flags().BoolVar(&upgradeClean, "clean", false, "Remove old versions after upgrade")
	chromeCmd.AddCommand(installCmd, uninstallCmd, upgradeCmd)
	rootCmd.AddCommand(chromeCmd)
}

func runInstall(_ *cobra.Command, _ []string) error {
	bin, err := install.Install(installChannel, installPath)
	if err != nil {
		return err
	}
	fmt.Println(bin)
	return nil
}

func runUninstall(_ *cobra.Command, _ []string) error {
	err := install.Uninstall(uninstallVer, uninstallPath)
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
	bin, err := install.Upgrade(upgradeChannel, upgradePath, upgradeClean)
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
