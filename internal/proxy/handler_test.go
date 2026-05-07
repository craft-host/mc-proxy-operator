package proxy

import (
	"context"
	"sync"
	"testing"
)

type mockHandler struct {
	name        string
	defaultPort int32
	started     bool
	mu          sync.Mutex
}

func (m *mockHandler) Name() string                                 { return m.name }
func (m *mockHandler) DefaultPort() int32                           { return m.defaultPort }
func (m *mockHandler) Start(_ context.Context, _ *RouteTable) error { return nil }
func (m *mockHandler) Stop() error                                  { return nil }
func (m *mockHandler) AddRoute(_ string, _ *Backend, _ int32) error { return nil }
func (m *mockHandler) RemoveRoute(_ string, _ int32) error          { return nil }

func TestHandlerRegistry_RegisterAndGet(t *testing.T) {
	registry := NewHandlerRegistry()
	handler := &mockHandler{name: "test", defaultPort: 12345}

	registry.Register(handler)

	got, ok := registry.Get("test")
	if !ok {
		t.Fatal("expected to find handler 'test'")
	}
	if got.Name() != "test" {
		t.Fatalf("expected handler name 'test', got %s", got.Name())
	}

	_, ok = registry.Get("noexiste")
	if ok {
		t.Fatal("expected not to find handler 'noexiste'")
	}
}

func TestHandlerRegistry_ListEditions(t *testing.T) {
	registry := NewHandlerRegistry()
	registry.Register(&mockHandler{name: "java", defaultPort: 25565})
	registry.Register(&mockHandler{name: "bedrock", defaultPort: 19132})

	editions := registry.ListEditions()
	if len(editions) != 2 {
		t.Fatalf("expected 2 editions, got %d", len(editions))
	}

	found := map[string]bool{}
	for _, e := range editions {
		found[e] = true
	}
	if !found["java"] || !found["bedrock"] {
		t.Fatal("expected both 'java' and 'bedrock' in editions")
	}
}

func TestHandlerRegistry_RegisterOverwrites(t *testing.T) {
	registry := NewHandlerRegistry()
	registry.Register(&mockHandler{name: "java", defaultPort: 25565})
	registry.Register(&mockHandler{name: "java", defaultPort: 99999})

	got, ok := registry.Get("java")
	if !ok {
		t.Fatal("expected to find handler 'java'")
	}
	if got.DefaultPort() != 99999 {
		t.Fatalf("expected default port 99999 (overwritten), got %d", got.DefaultPort())
	}
}
