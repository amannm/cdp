package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

type CDPMessage struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *CDPError       `json:"error,omitempty"`
}

type CDPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type CDPConn struct {
	Conn    *websocket.Conn
	NextID  int64
	Pending map[int64]chan *CDPMessage
	Events  chan *CDPMessage
	Mu      sync.Mutex
	Closed  bool
}

func DialCDP(wsURL string, withEvents bool) (*CDPConn, error) {
	Term.Info("connecting to CDP: %s\n", wsURL)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	c := &CDPConn{
		Conn:    conn,
		NextID:  1,
		Pending: make(map[int64]chan *CDPMessage),
	}
	if withEvents {
		c.Events = make(chan *CDPMessage, 100)
	}
	go c.readLoop()
	return c, nil
}

func (c *CDPConn) readLoop() {
	for {
		_, data, err := c.Conn.ReadMessage()
		if err != nil {
			if !c.Closed {
				Term.Info("ws read error: %v\n", err)
			}
			c.Mu.Lock()
			c.Closed = true
			for _, ch := range c.Pending {
				close(ch)
			}
			c.Pending = nil
			if c.Events != nil {
				close(c.Events)
			}
			c.Mu.Unlock()
			return
		}
		Term.Info("<- %s\n", string(data))
		var msg CDPMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			Term.Info("json decode error: %v\n", err)
			continue
		}
		if msg.ID != 0 {
			c.Mu.Lock()
			if ch, ok := c.Pending[msg.ID]; ok {
				ch <- &msg
				delete(c.Pending, msg.ID)
			}
			c.Mu.Unlock()
		} else if msg.Method != "" && c.Events != nil {
			c.Mu.Lock()
			if !c.Closed {
				select {
				case c.Events <- &msg:
				default:
					Term.Info("event buffer full, dropping: %s\n", msg.Method)
				}
			}
			c.Mu.Unlock()
		}
	}
}

func (c *CDPConn) Send(ctx context.Context, method string, params json.RawMessage, sessionID string) (*CDPMessage, error) {
	id := atomic.AddInt64(&c.NextID, 1)
	msg := CDPMessage{ID: id, Method: method, Params: params}
	if sessionID != "" {
		msg.SessionID = sessionID
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	Term.Info("-> %s\n", string(data))
	ch := make(chan *CDPMessage, 1)
	c.Mu.Lock()
	if c.Pending == nil {
		c.Mu.Unlock()
		return nil, fmt.Errorf("connection closed")
	}
	c.Pending[id] = ch
	c.Mu.Unlock()
	if err := c.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
		c.Mu.Lock()
		delete(c.Pending, id)
		c.Mu.Unlock()
		return nil, err
	}
	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("connection closed")
		}
		return resp, nil
	case <-ctx.Done():
		c.Mu.Lock()
		delete(c.Pending, id)
		c.Mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *CDPConn) Close() {
	_ = c.Conn.Close()
}

func (c *CDPConn) AttachToTarget(ctx context.Context, targetID string) (string, error) {
	attachParams, _ := json.Marshal(map[string]any{"targetId": targetID, "flatten": true})
	attachResp, err := c.Send(ctx, "Target.attachToTarget", attachParams, "")
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

func Send(ctx context.Context, wsURL, target, method string, params json.RawMessage) (*CDPMessage, error) {
	conn, err := DialCDP(wsURL, false)
	if err != nil {
		return nil, ErrRuntime("connecting: %v", err)
	}
	defer conn.Close()
	var sessionID string
	if target != "" && target != "browser" {
		sessionID, err = conn.AttachToTarget(ctx, target)
		if err != nil {
			return nil, ErrRuntime("attaching to target: %v", err)
		}
	}
	resp, err := conn.Send(ctx, method, params, sessionID)
	if err != nil {
		return nil, ErrRuntime("sending command: %v", err)
	}
	return resp, nil
}

func Listen(ctx context.Context, wsURL, target, domain string, eventCh chan<- *CDPMessage) error {
	conn, err := DialCDP(wsURL, true)
	if err != nil {
		return ErrRuntime("connecting: %v", err)
	}
	defer conn.Close()
	var sessionID string
	if target != "" {
		sessionID, err = conn.AttachToTarget(ctx, target)
		if err != nil {
			return ErrRuntime("attaching to target: %v", err)
		}
	}
	enableMethod := domain + ".enable"
	enableResp, err := conn.Send(ctx, enableMethod, nil, sessionID)
	if err != nil {
		return ErrRuntime("enabling %s: %v", domain, err)
	}
	if enableResp.Error != nil {
		return ErrUser("enable error: %s", enableResp.Error.Message)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-conn.Events:
			if !ok {
				return nil
			}
			select {
			case eventCh <- event:
			case <-ctx.Done():
				return nil
			}
		}
	}
}
