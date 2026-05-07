package proxy

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

type Backend struct {
	ServiceName string

	ServicePort int32

	Namespace string

	MaxPlayers int32

	Edition string

	AssignedPort int32

	Hostname string

	ActiveConnections atomic.Int32
}

func (b *Backend) Address() string {
	return fmt.Sprintf("%s.%s.svc.cluster.local:%d", b.ServiceName, b.Namespace, b.ServicePort)
}

type RouteTable struct {
	mu sync.RWMutex

	hostnameRoutes map[string]*Backend

	portRoutes map[int32]*Backend
}

func NewRouteTable() *RouteTable {
	return &RouteTable{
		hostnameRoutes: make(map[string]*Backend),
		portRoutes:     make(map[int32]*Backend),
	}
}

func (rt *RouteTable) SetHostnameRoute(hostname string, backend *Backend) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.hostnameRoutes[strings.ToLower(hostname)] = backend
}

func (rt *RouteTable) GetHostnameRoute(hostname string) (*Backend, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	backend, ok := rt.hostnameRoutes[strings.ToLower(hostname)]
	return backend, ok
}

func (rt *RouteTable) RemoveHostnameRoute(hostname string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.hostnameRoutes, strings.ToLower(hostname))
}

func (rt *RouteTable) SetPortRoute(port int32, backend *Backend) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.portRoutes[port] = backend
}

func (rt *RouteTable) GetPortRoute(port int32) (*Backend, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	backend, ok := rt.portRoutes[port]
	return backend, ok
}

func (rt *RouteTable) RemovePortRoute(port int32) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.portRoutes, port)
}

func (rt *RouteTable) HostnameCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.hostnameRoutes)
}

func (rt *RouteTable) PortCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.portRoutes)
}

func (rt *RouteTable) ListHostnames() []string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	hostnames := make([]string, 0, len(rt.hostnameRoutes))
	for h := range rt.hostnameRoutes {
		hostnames = append(hostnames, h)
	}
	return hostnames
}

func (rt *RouteTable) ListPorts() []int32 {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	ports := make([]int32, 0, len(rt.portRoutes))
	for p := range rt.portRoutes {
		ports = append(ports, p)
	}
	return ports
}

func (rt *RouteTable) IsPortInUse(port int32) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	_, ok := rt.portRoutes[port]
	return ok
}
