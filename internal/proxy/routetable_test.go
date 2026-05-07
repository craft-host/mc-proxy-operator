package proxy

import (
	"sync"
	"testing"
)

func TestRouteTable_HostnameRoutes_SetGetRemove(t *testing.T) {
	rt := NewRouteTable()
	backend := &Backend{ServiceName: "svc1", ServicePort: 25565, Namespace: "default"}

	rt.SetHostnameRoute("jugador1.example.com", backend)

	got, ok := rt.GetHostnameRoute("jugador1.example.com")
	if !ok {
		t.Fatal("expected to find hostname route")
	}
	if got.ServiceName != "svc1" {
		t.Fatalf("expected ServiceName 'svc1', got %s", got.ServiceName)
	}

	_, ok = rt.GetHostnameRoute("noexiste.example.com")
	if ok {
		t.Fatal("expected not to find non-existent hostname route")
	}

	rt.RemoveHostnameRoute("jugador1.example.com")
	_, ok = rt.GetHostnameRoute("jugador1.example.com")
	if ok {
		t.Fatal("expected hostname route to be removed")
	}
}

func TestRouteTable_HostnameRoutes_CaseInsensitive(t *testing.T) {
	rt := NewRouteTable()
	backend := &Backend{ServiceName: "svc1", ServicePort: 25565, Namespace: "default"}

	rt.SetHostnameRoute("HOST.Example.COM", backend)

	_, ok := rt.GetHostnameRoute("host.example.com")
	if !ok {
		t.Fatal("expected case-insensitive hostname match")
	}
}

func TestRouteTable_PortRoutes_SetGetRemove(t *testing.T) {
	rt := NewRouteTable()
	backend := &Backend{ServiceName: "svc2", ServicePort: 19132, Namespace: "default"}

	rt.SetPortRoute(19132, backend)

	got, ok := rt.GetPortRoute(19132)
	if !ok {
		t.Fatal("expected to find port route")
	}
	if got.ServiceName != "svc2" {
		t.Fatalf("expected ServiceName 'svc2', got %s", got.ServiceName)
	}

	_, ok = rt.GetPortRoute(99999)
	if ok {
		t.Fatal("expected not to find non-existent port route")
	}

	rt.RemovePortRoute(19132)
	_, ok = rt.GetPortRoute(19132)
	if ok {
		t.Fatal("expected port route to be removed")
	}
}

func TestRouteTable_HostnameAndPortIndependent(t *testing.T) {
	rt := NewRouteTable()
	javaBackend := &Backend{ServiceName: "java-svc", ServicePort: 25565, Namespace: "ns1"}
	bedrockBackend := &Backend{ServiceName: "bedrock-svc", ServicePort: 19132, Namespace: "ns2"}

	rt.SetHostnameRoute("player1.example.com", javaBackend)
	rt.SetPortRoute(19132, bedrockBackend)

	if rt.HostnameCount() != 1 {
		t.Fatalf("expected 1 hostname route, got %d", rt.HostnameCount())
	}
	if rt.PortCount() != 1 {
		t.Fatalf("expected 1 port route, got %d", rt.PortCount())
	}

	gotJava, ok := rt.GetHostnameRoute("player1.example.com")
	if !ok || gotJava.ServiceName != "java-svc" {
		t.Fatal("expected java hostname route to exist")
	}

	gotBedrock, ok := rt.GetPortRoute(19132)
	if !ok || gotBedrock.ServiceName != "bedrock-svc" {
		t.Fatal("expected bedrock port route to exist")
	}
}

func TestRouteTable_IsPortInUse(t *testing.T) {
	rt := NewRouteTable()

	if rt.IsPortInUse(19132) {
		t.Fatal("expected port 19132 to not be in use")
	}

	rt.SetPortRoute(19132, &Backend{ServiceName: "svc", ServicePort: 19132, Namespace: "default"})

	if !rt.IsPortInUse(19132) {
		t.Fatal("expected port 19132 to be in use")
	}

	rt.RemovePortRoute(19132)

	if rt.IsPortInUse(19132) {
		t.Fatal("expected port 19132 to not be in use after removal")
	}
}

func TestRouteTable_ConcurrentAccess(t *testing.T) {
	rt := NewRouteTable()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(2)

		go func(idx int) {
			defer wg.Done()
			hostname := "player" + string(rune(idx))
			rt.SetHostnameRoute(hostname, &Backend{ServiceName: "svc", Namespace: "ns"})
			rt.GetHostnameRoute(hostname)
			rt.RemoveHostnameRoute(hostname)
		}(i)

		go func(idx int) {
			defer wg.Done()
			port := int32(19132 + idx)
			rt.SetPortRoute(port, &Backend{ServiceName: "svc", Namespace: "ns"})
			rt.GetPortRoute(port)
			rt.RemovePortRoute(port)
		}(i)
	}

	wg.Wait()
}
