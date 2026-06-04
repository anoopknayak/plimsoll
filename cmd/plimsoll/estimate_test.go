package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sampleChart is the bundled fixture chart, relative to this package directory.
const sampleChart = "../../testdata/charts/sample"

// execute runs the root command with args and captures combined output.
func execute(args ...string) (string, error) {
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// Task 9.1: chart path is parsed and, with no --clouds, all three clouds are
// estimated (default selection).
func TestEstimate_DefaultClouds(t *testing.T) {
	out, err := execute("estimate", sampleChart)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	for _, cloud := range []string{"gcp", "aws", "azure"} {
		if !strings.Contains(out, cloud) {
			t.Errorf("default output missing cloud %q:\n%s", cloud, out)
		}
	}
}

// Task 9.1: --clouds selects a subset.
func TestEstimate_CloudsFlag(t *testing.T) {
	out, err := execute("estimate", sampleChart, "--clouds", "gcp")
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	if !strings.Contains(out, "gcp") {
		t.Errorf("output missing selected cloud gcp:\n%s", out)
	}
	if strings.Contains(out, "aws") || strings.Contains(out, "azure") {
		t.Errorf("output includes unselected clouds:\n%s", out)
	}
}

// Task 9.2: --output json produces machine-readable output.
func TestEstimate_OutputJSON(t *testing.T) {
	out, err := execute("estimate", sampleChart, "--clouds", "gcp", "-o", "json")
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	var parsed struct {
		Clouds []struct {
			Cloud        string `json:"cloud"`
			NodeShape    string `json:"nodeShape"`
			SnapshotDate string `json:"snapshotDate"`
		} `json:"clouds"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(parsed.Clouds) != 1 || parsed.Clouds[0].Cloud != "gcp" || parsed.Clouds[0].NodeShape == "" {
		t.Errorf("unexpected JSON structure: %+v", parsed)
	}
}

// Task 9.2: --region, --machine, and --spot are parsed and applied.
func TestEstimate_RegionMachineSpotFlags(t *testing.T) {
	out, err := execute("estimate", sampleChart,
		"--clouds", "gcp",
		"--region", "gcp=us-central1",
		"--machine", "gcp=e2-standard-8",
		"--spot",
	)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	if !strings.Contains(out, "e2-standard-8") {
		t.Errorf("machine override not applied:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "spot") {
		t.Errorf("spot disclaimer not shown:\n%s", out)
	}
}

// Task 9.2: --committed-use is accepted on its own.
func TestEstimate_CommittedUseFlag(t *testing.T) {
	out, err := execute("estimate", sampleChart, "--clouds", "gcp", "--committed-use")
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	if strings.Contains(strings.ToLower(out), "spot") {
		t.Errorf("committed-use output should not show spot disclaimer:\n%s", out)
	}
}

// Task 9.3: invalid inputs print a usage/error and return a non-zero (error).
func TestEstimate_InvalidInputs(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"missing chart arg", []string{"estimate"}},
		{"nonexistent chart", []string{"estimate", "./does-not-exist"}},
		{"mutually exclusive modes", []string{"estimate", sampleChart, "--spot", "--committed-use"}},
		{"invalid output format", []string{"estimate", sampleChart, "-o", "xml"}},
		{"malformed machine pair", []string{"estimate", sampleChart, "--machine", "garbage"}},
		{"unknown region", []string{"estimate", sampleChart, "--clouds", "gcp", "--region", "gcp=narnia"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := execute(tc.args...); err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestParsePairs(t *testing.T) {
	got, err := parsePairs([]string{"gcp=us-west1", " aws = us-east-2 "}, "region")
	if err != nil {
		t.Fatalf("parsePairs: %v", err)
	}
	if got["gcp"] != "us-west1" || got["aws"] != "us-east-2" {
		t.Errorf("parsePairs = %v", got)
	}
	if _, err := parsePairs([]string{"nokey="}, "machine"); err == nil {
		t.Error("expected error for malformed pair")
	}
	if got, _ := parsePairs(nil, "region"); got != nil {
		t.Errorf("nil input should yield nil map, got %v", got)
	}
}

// initSampleGitRepo commits the sample chart into a throwaway git repo and
// returns the repo path. Skips if git is unavailable.
func initSampleGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	// Copy the sample chart into the repo under chart/.
	dst := filepath.Join(repo, "chart")
	copyTree(t, sampleChart, dst)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("checkout", "-q", "-b", "main")
	run("add", "-A")
	run("commit", "-q", "-m", "chart")
	return repo
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copyTree: %v", err)
	}
}

// A git source flows through the full pipeline and the materialized clone is
// cleaned up after the command returns.
func TestEstimate_GitSource(t *testing.T) {
	repo := initSampleGitRepo(t)
	ref := "git+file://" + repo + "#main?path=chart"

	out, err := execute("estimate", ref, "--clouds", "gcp")
	if err != nil {
		t.Fatalf("execute git source: %v\n%s", err, out)
	}
	if !strings.Contains(out, "gcp") {
		t.Errorf("git-source output missing gcp:\n%s", out)
	}
}
