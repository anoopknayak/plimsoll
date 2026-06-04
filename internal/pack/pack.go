// Package pack simulates scheduling pods onto candidate node shapes using a
// deterministic, overhead-aware first-fit-decreasing (FFD) algorithm, and
// selects the cheapest feasible shape per cloud.
package pack

import (
	"fmt"
	"sort"
)

// NodeShape is a candidate machine type with its capacity and hourly price.
type NodeShape struct {
	Name     string
	VCPU     int
	MemBytes int64
	Hourly   float64
}

// allocatableCPUMilli returns the node's total schedulable CPU in millicores.
func (s NodeShape) allocatableCPUMilli() int64 { return int64(s.VCPU) * 1000 }

// Pod is a unit to schedule, described by its aggregate CPU and memory requests.
type Pod struct {
	Name     string
	CPUMilli int64
	MemBytes int64
}

// Overhead is the per-node capacity reserved for the kubelet, system daemons,
// and OS (kube-reserved plus a system-DaemonSet allowance).
type Overhead struct {
	CPUMilli int64
	MemBytes int64
}

// DefaultOverhead returns documented default reservations: roughly the capacity
// a managed node loses to kube-reserved and standard system DaemonSets.
func DefaultOverhead() Overhead {
	return Overhead{
		CPUMilli: 250,             // 250m kube/system reserved
		MemBytes: 768 * (1 << 20), // 768Mi reserved
	}
}

// Request describes everything to be packed onto a single shape.
type Request struct {
	// Pods are the regular pods to place.
	Pods []Pod
	// PerNode are pods that must run on every node (e.g. DaemonSets); they reduce
	// each node's allocatable capacity.
	PerNode []Pod
	// Overhead is the per-node reservation; zero value means no reservation.
	Overhead Overhead
}

// Result reports the outcome of packing onto a shape.
type Result struct {
	Shape    NodeShape
	Nodes    int
	Feasible bool
}

// Cost is the configuration's monthly-agnostic comparison cost: nodes × hourly.
func (r Result) Cost() float64 { return float64(r.Nodes) * r.Shape.Hourly }

type bin struct {
	cpu int64
	mem int64
}

// Pack runs FFD for a single node shape and returns the node count required.
func Pack(req Request, shape NodeShape) Result {
	allocCPU := shape.allocatableCPUMilli() - req.Overhead.CPUMilli
	allocMem := shape.MemBytes - req.Overhead.MemBytes

	// PerNode pods are reserved on every node before regular pods are placed.
	for _, p := range req.PerNode {
		allocCPU -= p.CPUMilli
		allocMem -= p.MemBytes
	}
	if allocCPU < 0 || allocMem < 0 {
		return Result{Shape: shape, Feasible: false}
	}

	pods := make([]Pod, len(req.Pods))
	copy(pods, req.Pods)
	sortPodsDescending(pods)

	var bins []bin
	for _, p := range pods {
		if p.CPUMilli > allocCPU || p.MemBytes > allocMem {
			// A single pod cannot fit even an empty node of this shape.
			return Result{Shape: shape, Feasible: false}
		}
		placed := false
		for i := range bins {
			if bins[i].cpu+p.CPUMilli <= allocCPU && bins[i].mem+p.MemBytes <= allocMem {
				bins[i].cpu += p.CPUMilli
				bins[i].mem += p.MemBytes
				placed = true
				break
			}
		}
		if !placed {
			bins = append(bins, bin{cpu: p.CPUMilli, mem: p.MemBytes})
		}
	}

	nodes := len(bins)
	// If only PerNode pods exist (no regular pods), at least one node is needed.
	if nodes == 0 && len(req.PerNode) > 0 {
		nodes = 1
	}
	return Result{Shape: shape, Nodes: nodes, Feasible: true}
}

// sortPodsDescending orders pods largest-first with a deterministic tie-break on
// name, so packing is repeatable.
func sortPodsDescending(pods []Pod) {
	sort.SliceStable(pods, func(i, j int) bool {
		a, b := pods[i], pods[j]
		if a.CPUMilli != b.CPUMilli {
			return a.CPUMilli > b.CPUMilli
		}
		if a.MemBytes != b.MemBytes {
			return a.MemBytes > b.MemBytes
		}
		return a.Name < b.Name
	})
}

// Select evaluates all candidate shapes and returns the cheapest feasible
// configuration (lowest nodes × hourly). Ties break on shape name.
func Select(req Request, shapes []NodeShape) (Result, error) {
	var best Result
	found := false
	for _, s := range shapes {
		res := Pack(req, s)
		if !res.Feasible {
			continue
		}
		if !found || res.Cost() < best.Cost() ||
			(res.Cost() == best.Cost() && res.Shape.Name < best.Shape.Name) {
			best = res
			found = true
		}
	}
	if !found {
		return Result{}, fmt.Errorf("no candidate node shape can host the workloads")
	}
	return best, nil
}

// SelectShape packs onto exactly one shape (an explicit --machine override).
func SelectShape(req Request, shape NodeShape) (Result, error) {
	res := Pack(req, shape)
	if !res.Feasible {
		return Result{}, fmt.Errorf("node shape %q cannot host the workloads", shape.Name)
	}
	return res, nil
}
