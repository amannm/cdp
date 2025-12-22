package cmd

import (
	"cdp/internal"
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
	inst, err := internal.ResolveInstance(listenName)
	if err != nil {
		return err
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
		errCh <- internal.Listen(ctx, inst.WsURL, listenTarget, domain, eventCh)
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
