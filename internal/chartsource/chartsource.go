// Package chartsource resolves the `estimate` positional argument into a local
// chart path. It detects whether the reference is a local path, a Git
// repository, an HTTP(S) packaged archive, an OCI reference, or a Helm
// repository URL, materializes remote charts into a managed temporary
// directory, and returns a cleanup function. The renderer always loads a local
// path; this package sits in front of it.
package chartsource

import "errors"

// Kind enumerates the supported chart-source types.
type Kind int

const (
	// KindLocal is a chart directory or packaged .tgz on the local filesystem.
	KindLocal Kind = iota
	// KindGit is a Git repository (optionally with a ref and sub-chart path).
	KindGit
	// KindHTTPArchive is a direct HTTP(S) URL to a packaged chart archive.
	KindHTTPArchive
	// KindOCI is an oci:// chart reference.
	KindOCI
	// KindHelmRepo is a Helm repository URL.
	KindHelmRepo
)

// String returns a human-readable name for the kind, used in error messages.
func (k Kind) String() string {
	switch k {
	case KindLocal:
		return "local"
	case KindGit:
		return "git"
	case KindHTTPArchive:
		return "http-archive"
	case KindOCI:
		return "oci"
	case KindHelmRepo:
		return "helm-repo"
	default:
		return "unknown"
	}
}

// Source is the parsed, I/O-free description of a chart reference.
type Source struct {
	Kind     Kind
	Location string // path, repo URL, oci ref, or archive URL (git+ prefix stripped)
	Ref      string // git branch/tag/commit, if any
	SubPath  string // sub-chart directory within a git repo, if any
	Version  string // chart version for oci/helm-repo sources, if any
}

// Resolved is the output of resolving a Source: a local chart path the renderer
// can load, plus a cleanup function that removes any materialized temp files.
type Resolved struct {
	ChartPath string
	Cleanup   func() error
}

// ErrGitNotInstalled is returned when a Git source is resolved but no `git`
// binary is available on PATH.
var ErrGitNotInstalled = errors.New("git is not installed: install git or pre-clone the chart and pass a local path")
