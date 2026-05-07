package portmanager

import (
	"fmt"
	"sync"
)

const (
	DefaultMinPort int32 = 19132

	DefaultMaxPort int32 = 29132
)

type PortManager struct {
	mu        sync.Mutex
	minPort   int32
	maxPort   int32
	usedPorts map[int32]string
}

func NewPortManager(minPort, maxPort int32) *PortManager {
	return &PortManager{
		minPort:   minPort,
		maxPort:   maxPort,
		usedPorts: make(map[int32]string),
	}
}

func (pm *PortManager) Allocate(hostname string) (int32, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for port := pm.minPort; port <= pm.maxPort; port++ {
		if _, used := pm.usedPorts[port]; !used {
			pm.usedPorts[port] = hostname
			return port, nil
		}
	}
	return 0, fmt.Errorf("no hay puertos disponibles en el rango %d-%d", pm.minPort, pm.maxPort)
}

func (pm *PortManager) AllocateSpecific(port int32, hostname string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if port < pm.minPort || port > pm.maxPort {
		return fmt.Errorf("puerto %d fuera del rango permitido (%d-%d)", port, pm.minPort, pm.maxPort)
	}

	if existingHost, used := pm.usedPorts[port]; used {
		return fmt.Errorf("puerto %d ya asignado a %s", port, existingHost)
	}

	pm.usedPorts[port] = hostname
	return nil
}

func (pm *PortManager) Release(port int32) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.usedPorts, port)
}

func (pm *PortManager) IsUsed(port int32) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	_, used := pm.usedPorts[port]
	return used
}

func (pm *PortManager) UsedCount() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return len(pm.usedPorts)
}

func (pm *PortManager) AvailableCount() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	total := pm.maxPort - pm.minPort + 1
	return int(total) - len(pm.usedPorts)
}
