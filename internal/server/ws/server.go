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
	"github.com/mesnet/mesnet/internal/version"
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
	conns       map[uint]*AgentConn
	onRecv      func(*AgentConn, Message)
	onHello     func(nodeID uint)
	onUnregister func(nodeID uint)
	pending     map[string]chan Message
	mu          sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		conns:   make(map[uint]*AgentConn),
		pending: make(map[string]chan Message),
	}
}

func (r *Registry) SetOnRecv(fn func(*AgentConn, Message)) {
	r.onRecv = fn
}

func (r *Registry) SetOnHello(fn func(nodeID uint)) {
	r.onHello = fn
}

func (r *Registry) SetOnUnregister(fn func(nodeID uint)) {
	r.onUnregister = fn
}

func (r *Registry) Register(nodeID uint, ac *AgentConn) {
	r.mu.Lock()
	// Close old connection if exists, so its deferred Unregister runs BEFORE onHello
	if old, ok := r.conns[nodeID]; ok && old.WS != ac.WS {
		old.WS.Close()
	}
	r.conns[nodeID] = ac
	r.mu.Unlock()
	logwatch.Debug("registry", fmt.Sprintf("agent registered node=%d name=%s", nodeID, ac.NodeName))
}

func (r *Registry) Unregister(nodeID uint, ac *AgentConn) {
	r.mu.Lock()
	// Only remove if this exact AgentConn is still registered (not replaced by a newer connection)
	removed := false
	if current, ok := r.conns[nodeID]; ok && current == ac {
		delete(r.conns, nodeID)
		removed = true
	}
	r.mu.Unlock()
	if removed {
		logwatch.Debug("registry", fmt.Sprintf("agent unregistered node=%d", nodeID))
	} else {
		return // old connection replaced by newer one, skip onUnregister
	}
	if r.onUnregister != nil {
		go r.onUnregister(nodeID)
	}
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

func extractRemoteIP(r *http.Request) string {
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx > 0 {
		ip = ip[:idx]
	}
	// Strip brackets from IPv6
	ip = strings.TrimPrefix(ip, "[")
	ip = strings.TrimSuffix(ip, "]")
	return ip
}

// HandleAgent handles a WebSocket upgrade for an Agent.
func HandleAgent(w http.ResponseWriter, r *http.Request, registry *Registry, db *gorm.DB) {
	parts := splitPath(r.URL.Path)
	var token string
	if len(parts) >= 3 && parts[len(parts)-2] == "agent" {
		token = parts[len(parts)-1]
	}

	var n struct {
		ID   uint
		Name string
	}
	if err := db.Table("nodes").Where("agent_token = ?", token).Select("id, name").Scan(&n).Error; err != nil || n.ID == 0 {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}

	remoteIP := extractRemoteIP(r)

	ac := &AgentConn{
		NodeID:   n.ID,
		NodeName: n.Name,
		WS:       ws,
		lastSeen: time.Now(),
	}

	registry.Register(n.ID, ac)
	logwatch.Info("agent", fmt.Sprintf("node %d (%s) connected from %s", n.ID, n.Name, remoteIP))
	defer func() {
		registry.Unregister(n.ID, ac)
		ws.Close()
		db.Table("nodes").Where("id = ?", n.ID).Updates(map[string]any{
			"connected": false,
			"last_seen": time.Now(),
		})
	}()

	db.Table("nodes").Where("id = ?", n.ID).Updates(map[string]any{
		"connected": true,
		"last_seen": time.Now(),
	})

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			log.Printf("agent %d read error: %v", n.ID, err)
			return
		}

		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("agent %d invalid message: %v", n.ID, err)
			continue
		}

		ac.lastSeen = time.Now()

		switch msg.Type {
		case "pong":
		case "hello":
			db.Table("nodes").Where("id = ?", n.ID).Updates(map[string]any{
				"connected": true,
				"last_seen": time.Now(),
			})

			var hello struct {
				Type       string `json:"type"`
				Name       string `json:"name"`
				Version    string `json:"version"`
				ListenPort int    `json:"listen_port"`
				PublicKey  string `json:"public_key"`
}
			if json.Unmarshal(raw, &hello) == nil {
				if hello.Version != "" {
					db.Table("nodes").Where("id = ?", n.ID).Update("agent_version", hello.Version)
					if hello.PublicKey != "" {
						db.Table("nodes").Where("id = ?", n.ID).Update("public_key", hello.PublicKey)
						log.Printf("agent %d public_key stored (len=%d)", n.ID, len(hello.PublicKey))
					} else {
						log.Printf("WARNING: agent %d sent empty public_key", n.ID)
					}
					}
				// Auto-set listen_addr from remote IP + reported port
				if hello.ListenPort > 0 {
					addr := fmt.Sprintf("%s:%d", remoteIP, hello.ListenPort)
					db.Table("nodes").Where("id = ?", n.ID).Update("listen_addr", addr)
					log.Printf("agent %d listen_addr auto-set to %s", n.ID, addr)
					}
				// Auto-update agent if server version > agent version
				serverVer := version.Current
				if hello.Version != "" && isOlderVersion(hello.Version, serverVer) {
					log.Printf("agent %d version %s != server %s, triggering update", n.ID, hello.Version, serverVer)
					go func() {
						ac.SendJSON(Message{Type: "cmd", ID: "auto_update", Action: "agent_update"})
					}()
					}
			}

			if registry.onHello != nil {
				go registry.onHello(n.ID)
			}
		case "result":
			registry.handleResult(msg)
		case "stats":
			if registry.onRecv != nil {
				registry.onRecv(ac, msg)
			}
		case "log":
			var logEntry struct {
				Level   string `json:"level"`
				Source  string `json:"source"`
				Message string `json:"message"`
			}
			if json.Unmarshal(msg.Data, &logEntry) == nil && logEntry.Message != "" {
				switch logEntry.Level {
				case "ERROR":
					logwatch.Error(logEntry.Source, logEntry.Message)
				case "WARN":
					logwatch.Warn(logEntry.Source, logEntry.Message)
				default:
					logwatch.Info(logEntry.Source, logEntry.Message)
				}
			}
		}
	}
}

func splitPath(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
}


// isOlderVersion compares semantic version strings (e.g. "v1.0.96", "1.0.96").
// Returns true if a < b.
func isOlderVersion(a, b string) bool {
	strip := func(v string) string {
		if len(v) > 0 && v[0] == 'v' {
			return v[1:]
		}
		return v
	}
	va, vb := strip(a), strip(b)
	if va == vb {
		return false
	}
	// Simple comparison by splitting on "."
	pa, pb := 0, 0
	for pa < len(va) || pb < len(vb) {
		na, nb := 0, 0
		for pa < len(va) && va[pa] != '.' {
			na = na*10 + int(va[pa]-'0')
			pa++
		}
		pa++ // skip .
		for pb < len(vb) && vb[pb] != '.' {
			nb = nb*10 + int(vb[pb]-'0')
			pb++
		}
		pb++ // skip .
		if na != nb {
			return na < nb
		}
	}
	return false
}
