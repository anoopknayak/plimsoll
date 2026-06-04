package chartsource

import (
	"context"
	"fmt"
	"os"
)

// Options configures resolution. Reserved for future knobs (auth, version
// override); kept as a struct so the public signature is stable.
type Options struct {
	// Version overrides the chart version for OCI/Helm-repo sources when the
	// reference itself does not carry one.
	Version string
}

// fetcher materializes a remote Source into destDir and returns the local chart
// path (a directory or a .tgz) the renderer should load.
type fetcher interface {
	fetch(ctx context.Context, src Source, destDir string) (chartPath string, err error)
}

// Resolver turns a chart reference into a local path. It owns the temp-dir
// lifecycle and dispatches remote kinds to per-kind fetchers.
type Resolver struct {
	fetchers map[Kind]fetcher
	// tempDir creates a new temporary directory; overridable in tests.
	tempDir func() (string, error)
}

// NewResolver builds a Resolver wired with the production fetchers.
func NewResolver() *Resolver {
	return &Resolver{
		fetchers: map[Kind]fetcher{
			KindGit:         &gitFetcher{},
			KindHTTPArchive: &httpArchiveFetcher{},
			KindOCI:         &helmDownloadFetcher{},
			KindHelmRepo:    &helmDownloadFetcher{},
		},
	}
}

// Resolve detects the source kind of ref and, for remote sources, materializes
// the chart into a managed temporary directory. Local sources are returned
// unchanged with a no-op cleanup.
func (r *Resolver) Resolve(ctx context.Context, ref string, opts Options) (Resolved, error) {
	src, err := Detect(ref)
	if err != nil {
		return Resolved{}, err
	}
	if opts.Version != "" && src.Version == "" {
		src.Version = opts.Version
	}

	if src.Kind == KindLocal {
		return Resolved{ChartPath: src.Location, Cleanup: noopCleanup}, nil
	}

	f, ok := r.fetchers[src.Kind]
	if !ok {
		return Resolved{}, fmt.Errorf("no fetcher for %s source", src.Kind)
	}

	destDir, err := r.mkTempDir()
	if err != nil {
		return Resolved{}, err
	}
	cleanup := func() error { return os.RemoveAll(destDir) }

	chartPath, err := f.fetch(ctx, src, destDir)
	if err != nil {
		_ = cleanup()
		return Resolved{}, fmt.Errorf("fetching %s %q: %w", src.Kind, src.Location, err)
	}
	return Resolved{ChartPath: chartPath, Cleanup: cleanup}, nil
}

func (r *Resolver) mkTempDir() (string, error) {
	if r.tempDir != nil {
		return r.tempDir()
	}
	return os.MkdirTemp("", "plimsoll-chart-*")
}

func noopCleanup() error { return nil }

// Resolve is a package-level convenience that uses a default Resolver.
func Resolve(ctx context.Context, ref string, opts Options) (Resolved, error) {
	return NewResolver().Resolve(ctx, ref, opts)
}
