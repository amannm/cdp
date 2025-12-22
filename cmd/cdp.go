package cmd

import (
	"context"
	"encoding/json"
	"fmt"
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
		} else if msg.Method != "" && c.events != nil {
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
	c.conn.Close()
}
