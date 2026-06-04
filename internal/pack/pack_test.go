package pack

import "testing"

const gi = 1 << 30

func shape(name string, vcpu int, memGi int64, hourly float64) NodeShape {
	return NodeShape{Name: name, VCPU: vcpu, MemBytes: memGi * gi, Hourly: hourly}
}

func pod(name string, cpuMilli int64, memGi int64) Pod {
	return Pod{Name: name, CPUMilli: cpuMilli, MemBytes: memGi * gi}
}

// Task 6.1: FFD packing returns correct node counts and is deterministic.
func TestPackBasicNodeCount(t *testing.T) {
	req := Request{Pods: []Pod{
		pod("a", 1500, 4), pod("b", 1500, 4), pod("c", 1500, 4), pod("d", 1500, 4),
	}}
	res := Pack(req, shape("n", 4, 16, 0.10))
	if !res.Feasible {
		t.Fatal("expected feasible")
	}
	// Each node fits 2 pods (3000m / 8Gi); 4 pods => 2 nodes.
	if res.Nodes != 2 {
		t.Errorf("Nodes = %d, want 2", res.Nodes)
	}
}

func TestPackDeterministic(t *testing.T) {
	req := Request{Pods: []Pod{
		pod("a", 900, 1), pod("b", 1700, 3), pod("c", 500, 2), pod("d", 1200, 5), pod("e", 300, 1),
	}}
	s := shape("n", 4, 16, 0.10)
	first := Pack(req, s)
	for i := 0; i < 5; i++ {
		if got := Pack(req, s); got.Nodes != first.Nodes {
			t.Fatalf("non-deterministic: run %d gave %d, first gave %d", i, got.Nodes, first.Nodes)
		}
	}
}

// Task 6.2: overhead reservation reduces allocatable capacity and is configurable.
func TestPackOverheadReducesCapacity(t *testing.T) {
	req := Request{Pods: []Pod{pod("a", 1500, 2), pod("b", 1500, 2)}}
	s := shape("n", 4, 16, 0.10)

	noOverhead := Pack(req, s)
	if noOverhead.Nodes != 1 {
		t.Errorf("without overhead Nodes = %d, want 1", noOverhead.Nodes)
	}

	req.Overhead = Overhead{CPUMilli: 2000}
	withOverhead := Pack(req, s)
	if withOverhead.Nodes != 2 {
		t.Errorf("with 2000m overhead Nodes = %d, want 2", withOverhead.Nodes)
	}
}

func TestDefaultOverheadIsNonZero(t *testing.T) {
	o := DefaultOverhead()
	if o.CPUMilli <= 0 || o.MemBytes <= 0 {
		t.Errorf("DefaultOverhead = %+v, want positive reservations", o)
	}
}

// Task 6.3: a pod larger than any node makes the shape infeasible.
func TestPackInfeasiblePodTooLarge(t *testing.T) {
	req := Request{Pods: []Pod{pod("big", 3000, 2)}}
	res := Pack(req, shape("small", 2, 8, 0.10))
	if res.Feasible {
		t.Error("expected infeasible for oversized pod")
	}
}

func TestPackInfeasibleWhenOverheadExceedsCapacity(t *testing.T) {
	req := Request{
		Pods:     []Pod{pod("a", 500, 1)},
		Overhead: Overhead{CPUMilli: 5000},
	}
	res := Pack(req, shape("n", 4, 16, 0.10))
	if res.Feasible {
		t.Error("expected infeasible when overhead exceeds capacity")
	}
}

// Task 6.4: auto-select cheapest feasible shape; override restricts to one.
func TestSelectCheapestFeasible(t *testing.T) {
	req := Request{Pods: []Pod{pod("a", 1500, 2), pod("b", 1500, 2)}}
	shapes := []NodeShape{
		shape("small", 2, 8, 0.10),  // 1 pod/node => 2 nodes => 0.20
		shape("large", 4, 16, 0.18), // both fit => 1 node => 0.18
	}
	res, err := Select(req, shapes)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if res.Shape.Name != "large" {
		t.Errorf("selected %q, want large (cheapest total)", res.Shape.Name)
	}
	if res.Nodes != 1 {
		t.Errorf("Nodes = %d, want 1", res.Nodes)
	}
}

func TestSelectNoFeasibleShape(t *testing.T) {
	req := Request{Pods: []Pod{pod("huge", 9000, 2)}}
	_, err := Select(req, []NodeShape{shape("small", 2, 8, 0.10), shape("med", 4, 16, 0.18)})
	if err == nil {
		t.Fatal("expected error when no shape is feasible")
	}
}

func TestSelectShapeOverride(t *testing.T) {
	req := Request{Pods: []Pod{pod("a", 1500, 2), pod("b", 1500, 2)}}
	res, err := SelectShape(req, shape("small", 2, 8, 0.10))
	if err != nil {
		t.Fatalf("SelectShape: %v", err)
	}
	if res.Shape.Name != "small" || res.Nodes != 2 {
		t.Errorf("override result = %+v, want small with 2 nodes", res)
	}
}

// DaemonSet pods are reserved on every node and reduce allocatable per node.
func TestPackPerNodeDaemonSet(t *testing.T) {
	req := Request{
		Pods:    []Pod{pod("a", 1500, 2), pod("b", 1500, 2)},
		PerNode: []Pod{pod("agent", 1000, 1)},
	}
	// allocatable per node 4000m - 1000m(agent) = 3000m; both pods (3000m) fit one node.
	res := Pack(req, shape("n", 4, 16, 0.10))
	if !res.Feasible || res.Nodes != 1 {
		t.Errorf("result = %+v, want feasible 1 node", res)
	}
}
