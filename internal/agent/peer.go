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
	peers         map[uint]*Peer
	listenAddr    string
	backbone      bool
	pendingTokens map[string]uint                                    // token → expected nodeID for incoming connections
	onRecv        func(nodeID uint, frame []byte)                    // called when data received from a peer
	onHandshake   func(nodeID uint, frame []byte) ([]byte, error)    // called when handshake frame received
	onProbeResp   func(nodeID uint, seq uint32)                      // called when probe response received
	mu            sync.RWMutex
}

func NewPeerManager(listenAddr string, backbone bool) *PeerManager {
	return &PeerManager{
		peers:         make(map[uint]*Peer),
		listenAddr:    listenAddr,
		backbone:      backbone,
		pendingTokens: make(map[string]uint),
	}
}

// ExpectConnection registers a pending token → nodeID mapping for an
// expected incoming peer connection.
func (pm *PeerManager) ExpectConnection(token string, nodeID uint) {
	pm.mu.Lock()
	pm.pendingTokens[token] = nodeID
	pm.mu.Unlock()
}

// SetOnRecv sets the callback for incoming peer data.
func (pm *PeerManager) SetOnRecv(fn func(nodeID uint, frame []byte)) {
	pm.onRecv = fn
}

// SetOnHandshake sets the callback for incoming Noise handshake frames.
func (pm *PeerManager) SetOnHandshake(fn func(nodeID uint, frame []byte) ([]byte, error)) {
	pm.onHandshake = fn
}

// SetOnProbeResponse sets the callback for incoming probe response frames.
func (pm *PeerManager) SetOnProbeResponse(fn func(nodeID uint, seq uint32)) {
	pm.onProbeResp = fn
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

	// Look up the expected node ID for this token
	pm.mu.Lock()
	nodeID, ok := pm.pendingTokens[token]
	if ok {
		delete(pm.pendingTokens, token) // one-time use
	}
	if !ok {
		nodeID = 0 // fallback: unknown peer
		log.Printf("incoming peer: unknown token %s..., using nodeID=0", token[:n])
	}
	pm.peers[nodeID] = &Peer{NodeID: nodeID, Conn: conn, LastSeen: time.Now()}
	pm.mu.Unlock()
	// Start reader for incoming data + handshake
	go pm.readLoop(nodeID, conn)
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

	// Start reader for incoming data + handshake responses
	go pm.readLoop(nodeID, conn)
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
		log.Printf("peer SendRaw: peer %d not connected", nodeID)
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

// readLoop reads binary frames from a peer connection and dispatches to
// onHandshake (for handshake frames) or onRecv (for data frames).
func (pm *PeerManager) readLoop(nodeID uint, conn *websocket.Conn) {
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("peer %d read error: %v", nodeID, err)
			pm.mu.Lock()
			// Only delete if this connection is still the active one
			if p, ok := pm.peers[nodeID]; ok && p.Conn == conn {
				delete(pm.peers, nodeID)
			}
			pm.mu.Unlock()
			return
		}
		if msgType != websocket.BinaryMessage {
			continue
		}

		// Update last seen
		pm.mu.RLock()
		if p, ok := pm.peers[nodeID]; ok {
			p.LastSeen = time.Now()
		}
		pm.mu.RUnlock()

		// Check if this is a handshake frame
		var flags uint16
		if len(data) >= 8 {
			flags = (uint16(data[6]) << 8) | uint16(data[7])
			if flags&FlagHandshake != 0 && pm.onHandshake != nil {
				response, err := pm.onHandshake(nodeID, data)
				if err != nil {
					log.Printf("peer %d handshake failed: %v", nodeID, err)
					return
				}
				// Send handshake response back
				if response != nil {
					conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
					conn.WriteMessage(websocket.BinaryMessage, response)
				}
				continue
			}
		}

		// Check if this is a probe frame → respond immediately
		if flags&FlagProbe != 0 {
			hdr, _, _ := DecodeFrameHeader(data)
			response := EncodeFrame(FlagProbeResponse, hdr.Seq, uint64(nodeID), nil)
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			conn.WriteMessage(websocket.BinaryMessage, response)
			continue
		}

		// Check if this is a probe response → notify probe tracker
		if flags&FlagProbeResponse != 0 && pm.onProbeResp != nil {
			hdr, _, _ := DecodeFrameHeader(data)
			pm.onProbeResp(nodeID, hdr.Seq)
			continue
		}

		// Normal data frame
		if pm.onRecv != nil {
			pm.onRecv(nodeID, data)
		}
	}
}

func _itoa(n int) string { return strconv.Itoa(n) }
