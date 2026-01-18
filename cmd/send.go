package cmd

import (
	"bufio"
	"cdp/internal"
	"cdp/internal/utility"
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
	sendWsURL   string
)

func init() {
	sendCmd.Flags().StringVarP(&sendName, "name", "n", "", "Browser instance name (default: first available)")
	sendCmd.Flags().StringVarP(&sendWsURL, "ws-url", "w", "", "Remote debugger URL (ws://..., http(s)://..., or host:port)")
	sendCmd.Flags().StringVarP(&sendTarget, "target", "t", "", "Target ID or 'browser' for browser-level commands")
	sendCmd.Flags().StringVarP(&sendParams, "params", "p", "", "JSON params (or pipe via stdin)")
	sendCmd.Flags().DurationVar(&sendTimeout, "timeout", 30*time.Second, "Response timeout")
	rootCmd.AddCommand(sendCmd)
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

func runSend(_ *cobra.Command, args []string) error {
	method := args[0]
	if sendWsURL != "" && sendName != "" {
		return utility.ErrUser("--ws-url and --name are mutually exclusive")
	}
	wsURL := sendWsURL
	if wsURL != "" {
		var err error
		wsURL, err = internal.ResolveWsURL(wsURL)
		if err != nil {
			return err
		}
	} else {
		inst, err := internal.ResolveInstance(sendName)
		if err != nil {
			return err
		}
		wsURL = inst.WsURL
	}
	params := sendParams
	if params == "" {
		var err error
		params, err = readParamsFromStdin()
		if err != nil {
			return utility.ErrUser("reading stdin: %v", err)
		}
	}
	var paramsJSON json.RawMessage
	if params != "" {
		if !json.Valid([]byte(params)) {
			return utility.ErrUser("invalid JSON params")
		}
		paramsJSON = json.RawMessage(params)
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	resp, err := internal.Send(ctx, wsURL, sendTarget, method, paramsJSON)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		errJSON, _ := json.Marshal(map[string]any{"error": resp.Error})
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
