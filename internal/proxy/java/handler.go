package java

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/luisito666/mc-proxy-operator/internal/proxy"
)

const (
	DefaultListenAddr = ":25565"

	HandshakeTimeout = 10 * time.Second

	DialTimeout = 5 * time.Second
)

type JavaProtocolHandler struct {
	listenAddr string
	listener   net.Listener
	routeTable *proxy.RouteTable
	wg         sync.WaitGroup
}

func NewJavaProtocolHandler(listenAddr string) *JavaProtocolHandler {
	return &JavaProtocolHandler{
		listenAddr: listenAddr,
	}
}

func (h *JavaProtocolHandler) Name() string {
	return "java"
}

func (h *JavaProtocolHandler) DefaultPort() int32 {
	return 25565
}

func (h *JavaProtocolHandler) Start(ctx context.Context, routeTable *proxy.RouteTable) error {
	h.routeTable = routeTable
	logger := log.FromContext(ctx).WithValues("handler", "java")

	var err error
	h.listener, err = net.Listen("tcp", h.listenAddr)
	if err != nil {
		return err
	}
	logger.Info("Java handler escuchando", "addr", h.listenAddr)

	go func() {
		<-ctx.Done()
		h.listener.Close()
	}()

	for {
		conn, err := h.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			logger.Error(err, "error aceptando conexión")
			continue
		}
		h.wg.Add(1)
		go h.handleConnection(ctx, conn)
	}

	h.wg.Wait()
	logger.Info("Java handler detenido")
	return nil
}

func (h *JavaProtocolHandler) Stop() error {
	if h.listener != nil {
		return h.listener.Close()
	}
	return nil
}

func (h *JavaProtocolHandler) AddRoute(hostname string, backend *proxy.Backend, assignedPort int32) error {
	return nil
}

func (h *JavaProtocolHandler) RemoveRoute(hostname string, assignedPort int32) error {
	return nil
}

func (h *JavaProtocolHandler) handleConnection(ctx context.Context, clientConn net.Conn) {
	defer h.wg.Done()
	defer clientConn.Close()

	logger := log.FromContext(ctx).WithValues("handler", "java")
	remoteAddr := clientConn.RemoteAddr().String()

	clientConn.SetDeadline(time.Now().Add(HandshakeTimeout))

	handshake, rawBytes, err := ReadHandshake(clientConn)
	if err != nil {
		logger.V(1).Info("handshake inválido", "remote", remoteAddr, "error", err)
		return
	}

	logger.Info("handshake recibido",
		"remote", remoteAddr,
		"hostname", handshake.ServerAddress,
		"protocol", handshake.ProtocolVersion,
		"state", handshake.NextState,
	)

	backend, found := h.routeTable.GetHostnameRoute(handshake.ServerAddress)
	if !found {
		logger.Info("ruta no encontrada", "hostname", handshake.ServerAddress)
		return
	}

	if backend.MaxPlayers > 0 && backend.ActiveConnections.Load() >= backend.MaxPlayers {
		logger.Info("servidor lleno", "hostname", handshake.ServerAddress)
		return
	}

	backendAddr := backend.Address()
	backendConn, err := net.DialTimeout("tcp", backendAddr, DialTimeout)
	if err != nil {
		logger.Error(err, "error conectando al backend", "backend", backendAddr)
		return
	}
	defer backendConn.Close()

	backend.ActiveConnections.Add(1)
	defer backend.ActiveConnections.Add(-1)

	if _, err := backendConn.Write(rawBytes); err != nil {
		logger.Error(err, "error reenviando handshake")
		return
	}

	clientConn.SetDeadline(time.Time{})

	logger.Info("proxy establecido", "remote", remoteAddr, "hostname", handshake.ServerAddress, "backend", backendAddr)

	startTime := time.Now()
	errCh := make(chan error, 2)
	var c2b, b2c int64

	go func() {
		n, err := io.Copy(backendConn, clientConn)
		c2b = n
		if tc, ok := backendConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		errCh <- err
	}()

	go func() {
		n, err := io.Copy(clientConn, backendConn)
		b2c = n
		if tc, ok := clientConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		errCh <- err
	}()

	<-errCh
	<-errCh

	logger.Info("conexión cerrada",
		"remote", remoteAddr,
		"hostname", handshake.ServerAddress,
		"duration", time.Since(startTime).String(),
		"clientToBackend", c2b,
		"backendToClient", b2c,
	)
}
