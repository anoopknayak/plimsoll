package chartsource

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initGitRepo creates a real local git repository with a chart sub-directory and
// returns its path. Skips the test if git is unavailable.
func initGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
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
	chartDir := filepath.Join(dir, "charts", "app")
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte("name: app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	return dir
}

func TestGitFetcherCloneWithSubPath(t *testing.T) {
	repo := initGitRepo(t)
	dest := t.TempDir()

	g := &gitFetcher{}
	src := Source{Kind: KindGit, Location: repo, Ref: "main", SubPath: "charts/app"}

	chartPath, err := g.fetch(context.Background(), src, dest)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(chartPath, "Chart.yaml")); err != nil {
		t.Fatalf("expected Chart.yaml at sub-path %q: %v", chartPath, err)
	}
}

func TestGitFetcherCloneRoot(t *testing.T) {
	repo := initGitRepo(t)
	dest := t.TempDir()

	g := &gitFetcher{}
	src := Source{Kind: KindGit, Location: repo}

	chartPath, err := g.fetch(context.Background(), src, dest)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(chartPath, "charts", "app", "Chart.yaml")); err != nil {
		t.Fatalf("expected repo root clone at %q: %v", chartPath, err)
	}
}

func TestGitFetcherMissingBinary(t *testing.T) {
	g := &gitFetcher{
		lookPath: func(string) (string, error) { return "", errors.New("not found") },
	}
	_, err := g.fetch(context.Background(), Source{Kind: KindGit, Location: "x"}, t.TempDir())
	if !errors.Is(err, ErrGitNotInstalled) {
		t.Fatalf("got %v, want ErrGitNotInstalled", err)
	}
}
