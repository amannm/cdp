package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

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
	events  chan *cdpMessage
	mu      sync.Mutex
	closed  bool
}

func dialCDP(wsURL string, withEvents bool) (*cdpConn, error) {
	if Verbose {
		_, _ = fmt.Fprintf(os.Stderr, "connecting to CDP: %s\n", wsURL)
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	c := &cdpConn{
		conn:    conn,
		nextID:  1,
		pending: make(map[int64]chan *cdpMessage),
	}
	if withEvents {
		c.events = make(chan *cdpMessage, 100)
	}
	go c.readLoop()
	return c, nil
}

func (c *cdpConn) readLoop() {
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if Verbose && !c.closed {
				_, _ = fmt.Fprintf(os.Stderr, "ws read error: %v\n", err)
			}
			c.mu.Lock()
			c.closed = true
			for _, ch := range c.pending {
				close(ch)
			}
			c.pending = nil
			if c.events != nil {
				close(c.events)
			}
			c.mu.Unlock()
			return
		}
		if Verbose {
			_, _ = fmt.Fprintf(os.Stderr, "<- %s\n", string(data))
		}
		var msg cdpMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			if Verbose {
				_, _ = fmt.Fprintf(os.Stderr, "json decode error: %v\n", err)
			}
			continue
		}
		if msg.ID != 0 {
			c.mu.Lock()
			if ch, ok := c.pending[msg.ID]; ok {
				ch <- &msg
				delete(c.pending, msg.ID)
			}
			c.mu.Unlock()
		} else if msg.Method != "" && c.events != nil {
			c.mu.Lock()
			if !c.closed {
				select {
				case c.events <- &msg:
				default:
					if Verbose {
						_, _ = fmt.Fprintf(os.Stderr, "event buffer full, dropping: %s\n", msg.Method)
					}
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *cdpConn) send(ctx context.Context, method string, params json.RawMessage, sessionID string) (*cdpMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	msg := cdpMessage{ID: id, Method: method, Params: params}
	if sessionID != "" {
		msg.SessionID = sessionID
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if Verbose {
		_, _ = fmt.Fprintf(os.Stderr, "-> %s\n", string(data))
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
	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("connection closed")
		}
		return resp, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *cdpConn) close() {
	_ = c.conn.Close()
}

func (c *cdpConn) attachToTarget(ctx context.Context, targetID string) (string, error) {
	attachParams, _ := json.Marshal(map[string]any{"targetId": targetID, "flatten": true})
	attachResp, err := c.send(ctx, "Target.attachToTarget", attachParams, "")
	if err != nil {
		return "", err
	}
	if attachResp.Error != nil {
		return "", fmt.Errorf("attach error: %s", attachResp.Error.Message)
	}
	var result struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attachResp.Result, &result); err != nil {
		return "", err
	}
	return result.SessionID, nil
}
