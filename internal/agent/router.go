package agent

import (
	"errors"
	"log"
	"sync"
)

const (
	// QueueSize is the per-peer send buffer capacity (packets).
	// At MTU 1500, 64 packets = ~96KB buffer per peer.
	QueueSize = 64

	// MaxFrameSize is the maximum size of a routed frame.
	MaxRouteFrameSize = 22 + 1500 + 16 // header + MTU + AEAD tag
)

// ErrNoRoute is returned when no route to a destination exists.
var ErrNoRoute = errors.New("no route to destination")

// ErrQueueFull is returned when a peer's send queue is full (backpressure).
var ErrQueueFull = errors.New("peer send queue full, dropping packet")

// PacketRouter provides async, non-blocking packet forwarding with
// per-peer buffered queues, preventing a slow peer from blocking all traffic.
type PacketRouter struct {
	peers  *PeerManager
	routes *RouteManager

	queues map[uint]chan []byte // peerID → buffered send channel
	mu     sync.RWMutex
	wg     sync.WaitGroup
	quit   chan struct{}
	pool   sync.Pool
}

// NewPacketRouter creates a router with the given peer manager and route table.
func NewPacketRouter(peers *PeerManager, routes *RouteManager) *PacketRouter {
	return &PacketRouter{
		peers:  peers,
		routes: routes,
		queues: make(map[uint]chan []byte),
		quit:   make(chan struct{}),
		pool: sync.Pool{
			New: func() any {
				buf := make([]byte, 0, MaxRouteFrameSize)
				return &buf
			},
		},
	}
}

// Route looks up the destination in the FIB and queues the packet for delivery.
// Returns ErrNoRoute if no matching route is found.
// Returns ErrQueueFull if the destination peer's buffer is saturated.
func (r *PacketRouter) Route(dstIP string, frame []byte) error {
	_, nextHop := r.routes.Lookup(dstIP)
	if nextHop == 0 {
		return ErrNoRoute
	}
	return r.SendTo(nextHop, frame)
}

// SendTo queues a frame for delivery to a specific peer. Non-blocking.
func (r *PacketRouter) SendTo(peerID uint, frame []byte) error {
	ch := r.getOrCreateQueue(peerID)

	// Non-blocking send — drop if queue is full (tail-drop policy)
	select {
	case ch <- frame:
		return nil
	default:
		return ErrQueueFull
	}
}

// getOrCreateQueue returns the send channel for a peer, creating it if needed.
func (r *PacketRouter) getOrCreateQueue(peerID uint) chan []byte {
	r.mu.RLock()
	ch, ok := r.queues[peerID]
	r.mu.RUnlock()
	if ok {
		return ch
	}

	// Create new queue (double-checked locking)
	r.mu.Lock()
	defer r.mu.Unlock()
	if ch, ok := r.queues[peerID]; ok {
		return ch
	}

	ch = make(chan []byte, QueueSize)
	r.queues[peerID] = ch

	r.wg.Add(1)
	go r.sendLoop(peerID, ch)
	log.Printf("router: started send queue for peer %d (cap=%d)", peerID, QueueSize)

	return ch
}

// sendLoop drains a peer's send queue and sends frames over the wire.
func (r *PacketRouter) sendLoop(peerID uint, ch <-chan []byte) {
	defer r.wg.Done()
	for {
		select {
		case <-r.quit:
			return
		case frame, ok := <-ch:
			if !ok {
				return
			}
			if err := r.peers.SendRaw(peerID, frame); err != nil {
				log.Printf("router: send to peer %d failed: %v", peerID, err)
			}
		}
	}
}

// RemovePeer removes a peer's send queue and stops its sender goroutine.
// Called when a peer disconnects.
func (r *PacketRouter) RemovePeer(peerID uint) {
	r.mu.Lock()
	ch, ok := r.queues[peerID]
	if ok {
		close(ch)
		delete(r.queues, peerID)
	}
	r.mu.Unlock()
	if ok {
		log.Printf("router: removed peer %d send queue", peerID)
	}
}

// Stop shuts down all sender goroutines.
func (r *PacketRouter) Stop() {
	close(r.quit)

	r.mu.Lock()
	for id, ch := range r.queues {
		close(ch)
		delete(r.queues, id)
	}
	r.mu.Unlock()

	r.wg.Wait()
	log.Printf("router: all senders stopped")
}

// QueueLen returns the number of queued packets for a peer (for monitoring).
func (r *PacketRouter) QueueLen(peerID uint) int {
	r.mu.RLock()
	ch, ok := r.queues[peerID]
	r.mu.RUnlock()
	if !ok {
		return 0
	}
	return len(ch)
}
