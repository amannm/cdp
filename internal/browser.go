package internal

import (
	"cdp/internal/install"
	"cdp/internal/utility"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Instance struct {
	Name        string    `json:"name"`
	PID         int       `json:"pid"`
	Port        int       `json:"port"`
	WsURL       string    `json:"wsUrl"`
	UserDataDir string    `json:"userDataDir"`
	Started     time.Time `json:"started"`
}

func GenerateName() string {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return "browser-" + hex.EncodeToString(b)
}

func FindChromeBinary() (string, error) {
	current := filepath.Join(utility.ChromeDir, "current")
	target, err := os.Readlink(current)
	if err != nil {
		return "", utility.ErrUser("no chrome installed (run 'chrome install' first)")
	}
	versionDir := filepath.Join(utility.ChromeDir, target)
	platform := install.DetectPlatform()
	return install.BinaryPath(versionDir, platform), nil
}

func WaitForPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}
		utility.Term.Info("waiting for port %d: %v\n", port, err)
		time.Sleep(100 * time.Millisecond)
	}
	return utility.ErrRuntime("timeout waiting for port %d", port)
}

func GetWsURL(port int) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	utility.Term.Info("fetching ws url from %s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	var info struct {
		WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
	}
	err = json.NewDecoder(resp.Body).Decode(&info)
	if err != nil {
		return "", err
	}
	return info.WebSocketDebuggerUrl, nil
}

func SaveInstance(inst *Instance) error {
	err := os.MkdirAll(utility.InstancesDir, 0755)
	if err != nil {
		return err
	}
	path := filepath.Join(utility.InstancesDir, inst.Name+".json")
	data, err := json.Marshal(inst)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func LoadInstance(name string) (*Instance, error) {
	path := filepath.Join(utility.InstancesDir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var inst Instance
	err = json.Unmarshal(data, &inst)
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

func RemoveInstance(name string) error {
	return os.Remove(filepath.Join(utility.InstancesDir, name+".json"))
}

func StopInstance(name string) error {
	inst, err := LoadInstance(name)
	if err != nil {
		return utility.ErrUser("instance %s not found", name)
	}
	stopped := false
	if inst.WsURL != "" {
		utility.Term.Info("attempting graceful shutdown via CDP for %s\n", name)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		conn, err := NewClient(inst.WsURL, false)
		if err == nil {
			defer conn.Close()
			_, err = conn.Send(ctx, "Browser.close", nil, "")
			if err == nil {
				proc, err := os.FindProcess(inst.PID)
				if err == nil {
					done := make(chan struct{})
					go func() {
						_, _ = proc.Wait()
						close(done)
					}()
					select {
					case <-done:
						utility.Term.Info("graceful shutdown successful for %s\n", name)
						stopped = true
					case <-time.After(3 * time.Second):
						utility.Term.Info("graceful shutdown timed out for %s\n", name)
					}
				}
			} else {
				utility.Term.Info("CDP Browser.close failed: %v\n", err)
			}
		} else {
			utility.Term.Info("CDP connection failed: %v\n", err)
		}
	}
	if !stopped && IsProcessAlive(inst.PID) {
		proc, err := os.FindProcess(inst.PID)
		if err == nil {
			utility.Term.Info("sending SIGTERM to %d\n", inst.PID)
			_ = proc.Signal(syscall.SIGTERM)
			done := make(chan struct{})
			go func() {
				_, _ = proc.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				utility.Term.Info("sending SIGKILL to %d\n", inst.PID)
				_ = proc.Kill()
			}
		}
	}
	if inst.UserDataDir != "" && strings.HasPrefix(inst.UserDataDir, os.TempDir()) {
		utility.Term.Info("cleaning up user data dir %s\n", inst.UserDataDir)
		_ = os.RemoveAll(inst.UserDataDir)
	}
	_ = RemoveInstance(name)
	utility.Term.Text("stopped %s\n", name)
	return nil
}

func IsProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func FindFirstInstance() (*Instance, error) {
	entries, err := os.ReadDir(utility.InstancesDir)
	if err != nil {
		return nil, utility.ErrUser("no instances found")
	}
	for _, e := range entries {
		name := e.Name()
		if len(name) > 5 && name[len(name)-5:] == ".json" {
			name = name[:len(name)-5]
			inst, err := LoadInstance(name)
			if err != nil {
				continue
			}
			if IsProcessAlive(inst.PID) {
				return inst, nil
			}
			_ = RemoveInstance(name)
		}
	}
	return nil, utility.ErrUser("no running instances")
}

func ResolveInstance(name string) (*Instance, error) {
	if name != "" {
		inst, err := LoadInstance(name)
		if err != nil {
			return nil, utility.ErrUser("instance %s not found", name)
		}
		if !IsProcessAlive(inst.PID) {
			_ = RemoveInstance(name)
			return nil, utility.ErrUser("instance %s not running", name)
		}
		return inst, nil
	}
	return FindFirstInstance()
}

type StartOptions struct {
	Name        string
	Port        int
	Headless    bool
	UserDataDir string
}

func StartBrowser(opts StartOptions) (*Instance, error) {
	binary, err := FindChromeBinary()
	if err != nil {
		return nil, err
	}
	name := opts.Name
	if name == "" {
		name = GenerateName()
	}
	_, err = LoadInstance(name)
	if err == nil {
		return nil, utility.ErrUser("instance %s already exists", name)
	}
	userDataDir := opts.UserDataDir
	tempDir := false
	if userDataDir == "" {
		userDataDir, err = os.MkdirTemp("", "cdp-"+name+"-")
		if err != nil {
			return nil, utility.ErrRuntime("creating temp dir: %v", err)
		}
		tempDir = true
	}
	cleanup := func() {
		if tempDir {
			_ = os.RemoveAll(userDataDir)
		}
	}
	port := opts.Port
	if port == 0 {
		port = 9222
		for i := 0; i < 100; i++ {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
			if err != nil {
				break
			}
			_ = resp.Body.Close()
			port++
		}
	}
	chromeArgs := []string{
		"--remote-debugging-port=" + fmt.Sprintf("%d", port),
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--remote-allow-origins=*",
		"--disable-infobars",
	}
	if opts.Headless {
		chromeArgs = append(chromeArgs, "--headless=new")
	}
	utility.Term.Info("starting chrome: %s %v\n", binary, chromeArgs)
	proc := exec.Command(binary, chromeArgs...)
	proc.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	err = proc.Start()
	if err != nil {
		cleanup()
		return nil, utility.ErrRuntime("starting chrome: %v", err)
	}
	err = WaitForPort(port, 30*time.Second)
	if err != nil {
		_ = proc.Process.Kill()
		cleanup()
		return nil, err
	}
	wsURL, err := GetWsURL(port)
	if err != nil {
		_ = proc.Process.Kill()
		cleanup()
		return nil, utility.ErrRuntime("getting ws url: %v", err)
	}
	inst := &Instance{
		Name:        name,
		PID:         proc.Process.Pid,
		Port:        port,
		WsURL:       wsURL,
		UserDataDir: userDataDir,
		Started:     time.Now(),
	}
	err = SaveInstance(inst)
	if err != nil {
		_ = proc.Process.Kill()
		cleanup()
		return nil, utility.ErrRuntime("saving instance: %v", err)
	}
	return inst, nil
}

func StopAllInstances() error {
	entries, err := os.ReadDir(utility.InstancesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return utility.ErrRuntime("reading instances dir: %v", err)
	}
	var failed []string
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		name = name[:len(name)-5]
		err := StopInstance(name)
		if err != nil {
			failed = append(failed, name)
		}
	}
	if len(failed) > 0 {
		return utility.ErrRuntime("failed to stop %d instance(s): %v", len(failed), failed)
	}
	return nil
}

func ListInstances() ([]*Instance, int, error) {
	entries, err := os.ReadDir(utility.InstancesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Instance{}, 0, nil
		}
		return nil, 0, utility.ErrRuntime("reading instances dir: %v", err)
	}
	var instances []*Instance
	var cleanupErrs int
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		name = name[:len(name)-5]
		inst, err := LoadInstance(name)
		if err != nil {
			continue
		}
		if !IsProcessAlive(inst.PID) {
			err := RemoveInstance(name)
			if err != nil {
				cleanupErrs++
			}
			continue
		}
		instances = append(instances, inst)
	}
	if instances == nil {
		instances = []*Instance{}
	}
	return instances, cleanupErrs, nil
}
