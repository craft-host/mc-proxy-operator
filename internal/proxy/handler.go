package proxy

import (
	"context"
	"fmt"
	"sync"
)

type ProtocolHandler interface {
	Name() string
	DefaultPort() int32
	Start(ctx context.Context, routeTable *RouteTable) error
	Stop() error
	AddRoute(hostname string, backend *Backend, assignedPort int32) error
	RemoveRoute(hostname string, assignedPort int32) error
}

type HandlerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]ProtocolHandler
}

func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]ProtocolHandler),
	}
}

func (r *HandlerRegistry) Register(handler ProtocolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[handler.Name()] = handler
}

func (r *HandlerRegistry) Get(edition string) (ProtocolHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[edition]
	return h, ok
}

func (r *HandlerRegistry) ListEditions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	editions := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		editions = append(editions, name)
	}
	return editions
}

func (r *HandlerRegistry) StartAll(ctx context.Context, routeTable *RouteTable) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, handler := range r.handlers {
		h := handler
		go func() {
			if err := h.Start(ctx, routeTable); err != nil {
				fmt.Printf("handler %s error: %v\n", h.Name(), err)
			}
		}()
	}

	return nil
}

func (r *HandlerRegistry) StopAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var lastErr error
	for _, handler := range r.handlers {
		if err := handler.Stop(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
