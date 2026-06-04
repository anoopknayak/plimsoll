package chartsource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeFetcher records the source it was asked to fetch and writes a marker file
// into destDir so tests can assert the temp dir was created and later removed.
type fakeFetcher struct {
	called  bool
	gotSrc  Source
	gotDest string
}

func (f *fakeFetcher) fetch(_ context.Context, src Source, destDir string) (string, error) {
	f.called = true
	f.gotSrc = src
	f.gotDest = destDir
	marker := filepath.Join(destDir, "chart.tgz")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		return "", err
	}
	return marker, nil
}

func newTestResolver(fake *fakeFetcher) *Resolver {
	return &Resolver{
		fetchers: map[Kind]fetcher{
			KindGit:         fake,
			KindHTTPArchive: fake,
			KindOCI:         fake,
			KindHelmRepo:    fake,
		},
	}
}

func TestResolveLocalPassThrough(t *testing.T) {
	fake := &fakeFetcher{}
	r := newTestResolver(fake)

	res, err := r.Resolve(context.Background(), "./testdata/charts/sample", Options{})
	if err != nil {
		t.Fatalf("Resolve local: %v", err)
	}
	if res.ChartPath != "./testdata/charts/sample" {
		t.Fatalf("local ChartPath = %q, want unchanged", res.ChartPath)
	}
	if fake.called {
		t.Fatal("fetcher should not be called for a local source")
	}
	if err := res.Cleanup(); err != nil {
		t.Fatalf("local cleanup should be a no-op: %v", err)
	}
}

func TestResolveRemoteFetchesAndCleansUp(t *testing.T) {
	fake := &fakeFetcher{}
	r := newTestResolver(fake)

	res, err := r.Resolve(context.Background(), "https://example.com/app-1.0.0.tgz", Options{})
	if err != nil {
		t.Fatalf("Resolve remote: %v", err)
	}
	if !fake.called {
		t.Fatal("fetcher was not called for a remote source")
	}
	if fake.gotSrc.Kind != KindHTTPArchive {
		t.Fatalf("fetcher got kind %v, want http-archive", fake.gotSrc.Kind)
	}
	if _, err := os.Stat(res.ChartPath); err != nil {
		t.Fatalf("expected materialized chart at %q: %v", res.ChartPath, err)
	}
	// The temp dir should exist before cleanup and be gone afterwards.
	tempDir := fake.gotDest
	if _, err := os.Stat(tempDir); err != nil {
		t.Fatalf("expected temp dir %q to exist: %v", tempDir, err)
	}
	if err := res.Cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("temp dir %q should be removed after cleanup, stat err = %v", tempDir, err)
	}
}

func TestResolveCleanupAfterDeferredError(t *testing.T) {
	fake := &fakeFetcher{}
	r := newTestResolver(fake)

	res, err := r.Resolve(context.Background(), "oci://reg.example.com/app:1.0.0", Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	tempDir := fake.gotDest

	// Simulate the CLI: defer cleanup, then a later stage errors out.
	func() {
		defer res.Cleanup() //nolint:errcheck
		_ = "later pipeline stage fails here"
	}()

	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("temp dir %q should be removed by deferred cleanup, stat err = %v", tempDir, err)
	}
}
