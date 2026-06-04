package model

import "testing"

func samplePod() PodSpec {
	return PodSpec{Containers: []Container{
		{Name: "app", CPURequest: 500, MemRequest: 512 << 20, CPULimit: 1000, MemLimit: 1 << 30},
		{Name: "sidecar", CPURequest: 100, MemRequest: 128 << 20, CPULimit: 200, MemLimit: 256 << 20},
	}}
}

func TestPodSpecTotals(t *testing.T) {
	p := samplePod()
	if got, want := p.TotalCPURequest(), int64(600); got != want {
		t.Errorf("TotalCPURequest = %d, want %d", got, want)
	}
	if got, want := p.TotalMemRequest(), int64(512<<20+128<<20); got != want {
		t.Errorf("TotalMemRequest = %d, want %d", got, want)
	}
	if got, want := p.TotalCPULimit(), int64(1200); got != want {
		t.Errorf("TotalCPULimit = %d, want %d", got, want)
	}
	if got, want := p.TotalMemLimit(), int64(1<<30+256<<20); got != want {
		t.Errorf("TotalMemLimit = %d, want %d", got, want)
	}
}

func TestPodSpecTotalsEmpty(t *testing.T) {
	var p PodSpec
	if p.TotalCPURequest() != 0 || p.TotalMemRequest() != 0 {
		t.Errorf("empty pod totals should be zero, got cpu=%d mem=%d", p.TotalCPURequest(), p.TotalMemRequest())
	}
}

func TestReplicaBoundsIsStatic(t *testing.T) {
	if !(ReplicaBounds{Min: 3, Max: 3}).IsStatic() {
		t.Error("equal bounds should be static")
	}
	if (ReplicaBounds{Min: 2, Max: 10}).IsStatic() {
		t.Error("differing bounds should not be static")
	}
}
