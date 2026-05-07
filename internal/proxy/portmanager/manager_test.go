package portmanager

import (
	"sync"
	"testing"
)

func TestPortManager_Allocate(t *testing.T) {
	pm := NewPortManager(19132, 19135)

	p1, err := pm.Allocate("host1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p1 != 19132 {
		t.Fatalf("expected port 19132, got %d", p1)
	}

	p2, err := pm.Allocate("host2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p2 != 19133 {
		t.Fatalf("expected port 19133, got %d", p2)
	}

	p3, _ := pm.Allocate("host3")
	p4, _ := pm.Allocate("host4")
	if p3 != 19134 || p4 != 19135 {
		t.Fatalf("expected ports 19134 and 19135, got %d and %d", p3, p4)
	}

	_, err = pm.Allocate("host5")
	if err == nil {
		t.Fatal("expected error when no ports available")
	}
}

func TestPortManager_AllocateSpecific(t *testing.T) {
	pm := NewPortManager(19132, 19135)

	err := pm.AllocateSpecific(19133, "host1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = pm.AllocateSpecific(19133, "host2")
	if err == nil {
		t.Fatal("expected error when port already allocated")
	}

	err = pm.AllocateSpecific(99999, "host3")
	if err == nil {
		t.Fatal("expected error when port out of range")
	}
}

func TestPortManager_Release(t *testing.T) {
	pm := NewPortManager(19132, 19135)

	port, _ := pm.Allocate("host1")
	if !pm.IsUsed(port) {
		t.Fatal("expected port to be in use")
	}

	pm.Release(port)
	if pm.IsUsed(port) {
		t.Fatal("expected port to be released")
	}
}

func TestPortManager_AllocateAfterRelease(t *testing.T) {
	pm := NewPortManager(19132, 19134)

	pm.Allocate("host1")
	pm.Allocate("host2")
	pm.Allocate("host3")

	_, err := pm.Allocate("host4")
	if err == nil {
		t.Fatal("expected no ports available")
	}

	pm.Release(19133)

	port, err := pm.Allocate("host4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 19133 {
		t.Fatalf("expected released port 19133, got %d", port)
	}
}

func TestPortManager_ConcurrentAllocations(t *testing.T) {
	pm := NewPortManager(19132, 19232)
	var wg sync.WaitGroup
	ports := make(chan int32, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p, err := pm.Allocate("host" + string(rune(idx)))
			if err == nil {
				ports <- p
			}
		}(i)
	}

	wg.Wait()
	close(ports)

	seen := map[int32]bool{}
	for p := range ports {
		if seen[p] {
			t.Fatalf("duplicate port allocated: %d", p)
		}
		seen[p] = true
	}

	if pm.UsedCount() != len(seen) {
		t.Fatalf("expected %d used ports, got %d", len(seen), pm.UsedCount())
	}
}
