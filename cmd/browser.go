package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

type Instance struct {
	Name        string    `json:"name"`
	PID         int       `json:"pid"`
	Port        int       `json:"port"`
	WsURL       string    `json:"wsUrl"`
	UserDataDir string    `json:"userDataDir"`
	Started     time.Time `json:"started"`
}

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

func generateName() string {
	b := make([]byte, 4)
	rand.Read(b)
	return "browser-" + hex.EncodeToString(b)
}

func findChromeBinary() (string, error) {
	current := filepath.Join(ChromiumDir, "current")
	target, err := os.Readlink(current)
	if err != nil {
		return "", ErrUser("no chromium installed (run 'chromium install' first)")
	}
	versionDir := filepath.Join(ChromiumDir, target)
	platform := detectPlatform()
	return binaryPath(versionDir, platform), nil
}

func waitForPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		if Verbose {
			fmt.Fprintf(os.Stderr, "waiting for port %d: %v\n", port, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ErrRuntime("timeout waiting for port %d", port)
}

func getWsURL(port int) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	if Verbose {
		fmt.Fprintf(os.Stderr, "fetching ws url from %s\n", url)
	}
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var info struct {
		WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	return info.WebSocketDebuggerUrl, nil
}

func saveInstance(inst *Instance) error {
	if err := os.MkdirAll(InstancesDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(InstancesDir, inst.Name+".json")
	data, err := json.Marshal(inst)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func loadInstance(name string) (*Instance, error) {
	path := filepath.Join(InstancesDir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var inst Instance
	if err := json.Unmarshal(data, &inst); err != nil {
		return nil, err
	}
	return &inst, nil
}

func removeInstance(name string) error {
	return os.Remove(filepath.Join(InstancesDir, name+".json"))
}

func runStart(cmd *cobra.Command, args []string) error {
	binary, err := findChromeBinary()
	if err != nil {
		return err
	}
	name := startName
	if name == "" {
		name = generateName()
	}
	if _, err := loadInstance(name); err == nil {
		return ErrUser("instance %s already exists", name)
	}
	userDataDir := startUserDataDir
	tempDir := false
	if userDataDir == "" {
		userDataDir, err = os.MkdirTemp("", "cdp-"+name+"-")
		if err != nil {
			return ErrRuntime("creating temp dir: %v", err)
		}
		tempDir = true
	}
	cleanup := func() {
		if tempDir {
			os.RemoveAll(userDataDir)
		}
	}
	port := startPort
	if port == 0 {
		port = 9222
		for i := 0; i < 100; i++ {
			if _, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port)); err != nil {
				break
			}
			port++
		}
	}
	chromeArgs := []string{
		"--remote-debugging-port=" + strconv.Itoa(port),
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--remote-allow-origins=*",
	}
	if startHeadless {
		chromeArgs = append(chromeArgs, "--headless=new")
	}
	if Verbose {
		fmt.Fprintf(os.Stderr, "starting chrome: %s %v\n", binary, chromeArgs)
	}
	proc := exec.Command(binary, chromeArgs...)
	proc.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := proc.Start(); err != nil {
		cleanup()
		return ErrRuntime("starting chrome: %v", err)
	}
	if err := waitForPort(port, 30*time.Second); err != nil {
		proc.Process.Kill()
		cleanup()
		return err
	}
	wsURL, err := getWsURL(port)
	if err != nil {
		proc.Process.Kill()
		cleanup()
		return ErrRuntime("getting ws url: %v", err)
	}
	inst := &Instance{
		Name:        name,
		PID:         proc.Process.Pid,
		Port:        port,
		WsURL:       wsURL,
		UserDataDir: userDataDir,
		Started:     time.Now(),
	}
	if err := saveInstance(inst); err != nil {
		proc.Process.Kill()
		cleanup()
		return ErrRuntime("saving instance: %v", err)
	}
	out, _ := json.Marshal(inst)
	fmt.Println(string(out))
	return nil
}

func runStop(cmd *cobra.Command, args []string) error {
	if !stopAll && stopName == "" {
		return ErrUser("--name or --all required")
	}
	if stopAll {
		entries, err := os.ReadDir(InstancesDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return ErrRuntime("reading instances dir: %v", err)
		}
		var failed []string
		for _, e := range entries {
			name := e.Name()
			if filepath.Ext(name) != ".json" {
				continue
			}
			name = name[:len(name)-5]
			if err := stopInstance(name); err != nil {
				failed = append(failed, name)
			}
		}
		if len(failed) > 0 {
			return ErrRuntime("failed to stop %d instance(s): %v", len(failed), failed)
		}
		return nil
	}
	return stopInstance(stopName)
}

func stopInstance(name string) error {
	inst, err := loadInstance(name)
	if err != nil {
		return ErrUser("instance %s not found", name)
	}

	// Try CDP Browser.close first
	if inst.WsURL != "" {
		if Verbose {
			fmt.Fprintf(os.Stderr, "attempting graceful shutdown via CDP for %s\n", name)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// We don't need events for closing
		conn, err := dialCDP(inst.WsURL, false)
		if err == nil {
			defer conn.close()
			_, err = conn.send(ctx, "Browser.close", nil, "")
			if err == nil {
				// Wait for process to exit
				if proc, err := os.FindProcess(inst.PID); err == nil {
					done := make(chan struct{})
					go func() {
						proc.Wait()
						close(done)
					}()
					select {
					case <-done:
						if Verbose {
							fmt.Fprintf(os.Stderr, "graceful shutdown successful for %s\n", name)
						}
						// Process exited, proceed to cleanup
						goto Cleanup
					case <-time.After(3 * time.Second):
						if Verbose {
							fmt.Fprintf(os.Stderr, "graceful shutdown timed out for %s\n", name)
						}
					}
				}
			} else if Verbose {
				fmt.Fprintf(os.Stderr, "CDP Browser.close failed: %v\n", err)
			}
		} else if Verbose {
			fmt.Fprintf(os.Stderr, "CDP connection failed: %v\n", err)
		}
	}

	// Fallback to SIGTERM/SIGKILL
	if proc, err := os.FindProcess(inst.PID); err == nil {
		if Verbose {
			fmt.Fprintf(os.Stderr, "sending SIGTERM to %d\n", inst.PID)
		}
		proc.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			proc.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			if Verbose {
				fmt.Fprintf(os.Stderr, "sending SIGKILL to %d\n", inst.PID)
			}
			proc.Kill()
		}
	}

Cleanup:
	if inst.UserDataDir != "" && filepath.HasPrefix(inst.UserDataDir, os.TempDir()) {
		if Verbose {
			fmt.Fprintf(os.Stderr, "cleaning up user data dir %s\n", inst.UserDataDir)
		}
		os.RemoveAll(inst.UserDataDir)
	}
	removeInstance(name)
	fmt.Printf("stopped %s\n", name)
	return nil
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func runList(cmd *cobra.Command, args []string) error {
	entries, err := os.ReadDir(InstancesDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("[]")
			return nil
		}
		return ErrRuntime("reading instances dir: %v", err)
	}
	var instances []*Instance
	var cleanupErrs int
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		name = name[:len(name)-5]
		inst, err := loadInstance(name)
		if err != nil {
			continue
		}
		if !isProcessAlive(inst.PID) {
			if err := removeInstance(name); err != nil {
				cleanupErrs++
			}
			continue
		}
		instances = append(instances, inst)
	}
	if instances == nil {
		instances = []*Instance{}
	}
	out, _ := json.Marshal(instances)
	fmt.Println(string(out))
	if cleanupErrs > 0 {
		fmt.Fprintf(os.Stderr, "warning: failed to cleanup %d stale instance(s)\n", cleanupErrs)
	}
	return nil
}
