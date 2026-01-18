package cmd

import (
	"cdp/internal"
	"cdp/internal/utility"
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
	listenWsURL  string
)

func init() {
	listenCmd.Flags().StringVarP(&listenName, "name", "n", "", "Browser instance name (default: first available)")
	listenCmd.Flags().StringVarP(&listenWsURL, "ws-url", "w", "", "Remote debugger URL (ws://..., http(s)://..., or host:port)")
	listenCmd.Flags().StringVarP(&listenTarget, "target", "t", "", "Target ID")
	listenCmd.Flags().StringVarP(&listenFilter, "filter", "f", "", "Event name filter (e.g., Page.loadEventFired)")
	listenCmd.Flags().IntVarP(&listenCount, "count", "c", 0, "Exit after N events (0 = unlimited)")
	rootCmd.AddCommand(listenCmd)
}

func runListen(_ *cobra.Command, args []string) error {
	domain := args[0]
	if listenWsURL != "" && listenName != "" {
		return utility.ErrUser("--ws-url and --name are mutually exclusive")
	}
	wsURL := listenWsURL
	if wsURL != "" {
		var err error
		wsURL, err = internal.ResolveWsURL(wsURL)
		if err != nil {
			return err
		}
	} else {
		inst, err := internal.ResolveInstance(listenName)
		if err != nil {
			return err
		}
		wsURL = inst.WsURL
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()
	eventCh := make(chan *internal.CDPMessage, 100)
	errCh := make(chan error, 1)
	go func() {
		errCh <- internal.Listen(ctx, wsURL, listenTarget, domain, eventCh)
	}()
	count := 0
	for {
		select {
		case err := <-errCh:
			return err
		case event := <-eventCh:
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
			data, err := json.Marshal(out)
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			count++
			if listenCount > 0 && count >= listenCount {
				return nil
			}
		case <-ctx.Done():
			return nil
		}
	}
}
