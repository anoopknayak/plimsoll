package model

import "testing"

// Task 2.1: assert the shape and zero-values of the core model types.

func TestResourceModelZeroValue(t *testing.T) {
	var m ResourceModel
	if m.Workloads != nil {
		t.Errorf("zero ResourceModel.Workloads = %v, want nil", m.Workloads)
	}
	if m.Volumes != nil {
		t.Errorf("zero ResourceModel.Volumes = %v, want nil", m.Volumes)
	}
	if m.LoadBalancers != nil {
		t.Errorf("zero ResourceModel.LoadBalancers = %v, want nil", m.LoadBalancers)
	}
}

func TestWorkloadZeroValue(t *testing.T) {
	var w Workload
	if w.Name != "" {
		t.Errorf("zero Workload.Name = %q, want empty", w.Name)
	}
	if w.Kind != "" {
		t.Errorf("zero Workload.Kind = %q, want empty", w.Kind)
	}
	if w.Replicas.Min != 0 || w.Replicas.Max != 0 {
		t.Errorf("zero Workload.Replicas = %+v, want {0,0}", w.Replicas)
	}
	if w.Pod.Containers != nil {
		t.Errorf("zero Workload.Pod.Containers = %v, want nil", w.Pod.Containers)
	}
}

func TestReplicaBoundsShape(t *testing.T) {
	b := ReplicaBounds{Min: 2, Max: 10}
	if b.Min != 2 || b.Max != 10 {
		t.Errorf("ReplicaBounds = %+v, want {2,10}", b)
	}
}

func TestContainerShape(t *testing.T) {
	c := Container{
		Name:       "api",
		CPURequest: 500,
		MemRequest: 512 << 20,
		CPULimit:   1000,
		MemLimit:   1 << 30,
	}
	if c.Name != "api" || c.CPURequest != 500 || c.MemRequest != 512<<20 {
		t.Errorf("Container request fields wrong: %+v", c)
	}
	if c.CPULimit != 1000 || c.MemLimit != 1<<30 {
		t.Errorf("Container limit fields wrong: %+v", c)
	}
}

func TestVolumeShape(t *testing.T) {
	v := Volume{Name: "data", SizeBytes: 50 << 30, StorageClass: "standard", AccessMode: "ReadWriteOnce"}
	if v.Name != "data" || v.SizeBytes != 50<<30 || v.StorageClass != "standard" || v.AccessMode != "ReadWriteOnce" {
		t.Errorf("Volume fields wrong: %+v", v)
	}
}

func TestLoadBalancerShape(t *testing.T) {
	lb := LoadBalancer{Name: "api", Ports: []int32{80, 443}}
	if lb.Name != "api" || len(lb.Ports) != 2 || lb.Ports[0] != 80 {
		t.Errorf("LoadBalancer fields wrong: %+v", lb)
	}
}

func TestWorkloadKindConstants(t *testing.T) {
	cases := map[WorkloadKind]string{
		KindDeployment:  "Deployment",
		KindStatefulSet: "StatefulSet",
		KindDaemonSet:   "DaemonSet",
		KindReplicaSet:  "ReplicaSet",
	}
	for k, want := range cases {
		if string(k) != want {
			t.Errorf("WorkloadKind = %q, want %q", k, want)
		}
	}
}
