package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <method>",
	Short: "Send a CDP command and return the response",
	Args:  cobra.ExactArgs(1),
	RunE:  runSend,
}

var (
	sendName    string
	sendTarget  string
	sendParams  string
	sendTimeout time.Duration
)

func init() {
	sendCmd.Flags().StringVar(&sendName, "name", "", "Browser instance name (default: first available)")
	sendCmd.Flags().StringVar(&sendTarget, "target", "", "Target ID or 'browser' for browser-level commands")
	sendCmd.Flags().StringVar(&sendParams, "params", "", "JSON params (or pipe via stdin)")
	sendCmd.Flags().DurationVar(&sendTimeout, "timeout", 30*time.Second, "Response timeout")
	rootCmd.AddCommand(sendCmd)
}

func findFirstInstance() (*Instance, error) {
	entries, err := os.ReadDir(InstancesDir)
	if err != nil {
		return nil, ErrUser("no instances found")
	}
	for _, e := range entries {
		name := e.Name()
		if len(name) > 5 && name[len(name)-5:] == ".json" {
			name = name[:len(name)-5]
			inst, err := loadInstance(name)
			if err != nil {
				continue
			}
			if isProcessAlive(inst.PID) {
				return inst, nil
			}
			removeInstance(name)
		}
	}
	return nil, ErrUser("no running instances")
}

func readParamsFromStdin() (string, error) {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", nil
	}
	scanner := bufio.NewScanner(os.Stdin)
	var result string
	for scanner.Scan() {
		result += scanner.Text()
	}
	return result, scanner.Err()
}

func runSend(cmd *cobra.Command, args []string) error {
	method := args[0]
	var inst *Instance
	var err error
	if sendName != "" {
		inst, err = loadInstance(sendName)
		if err != nil {
			return ErrUser("instance %s not found", sendName)
		}
		if !isProcessAlive(inst.PID) {
			removeInstance(sendName)
			return ErrUser("instance %s not running", sendName)
		}
	} else {
		inst, err = findFirstInstance()
		if err != nil {
			return err
		}
	}
	params := sendParams
	if params == "" {
		params, _ = readParamsFromStdin()
	}
	var paramsJSON json.RawMessage
	if params != "" {
		if !json.Valid([]byte(params)) {
			return ErrUser("invalid JSON params")
		}
		paramsJSON = json.RawMessage(params)
	}
	conn, err := dialCDP(inst.WsURL, false)
	if err != nil {
		return ErrRuntime("connecting: %v", err)
	}
	defer conn.close()
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	var sessionID string
	if sendTarget != "" && sendTarget != "browser" {
		attachParams, _ := json.Marshal(map[string]interface{}{"targetId": sendTarget, "flatten": true})
		attachResp, err := conn.send(ctx, "Target.attachToTarget", attachParams, "")
		if err != nil {
			return ErrRuntime("attaching to target: %v", err)
		}
		if attachResp.Error != nil {
			return ErrUser("attach error: %s", attachResp.Error.Message)
		}
		var result struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(attachResp.Result, &result); err != nil {
			return ErrRuntime("parsing attach response: %v", err)
		}
		sessionID = result.SessionID
	}
	resp, err := conn.send(ctx, method, paramsJSON, sessionID)
	if err != nil {
		return ErrRuntime("sending command: %v", err)
	}
	if resp.Error != nil {
		errJSON, _ := json.Marshal(map[string]interface{}{"error": resp.Error})
		fmt.Println(string(errJSON))
		return nil
	}
	if resp.Result != nil {
		fmt.Println(string(resp.Result))
	} else {
		fmt.Println("{}")
	}
	return nil
}
