package render

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

const sampleChart = "../../testdata/charts/sample"

// Task 3.1: rendering the sample chart with defaults yields the expected
// manifests (golden file) without contacting a cluster.
func TestRenderDefaultsGolden(t *testing.T) {
	got, err := Render(Options{ChartPath: sampleChart})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	golden := filepath.Join("testdata", "golden", "sample-default.yaml")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("rendered manifests differ from golden.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderProducesExpectedKinds(t *testing.T) {
	got, err := Render(Options{ChartPath: sampleChart})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	for _, kind := range []string{
		"kind: Deployment",
		"kind: StatefulSet",
		"kind: Service",
		"kind: HorizontalPodAutoscaler",
	} {
		if !strings.Contains(got, kind) {
			t.Errorf("rendered output missing %q", kind)
		}
	}
}

// Task 3.2: values files and --set overrides apply with correct precedence.
func TestRenderValuesAndSetPrecedence(t *testing.T) {
	// Default api.replicas is 3; the values file sets it to 5; --set wins at 9.
	got, err := Render(Options{
		ChartPath:   sampleChart,
		ValuesFiles: []string{filepath.Join("testdata", "override-values.yaml")},
		SetValues:   []string{"api.replicas=9"},
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(got, "replicas: 9") {
		t.Errorf("expected --set override replicas: 9 to win, output:\n%s", got)
	}
	if strings.Contains(got, "replicas: 5") {
		t.Errorf("values-file replicas: 5 should have been overridden by --set")
	}
}

func TestRenderValuesFileApplies(t *testing.T) {
	// With only the values file, api.replicas should be 5 (not the default 3).
	got, err := Render(Options{
		ChartPath:   sampleChart,
		ValuesFiles: []string{filepath.Join("testdata", "override-values.yaml")},
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(got, "replicas: 5") {
		t.Errorf("expected values-file replicas: 5, output:\n%s", got)
	}
}

// Task 3.3: a deliberately broken chart returns a clear error.
func TestRenderBrokenChartErrors(t *testing.T) {
	_, err := Render(Options{ChartPath: "testdata/broken-chart"})
	if err == nil {
		t.Fatal("expected an error rendering the broken chart, got nil")
	}
	if !strings.Contains(err.Error(), "mandatory") {
		t.Errorf("error should surface the underlying Helm message, got: %v", err)
	}
}

func TestRenderMissingChartErrors(t *testing.T) {
	_, err := Render(Options{ChartPath: "testdata/does-not-exist"})
	if err == nil {
		t.Fatal("expected an error for a missing chart path, got nil")
	}
}
