package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mesnet/mesnet/internal/server/logwatch"
	"gorm.io/gorm"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Message represents the JSON protocol between Agent and Control Plane.
type Message struct {
	Type   string          `json:"type"`
	ID     string          `json:"id,omitempty"`
	Action string          `json:"action,omitempty"`
	Args   json.RawMessage `json:"args,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
	Status string          `json:"status,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// AgentConn wraps a single Agent WebSocket connection.
type AgentConn struct {
	NodeID   uint
	NodeName string
	WS       *websocket.Conn
	mu       sync.Mutex
	lastSeen time.Time
}

func (ac *AgentConn) SendJSON(v any) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.WS.WriteJSON(v)
}

func (ac *AgentConn) SendPing() error {
	return ac.SendJSON(Message{Type: "ping"})
}

// Registry manages active Agent WebSocket connections.
type Registry struct {
	conns    map[uint]*AgentConn
	onRecv   func(*AgentConn, Message)
	onHello  func(nodeID uint) // called when agent sends hello (first connect)
	pending  map[string]chan Message
	mu       sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		conns:    make(map[uint]*AgentConn),
		pending:  make(map[string]chan Message),
	}
}

func (r *Registry) SetOnRecv(fn func(*AgentConn, Message)) {
	r.onRecv = fn
}

func (r *Registry) SetOnHello(fn func(nodeID uint)) {
	r.onHello = fn
}

func (r *Registry) Register(nodeID uint, ac *AgentConn) {
	r.mu.Lock()
	r.conns[nodeID] = ac
	r.mu.Unlock()
	log.Printf("agent registered: node=%d name=%s", nodeID, ac.NodeName)
}

func (r *Registry) Unregister(nodeID uint) {
	r.mu.Lock()
	delete(r.conns, nodeID)
	r.mu.Unlock()
	log.Printf("agent unregistered: node=%d", nodeID)
}

func (r *Registry) IsOnline(nodeID uint) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.conns[nodeID]
	return ok
}

func (r *Registry) GetConn(nodeID uint) *AgentConn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.conns[nodeID]
}

func (r *Registry) ListOnline() []uint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]uint, 0, len(r.conns))
	for id := range r.conns {
		ids = append(ids, id)
	}
	return ids
}

// SendCmd sends a command to an Agent and waits for the result.
func (r *Registry) SendCmd(nodeID uint, action string, args any, timeout time.Duration) (*Message, error) {
	ac := r.GetConn(nodeID)
	if ac == nil {
		return nil, ErrAgentOffline
	}

	cmdID := action + "_" + time.Now().Format("150405.000")
	argsJSON, _ := json.Marshal(args)

	msg := Message{
		Type:   "cmd",
		ID:     cmdID,
		Action: action,
		Args:   argsJSON,
	}

	ch := make(chan Message, 1)
	r.mu.Lock()
	r.pending[cmdID] = ch
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.pending, cmdID)
		r.mu.Unlock()
	}()

	if err := ac.SendJSON(msg); err != nil {
		return nil, err
	}

	select {
	case result := <-ch:
		return &result, nil
	case <-time.After(timeout):
		return nil, ErrTimeout
	}
}

func (r *Registry) handleResult(msg Message) {
	r.mu.RLock()
	ch, ok := r.pending[msg.ID]
	r.mu.RUnlock()
	if ok {
		ch <- msg
	}
}

func (r *Registry) Run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.RLock()
		for _, ac := range r.conns {
			if err := ac.SendPing(); err != nil {
				log.Printf("ping failed for node %d: %v", ac.NodeID, err)
			}
		}
		r.mu.RUnlock()
	}
}

// HandleAgent handles a WebSocket upgrade for an Agent.
func HandleAgent(w http.ResponseWriter, r *http.Request, registry *Registry, db *gorm.DB) {
	// Extract token from URL: /ws/agent/<token>
	parts := splitPath(r.URL.Path)
	var token string
	if len(parts) >= 3 && parts[len(parts)-2] == "agent" {
		token = parts[len(parts)-1]
	}

	// Validate token and find node
	var node struct {
		ID   uint
		Name string
	}
	if err := db.Table("nodes").Where("agent_token = ?", token).Select("id, name").Scan(&node).Error; err != nil || node.ID == 0 {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}

	ac := &AgentConn{
		NodeID:   node.ID,
		NodeName: node.Name,
		WS:       ws,
		lastSeen: time.Now(),
	}

	registry.Register(node.ID, ac)
	logwatch.Info("agent", fmt.Sprintf("node %d (%s) connected", node.ID, node.Name))
	defer func() {
		registry.Unregister(node.ID)
		ws.Close()
		db.Table("nodes").Where("id = ?", node.ID).Updates(map[string]any{
			"connected": false,
			"last_seen": time.Now(),
		})
	}()

	// Mark connected
	db.Table("nodes").Where("id = ?", node.ID).Updates(map[string]any{
		"connected": true,
		"last_seen": time.Now(),
	})

	// Read loop
	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			log.Printf("agent %d read error: %v", node.ID, err)
			return
		}

		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("agent %d invalid message: %v", node.ID, err)
			continue
		}

		ac.lastSeen = time.Now()

		switch msg.Type {
		case "pong":
			// heartbeat ack
		case "hello":
			db.Table("nodes").Where("id = ?", node.ID).Updates(map[string]any{
				"connected": true,
				"last_seen": time.Now(),
			})

			// Record agent version
			var hello struct {
				Type    string `json:"type"`
				Name    string `json:"name"`
				Version string `json:"version"`
			}
			if json.Unmarshal(raw, &hello) == nil && hello.Version != "" {
				db.Table("nodes").Where("id = ?", node.ID).Update("agent_version", hello.Version)
			}

			if registry.onHello != nil {
				go registry.onHello(node.ID)
			}
		case "result":
			registry.handleResult(msg)
		case "stats":
			if registry.onRecv != nil {
				registry.onRecv(ac, msg)
			}
		}
	}
}

func splitPath(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
}
