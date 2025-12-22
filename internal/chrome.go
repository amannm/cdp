package internal

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
)

const VersionURL = "https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json"

type VersionInfo struct {
	Channels map[string]ChannelInfo `json:"channels"`
}

type ChannelInfo struct {
	Version   string              `json:"version"`
	Downloads map[string][]DLInfo `json:"downloads"`
}

type DLInfo struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

func DetectPlatform() string {
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

func FetchVersionInfo() (*VersionInfo, error) {
	if Verbose {
		_, _ = fmt.Fprintf(os.Stderr, "fetching version info from %s\n", VersionURL)
	}
	resp, err := http.Get(VersionURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var info VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

func GetDownloadURL(channelName, platform string) (string, string, error) {
	info, err := FetchVersionInfo()
	if err != nil {
		return "", "", err
	}
	channel, ok := info.Channels[channelName]
	if !ok {
		return "", "", fmt.Errorf("unknown channel: %s", channelName)
	}
	downloads, ok := channel.Downloads["chrome"]
	if !ok {
		return "", "", fmt.Errorf("no chrome download for channel %s", channelName)
	}
	for _, dl := range downloads {
		if dl.Platform == platform {
			return dl.URL, channel.Version, nil
		}
	}
	return "", "", fmt.Errorf("no download for platform %s", platform)
}

func DownloadFile(url, dest string) error {
	if Verbose {
		_, _ = fmt.Fprintf(os.Stderr, "downloading %s to %s\n", url, dest)
	}
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	expectedLen := resp.ContentLength
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	if expectedLen > 0 && written != expectedLen {
		return fmt.Errorf("download incomplete: got %d bytes, expected %d", written, expectedLen)
	}
	return nil
}

func ExtractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid path: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(path, f.Mode())
			continue
		}
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			_ = out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func InstallChrome(base, version, dlURL string) (string, error) {
	versionDir := filepath.Join(base, version)
	if _, err := os.Stat(versionDir); err == nil {
		return versionDir, nil
	}
	_ = os.MkdirAll(base, 0755)
	tmpZip := filepath.Join(base, "chrome.zip")
	defer func() { _ = os.Remove(tmpZip) }()
	if err := DownloadFile(dlURL, tmpZip); err != nil {
		return "", err
	}
	if err := ExtractZip(tmpZip, versionDir); err != nil {
		_ = os.RemoveAll(versionDir)
		return "", err
	}
	return versionDir, nil
}

func BinaryPath(versionDir, platform string) string {
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

func Install(channel, base string) (string, error) {
	platform := DetectPlatform()
	if platform == "" {
		return "", ErrUser("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	dlURL, version, err := GetDownloadURL(channel, platform)
	if err != nil {
		return "", ErrRuntime("getting download url: %v", err)
	}
	if base == "" {
		base = ChromeDir
	}
	versionDir, err := InstallChrome(base, version, dlURL)
	if err != nil {
		return "", ErrRuntime("installing: %v", err)
	}
	current := filepath.Join(base, "current")
	_ = os.Remove(current)
	if err := os.Symlink(version, current); err != nil {
		return "", ErrRuntime("creating symlink: %v", err)
	}
	return BinaryPath(versionDir, platform), nil
}

func Uninstall(version, base string) error {
	if base == "" {
		base = ChromeDir
	}
	if version != "" {
		target := filepath.Join(base, version)
		if err := os.RemoveAll(target); err != nil {
			return ErrRuntime("removing %s: %v", version, err)
		}
		current := filepath.Join(base, "current")
		if link, err := os.Readlink(current); err == nil && link == version {
			_ = os.Remove(current)
		}
		return nil
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return ErrRuntime("reading directory: %v", err)
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(base, e.Name()))
	}
	return nil
}

func Upgrade(channel, base string, clean bool) (string, error) {
	platform := DetectPlatform()
	if platform == "" {
		return "", ErrUser("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if base == "" {
		base = ChromeDir
	}
	current := filepath.Join(base, "current")
	currentVer, err := os.Readlink(current)
	if err != nil {
		return "", ErrUser("no installation found (use 'chrome install' first)")
	}
	dlURL, version, err := GetDownloadURL(channel, platform)
	if err != nil {
		return "", ErrRuntime("getting download url: %v", err)
	}
	if version == currentVer {
		return "", nil // Already up to date
	}
	versionDir, err := InstallChrome(base, version, dlURL)
	if err != nil {
		return "", ErrRuntime("installing: %v", err)
	}
	_ = os.Remove(current)
	if err := os.Symlink(version, current); err != nil {
		return "", ErrRuntime("updating symlink: %v", err)
	}
	if clean && currentVer != version {
		_ = os.RemoveAll(filepath.Join(base, currentVer))
	}
	return BinaryPath(versionDir, platform), nil
}
