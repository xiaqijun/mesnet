package agent

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mesnet/mesnet/internal/version"
)

type WSClient struct {
	url      string
	handler  *Handler
	conn     *websocket.Conn
	peerPort int
	mu       sync.Mutex
	closed   bool
}

func NewWSClient(url string, handler *Handler, peerPort int) *WSClient {
	return &WSClient{
		url:      url,
		handler:  handler,
		peerPort: peerPort,
	}
}

func (c *WSClient) Connect() {
	backoff := 1 * time.Second
	maxBackoff := 60 * time.Second

	for {
		conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
		if err != nil {
			log.Printf("ws connect failed: %v, retrying in %v", err, backoff)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		log.Printf("ws connected to control plane")
		backoff = 1 * time.Second
		c.mu.Lock()
		c.conn = conn
		c.closed = false
		c.mu.Unlock()

		c.SendJSON(map[string]any{
			"type":        "hello",
			"name":        "agent",
			"version":     version.Current,
			"listen_port": c.peerPort,
		})

		c.readLoop(conn)

		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
	}
}

func (c *WSClient) readLoop(conn *websocket.Conn) {
	for {
		msgType, raw, err := conn.ReadMessage()
		if err != nil {
			log.Printf("ws read error: %v", err)
			return
		}

		// Binary relayed data from another agent via control plane
		if msgType == websocket.BinaryMessage {
			c.handleRelay(raw)
			continue
		}

		var msg map[string]json.RawMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		typeStr := ""
		json.Unmarshal(msg["type"], &typeStr)

		switch typeStr {
		case "ping":
			c.SendJSON(map[string]string{"type": "pong"})
		case "cmd":
			c.handler.Handle(msg)
		}
	}
}

// handleRelay receives data relayed from another agent via control plane.
func (c *WSClient) handleRelay(raw []byte) {
	if c.handler.agent.onRecvRelay != nil {
		c.handler.agent.onRecvRelay(raw)
	}
}

// RelayTo sends encrypted tunnel data to another agent via control plane relay.
func (c *WSClient) RelayTo(targetNodeID uint, frame []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.closed {
		return
	}
	// Send as binary with target encoded in first 8 bytes
	header := make([]byte, 8+len(frame))
	header[0] = byte(targetNodeID >> 24)
	header[1] = byte(targetNodeID >> 16)
	header[2] = byte(targetNodeID >> 8)
	header[3] = byte(targetNodeID)
	copy(header[8:], frame)
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	c.conn.WriteMessage(websocket.BinaryMessage, header)
}

func (c *WSClient) SendJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.closed {
		return nil
	}
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteJSON(v)
}

func (c *WSClient) SendRaw(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.closed {
		return nil
	}
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *WSClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil && !c.closed
}

func (c *WSClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.conn != nil {
		c.conn.Close()
	}
}
