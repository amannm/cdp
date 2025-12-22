package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/gorilla/websocket"
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

type eventConn struct {
	conn    *websocket.Conn
	nextID  int64
	pending map[int64]chan *cdpMessage
	events  chan *cdpMessage
	mu      sync.Mutex
	closed  bool
}

func newEventConn(wsURL string) (*eventConn, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	c := &eventConn{
		conn:    conn,
		nextID:  1,
		pending: make(map[int64]chan *cdpMessage),
		events:  make(chan *cdpMessage, 100),
	}
	go c.readLoop()
	return c, nil
}

func (c *eventConn) readLoop() {
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			c.closed = true
			for _, ch := range c.pending {
				close(ch)
			}
			c.pending = nil
			close(c.events)
			c.mu.Unlock()
			return
		}
		var msg cdpMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.ID != 0 {
			c.mu.Lock()
			if ch, ok := c.pending[msg.ID]; ok {
				ch <- &msg
				delete(c.pending, msg.ID)
			}
			c.mu.Unlock()
		} else if msg.Method != "" {
			c.mu.Lock()
			if !c.closed {
				select {
				case c.events <- &msg:
				default:
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *eventConn) send(method string, params json.RawMessage, sessionID string) (*cdpMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	msg := cdpMessage{ID: id, Method: method, Params: params}
	if sessionID != "" {
		msg.SessionID = sessionID
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	ch := make(chan *cdpMessage, 1)
	c.mu.Lock()
	if c.pending == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("connection closed")
	}
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	resp, ok := <-ch
	if !ok {
		return nil, fmt.Errorf("connection closed")
	}
	return resp, nil
}

func (c *eventConn) close() {
	c.conn.Close()
}

func runListen(cmd *cobra.Command, args []string) error {
	domain := args[0]
	var inst *Instance
	var err error
	if listenName != "" {
		inst, err = loadInstance(listenName)
		if err != nil {
			return fmt.Errorf("instance %s not found", listenName)
		}
		if !isProcessAlive(inst.PID) {
			removeInstance(listenName)
			return fmt.Errorf("instance %s not running", listenName)
		}
	} else {
		inst, err = findFirstInstance()
		if err != nil {
			return err
		}
	}
	conn, err := newEventConn(inst.WsURL)
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	defer conn.close()
	var sessionID string
	if listenTarget != "" {
		attachResp, err := conn.send("Target.attachToTarget", json.RawMessage(fmt.Sprintf(`{"targetId":"%s","flatten":true}`, listenTarget)), "")
		if err != nil {
			return fmt.Errorf("attaching to target: %w", err)
		}
		if attachResp.Error != nil {
			return fmt.Errorf("attach error: %s", attachResp.Error.Message)
		}
		var result struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(attachResp.Result, &result); err != nil {
			return fmt.Errorf("parsing attach response: %w", err)
		}
		sessionID = result.SessionID
	}
	enableMethod := domain + ".enable"
	enableResp, err := conn.send(enableMethod, nil, sessionID)
	if err != nil {
		return fmt.Errorf("enabling %s: %w", domain, err)
	}
	if enableResp.Error != nil {
		return fmt.Errorf("enable error: %s", enableResp.Error.Message)
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
			out := map[string]interface{}{"method": event.Method}
			if event.Params != nil {
				var params interface{}
				json.Unmarshal(event.Params, &params)
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
