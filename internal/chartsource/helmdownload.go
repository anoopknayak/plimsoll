package chartsource

import (
	"context"
	"fmt"
	"io"
	"strings"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
)

// helmDownloadFetcher pulls OCI and Helm-repository charts using the Helm SDK
// downloader. It never contacts a cluster.
type helmDownloadFetcher struct{}

func (d *helmDownloadFetcher) fetch(_ context.Context, src Source, destDir string) (string, error) {
	// A plain Helm-repository URL without a resolvable chart name cannot be
	// rendered; guide the user to an unambiguous form.
	if src.Kind == KindHelmRepo && !looksLikeChartRef(src.Location) {
		return "", fmt.Errorf("ambiguous Helm repository URL %q: provide an oci:// reference or a direct .tgz URL", src.Location)
	}

	settings := cli.New()

	regClient, err := registry.NewClient(
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(io.Discard),
	)
	if err != nil {
		return "", err
	}

	dl := downloader.ChartDownloader{
		Out:              io.Discard,
		Verify:           downloader.VerifyNever,
		Getters:          getter.All(settings),
		RegistryClient:   regClient,
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
	}

	saved, _, err := dl.DownloadTo(src.Location, src.Version, destDir)
	if err != nil {
		return "", err
	}
	return saved, nil
}

// looksLikeChartRef reports whether a Helm-repo location plausibly points at a
// specific chart (has a path segment) rather than a bare repository root.
func looksLikeChartRef(loc string) bool {
	trimmed := strings.TrimRight(loc, "/")
	// Strip scheme.
	if i := strings.Index(trimmed, "://"); i >= 0 {
		trimmed = trimmed[i+3:]
	}
	// A bare host (no extra path segment) is not a chart reference.
	return strings.Count(trimmed, "/") >= 1
}
