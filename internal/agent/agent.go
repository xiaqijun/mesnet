package agent

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"
)

// Config holds Agent configuration.
type Config struct {
	Name       string
	ServerURL  string // wss://cp:443/ws/agent/<token>
	ListenAddr string // :443, only used if Backbone=true
	Backbone   bool   // backbone nodes listen, leaf nodes only dial out
	Token      string // extracted from ServerURL
}

// Agent orchestrates management connection, peer connections, TUN, and tunnels.
type Agent struct {
	cfg    Config
	ws     *WSClient
	peers  *PeerManager
	tun    *TUNDevice
	router *PacketRouter

	keyPair  *KeyPair                    // Curve25519 static keypair
	channels map[uint]*SecureChannel     // active secure channels, keyed by nodeID
	peerKeys map[uint][]byte             // known peer public keys, keyed by nodeID

	peerPort int
	crypto   *Crypto
	routes   *RouteManager
	stats    *StatsCollector
	probe    *Probe

	handler *Handler

	mu   sync.Mutex
	quit chan struct{}
}

func New(name, serverURL, listenAddr string, backbone bool) *Agent {
	token := extractToken(serverURL)

	cfg := Config{
		Name:       name,
		ServerURL:  serverURL,
		ListenAddr: listenAddr,
		Backbone:   backbone,
		Token:      token,
	}

	kp, err := GenerateKeyPair()
	if err != nil {
		log.Printf("WARNING: keypair generation failed: %v", err)
	}

	a := &Agent{
		cfg:      cfg,
		quit:     make(chan struct{}),
		peers:    NewPeerManager(listenAddr, backbone),
		keyPair:  kp,
		channels: make(map[uint]*SecureChannel),
		peerKeys: make(map[uint][]byte),
	}

	a.handler = NewHandler(a)
	a.crypto = NewCrypto()
	a.tun = NewTUNDevice()
	a.routes = NewRouteManager()
	a.router = NewPacketRouter(a.peers, a.routes)
	a.stats = NewStatsCollector(a)
	a.probe = NewProbe(a.peers)

	return a
}

func (a *Agent) Start() error {
	// Only backbone nodes listen for incoming peer connections
	if a.cfg.Backbone {
		if port, err := a.peers.Listen(); err != nil {
			a.peerPort = port
			log.Printf("peer listen failed (non-fatal): %v", err)
		} else if port > 0 {
			a.peerPort = port
			log.Printf("backbone node: listening on :%d", port)
		}
	} else {
		log.Printf("leaf node: outbound-only mode")
	}

	// Handle incoming peer data: decrypt, forward to TUN, or relay
	a.peers.SetOnRecv(func(nodeID uint, frame []byte) {
		tun := NewTunnel(a)
		plaintext, err := tun.ReceiveEncrypted(nodeID, frame)
		if err != nil {
			log.Printf("onRecv: decrypt from peer %d failed: %v", nodeID, err)
			return
		}
		if plaintext == nil {
			return // non-data frame
		}

		dstIP := extractDstIP(plaintext)
		_, nextHop := a.routes.Lookup(dstIP)

		// Relay first — forward to next hop if not for us
		if nextHop != 0 && nextHop != nodeID {
			tun.SendEncrypted(nextHop, plaintext)
			return
		}

		// Check if this IP belongs to one of our local subnets
		if isLocalIP(a.tun.IP(), dstIP) || a.isInOurSubnets(dstIP) {
			a.tun.Write(plaintext)
			return
		}
	})

	// Handle incoming handshake frames at peer level.
	// This callback handles BOTH directions:
	//   - If a SecureChannel already exists for the peer but is not established,
	//     this is a handshake response → CompleteHandshake
	//   - Otherwise it's a new incoming handshake → AcceptHandshake
	a.peers.SetOnHandshake(func(nodeID uint, frame []byte) ([]byte, error) {
		hdr, payload, err := DecodeFrameHeader(frame)
		if err != nil || hdr.Flags&FlagHandshake == 0 {
			return nil, err
		}

		a.mu.Lock()
		ch, exists := a.channels[nodeID]
		a.mu.Unlock()

		if exists {
			if !ch.IsEstablished() {
				// Handshake response: we initiated, they responded
				if err := ch.CompleteHandshake(payload); err != nil {
					log.Printf("handshake: complete failed for peer %d: %v", nodeID, err)
					return nil, err
				}
				log.Printf("handshake: completed with peer %d", nodeID)
				return nil, nil
			}
			// Peer restarted with new ephemeral key — wipe old channel and re-handshake
			ch.Wipe()
			a.mu.Lock()
			delete(a.channels, nodeID)
			a.mu.Unlock()
			log.Printf("handshake: peer %d reconnected, re-handshaking", nodeID)
			// Fall through to new incoming handshake below
		}

		// New incoming handshake: we are the responder
		peerPub, ok := a.peerKeys[nodeID]
		if !ok {
			log.Printf("handshake: no public key for peer %d", nodeID)
			return nil, ErrHandshakeFailed
		}
		ch = NewSecureChannel(a.keyPair, peerPub)
		response, err := ch.AcceptHandshake(payload)
		if err != nil {
			log.Printf("handshake: accept failed for peer %d: %v", nodeID, err)
			return nil, err
		}
		a.mu.Lock()
		a.channels[nodeID] = ch
		a.mu.Unlock()
		log.Printf("handshake: accepted from peer %d", nodeID)
		return response, nil
	})

	// Connect to control plane
	a.ws = NewWSClient(a.cfg.ServerURL, a.handler, a.peerPort, a.PublicKeyHex())
	go a.ws.Connect()

	// Start stats collector
	go a.stats.Run(a.quit)

	// Start latency prober
	go a.probe.Run(a.quit)

	// Start TUN packet forwarding loop
	tunnel := NewTunnel(a)
	go tunnel.Run()

	// Fast failover: when a peer disconnects, immediately switch routes to backup
		a.peers.SetOnReconnect(func(nodeID uint) {
		// Reset send queue so new WebSocket is used
		a.router.RemovePeer(nodeID)
	})

	a.peers.SetOnDisconnect(func(nodeID uint) {
		// Find alternative routes through remaining peers
		remainingPeers := a.peers.ListPeers()
		for _, route := range a.routes.List() {
			if route.NextHop == nodeID {
				// Try to find a new nextHop through another peer
				for _, altPeer := range remainingPeers {
					if altPeer != nodeID {
						route.NextHop = altPeer
						log.Printf("failover: route %s switched from peer %d to %d", route.Subnet, nodeID, altPeer)
						break
					}
				}
			}
		}
	})

	return nil
}

func (a *Agent) Stop() {
	close(a.quit)
	if a.ws != nil {
		a.ws.Close()
	}
	a.router.Stop()
	a.peers.Close()
	if a.tun != nil {
		a.tun.Destroy()
	}
	// Wipe secure channels
	for _, ch := range a.channels {
		ch.Wipe()
	}
}

// PublicKeyHex returns the agent's static Curve25519 public key as hex.
func (a *Agent) PublicKeyHex() string {
	if a.keyPair == nil {
		return ""
	}
	return hexEncode(a.keyPair.PublicKey)
}

func (a *Agent) HandleCommand(action string, args json.RawMessage) (any, error) {
	switch action {
	case "tun_setup":
		var params struct {
			IP string `json:"ip"`
		}
		json.Unmarshal(args, &params)
		if err := a.tun.Create(params.IP); err != nil {
			return nil, err
		}
		// Sync any routes that arrived before TUN was ready
		a.routes.FlushKernel()
		return nil, nil

	case "tun_destroy":
		return nil, a.tun.Destroy()

	case "route_add":
		var params struct {
			Subnet   string `json:"subnet"`
			NodeID   uint   `json:"node_id"`
			NextHop  uint   `json:"next_hop"`
			TunnelID uint   `json:"tunnel_id"`
		}
		json.Unmarshal(args, &params)
		return nil, a.routes.Add(params.Subnet, params.NodeID, params.NextHop)

	case "route_del":
		var params struct {
			Subnet string `json:"subnet"`
		}
		json.Unmarshal(args, &params)
		return nil, a.routes.Del(params.Subnet)

	case "peer_connect":
		var params struct {
			NodeID    uint   `json:"node_id"`
			PeerAddr  string `json:"peer_addr"`
			PeerToken string `json:"peer_token"`
			PublicKey string `json:"public_key"`
			TunnelID  uint   `json:"tunnel_id"`
		}
		json.Unmarshal(args, &params)

		// Store peer public key for handshake
		if params.PublicKey != "" {
			pk := hexDecode(params.PublicKey)
			if len(pk) == 32 {
				a.mu.Lock()
				a.peerKeys[params.NodeID] = pk
				a.mu.Unlock()
			}
		}

		// Skip if we already have a channel for this peer (any state)
		a.mu.Lock()
		_, hasChannel := a.channels[params.NodeID]
		a.mu.Unlock()
		if hasChannel {
			// Already connected, just re-send routes if needed
			return nil, nil
		}

		// Create SecureChannel and generate init handshake frame
		peerPub := a.peerKeys[params.NodeID]
		ch := NewSecureChannel(a.keyPair, peerPub)
		initFrame, err := ch.InitiateHandshake()
		if err != nil {
			return nil, err
		}
		a.mu.Lock()
		a.channels[params.NodeID] = ch
		a.mu.Unlock()

		// Register expected incoming connection (peer dials us using our token)
		a.peers.ExpectConnection(a.cfg.Token, params.NodeID)

		// Connect to peer
		if err := a.peers.Connect(params.NodeID, params.PeerAddr, params.PeerToken); err != nil {
			return nil, err
		}

		// Send init handshake frame (contains our ephemeral public key)
		a.peers.SendRaw(params.NodeID, initFrame)
		return nil, nil

		case "peer_accept":
			var params struct {
				NodeID    uint   `json:"node_id"`
				Token     string `json:"token"`
				PublicKey string `json:"public_key"`
			}
			json.Unmarshal(args, &params)

			// Store peer public key for incoming handshake
			if params.PublicKey != "" {
				pk := hexDecode(params.PublicKey)
				if len(pk) == 32 {
					a.mu.Lock()
					a.peerKeys[params.NodeID] = pk
					a.mu.Unlock()
				}
			}

			// Register expected incoming connection (peer dials us with this token)
			a.peers.ExpectConnection(params.Token, params.NodeID)
			return nil, nil

		case "peer_disconnect":
			var params struct {
				PeerID uint `json:"peer_id"`
			}
			json.Unmarshal(args, &params)
			a.peers.Disconnect(params.PeerID)
			a.mu.Lock()
			if ch, ok := a.channels[params.PeerID]; ok {
				ch.Wipe()
				delete(a.channels, params.PeerID)
			}
			a.mu.Unlock()
			return nil, nil

		case "tunnel_test":
			var params struct {
				NodeID uint `json:"node_id"`
			}
			json.Unmarshal(args, &params)

			a.mu.Lock()
			ch := a.channels[params.NodeID]
			a.mu.Unlock()

			var channelStatus string
			switch {
			case ch == nil:
				channelStatus = "none"
			case ch.IsEstablished():
				channelStatus = "established"
			default:
				channelStatus = "establishing"
			}

			peerConnected := false
			for _, pid := range a.peers.ListPeers() {
				if pid == params.NodeID {
					peerConnected = true
					break
				}
			}

			// Check if we have routes pointing to this peer
			hasRoutes := false
			for _, rte := range a.routes.List() {
				if rte.NextHop == params.NodeID || rte.NodeID == params.NodeID {
					hasRoutes = true
					break
				}
			}

			_, hasPeerKey := a.peerKeys[params.NodeID]

			var rtt float64
			latencies := a.probe.Latencies()
			if v, ok := latencies[params.NodeID]; ok {
				rtt = v
			}

			return map[string]any{
				"target_id":           params.NodeID,
				"channel_established": ch != nil && ch.IsEstablished(),
				"channel_status":      channelStatus,
				"peer_connected":      peerConnected,
				"has_peer_key":        hasPeerKey,
				"has_routes":          hasRoutes,
				"total_routes":        len(a.routes.List()),
				"total_peers":         len(a.peers.ListPeers()),
				"rtt_ms":              rtt,
			}, nil

			case "backbone_probe":
			var params struct {
				Addrs []struct {
					ID   uint   `json:"id"`
					Addr string `json:"addr"`
				} `json:"addrs"`
			}
			json.Unmarshal(args, &params)

			type probeResult struct {
				ID    uint   `json:"id"`
				Addr  string `json:"addr"`
				RTTMS int64  `json:"rtt_ms"`
			}
			var results []probeResult
			for _, a := range params.Addrs {
				start := time.Now()
				conn, err := net.DialTimeout("tcp", a.Addr, 3*time.Second)
				rtt := time.Since(start).Milliseconds()
				if err != nil {
					rtt = -1
				} else {
					conn.Close()
				}
				results = append(results, probeResult{ID: a.ID, Addr: a.Addr, RTTMS: rtt})
			}
			return map[string]any{"results": results}, nil

		case "subnet_detect":
		subnets, err := a.routes.DetectSubnets()
		return map[string]any{"subnets": subnets}, err

	case "diagnose":
		result := a.Diagnose()
		return result, nil

	case "status":
		result := a.CollectStatus()
		return result, nil

	case "agent_update":
		log.Printf("self-update triggered by control plane")
		if err := SelfUpdate(); err != nil {
			return nil, err
		}
		return map[string]string{"status": "restarting"}, nil
	}

	log.Printf("unknown command: %s", action)
	return nil, nil
}

func (a *Agent) Diagnose() map[string]any {
	return map[string]any{
		"ws_connected": a.ws != nil && a.ws.IsConnected(),
		"tun_up":       a.tun.IsUp(),
		"peers":        a.peers.ListPeers(),
		"routes":       a.routes.List(),
	}
}

func (a *Agent) CollectStatus() map[string]any {
	return map[string]any{
		"tun_ip":   a.tun.IP(),
		"tun_up":   a.tun.IsUp(),
		"backbone": a.cfg.Backbone,
		"peers":    len(a.peers.ListPeers()),
		"routes":   a.routes.List(),
		"subnets":  a.getSubnets(),
	}
}

func (a *Agent) getSubnets() []string {
	return a.routes.getSubnets()
}

func (a *Agent) getMyNodeID() uint {
	return 0
}

// isInOurSubnets checks if an IP belongs to any of our local subnets.
func (a *Agent) isInOurSubnets(ip string) bool {
	subnets := a.routes.getSubnets()
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, s := range subnets {
		_, cidr, err := net.ParseCIDR(s)
		if err != nil {
			continue
		}
		if cidr.Contains(parsed) {
			return true
		}
	}
	return false
}

// isLocalIP checks if an IP is the TUN IP or loopback.
func isLocalIP(tunIP, ip string) bool {
	return ip == tunIP || ip == "127.0.0.1" || ip == "::1"
}

func extractToken(url string) string {
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == '/' {
			return url[i+1:]
		}
	}
	return ""
}

func (a *Agent) OnStats(fn func(string, uint64, uint64)) {
	a.stats.onReport = fn
}

// hexEncode encodes bytes to a hex string.
func hexEncode(b []byte) string {
	const hex = "0123456789abcdef"
	r := make([]byte, len(b)*2)
	for i, v := range b {
		r[i*2] = hex[v>>4]
		r[i*2+1] = hex[v&0xf]
	}
	return string(r)
}

// hexDecode decodes a hex string to bytes.
func hexDecode(s string) []byte {
	if len(s)%2 != 0 {
		return nil
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		b[i/2] = (hexDigit(s[i]) << 4) | hexDigit(s[i+1])
	}
	return b
}

func hexDigit(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}
