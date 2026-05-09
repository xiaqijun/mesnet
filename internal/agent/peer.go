package agent

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Peer represents a direct WebSocket connection to another Agent.
type Peer struct {
	NodeID   uint
	Conn     *websocket.Conn
	Outbound bool // true=we dialed out, false=they dialed in
	mu       sync.Mutex
}

// PeerManager handles Agent-to-Agent direct WSS connections.
type PeerManager struct {
	listenAddr string
	backbone   bool // only backbone nodes accept incoming connections
	peers      map[uint]*Peer
	mu         sync.RWMutex
}

func NewPeerManager(listenAddr string, backbone bool) *PeerManager {
	return &PeerManager{
		listenAddr: listenAddr,
		backbone:   backbone,
		peers:      make(map[uint]*Peer),
	}
}

// Listen starts the WSS server for incoming peer connections.
// Only called if backbone=true.
func (pm *PeerManager) Listen() error {
	if !pm.backbone {
		return nil
	}

	http.HandleFunc("/agent-peer/", pm.handleIncoming)
	go func() {
		log.Printf("peer listener starting on %s", pm.listenAddr)
		if err := http.ListenAndServeTLS(pm.listenAddr, "", "", nil); err != nil {
			log.Printf("peer listener error: %v", err)
		}
	}()
	return nil
}

func (pm *PeerManager) handleIncoming(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// Token from URL path: /agent-peer/<token>
	token := r.URL.Path[len("/agent-peer/"):]
	log.Printf("incoming peer connection, token=%s...", token[:8]+"...")

	// Create peer entry — nodeID will be set after first message
	peer := &Peer{
		Conn:     conn,
		Outbound: false,
	}

	pm.mu.Lock()
	pm.peers[0] = peer // placeholder, updated on identification
	pm.mu.Unlock()
}

// Connect initiates a WSS connection to another Agent (outbound).
func (pm *PeerManager) Connect(nodeID uint, addr, token string) error {
	url := fmt.Sprintf("wss://%s/agent-peer/%s", addr, token)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("peer connect %d at %s: %w", nodeID, addr, err)
	}

	peer := &Peer{
		NodeID:   nodeID,
		Conn:     conn,
		Outbound: true,
	}

	pm.mu.Lock()
	pm.peers[nodeID] = peer
	pm.mu.Unlock()

	log.Printf("connected to peer %d at %s", nodeID, addr)
	return nil
}

// Disconnect closes a peer connection.
func (pm *PeerManager) Disconnect(nodeID uint) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if p, ok := pm.peers[nodeID]; ok {
		p.Conn.Close()
		delete(pm.peers, nodeID)
		log.Printf("disconnected from peer %d", nodeID)
	}
}

// SendRaw sends a binary frame to a peer.
func (pm *PeerManager) SendRaw(nodeID uint, data []byte) error {
	pm.mu.RLock()
	p, ok := pm.peers[nodeID]
	pm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("peer %d not connected", nodeID)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return p.Conn.WriteMessage(websocket.BinaryMessage, data)
}

// ListPeers returns IDs of all connected peers.
func (pm *PeerManager) ListPeers() []uint {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	ids := make([]uint, 0, len(pm.peers))
	for id := range pm.peers {
		ids = append(ids, id)
	}
	return ids
}

// IsConnected checks if we have a peer connection to a node.
func (pm *PeerManager) IsConnected(nodeID uint) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	_, ok := pm.peers[nodeID]
	return ok
}

// Close shuts down all peer connections.
func (pm *PeerManager) Close() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for id, p := range pm.peers {
		p.Conn.Close()
		delete(pm.peers, id)
	}
}
