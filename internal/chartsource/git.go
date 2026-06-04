package chartsource

import (
	"context"
	"os/exec"
	"path/filepath"
)

// gitFetcher clones a Git repository using the host `git` binary. The exec and
// lookPath hooks are injectable so tests can stub binary presence.
type gitFetcher struct {
	lookPath func(string) (string, error)
	run      func(ctx context.Context, dir, name string, args ...string) error
}

func (g *gitFetcher) lookup(name string) (string, error) {
	if g.lookPath != nil {
		return g.lookPath(name)
	}
	return exec.LookPath(name)
}

func (g *gitFetcher) exec(ctx context.Context, dir, name string, args ...string) error {
	if g.run != nil {
		return g.run(ctx, dir, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return &gitError{args: args, output: string(out), err: err}
	}
	return nil
}

func (g *gitFetcher) fetch(ctx context.Context, src Source, destDir string) (string, error) {
	if _, err := g.lookup("git"); err != nil {
		return "", ErrGitNotInstalled
	}

	repoDir := filepath.Join(destDir, "repo")

	// Try a shallow clone at the requested ref (branch or tag). If a ref is
	// given but is a commit SHA, --branch fails; fall back to a full clone plus
	// an explicit checkout.
	cloneArgs := []string{"clone", "--depth", "1"}
	if src.Ref != "" {
		cloneArgs = append(cloneArgs, "--branch", src.Ref)
	}
	cloneArgs = append(cloneArgs, src.Location, repoDir)

	if err := g.exec(ctx, destDir, "git", cloneArgs...); err != nil {
		if src.Ref == "" {
			return "", err
		}
		// Fallback: full clone then checkout the ref (handles commit SHAs).
		if err := g.exec(ctx, destDir, "git", "clone", src.Location, repoDir); err != nil {
			return "", err
		}
		if err := g.exec(ctx, repoDir, "git", "checkout", src.Ref); err != nil {
			return "", err
		}
	}

	chartPath := repoDir
	if src.SubPath != "" {
		chartPath = filepath.Join(repoDir, src.SubPath)
	}
	return chartPath, nil
}

// gitError carries git's combined output so failures are diagnosable.
type gitError struct {
	args   []string
	output string
	err    error
}

func (e *gitError) Error() string {
	if e.output != "" {
		return "git " + e.args[0] + ": " + e.err.Error() + ": " + e.output
	}
	return "git " + e.args[0] + ": " + e.err.Error()
}

func (e *gitError) Unwrap() error { return e.err }
