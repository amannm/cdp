package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <method>",
	Short: "Send a CDP command and return the response",
	Args:  cobra.ExactArgs(1),
	RunE:  runSend,
}

var (
	sendName   string
	sendTarget string
	sendParams string
)

func init() {
	sendCmd.Flags().StringVar(&sendName, "name", "", "Browser instance name (default: first available)")
	sendCmd.Flags().StringVar(&sendTarget, "target", "", "Target ID or 'browser' for browser-level commands")
	sendCmd.Flags().StringVar(&sendParams, "params", "", "JSON params (or pipe via stdin)")
	rootCmd.AddCommand(sendCmd)
}

type cdpMessage struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpError       `json:"error,omitempty"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cdpConn struct {
	conn    *websocket.Conn
	nextID  int64
	pending map[int64]chan *cdpMessage
	mu      sync.Mutex
}

func newCDPConn(wsURL string) (*cdpConn, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	c := &cdpConn{
		conn:    conn,
		nextID:  1,
		pending: make(map[int64]chan *cdpMessage),
	}
	go c.readLoop()
	return c, nil
}

func (c *cdpConn) readLoop() {
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			for _, ch := range c.pending {
				close(ch)
			}
			c.pending = nil
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
		}
	}
}

func (c *cdpConn) send(method string, params json.RawMessage, sessionID string) (*cdpMessage, error) {
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

func (c *cdpConn) close() {
	c.conn.Close()
}

func findFirstInstance() (*Instance, error) {
	entries, err := os.ReadDir(InstancesDir)
	if err != nil {
		return nil, fmt.Errorf("no instances found")
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
	return nil, fmt.Errorf("no running instances")
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
			return fmt.Errorf("instance %s not found", sendName)
		}
		if !isProcessAlive(inst.PID) {
			removeInstance(sendName)
			return fmt.Errorf("instance %s not running", sendName)
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
			return fmt.Errorf("invalid JSON params")
		}
		paramsJSON = json.RawMessage(params)
	}
	conn, err := newCDPConn(inst.WsURL)
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	defer conn.close()
	var sessionID string
	if sendTarget != "" && sendTarget != "browser" {
		attachResp, err := conn.send("Target.attachToTarget", json.RawMessage(fmt.Sprintf(`{"targetId":"%s","flatten":true}`, sendTarget)), "")
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
	resp, err := conn.send(method, paramsJSON, sessionID)
	if err != nil {
		return err
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
