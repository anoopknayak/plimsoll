package pricing

import (
	"errors"
	"testing"
)

// Task 5.2: embedded snapshot loads and exposes its date.
func TestLoadEmbeddedSnapshots(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, cloud := range []string{"gcp", "aws", "azure"} {
		s, err := c.Snapshot(cloud)
		if err != nil {
			t.Fatalf("Snapshot(%q): %v", cloud, err)
		}
		if s.Date == "" {
			t.Errorf("%s snapshot has no date", cloud)
		}
		if len(s.Regions) == 0 {
			t.Errorf("%s snapshot has no regions", cloud)
		}
	}
}

// Task 5.3: region-specific node lookup; missing region/SKU → not-found.
func TestNodeLookupSuccess(t *testing.T) {
	c, _ := Load()
	s, _ := c.Snapshot("gcp")
	r, err := s.Region("us-central1")
	if err != nil {
		t.Fatalf("Region: %v", err)
	}
	n, err := r.Node("e2-standard-4")
	if err != nil {
		t.Fatalf("Node: %v", err)
	}
	if n.VCPU != 4 || n.MemBytes != 17179869184 {
		t.Errorf("capacity = %d vCPU / %d bytes, want 4 / 17179869184", n.VCPU, n.MemBytes)
	}
	if n.OnDemandHourly != 0.134012 {
		t.Errorf("on-demand = %v, want 0.134012", n.OnDemandHourly)
	}
}

func TestMissingRegionIsNotFound(t *testing.T) {
	c, _ := Load()
	s, _ := c.Snapshot("gcp")
	_, err := s.Region("antarctica-south1")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("missing region error = %v, want ErrNotFound", err)
	}
}

func TestMissingNodeIsNotFound(t *testing.T) {
	c, _ := Load()
	s, _ := c.Snapshot("gcp")
	r, _ := s.Region("us-central1")
	_, err := r.Node("does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("missing node error = %v, want ErrNotFound", err)
	}
}

func TestMissingDiskIsNotFound(t *testing.T) {
	c, _ := Load()
	s, _ := c.Snapshot("gcp")
	r, _ := s.Region("us-central1")
	_, err := r.Disk("nonexistent-disk")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("missing disk error = %v, want ErrNotFound", err)
	}
}

func TestMissingCloudIsNotFound(t *testing.T) {
	c, _ := Load()
	_, err := c.Snapshot("oracle")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("missing cloud error = %v, want ErrNotFound", err)
	}
}

// Task 5.4: pricing modes — on-demand default, committed discount, spot indicative.
func TestPricingModes(t *testing.T) {
	c, _ := Load()
	s, _ := c.Snapshot("gcp")
	r, _ := s.Region("us-central1")
	n, _ := r.Node("e2-standard-4")

	rate, indicative := n.Hourly(OnDemand)
	if rate != 0.134012 || indicative {
		t.Errorf("on-demand = (%v, %v), want (0.134012, false)", rate, indicative)
	}

	rate, indicative = n.Hourly(CommittedUse)
	if rate != 0.084400 || indicative {
		t.Errorf("committed = (%v, %v), want (0.0844, false)", rate, indicative)
	}
	if n.CommittedHourly >= n.OnDemandHourly {
		t.Errorf("committed rate should be a discount vs on-demand")
	}

	rate, indicative = n.Hourly(Spot)
	if rate != 0.040200 || !indicative {
		t.Errorf("spot = (%v, %v), want (0.0402, true)", rate, indicative)
	}
}

func TestControlPlaneAndLoadBalancer(t *testing.T) {
	c, _ := Load()
	gcp, _ := c.Snapshot("gcp")
	if gcp.ControlPlaneMonthly != 73.00 {
		t.Errorf("gcp control plane = %v, want 73.00", gcp.ControlPlaneMonthly)
	}
	azure, _ := c.Snapshot("azure")
	if azure.ControlPlaneMonthly != 0 {
		t.Errorf("azure control plane = %v, want 0 (free tier)", azure.ControlPlaneMonthly)
	}
	r, _ := gcp.Region("us-central1")
	if r.LoadBalancerMonthly <= 0 {
		t.Errorf("gcp LB monthly = %v, want > 0", r.LoadBalancerMonthly)
	}
}

func TestClouds(t *testing.T) {
	c, _ := Load()
	got := map[string]bool{}
	for _, cl := range c.Clouds() {
		got[cl] = true
	}
	for _, want := range []string{"gcp", "aws", "azure"} {
		if !got[want] {
			t.Errorf("Clouds() missing %q (got %v)", want, c.Clouds())
		}
	}
}

func TestLoadFSEmptyDirErrors(t *testing.T) {
	_, err := LoadFS(seedFS, "nonexistent-dir")
	if err == nil {
		t.Fatal("expected error loading from nonexistent dir")
	}
}
