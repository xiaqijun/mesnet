package agent

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Peer struct {
	NodeID   uint
	Conn     *websocket.Conn
	LastSeen time.Time
}

type PeerManager struct {
	peers      map[uint]*Peer
	listenAddr string
	backbone   bool
	mu         sync.RWMutex
}

func NewPeerManager(listenAddr string, backbone bool) *PeerManager {
	return &PeerManager{
		peers:      make(map[uint]*Peer),
		listenAddr: listenAddr,
		backbone:   backbone,
	}
}

func (pm *PeerManager) Listen() (int, error) {
	if !pm.backbone {
		return 0, nil
	}
	ports := []int{443, 444, 8443, 9443, 10443}
	if pm.listenAddr != "" && pm.listenAddr != ":443" {
		if p, err := strconv.Atoi(strings.TrimPrefix(pm.listenAddr, ":")); err == nil {
			ports = append([]int{p}, ports...)
		}
	}
	for _, port := range ports {
		ln, err := net.Listen("tcp", ":"+_itoa(port))
		if err != nil {
			continue
		}
		log.Printf("peer listener started on :%d", port)
		http.HandleFunc("/agent-peer/", pm.handleIncoming)
		go func() { http.Serve(ln, nil) }()
		return port, nil
	}
	log.Printf("peer listener: no port available")
	return 0, nil
}

func (pm *PeerManager) handleIncoming(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	token := r.URL.Path[len("/agent-peer/"):]
	n := len(token)
	if n > 8 {
		n = 8
	}
	log.Printf("incoming peer connection, token=%s...", token[:n])
	pm.mu.Lock()
	pm.peers[0] = &Peer{Conn: conn, LastSeen: time.Now()}
	pm.mu.Unlock()
}

func (pm *PeerManager) Connect(nodeID uint, addr, token string) error {
	conn, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/agent-peer/"+token, nil)
	if err != nil {
		return err
	}
	pm.mu.Lock()
	pm.peers[nodeID] = &Peer{NodeID: nodeID, Conn: conn, LastSeen: time.Now()}
	pm.mu.Unlock()
	log.Printf("connected to peer %d at %s", nodeID, addr)
	return nil
}

func (pm *PeerManager) Disconnect(nodeID uint) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if p, ok := pm.peers[nodeID]; ok {
		p.Conn.Close()
		delete(pm.peers, nodeID)
	}
}

func (pm *PeerManager) ListPeers() []uint {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	ids := make([]uint, 0, len(pm.peers))
	for id := range pm.peers {
		ids = append(ids, id)
	}
	return ids
}

func (pm *PeerManager) SendRaw(nodeID uint, data []byte) error {
	pm.mu.RLock()
	p := pm.peers[nodeID]
	pm.mu.RUnlock()
	if p == nil {
		return nil
	}
	p.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return p.Conn.WriteMessage(websocket.BinaryMessage, data)
}

func (pm *PeerManager) Close() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for id, p := range pm.peers {
		p.Conn.Close()
		delete(pm.peers, id)
	}
}

func _itoa(n int) string { return strconv.Itoa(n) }
