package chartsource

import (
	"context"
	"errors"
	"testing"
)

func TestKindString(t *testing.T) {
	cases := map[Kind]string{
		KindLocal:       "local",
		KindGit:         "git",
		KindHTTPArchive: "http-archive",
		KindOCI:         "oci",
		KindHelmRepo:    "helm-repo",
		Kind(99):        "unknown",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestLooksLikeChartRef(t *testing.T) {
	cases := map[string]bool{
		"https://charts.example.com":       false,
		"https://charts.example.com/":      false,
		"https://charts.example.com/app":   true,
		"oci://reg.example.com/charts/app": true,
	}
	for loc, want := range cases {
		if got := looksLikeChartRef(loc); got != want {
			t.Errorf("looksLikeChartRef(%q) = %v, want %v", loc, got, want)
		}
	}
}

func TestPackageResolveLocal(t *testing.T) {
	res, err := Resolve(context.Background(), "./some/local/chart", Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.ChartPath != "./some/local/chart" {
		t.Fatalf("ChartPath = %q, want unchanged", res.ChartPath)
	}
	if err := res.Cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

func TestResolveDetectError(t *testing.T) {
	_, err := Resolve(context.Background(), "", Options{})
	if err == nil {
		t.Fatal("expected error for empty reference")
	}
}

func TestResolveOptionsVersionApplied(t *testing.T) {
	fake := &fakeFetcher{}
	r := newTestResolver(fake)
	_, err := r.Resolve(context.Background(), "oci://reg.example.com/charts/app", Options{Version: "2.0.0"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if fake.gotSrc.Version != "2.0.0" {
		t.Fatalf("version = %q, want 2.0.0 from Options", fake.gotSrc.Version)
	}
}

func TestGitErrorMessage(t *testing.T) {
	e := &gitError{args: []string{"clone"}, output: "boom", err: errors.New("exit 1")}
	if e.Error() == "" || !errors.Is(e, e.err) {
		t.Fatalf("unexpected gitError behavior: %q", e.Error())
	}
	e2 := &gitError{args: []string{"checkout"}, err: errors.New("exit 1")}
	if e2.Error() == "" {
		t.Fatal("expected non-empty error without output")
	}
}
