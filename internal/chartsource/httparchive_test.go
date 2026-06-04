package chartsource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestHTTPArchiveFetcherDownloads(t *testing.T) {
	const body = "fake-chart-tarball"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	h := &httpArchiveFetcher{client: srv.Client()}
	dest := t.TempDir()

	chartPath, err := h.fetch(context.Background(), Source{Kind: KindHTTPArchive, Location: srv.URL + "/app.tgz"}, dest)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	got, err := os.ReadFile(chartPath)
	if err != nil {
		t.Fatalf("read downloaded archive: %v", err)
	}
	if string(got) != body {
		t.Fatalf("downloaded content = %q, want %q", got, body)
	}
}

func TestHTTPArchiveFetcherNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := &httpArchiveFetcher{client: srv.Client()}
	_, err := h.fetch(context.Background(), Source{Kind: KindHTTPArchive, Location: srv.URL + "/missing.tgz"}, t.TempDir())
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestHelmRepoAmbiguousURL(t *testing.T) {
	d := &helmDownloadFetcher{}
	_, err := d.fetch(context.Background(), Source{Kind: KindHelmRepo, Location: "https://charts.example.com"}, t.TempDir())
	if err == nil {
		t.Fatal("expected ambiguous Helm-repo URL error")
	}
}
