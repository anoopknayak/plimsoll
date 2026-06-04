// Package render turns a Helm chart plus values into Kubernetes manifests using
// the Helm SDK in-process. It never contacts a cluster: rendering is performed
// client-side with a dry-run install, exactly like `helm template`.
package render

import (
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
)

// DefaultKubeVersion is the Kubernetes version charts are rendered against when
// no explicit version is supplied. It is intentionally a recent stable release
// so charts that declare a modern kubeVersion floor render successfully. The
// Helm SDK otherwise defaults to v1.20.0, which is too old for many charts.
const DefaultKubeVersion = "v1.31.0"

// Options configures a render.
type Options struct {
	// ChartPath is the path to a chart directory or packaged chart.
	ChartPath string
	// ValuesFiles are paths to values YAML files, applied in order (later wins).
	ValuesFiles []string
	// SetValues are inline "key=value" overrides, applied after values files.
	SetValues []string
	// ReleaseName is the Helm release name; defaults to "release".
	ReleaseName string
	// Namespace is the target namespace; defaults to "default".
	Namespace string
	// KubeVersion is the Kubernetes version to render against; defaults to
	// DefaultKubeVersion when empty.
	KubeVersion string
}

func (o Options) releaseName() string {
	if o.ReleaseName != "" {
		return o.ReleaseName
	}
	return "release"
}

func (o Options) namespace() string {
	if o.Namespace != "" {
		return o.Namespace
	}
	return "default"
}

// Render loads the chart, merges values with standard Helm precedence, and
// returns the concatenated rendered manifest YAML. Any chart-loading or
// template error is returned verbatim so callers can surface it to the user.
func Render(opts Options) (string, error) {
	chart, err := loader.Load(opts.ChartPath)
	if err != nil {
		return "", fmt.Errorf("loading chart %q: %w", opts.ChartPath, err)
	}

	settings := cli.New()
	valueOpts := &values.Options{
		ValueFiles: opts.ValuesFiles,
		Values:     opts.SetValues,
	}
	vals, err := valueOpts.MergeValues(getter.All(settings))
	if err != nil {
		return "", fmt.Errorf("merging values: %w", err)
	}

	// A client-only dry-run install renders templates without any cluster.
	cfg := &action.Configuration{}
	inst := action.NewInstall(cfg)
	inst.DryRun = true
	inst.ClientOnly = true
	inst.Replace = true
	inst.ReleaseName = opts.releaseName()
	inst.Namespace = opts.namespace()
	inst.IncludeCRDs = true

	// Render against a modern Kubernetes version so charts with a kubeVersion
	// constraint are not rejected by the SDK's legacy default (v1.20.0).
	kubeVersion := opts.KubeVersion
	if kubeVersion == "" {
		kubeVersion = DefaultKubeVersion
	}
	parsed, err := chartutil.ParseKubeVersion(kubeVersion)
	if err != nil {
		return "", fmt.Errorf("parsing kube version %q: %w", kubeVersion, err)
	}
	inst.KubeVersion = parsed

	rel, err := inst.Run(chart, vals)
	if err != nil {
		return "", fmt.Errorf("rendering chart: %w", err)
	}
	return rel.Manifest, nil
}
