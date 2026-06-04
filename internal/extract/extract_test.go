package extract

import (
	"testing"

	"github.com/anomalyco/plimsoll/internal/model"
)

// findWorkload returns the workload with the given name, or fails the test.
func findWorkload(t *testing.T, m model.ResourceModel, name string) model.Workload {
	t.Helper()
	for _, w := range m.Workloads {
		if w.Name == name {
			return w
		}
	}
	t.Fatalf("workload %q not found in %+v", name, m.Workloads)
	return model.Workload{}
}

// Task 4.1: controllers become workloads with per-container CPU/mem requests+limits.
func TestExtractWorkloadResources(t *testing.T) {
	manifests := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: 500m
              memory: 512Mi
            limits:
              cpu: "1"
              memory: 1Gi
        - name: sidecar
          resources:
            requests:
              cpu: 100m
              memory: 64Mi
`
	m, err := Extract(manifests)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	w := findWorkload(t, m, "web")
	if w.Kind != model.KindDeployment {
		t.Errorf("Kind = %q, want Deployment", w.Kind)
	}
	if len(w.Pod.Containers) != 2 {
		t.Fatalf("got %d containers, want 2", len(w.Pod.Containers))
	}
	app := w.Pod.Containers[0]
	if app.CPURequest != 500 || app.MemRequest != 512<<20 {
		t.Errorf("app requests = cpu %d mem %d, want 500 / %d", app.CPURequest, app.MemRequest, 512<<20)
	}
	if app.CPULimit != 1000 || app.MemLimit != 1<<30 {
		t.Errorf("app limits = cpu %d mem %d, want 1000 / %d", app.CPULimit, app.MemLimit, 1<<30)
	}
	side := w.Pod.Containers[1]
	if side.CPURequest != 100 || side.MemRequest != 64<<20 {
		t.Errorf("sidecar requests = cpu %d mem %d, want 100 / %d", side.CPURequest, side.MemRequest, 64<<20)
	}
	if side.CPULimit != 0 || side.MemLimit != 0 {
		t.Errorf("sidecar limits should be zero when unset, got cpu %d mem %d", side.CPULimit, side.MemLimit)
	}
}

func TestExtractAllControllerKinds(t *testing.T) {
	manifests := `
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: sts
spec:
  replicas: 2
  template:
    spec:
      containers:
        - name: c
          resources:
            requests:
              cpu: 250m
              memory: 128Mi
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: ds
spec:
  template:
    spec:
      containers:
        - name: c
          resources:
            requests:
              cpu: 100m
              memory: 64Mi
---
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: rs
spec:
  replicas: 4
  template:
    spec:
      containers:
        - name: c
          resources:
            requests:
              cpu: 50m
              memory: 32Mi
`
	m, err := Extract(manifests)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(m.Workloads) != 3 {
		t.Fatalf("got %d workloads, want 3", len(m.Workloads))
	}
	if findWorkload(t, m, "sts").Kind != model.KindStatefulSet {
		t.Error("sts kind wrong")
	}
	if findWorkload(t, m, "ds").Kind != model.KindDaemonSet {
		t.Error("ds kind wrong")
	}
	if findWorkload(t, m, "rs").Kind != model.KindReplicaSet {
		t.Error("rs kind wrong")
	}
}

// Task 4.2: PVCs and volumeClaimTemplates become volumes.
func TestExtractStandalonePVC(t *testing.T) {
	manifests := `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: cache
spec:
  accessModes: ["ReadWriteOnce"]
  storageClassName: fast
  resources:
    requests:
      storage: 20Gi
`
	m, err := Extract(manifests)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(m.Volumes) != 1 {
		t.Fatalf("got %d volumes, want 1", len(m.Volumes))
	}
	v := m.Volumes[0]
	if v.Name != "cache" || v.SizeBytes != 20<<30 || v.StorageClass != "fast" || v.AccessMode != "ReadWriteOnce" {
		t.Errorf("volume = %+v", v)
	}
}

func TestExtractVolumeClaimTemplate(t *testing.T) {
	manifests := `
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: db
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: db
          resources:
            requests:
              cpu: "1"
              memory: 1Gi
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: standard
        resources:
          requests:
            storage: 50Gi
`
	m, err := Extract(manifests)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(m.Volumes) != 1 {
		t.Fatalf("got %d volumes, want 1", len(m.Volumes))
	}
	v := m.Volumes[0]
	if v.SizeBytes != 50<<30 || v.StorageClass != "standard" || v.AccessMode != "ReadWriteOnce" {
		t.Errorf("volume = %+v", v)
	}
}

// Task 4.3: only LoadBalancer services produce load-balancer entries.
func TestExtractLoadBalancerServicesOnly(t *testing.T) {
	manifests := `
apiVersion: v1
kind: Service
metadata:
  name: public
spec:
  type: LoadBalancer
  ports:
    - port: 80
    - port: 443
---
apiVersion: v1
kind: Service
metadata:
  name: internal
spec:
  type: ClusterIP
  ports:
    - port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: nodeport
spec:
  type: NodePort
  ports:
    - port: 30000
`
	m, err := Extract(manifests)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(m.LoadBalancers) != 1 {
		t.Fatalf("got %d load balancers, want 1", len(m.LoadBalancers))
	}
	lb := m.LoadBalancers[0]
	if lb.Name != "public" {
		t.Errorf("lb name = %q, want public", lb.Name)
	}
	if len(lb.Ports) != 2 || lb.Ports[0] != 80 || lb.Ports[1] != 443 {
		t.Errorf("lb ports = %v, want [80 443]", lb.Ports)
	}
}

// Task 4.4: replica bounds from HPA, static replicas, DaemonSet handling.
func TestExtractHPABounds(t *testing.T) {
	manifests := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: c
          resources:
            requests:
              cpu: 100m
              memory: 64Mi
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: api
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api
  minReplicas: 2
  maxReplicas: 10
`
	m, err := Extract(manifests)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	w := findWorkload(t, m, "api")
	if w.Replicas.Min != 2 || w.Replicas.Max != 10 {
		t.Errorf("replica bounds = %+v, want {2,10}", w.Replicas)
	}
}

func TestExtractStaticReplicas(t *testing.T) {
	manifests := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: c
          resources:
            requests:
              cpu: 100m
              memory: 64Mi
`
	m, err := Extract(manifests)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	w := findWorkload(t, m, "api")
	if w.Replicas.Min != 3 || w.Replicas.Max != 3 {
		t.Errorf("replica bounds = %+v, want {3,3}", w.Replicas)
	}
}

func TestExtractDefaultReplicasWhenUnset(t *testing.T) {
	manifests := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  template:
    spec:
      containers:
        - name: c
          resources:
            requests:
              cpu: 100m
              memory: 64Mi
`
	m, err := Extract(manifests)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	w := findWorkload(t, m, "api")
	if w.Replicas.Min != 1 || w.Replicas.Max != 1 {
		t.Errorf("replica bounds = %+v, want {1,1} default", w.Replicas)
	}
}

func TestExtractDaemonSetReplicas(t *testing.T) {
	manifests := `
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: agent
spec:
  template:
    spec:
      containers:
        - name: c
          resources:
            requests:
              cpu: 100m
              memory: 64Mi
`
	m, err := Extract(manifests)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	w := findWorkload(t, m, "agent")
	if w.Kind != model.KindDaemonSet {
		t.Errorf("kind = %q, want DaemonSet", w.Kind)
	}
	// A DaemonSet runs one pod per node; extraction records a single template pod
	// and the packer expands it per node.
	if w.Replicas.Min != 1 || w.Replicas.Max != 1 {
		t.Errorf("daemonset bounds = %+v, want {1,1}", w.Replicas)
	}
}

func TestExtractIgnoresUnknownKinds(t *testing.T) {
	manifests := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
data:
  a: b
`
	m, err := Extract(manifests)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(m.Workloads) != 0 || len(m.Volumes) != 0 || len(m.LoadBalancers) != 0 {
		t.Errorf("expected empty model for unknown kinds, got %+v", m)
	}
}

func TestExtractInvalidYAMLErrors(t *testing.T) {
	_, err := Extract("this: is: not: valid: kubernetes: : :\n  - bad")
	if err == nil {
		t.Fatal("expected error for invalid manifest, got nil")
	}
}
