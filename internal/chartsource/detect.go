package chartsource

import (
	"fmt"
	"strings"
)

// Detect classifies a chart reference into a typed Source without performing
// any I/O. It parses an optional Git ref (#ref) and sub-chart path (?path=) and
// an optional OCI version (trailing :tag). Detection is first-match-wins in the
// order: OCI, Git, HTTP archive, Helm repo, then local (the default).
func Detect(ref string) (Source, error) {
	if strings.TrimSpace(ref) == "" {
		return Source{}, fmt.Errorf("empty chart reference")
	}

	// OCI references are unambiguous.
	if strings.HasPrefix(ref, "oci://") {
		loc, version := splitOCIVersion(ref)
		return Source{Kind: KindOCI, Location: loc, Version: version}, nil
	}

	// Git: explicit git+ prefix, scp-style git@host:path, or any .git URL.
	if strings.HasPrefix(ref, "git+") || strings.HasPrefix(ref, "git@") || isDotGit(ref) {
		return detectGit(ref)
	}

	// HTTP(S): archive vs. plain Helm-repo URL.
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		if hasArchiveSuffix(ref) {
			return Source{Kind: KindHTTPArchive, Location: ref}, nil
		}
		return Source{Kind: KindHelmRepo, Location: ref}, nil
	}

	// Everything else is treated as a local path (directory or .tgz).
	return Source{Kind: KindLocal, Location: ref}, nil
}

// detectGit parses a Git reference, stripping the optional git+ prefix and
// extracting #ref and ?path= components. Unknown query keys are rejected.
func detectGit(ref string) (Source, error) {
	loc := strings.TrimPrefix(ref, "git+")

	var fragment, query string
	// Fragment (#ref) and query (?path=) may appear in either order.
	if i := strings.IndexAny(loc, "#?"); i >= 0 {
		rest := loc[i:]
		loc = loc[:i]
		// Walk the remaining components, splitting on the next delimiter.
		for len(rest) > 0 {
			delim := rest[0]
			rest = rest[1:]
			j := strings.IndexAny(rest, "#?")
			var val string
			if j >= 0 {
				val = rest[:j]
				rest = rest[j:]
			} else {
				val = rest
				rest = ""
			}
			switch delim {
			case '#':
				fragment = val
			case '?':
				query = val
			}
		}
	}

	src := Source{Kind: KindGit, Location: loc, Ref: fragment}
	if query != "" {
		for _, pair := range strings.Split(query, "&") {
			k, v, _ := strings.Cut(pair, "=")
			switch k {
			case "path":
				src.SubPath = v
			case "ref":
				if src.Ref == "" {
					src.Ref = v
				}
			default:
				return Source{}, fmt.Errorf("invalid git query parameter %q", k)
			}
		}
	}
	return src, nil
}

// splitOCIVersion separates a trailing :tag version from an oci:// reference,
// taking care not to treat the scheme's own colon as a version separator.
func splitOCIVersion(ref string) (loc, version string) {
	// Strip scheme, find a colon in the remainder.
	const scheme = "oci://"
	rest := ref[len(scheme):]
	if i := strings.LastIndex(rest, ":"); i >= 0 && !strings.Contains(rest[i:], "/") {
		return scheme + rest[:i], rest[i+1:]
	}
	return ref, ""
}

// isDotGit reports whether a URL-like ref ends with .git (ignoring #/? suffixes).
func isDotGit(ref string) bool {
	base := ref
	if i := strings.IndexAny(base, "#?"); i >= 0 {
		base = base[:i]
	}
	return strings.HasSuffix(base, ".git")
}

// hasArchiveSuffix reports whether a URL points at a packaged chart archive.
func hasArchiveSuffix(ref string) bool {
	base := ref
	if i := strings.IndexAny(base, "#?"); i >= 0 {
		base = base[:i]
	}
	return strings.HasSuffix(base, ".tgz") || strings.HasSuffix(base, ".tar.gz")
}
