package chartsource

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// httpArchiveFetcher downloads a packaged chart archive over HTTP(S).
type httpArchiveFetcher struct {
	client *http.Client
}

func (h *httpArchiveFetcher) fetch(ctx context.Context, src Source, destDir string) (string, error) {
	client := h.client
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.Location, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %s", resp.Status)
	}

	dest := filepath.Join(destDir, "chart.tgz")
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return dest, nil
}
