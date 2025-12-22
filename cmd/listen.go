package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var listenCmd = &cobra.Command{
	Use:   "listen <domain>",
	Short: "Subscribe to CDP events and stream them",
	Args:  cobra.ExactArgs(1),
	RunE:  runListen,
}

var (
	listenName   string
	listenTarget string
	listenFilter string
	listenCount  int
)

func init() {
	listenCmd.Flags().StringVar(&listenName, "name", "", "Browser instance name (default: first available)")
	listenCmd.Flags().StringVar(&listenTarget, "target", "", "Target ID")
	listenCmd.Flags().StringVar(&listenFilter, "filter", "", "Event name filter (e.g., Page.loadEventFired)")
	listenCmd.Flags().IntVar(&listenCount, "count", 0, "Exit after N events (0 = unlimited)")
	rootCmd.AddCommand(listenCmd)
}

func runListen(_ *cobra.Command, args []string) error {
	domain := args[0]
	inst, err := resolveInstance(listenName)
	if err != nil {
		return err
	}
	conn, err := dialCDP(inst.WsURL, true)
	if err != nil {
		return ErrRuntime("connecting: %v", err)
	}
	defer conn.close()
	ctx := context.Background()
	var sessionID string
	if listenTarget != "" {
		sessionID, err = conn.attachToTarget(ctx, listenTarget)
		if err != nil {
			return ErrRuntime("attaching to target: %v", err)
		}
	}
	enableMethod := domain + ".enable"
	enableResp, err := conn.send(ctx, enableMethod, nil, sessionID)
	if err != nil {
		return ErrRuntime("enabling %s: %v", domain, err)
	}
	if enableResp.Error != nil {
		return ErrUser("enable error: %s", enableResp.Error.Message)
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	count := 0
	for {
		select {
		case <-sigCh:
			return nil
		case event, ok := <-conn.events:
			if !ok {
				return nil
			}
			if listenFilter != "" && !strings.HasPrefix(event.Method, listenFilter) {
				continue
			}
			out := map[string]any{"method": event.Method}
			if event.Params != nil {
				var params any
				err := json.Unmarshal(event.Params, &params)
				if err != nil {
					return err
				}
				out["params"] = params
			}
			data, _ := json.Marshal(out)
			fmt.Println(string(data))
			count++
			if listenCount > 0 && count >= listenCount {
				return nil
			}
		}
	}
}
