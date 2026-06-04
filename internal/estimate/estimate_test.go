package estimate

import (
	"os"
	"strings"
	"testing"

	"github.com/anomalyco/plimsoll/internal/model"
	"github.com/anomalyco/plimsoll/internal/pricing"
)

// loadCatalog loads the controlled non-linear fixture catalog under testdata.
func loadCatalog(t *testing.T) *pricing.Catalog {
	t.Helper()
	cat, err := pricing.LoadFS(os.DirFS("testdata/pricing"), ".")
	if err != nil {
		t.Fatalf("LoadFS: %v", err)
	}
	return cat
}

// r1Region pins the fixture region for gcp so tests don't depend on the
// production default region.
var r1Region = map[string]string{"gcp": "r1"}

// gcpDiskMapping maps storageClasses onto the fixture's disk names so the
// default (production) mapping doesn't interfere with deterministic tests.
func gcpDiskMapping(byClass map[string]string, def string) map[string]DiskMapping {
	return map[string]DiskMapping{"gcp": {ByClass: byClass, Default: def}}
}

func approx(t *testing.T, label string, got, want float64) {
	t.Helper()
	if d := got - want; d > 1e-6 || d < -1e-6 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

// deployment builds a single Deployment workload with the given replica bounds
// and a one-container pod requesting cpuMilli/memBytes.
func deployment(name string, min, max int, cpuMilli, memBytes int64) model.Workload {
	return model.Workload{
		Name:     name,
		Kind:     model.KindDeployment,
		Replicas: model.ReplicaBounds{Min: min, Max: max},
		Pod: model.PodSpec{Containers: []model.Container{
			{Name: name, CPURequest: cpuMilli, MemRequest: memBytes},
		}},
	}
}

// Task 7.1: per-cloud total = compute + storage + LB + control plane, all
// normalized to a monthly figure (730h). Static replicas → min == max.
func TestEstimate_AggregateAndNormalization(t *testing.T) {
	cat := loadCatalog(t)
	m := model.ResourceModel{
		Workloads:     []model.Workload{deployment("api", 2, 2, 1000, 2*gib)},
		Volumes:       []model.Volume{{Name: "data", SizeBytes: 10 * gib, StorageClass: "standard"}},
		LoadBalancers: []model.LoadBalancer{{Name: "api"}},
	}
	opts := Options{Regions: r1Region, DiskMappings: gcpDiskMapping(map[string]string{"standard": "standard"}, "standard")}

	res, err := Estimate(m, cat, opts)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if len(res.Clouds) != 1 {
		t.Fatalf("clouds = %d, want 1", len(res.Clouds))
	}
	ce := res.Clouds[0]

	// Two 1000m/2Gi pods both fit a single 2vCPU/8Gi "small" node (cheaper than large).
	if ce.NodeShape != "small" {
		t.Errorf("NodeShape = %q, want small", ce.NodeShape)
	}
	if ce.NodeCountMin != 1 || ce.NodeCountMax != 1 {
		t.Errorf("NodeCount min/max = %d/%d, want 1/1", ce.NodeCountMin, ce.NodeCountMax)
	}

	// compute = 1 node × $0.10/h × 730h = 73.00
	approx(t, "Max.Compute", ce.Max.Compute, 73.00)
	// storage = 10 GiB × $0.10 = 1.00
	approx(t, "Max.Storage", ce.Max.Storage, 1.00)
	approx(t, "Max.LoadBalancer", ce.Max.LoadBalancer, 18.00)
	approx(t, "Max.ControlPlane", ce.Max.ControlPlane, 73.00)
	approx(t, "Max.Total", ce.Max.Total(), 165.00)

	// Static replicas → min == max.
	approx(t, "Min.Total", ce.Min.Total(), ce.Max.Total())
	if ce.SnapshotDate != "2026-06-01" {
		t.Errorf("SnapshotDate = %q, want 2026-06-01", ce.SnapshotDate)
	}
	if res.Spot {
		t.Error("Spot = true, want false")
	}
}

// Task 7.2: min uses min replicas, max uses max replicas (a real range), and a
// static workload collapses the range.
func TestEstimate_ReplicaRange(t *testing.T) {
	cat := loadCatalog(t)

	t.Run("range", func(t *testing.T) {
		// 1500m pods: two won't share a small node (2000m alloc), so max packs 2 nodes.
		m := model.ResourceModel{Workloads: []model.Workload{deployment("api", 1, 2, 1500, 2*gib)}}
		res, err := Estimate(m, cat, Options{Regions: r1Region})
		if err != nil {
			t.Fatalf("Estimate: %v", err)
		}
		ce := res.Clouds[0]
		if ce.NodeShape != "small" {
			t.Fatalf("NodeShape = %q, want small", ce.NodeShape)
		}
		if ce.NodeCountMin != 1 || ce.NodeCountMax != 2 {
			t.Errorf("NodeCount min/max = %d/%d, want 1/2", ce.NodeCountMin, ce.NodeCountMax)
		}
		// min: 1 node compute (73) + control plane (73) = 146
		approx(t, "Min.Total", ce.Min.Total(), 146.00)
		// max: 2 nodes compute (146) + control plane (73) = 219
		approx(t, "Max.Total", ce.Max.Total(), 219.00)
	})

	t.Run("collapsed", func(t *testing.T) {
		m := model.ResourceModel{Workloads: []model.Workload{deployment("api", 2, 2, 1500, 2*gib)}}
		res, err := Estimate(m, cat, Options{Regions: r1Region})
		if err != nil {
			t.Fatalf("Estimate: %v", err)
		}
		ce := res.Clouds[0]
		if ce.NodeCountMin != ce.NodeCountMax {
			t.Errorf("static range not collapsed: min=%d max=%d", ce.NodeCountMin, ce.NodeCountMax)
		}
		approx(t, "collapsed Min==Max", ce.Min.Total(), ce.Max.Total())
	})
}

// Task 7.3: the result carries the selected node shape/count and a populated
// category breakdown.
func TestEstimate_BreakdownAndShapePresent(t *testing.T) {
	cat := loadCatalog(t)
	m := model.ResourceModel{
		Workloads:     []model.Workload{deployment("api", 1, 1, 1000, 2*gib)},
		Volumes:       []model.Volume{{Name: "data", SizeBytes: 10 * gib, StorageClass: "fast"}},
		LoadBalancers: []model.LoadBalancer{{Name: "api"}},
	}
	opts := Options{Regions: r1Region, DiskMappings: gcpDiskMapping(map[string]string{"fast": "ssd"}, "standard")}

	res, err := Estimate(m, cat, opts)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	ce := res.Clouds[0]
	if ce.NodeShape == "" || ce.NodeCountMax < 1 {
		t.Errorf("missing shape/count: shape=%q count=%d", ce.NodeShape, ce.NodeCountMax)
	}
	if ce.Region != "r1" {
		t.Errorf("Region = %q, want r1", ce.Region)
	}
	if ce.Max.Compute <= 0 || ce.Max.Storage <= 0 || ce.Max.LoadBalancer <= 0 || ce.Max.ControlPlane <= 0 {
		t.Errorf("breakdown has non-positive category: %+v", ce.Max)
	}
	// fast → ssd: 10 GiB × $0.20 = 2.00
	approx(t, "Max.Storage(ssd)", ce.Max.Storage, 2.00)
}

// Task 7.4: known storageClass maps to the configured disk; an unknown class
// falls back to the default disk and records a warning.
func TestEstimate_StorageClassMapping(t *testing.T) {
	cat := loadCatalog(t)
	m := model.ResourceModel{
		Volumes: []model.Volume{
			{Name: "known", SizeBytes: 10 * gib, StorageClass: "fast"},
			{Name: "mystery", SizeBytes: 5 * gib, StorageClass: "weird-class"},
		},
	}
	opts := Options{Regions: r1Region, DiskMappings: gcpDiskMapping(map[string]string{"fast": "ssd"}, "standard")}

	res, err := Estimate(m, cat, opts)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	ce := res.Clouds[0]
	// known: 10 × 0.20 = 2.00; unknown → default "standard": 5 × 0.10 = 0.50.
	approx(t, "Storage(known+default)", ce.Max.Storage, 2.50)

	if len(res.Warnings) == 0 {
		t.Fatal("expected a warning for unmapped storageClass, got none")
	}
	var found bool
	for _, w := range res.Warnings {
		if strings.Contains(w, "weird-class") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings missing unmapped class: %v", res.Warnings)
	}
}

// Spot mode selects the indicative spot rate and flags the result.
func TestEstimate_SpotMode(t *testing.T) {
	cat := loadCatalog(t)
	m := model.ResourceModel{Workloads: []model.Workload{deployment("api", 1, 1, 1000, 2*gib)}}

	res, err := Estimate(m, cat, Options{Regions: r1Region, Spot: true})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if !res.Spot {
		t.Error("Spot = false, want true")
	}
	// 1 small node at spot $0.03/h × 730 = 21.90
	approx(t, "Max.Compute(spot)", res.Clouds[0].Max.Compute, 21.90)
}

// A --machine override pins the node shape regardless of cost.
func TestEstimate_MachineOverride(t *testing.T) {
	cat := loadCatalog(t)
	m := model.ResourceModel{Workloads: []model.Workload{deployment("api", 1, 1, 1000, 2*gib)}}

	res, err := Estimate(m, cat, Options{Regions: r1Region, Machines: map[string]string{"gcp": "large"}})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if res.Clouds[0].NodeShape != "large" {
		t.Errorf("NodeShape = %q, want large (overridden)", res.Clouds[0].NodeShape)
	}
}

// Unknown region/cloud surfaces an error rather than a zero price.
func TestEstimate_UnknownRegion(t *testing.T) {
	cat := loadCatalog(t)
	m := model.ResourceModel{Workloads: []model.Workload{deployment("api", 1, 1, 1000, 2*gib)}}

	_, err := Estimate(m, cat, Options{Regions: map[string]string{"gcp": "nowhere"}})
	if err == nil {
		t.Fatal("expected error for unknown region, got nil")
	}
}
