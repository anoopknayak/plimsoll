package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Task 9.5: build the actual binary and run it against the sample chart for all
// three clouds, exercising the full render→extract→pack→price→output pipeline
// as a user would.
func TestEndToEnd_Binary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary e2e test in -short mode")
	}

	bin := filepath.Join(t.TempDir(), "plimsoll")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("building binary: %v\n%s", err, out)
	}

	chart, err := filepath.Abs(sampleChart)
	if err != nil {
		t.Fatalf("resolving chart path: %v", err)
	}

	out, err := exec.Command(bin, "estimate", chart, "--clouds", "gcp,aws,azure").CombinedOutput()
	if err != nil {
		t.Fatalf("running binary: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{"gcp", "aws", "azure", "Pricing snapshot:", "Breakdown"} {
		if !strings.Contains(got, want) {
			t.Errorf("e2e output missing %q:\n%s", want, got)
		}
	}

	// JSON output should also be valid end-to-end.
	jsonOut, err := exec.Command(bin, "estimate", chart, "-o", "json").CombinedOutput()
	if err != nil {
		t.Fatalf("running binary (json): %v\n%s", err, jsonOut)
	}
	if !strings.Contains(string(jsonOut), `"clouds"`) {
		t.Errorf("e2e json output malformed:\n%s", jsonOut)
	}
}
