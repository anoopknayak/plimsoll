package output

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anomalyco/plimsoll/internal/estimate"
)

var update = flag.Bool("update", false, "update golden files")

// sampleResult is a fixed two-cloud estimate used across the golden tests.
func sampleResult() estimate.Result {
	gcp := estimate.CloudEstimate{
		Cloud: "gcp", Region: "us-central1", NodeShape: "small",
		NodeCountMin: 1, NodeCountMax: 2, SnapshotDate: "2026-06-01",
		Min: estimate.Breakdown{Compute: 73.00, Storage: 1.00, LoadBalancer: 18.00, ControlPlane: 73.00},
		Max: estimate.Breakdown{Compute: 146.00, Storage: 1.00, LoadBalancer: 18.00, ControlPlane: 73.00},
	}
	aws := estimate.CloudEstimate{
		Cloud: "aws", Region: "us-east-1", NodeShape: "m6i.large",
		NodeCountMin: 1, NodeCountMax: 1, SnapshotDate: "2026-06-01",
		Min: estimate.Breakdown{Compute: 70.00, Storage: 1.20, LoadBalancer: 16.00, ControlPlane: 73.00},
		Max: estimate.Breakdown{Compute: 70.00, Storage: 1.20, LoadBalancer: 16.00, ControlPlane: 73.00},
	}
	return estimate.Result{Clouds: []estimate.CloudEstimate{gcp, aws}}
}

func render(t *testing.T, res estimate.Result, f Format) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Render(&buf, res, f); err != nil {
		t.Fatalf("Render(%s): %v", f, err)
	}
	return buf.String()
}

// assertGolden compares got against testdata/golden/<name>, regenerating it when
// -update is set.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	golden := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatalf("mkdir golden: %v", err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading golden (run with -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("output differs from golden %s.\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// Task 8.1: table output with side-by-side ranges + breakdown.
func TestRenderTable(t *testing.T) {
	assertGolden(t, "table.txt", render(t, sampleResult(), Table))
}

// Task 8.2: JSON output with totals, ranges, breakdown, node shapes, snapshot date.
func TestRenderJSON(t *testing.T) {
	got := render(t, sampleResult(), JSON)
	assertGolden(t, "estimate.json", got)
	for _, want := range []string{`"nodeShape": "small"`, `"snapshotDate": "2026-06-01"`, `"compute": 146`} {
		if !strings.Contains(got, want) {
			t.Errorf("JSON missing %q", want)
		}
	}
}

// Task 8.3: Markdown output for PR comments.
func TestRenderMarkdown(t *testing.T) {
	got := render(t, sampleResult(), Markdown)
	assertGolden(t, "estimate.md", got)
	if !strings.Contains(got, "| Cloud | Region |") {
		t.Errorf("markdown missing table header:\n%s", got)
	}
}

// Task 8.4: snapshot date always shown in every format.
func TestSnapshotDateAlwaysShown(t *testing.T) {
	res := sampleResult()
	for _, f := range []Format{Table, JSON, Markdown} {
		out := render(t, res, f)
		if !strings.Contains(out, "2026-06-01") {
			t.Errorf("format %s does not show snapshot date:\n%s", f, out)
		}
	}
}

// Task 8.4: spot disclaimer shown only when spot pricing is used.
func TestSpotDisclaimer(t *testing.T) {
	res := sampleResult()

	for _, f := range []Format{Table, Markdown} {
		if strings.Contains(strings.ToLower(render(t, res, f)), "spot") {
			t.Errorf("format %s shows spot disclaimer without spot pricing", f)
		}
	}

	res.Spot = true
	for _, f := range []Format{Table, Markdown} {
		out := strings.ToLower(render(t, res, f))
		if !strings.Contains(out, "spot") || !strings.Contains(out, "indicative") {
			t.Errorf("format %s missing spot/indicative disclaimer:\n%s", f, render(t, res, f))
		}
	}
}

func TestParseFormat(t *testing.T) {
	cases := map[string]Format{
		"": Table, "table": Table, "TABLE": Table,
		"json": JSON, "Json": JSON,
		"markdown": Markdown, "md": Markdown, " MD ": Markdown,
	}
	for in, want := range cases {
		got, err := ParseFormat(in)
		if err != nil || got != want {
			t.Errorf("ParseFormat(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Error("ParseFormat(xml) should error")
	}
}

func TestRenderUnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleResult(), Format("xml")); err == nil {
		t.Error("Render with unknown format should error")
	}
}

func TestRenderWarnings(t *testing.T) {
	res := sampleResult()
	res.Warnings = []string{"gcp: storageClass \"weird\" not mapped; using default"}
	for _, f := range []Format{Table, Markdown} {
		if !strings.Contains(render(t, res, f), "weird") {
			t.Errorf("format %s did not render warnings", f)
		}
	}
}
