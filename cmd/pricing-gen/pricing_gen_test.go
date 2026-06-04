package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anomalyco/plimsoll/internal/pricing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return b
}

func mustNode(t *testing.T, s pricing.Snapshot, region, shape string) pricing.NodePrice {
	t.Helper()
	reg, ok := s.Regions[region]
	if !ok {
		t.Fatalf("region %q absent from snapshot", region)
	}
	n, ok := reg.Nodes[shape]
	if !ok {
		t.Fatalf("node %q absent from region %q", shape, region)
	}
	return n
}

// Task 10.3: GCP Cloud Billing Catalog → normalized snapshot. Node prices are
// assembled from separate vCPU-hour and RAM-hour SKUs.
func TestNormalizeGCP(t *testing.T) {
	snap, err := normalizeGCP(readFixture(t, "gcp_catalog.json"), "us-central1", "2026-06-01")
	if err != nil {
		t.Fatalf("normalizeGCP: %v", err)
	}
	if snap.Cloud != "gcp" || snap.Date != "2026-06-01" || snap.Currency != "USD" {
		t.Errorf("snapshot header wrong: %+v", snap)
	}
	n := mustNode(t, snap, "us-central1", "e2-standard-2")
	// 2×0.021811 + 8×0.002923 = 0.067006 → 0.0670
	if n.OnDemandHourly != 0.0670 {
		t.Errorf("e2-standard-2 on-demand = %v, want 0.0670", n.OnDemandHourly)
	}
	// 2×0.006543 + 8×0.000877 = 0.020102 → 0.0201
	if n.SpotHourly != 0.0201 {
		t.Errorf("e2-standard-2 spot = %v, want 0.0201", n.SpotHourly)
	}
	if n.CommittedHourly != round4(0.067006*committedMultiplier) {
		t.Errorf("e2-standard-2 committed = %v", n.CommittedHourly)
	}
	if d := snap.Regions["us-central1"].Disks["pd-balanced"]; d.PerGiBMonthly != 0.10 {
		t.Errorf("pd-balanced disk = %v, want 0.10", d.PerGiBMonthly)
	}
	if snap.ControlPlaneMonthly != 72.00 {
		t.Errorf("control plane = %v, want 72.00", snap.ControlPlaneMonthly)
	}
}

// Task 10.2: AWS Price List bulk JSON → normalized snapshot. Only Linux/Shared
// on-demand instances are taken; spot is derived deterministically.
func TestNormalizeAWS(t *testing.T) {
	snap, err := normalizeAWS(readFixture(t, "aws_pricelist.json"), "us-east-1", "2026-06-01")
	if err != nil {
		t.Fatalf("normalizeAWS: %v", err)
	}
	reg := snap.Regions["us-east-1"]
	if len(reg.Nodes) != 2 {
		t.Errorf("nodes = %d, want 2 (Windows variant excluded): %v", len(reg.Nodes), reg.Nodes)
	}
	n := mustNode(t, snap, "us-east-1", "m5.large")
	if n.OnDemandHourly != 0.0960 {
		t.Errorf("m5.large on-demand = %v, want 0.0960", n.OnDemandHourly)
	}
	if n.SpotHourly != round4(0.096*awsSpotMultiplier) {
		t.Errorf("m5.large spot = %v", n.SpotHourly)
	}
	if d := reg.Disks["gp3"]; d.PerGiBMonthly != 0.08 {
		t.Errorf("gp3 disk = %v, want 0.08", d.PerGiBMonthly)
	}
}

// Task 10.1: Azure Retail Prices → normalized snapshot. On-demand and Spot are
// matched by SKU; other regions are ignored.
func TestNormalizeAzure(t *testing.T) {
	snap, err := normalizeAzure(readFixture(t, "azure_retail.json"), "eastus", "2026-06-01")
	if err != nil {
		t.Fatalf("normalizeAzure: %v", err)
	}
	reg := snap.Regions["eastus"]
	if len(reg.Nodes) != 2 {
		t.Errorf("nodes = %d, want 2: %v", len(reg.Nodes), reg.Nodes)
	}
	n := mustNode(t, snap, "eastus", "Standard_D2s_v5")
	if n.OnDemandHourly != 0.0960 || n.SpotHourly != 0.0192 {
		t.Errorf("D2s_v5 od/spot = %v/%v, want 0.0960/0.0192", n.OnDemandHourly, n.SpotHourly)
	}
	if reg.Disks["StandardSSD_LRS"].PerGiBMonthly != 0.075 {
		t.Errorf("StandardSSD_LRS = %v, want 0.075", reg.Disks["StandardSSD_LRS"].PerGiBMonthly)
	}
	// AKS standard tier is free.
	if snap.ControlPlaneMonthly != 0 {
		t.Errorf("azure control plane = %v, want 0", snap.ControlPlaneMonthly)
	}
}

// Task 10.4: malformed source data fails validation loudly and writes nothing.
func TestGenerate_MalformedFailsAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	_, err := generate("azure", readFixture(t, "azure_malformed.json"), "eastus", "2026-06-01", dir)
	if err == nil {
		t.Fatal("expected schema validation error, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "azure.json")); !os.IsNotExist(statErr) {
		t.Errorf("snapshot file should not exist after validation failure (stat err: %v)", statErr)
	}
}

func TestValidateSnapshot(t *testing.T) {
	good, _ := normalizeAWS(readFixture(t, "aws_pricelist.json"), "us-east-1", "2026-06-01")
	if err := validateSnapshot(good); err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
	cases := map[string]pricing.Snapshot{
		"no cloud":   {Date: "d", Regions: map[string]pricing.Region{"r": {Nodes: map[string]pricing.NodePrice{"n": {VCPU: 1, MemBytes: 1, OnDemandHourly: 1}}}}},
		"no date":    {Cloud: "c", Regions: map[string]pricing.Region{"r": {Nodes: map[string]pricing.NodePrice{"n": {VCPU: 1, MemBytes: 1, OnDemandHourly: 1}}}}},
		"no regions": {Cloud: "c", Date: "d"},
		"no nodes":   {Cloud: "c", Date: "d", Regions: map[string]pricing.Region{"r": {}}},
		"bad price":  {Cloud: "c", Date: "d", Regions: map[string]pricing.Region{"r": {Nodes: map[string]pricing.NodePrice{"n": {VCPU: 1, MemBytes: 1, OnDemandHourly: 0}}}}},
	}
	for name, s := range cases {
		if err := validateSnapshot(s); err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
}

// Task 10.5: a successful generation writes a dated, schema-valid snapshot file
// that the pricing loader can read back.
func TestGenerate_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path, err := generate("gcp", readFixture(t, "gcp_catalog.json"), "us-central1", "2026-06-01", dir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if path != filepath.Join(dir, "gcp.json") {
		t.Errorf("path = %q", path)
	}
	cat, err := pricing.LoadFS(os.DirFS(dir), ".")
	if err != nil {
		t.Fatalf("loading generated snapshot: %v", err)
	}
	snap, err := cat.Snapshot("gcp")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := snap.Region("us-central1"); err != nil {
		t.Errorf("generated snapshot missing region: %v", err)
	}
}

func TestGenerate_UnknownCloud(t *testing.T) {
	if _, err := generate("oracle", []byte(`{}`), "r", "d", t.TempDir()); err == nil {
		t.Error("expected error for unknown cloud")
	}
}

// The CLI wires flags through to generate and validates inputs.
func TestGenCmd(t *testing.T) {
	dir := t.TempDir()

	cmd := newGenCmd()
	cmd.SetArgs([]string{
		"--cloud", "aws", "--region", "us-east-1",
		"--source", filepath.Join("testdata", "aws_pricelist.json"),
		"--out", dir, "--date", "2026-06-01",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gen cmd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "aws.json")); err != nil {
		t.Errorf("expected aws.json written: %v", err)
	}

	// Invalid cloud is rejected.
	bad := newGenCmd()
	bad.SetArgs([]string{"--cloud", "oracle", "--region", "x", "--source", "y"})
	if err := bad.Execute(); err == nil {
		t.Error("expected error for invalid cloud")
	}

	// Missing source/url is rejected.
	noSrc := newGenCmd()
	noSrc.SetArgs([]string{"--cloud", "aws", "--region", "us-east-1"})
	if err := noSrc.Execute(); err == nil {
		t.Error("expected error when neither --source nor --url provided")
	}
}
