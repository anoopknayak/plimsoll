// Package model defines the cloud-neutral resource model that is the central
// contract of plimsoll. Extraction produces a ResourceModel; packing, pricing
// and estimation consume it. Keeping it provider-agnostic lets the costing
// logic be written once and parameterized by cloud.
package model

// WorkloadKind enumerates the Kubernetes controller kinds plimsoll understands.
type WorkloadKind string

const (
	KindDeployment  WorkloadKind = "Deployment"
	KindStatefulSet WorkloadKind = "StatefulSet"
	KindDaemonSet   WorkloadKind = "DaemonSet"
	KindReplicaSet  WorkloadKind = "ReplicaSet"
)

// ResourceModel is the provider-agnostic description of everything in a chart
// that drives cost: workloads (compute), volumes (storage) and LoadBalancer
// services (networking).
type ResourceModel struct {
	Workloads     []Workload
	Volumes       []Volume
	LoadBalancers []LoadBalancer
}

// Workload is a single controller (Deployment/StatefulSet/etc.) with its replica
// bounds and the pod template that each replica instantiates.
type Workload struct {
	Name     string
	Kind     WorkloadKind
	Replicas ReplicaBounds
	Pod      PodSpec
}

// ReplicaBounds captures the minimum and maximum number of replicas a workload
// may run at. With an HPA these come from min/maxReplicas; otherwise both equal
// the static replica count.
type ReplicaBounds struct {
	Min int
	Max int
}

// PodSpec is a pod template: the set of containers scheduled together as a unit.
type PodSpec struct {
	Containers []Container
}

// Container holds a single container's resource requests and limits. CPU is in
// millicores; memory is in bytes. A zero value means "unset".
type Container struct {
	Name       string
	CPURequest int64 // millicores
	MemRequest int64 // bytes
	CPULimit   int64 // millicores
	MemLimit   int64 // bytes
}

// Volume is a persistent volume claim (or StatefulSet volumeClaimTemplate).
type Volume struct {
	Name         string
	SizeBytes    int64
	StorageClass string
	AccessMode   string
}

// LoadBalancer represents a Service of type LoadBalancer.
type LoadBalancer struct {
	Name  string
	Ports []int32
}
