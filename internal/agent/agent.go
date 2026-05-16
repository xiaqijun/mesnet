package agent

import (
	"encoding/json"
	"log"
	"net"
	"sync"
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
	crypto *Crypto
	routes *RouteManager
	stats  *StatsCollector
	probe  *Probe

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

	a := &Agent{
		cfg:   cfg,
		quit:  make(chan struct{}),
		peers: NewPeerManager(listenAddr, backbone),
	}

	a.handler = NewHandler(a)
	a.crypto = NewCrypto()
	a.tun = NewTUNDevice()
	a.routes = NewRouteManager()
	a.stats = NewStatsCollector(a)
	a.probe = NewProbe(a.peers)

	return a
}

func (a *Agent) Start() error {
	// Only backbone nodes listen for incoming peer connections
	if a.cfg.Backbone {
		if port, err := a.peers.Listen(); err != nil {
			log.Printf("peer listen failed (non-fatal): %v", err)
		} else if port > 0 {
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
			return
		}

		dstIP := extractDstIP(plaintext)
		_, nextHop := a.routes.Lookup(dstIP)

		// Check if this IP belongs to one of our local subnets
		if isLocalIP(a.tun.IP(), dstIP) || a.isInOurSubnets(dstIP) {
			a.tun.Write(plaintext)
			return
		}

		// Relay: forward to next hop if not for us
		if nextHop != 0 && nextHop != nodeID {
			tun.SendEncrypted(nextHop, plaintext)
		}
	})

	// Connect to control plane
	a.ws = NewWSClient(a.cfg.ServerURL, a.handler)
	go a.ws.Connect()

	// Start stats collector
	go a.stats.Run(a.quit)

	// Start latency prober
	go a.probe.Run(a.quit)

	return nil
}

func (a *Agent) Stop() {
	close(a.quit)
	if a.ws != nil {
		a.ws.Close()
	}
	a.peers.Close()
	if a.tun != nil {
		a.tun.Destroy()
	}
}

func (a *Agent) HandleCommand(action string, args json.RawMessage) (any, error) {
	switch action {
	case "tun_setup":
		var params struct {
			IP string `json:"ip"`
		}
		json.Unmarshal(args, &params)
		return nil, a.tun.Create(params.IP)

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
			TunnelID  uint   `json:"tunnel_id"`
		}
		json.Unmarshal(args, &params)
		return nil, a.peers.Connect(params.NodeID, params.PeerAddr, params.PeerToken)

	case "peer_disconnect":
		var params struct {
			PeerID uint `json:"peer_id"`
		}
		json.Unmarshal(args, &params)
		a.peers.Disconnect(params.PeerID)
		return nil, nil

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
