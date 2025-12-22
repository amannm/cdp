package cmd

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const versionURL = "https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json"

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

type versionInfo struct {
	Channels map[string]channelInfo `json:"channels"`
}

type channelInfo struct {
	Version   string              `json:"version"`
	Downloads map[string][]dlInfo `json:"downloads"`
}

type dlInfo struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

func detectPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "mac-arm64"
		}
		return "mac-x64"
	case "linux":
		return "linux64"
	case "windows":
		return "win64"
	}
	return ""
}

func fetchVersionInfo() (*versionInfo, error) {
	if Verbose {
		_, _ = fmt.Fprintf(os.Stderr, "fetching version info from %s\n", versionURL)
	}
	resp, err := http.Get(versionURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var info versionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

func downloadFile(url, dest string) error {
	if Verbose {
		_, _ = fmt.Fprintf(os.Stderr, "downloading %s to %s\n", url, dest)
	}
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	expectedLen := resp.ContentLength
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	if expectedLen > 0 && written != expectedLen {
		return fmt.Errorf("download incomplete: got %d bytes, expected %d", written, expectedLen)
	}
	return nil
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid path: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
			continue
		}
		os.MkdirAll(filepath.Dir(path), 0755)
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func runInstall(cmd *cobra.Command, args []string) error {
	platform := detectPlatform()
	if platform == "" {
		return ErrUser("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	info, err := fetchVersionInfo()
	if err != nil {
		return ErrRuntime("fetching version info: %v", err)
	}
	channel, ok := info.Channels[installChannel]
	if !ok {
		return ErrUser("unknown channel: %s", installChannel)
	}
	downloads, ok := channel.Downloads["chrome"]
	if !ok {
		return ErrRuntime("no chrome download for channel %s", installChannel)
	}
	var dlURL string
	for _, dl := range downloads {
		if dl.Platform == platform {
			dlURL = dl.URL
			break
		}
	}
	if dlURL == "" {
		return ErrRuntime("no download for platform %s", platform)
	}
	base := installPath
	if base == "" {
		base = ChromiumDir
	}
	versionDir := filepath.Join(base, channel.Version)
	if _, err := os.Stat(versionDir); err == nil {
		fmt.Println(binaryPath(versionDir, platform))
		return nil
	}
	os.MkdirAll(base, 0755)
	tmpZip := filepath.Join(base, "chrome.zip")
	defer os.Remove(tmpZip)
	if err := downloadFile(dlURL, tmpZip); err != nil {
		return ErrRuntime("downloading: %v", err)
	}
	if err := extractZip(tmpZip, versionDir); err != nil {
		os.RemoveAll(versionDir)
		return ErrRuntime("extracting: %v", err)
	}
	current := filepath.Join(base, "current")
	os.Remove(current)
	if err := os.Symlink(channel.Version, current); err != nil {
		return ErrRuntime("creating symlink: %v", err)
	}
	fmt.Println(binaryPath(versionDir, platform))
	return nil
}

func binaryPath(versionDir, platform string) string {
	switch {
	case strings.HasPrefix(platform, "mac"):
		return filepath.Join(versionDir, "chrome-"+platform, "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")
	case platform == "linux64":
		return filepath.Join(versionDir, "chrome-linux64", "chrome")
	case platform == "win64":
		return filepath.Join(versionDir, "chrome-win64", "chrome.exe")
	}
	return versionDir
}

func runUninstall(cmd *cobra.Command, args []string) error {
	base := uninstallPath
	if base == "" {
		base = ChromiumDir
	}
	if uninstallVer != "" {
		target := filepath.Join(base, uninstallVer)
		if err := os.RemoveAll(target); err != nil {
			return ErrRuntime("removing %s: %v", uninstallVer, err)
		}
		current := filepath.Join(base, "current")
		if link, err := os.Readlink(current); err == nil && link == uninstallVer {
			os.Remove(current)
		}
		fmt.Println("removed", uninstallVer)
		return nil
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("nothing to remove")
			return nil
		}
		return ErrRuntime("reading directory: %v", err)
	}
	for _, e := range entries {
		os.RemoveAll(filepath.Join(base, e.Name()))
	}
	fmt.Println("removed all")
	return nil
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	platform := detectPlatform()
	if platform == "" {
		return ErrUser("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	base := upgradePath
	if base == "" {
		base = ChromiumDir
	}
	current := filepath.Join(base, "current")
	currentVer, err := os.Readlink(current)
	if err != nil {
		return ErrUser("no installation found (use 'chromium install' first)")
	}
	info, err := fetchVersionInfo()
	if err != nil {
		return ErrRuntime("fetching version info: %v", err)
	}
	channel, ok := info.Channels[upgradeChannel]
	if !ok {
		return ErrUser("unknown channel: %s", upgradeChannel)
	}
	if channel.Version == currentVer {
		fmt.Println("already up to date")
		return nil
	}
	downloads, ok := channel.Downloads["chrome"]
	if !ok {
		return ErrRuntime("no chrome download for channel %s", upgradeChannel)
	}
	var dlURL string
	for _, dl := range downloads {
		if dl.Platform == platform {
			dlURL = dl.URL
			break
		}
	}
	if dlURL == "" {
		return ErrRuntime("no download for platform %s", platform)
	}
	versionDir := filepath.Join(base, channel.Version)
	if _, err := os.Stat(versionDir); err != nil {
		tmpZip := filepath.Join(base, "chrome.zip")
		defer os.Remove(tmpZip)
		if err := downloadFile(dlURL, tmpZip); err != nil {
			return ErrRuntime("downloading: %v", err)
		}
		if err := extractZip(tmpZip, versionDir); err != nil {
			os.RemoveAll(versionDir)
			return ErrRuntime("extracting: %v", err)
		}
	}
	os.Remove(current)
	if err := os.Symlink(channel.Version, current); err != nil {
		return ErrRuntime("updating symlink: %v", err)
	}
	if upgradeClean && currentVer != channel.Version {
		os.RemoveAll(filepath.Join(base, currentVer))
	}
	fmt.Println(binaryPath(versionDir, platform))
	return nil
}
