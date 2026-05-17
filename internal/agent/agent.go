package agent

import (
	"encoding/json"
	"log"
	"net"
	"sync"
)

type Config struct {
	Name       string
	ServerURL  string
	ListenAddr string
	Backbone   bool
	Token      string
}

type Agent struct {
	cfg    Config
	ws     *WSClient
	peers  *PeerManager
	tun    *TUNDevice
	peerPort int
	crypto *Crypto
	routes *RouteManager
	stats  *StatsCollector
	probe  *Probe

	handler     *Handler
	onRecvRelay func([]byte)

	mu   sync.Mutex
	quit chan struct{}
}

func New(name, serverURL, listenAddr string, backbone bool) *Agent {
	token := extractToken(serverURL)
	cfg := Config{Name: name, ServerURL: serverURL, ListenAddr: listenAddr, Backbone: backbone, Token: token}
	a := &Agent{cfg: cfg, quit: make(chan struct{}), peers: NewPeerManager(listenAddr, backbone)}
	a.handler = NewHandler(a)
	a.crypto = NewCrypto()
	a.tun = NewTUNDevice()
	a.routes = NewRouteManager()
	a.stats = NewStatsCollector(a)
	a.probe = NewProbe(a.peers)
	return a
}

func (a *Agent) Start() error {
	if a.cfg.Backbone {
		if port, err := a.peers.Listen(); err == nil && port > 0 {
			a.peerPort = port
			log.Printf("backbone node: listening on :%d", port)
		} else {
			log.Printf("peer listen: port=%d err=%v (non-fatal, using relay)", port, err)
		}
	} else {
		log.Printf("leaf node: relay-only mode")
	}

	// Handle data relayed from other agents via control plane
	a.onRecvRelay = func(frame []byte) {
		tun := NewTunnel(a)
		plaintext, err := tun.ReceiveEncrypted(0, frame)
		if err != nil {
			return
		}
		dstIP := extractDstIP(plaintext)
		_, nextHop := a.routes.Lookup(dstIP)
		if isLocalIP(a.tun.IP(), dstIP) || a.isInOurSubnets(dstIP) {
			a.tun.Write(plaintext)
			return
		}
		if nextHop != 0 {
			tun.SendEncrypted(nextHop, plaintext)
		}
	}

	a.ws = NewWSClient(a.cfg.ServerURL, a.handler, a.peerPort)
	go a.ws.Connect()
	go a.stats.Run(a.quit)
	go a.probe.Run(a.quit)

	tunnel := NewTunnel(a)
	go tunnel.Run()

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
		var params struct{ IP string `json:"ip"` }
		json.Unmarshal(args, &params)
		return nil, a.tun.Create(params.IP)

	case "tun_destroy":
		return nil, a.tun.Destroy()

	case "route_add":
		var params struct {
			Subnet  string `json:"subnet"`
			NodeID  uint   `json:"node_id"`
			NextHop uint   `json:"next_hop"`
		}
		json.Unmarshal(args, &params)
		return nil, a.routes.Add(params.Subnet, params.NodeID, params.NextHop)
	case "route_del":
		var params struct{ Subnet string `json:"subnet"` }
		json.Unmarshal(args, &params)
		return nil, a.routes.Del(params.Subnet)

	case "peer_connect", "peer_disconnect":
		return nil, nil

	case "subnet_detect":
		subnets, err := a.routes.DetectSubnets()
		return map[string]any{"subnets": subnets}, err

	case "diagnose":
		return a.Diagnose(), nil

	case "status":
		return a.CollectStatus(), nil

	case "agent_update":
		log.Printf("self-update triggered")
		if err := SelfUpdate(); err != nil {
			return nil, err
		}
		return map[string]string{"status": "restarting"}, nil
	}
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
		"tun_ip": a.tun.IP(), "tun_up": a.tun.IsUp(),
		"backbone": a.cfg.Backbone, "peers": len(a.peers.ListPeers()),
		"routes": a.routes.List(), "subnets": a.getSubnets(),
	}
}

func (a *Agent) getSubnets() []string           { return a.routes.getSubnets() }
func (a *Agent) getMyNodeID() uint              { return 0 }

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
