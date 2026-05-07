package bedrock

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/luisito666/mc-proxy-operator/internal/proxy"
)

const (
	UDPBufferSize = 2048

	SessionTimeout = 60 * time.Second

	SessionCleanupInterval = 30 * time.Second
)

type udpSession struct {
	clientAddr   *net.UDPAddr
	backendConn  *net.UDPConn
	lastActivity time.Time
	backend      *proxy.Backend
}

type bedrockListener struct {
	port     int32
	conn     *net.UDPConn
	hostname string
	backend  *proxy.Backend
	sessions map[string]*udpSession
	mu       sync.RWMutex
	cancel   context.CancelFunc
}

type BedrockProtocolHandler struct {
	mu         sync.RWMutex
	listeners  map[int32]*bedrockListener
	routeTable *proxy.RouteTable
}

func NewBedrockProtocolHandler() *BedrockProtocolHandler {
	return &BedrockProtocolHandler{
		listeners: make(map[int32]*bedrockListener),
	}
}

func (h *BedrockProtocolHandler) Name() string {
	return "bedrock"
}

func (h *BedrockProtocolHandler) DefaultPort() int32 {
	return 19132
}

func (h *BedrockProtocolHandler) Start(ctx context.Context, routeTable *proxy.RouteTable) error {
	h.routeTable = routeTable
	logger := log.FromContext(ctx).WithValues("handler", "bedrock")
	logger.Info("Bedrock handler iniciado (listeners se crean dinámicamente)")

	go h.cleanupLoop(ctx)

	<-ctx.Done()
	logger.Info("Bedrock handler detenido")
	return nil
}

func (h *BedrockProtocolHandler) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	for port, listener := range h.listeners {
		listener.cancel()
		listener.conn.Close()
		delete(h.listeners, port)
	}
	return nil
}

func (h *BedrockProtocolHandler) AddRoute(hostname string, backend *proxy.Backend, assignedPort int32) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.listeners[assignedPort]; exists {
		h.listeners[assignedPort].backend = backend
		h.listeners[assignedPort].hostname = hostname
		return nil
	}

	addr := &net.UDPAddr{Port: int(assignedPort)}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("error abriendo UDP en puerto %d: %w", assignedPort, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	listener := &bedrockListener{
		port:     assignedPort,
		conn:     conn,
		hostname: hostname,
		backend:  backend,
		sessions: make(map[string]*udpSession),
		cancel:   cancel,
	}

	h.listeners[assignedPort] = listener

	go h.readLoop(ctx, listener)

	return nil
}

func (h *BedrockProtocolHandler) RemoveRoute(hostname string, assignedPort int32) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	listener, exists := h.listeners[assignedPort]
	if !exists {
		return nil
	}

	listener.cancel()
	listener.conn.Close()

	listener.mu.Lock()
	for _, session := range listener.sessions {
		session.backendConn.Close()
		session.backend.ActiveConnections.Add(-1)
	}
	listener.mu.Unlock()

	delete(h.listeners, assignedPort)
	return nil
}

func (h *BedrockProtocolHandler) readLoop(ctx context.Context, listener *bedrockListener) {
	buf := make([]byte, UDPBufferSize)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		listener.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, clientAddr, err := listener.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			continue
		}

		data := make([]byte, n)
		copy(data, buf[:n])

		session, err := h.getOrCreateSession(listener, clientAddr)
		if err != nil {
			continue
		}

		session.lastActivity = time.Now()

		session.backendConn.Write(data)
	}
}

func (h *BedrockProtocolHandler) getOrCreateSession(listener *bedrockListener, clientAddr *net.UDPAddr) (*udpSession, error) {
	key := clientAddr.String()

	listener.mu.RLock()
	session, exists := listener.sessions[key]
	listener.mu.RUnlock()

	if exists {
		return session, nil
	}

	backend := listener.backend
	if backend.MaxPlayers > 0 && backend.ActiveConnections.Load() >= backend.MaxPlayers {
		return nil, fmt.Errorf("servidor lleno")
	}

	backendAddr, err := net.ResolveUDPAddr("udp", backend.Address())
	if err != nil {
		return nil, err
	}
	backendConn, err := net.DialUDP("udp", nil, backendAddr)
	if err != nil {
		return nil, err
	}

	session = &udpSession{
		clientAddr:   clientAddr,
		backendConn:  backendConn,
		lastActivity: time.Now(),
		backend:      backend,
	}

	backend.ActiveConnections.Add(1)

	listener.mu.Lock()
	listener.sessions[key] = session
	listener.mu.Unlock()

	go h.backendReadLoop(listener, session)

	return session, nil
}

func (h *BedrockProtocolHandler) backendReadLoop(listener *bedrockListener, session *udpSession) {
	buf := make([]byte, UDPBufferSize)

	for {
		session.backendConn.SetReadDeadline(time.Now().Add(SessionTimeout))
		n, err := session.backendConn.Read(buf)
		if err != nil {
			break
		}

		session.lastActivity = time.Now()
		listener.conn.WriteToUDP(buf[:n], session.clientAddr)
	}

	listener.mu.Lock()
	delete(listener.sessions, session.clientAddr.String())
	listener.mu.Unlock()

	session.backend.ActiveConnections.Add(-1)
	session.backendConn.Close()
}

func (h *BedrockProtocolHandler) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(SessionCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.cleanupExpiredSessions()
		}
	}
}

func (h *BedrockProtocolHandler) cleanupExpiredSessions() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	now := time.Now()
	for _, listener := range h.listeners {
		listener.mu.Lock()
		for key, session := range listener.sessions {
			if now.Sub(session.lastActivity) > SessionTimeout {
				session.backendConn.Close()
				session.backend.ActiveConnections.Add(-1)
				delete(listener.sessions, key)
			}
		}
		listener.mu.Unlock()
	}
}
