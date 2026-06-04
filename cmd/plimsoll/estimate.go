package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anomalyco/plimsoll/internal/estimate"
	"github.com/anomalyco/plimsoll/internal/extract"
	"github.com/anomalyco/plimsoll/internal/output"
	"github.com/anomalyco/plimsoll/internal/pack"
	"github.com/anomalyco/plimsoll/internal/pricing"
	"github.com/anomalyco/plimsoll/internal/render"
)

// estimateFlags holds the parsed CLI inputs for the estimate command.
type estimateFlags struct {
	valuesFiles  []string
	setValues    []string
	clouds       []string
	regions      []string
	machines     []string
	spot         bool
	committedUse bool
	output       string
}

// newRootCmd builds the plimsoll root command with its subcommands.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "plimsoll",
		Short:         "Estimate a Helm chart's monthly multi-cloud cost without a cluster",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newEstimateCmd())
	return root
}

// newEstimateCmd builds the `plimsoll estimate` command.
func newEstimateCmd() *cobra.Command {
	f := &estimateFlags{}
	cmd := &cobra.Command{
		Use:   "estimate <chart>",
		Short: "Estimate the monthly cost of a Helm chart across clouds",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := pricing.Load()
			if err != nil {
				return fmt.Errorf("loading pricing snapshot: %w", err)
			}
			return runEstimate(cmd.OutOrStdout(), args[0], f, cat)
		},
	}
	flags := cmd.Flags()
	flags.StringArrayVarP(&f.valuesFiles, "values", "f", nil, "values YAML file (repeatable)")
	flags.StringArrayVar(&f.setValues, "set", nil, "inline value override key=value (repeatable)")
	flags.StringSliceVar(&f.clouds, "clouds", nil, "clouds to estimate (default: gcp,aws,azure)")
	flags.StringArrayVar(&f.regions, "region", nil, "region override <cloud>=<region> (repeatable)")
	flags.StringArrayVar(&f.machines, "machine", nil, "node shape override <cloud>=<type> (repeatable)")
	flags.BoolVar(&f.spot, "spot", false, "use spot/preemptible pricing (indicative)")
	flags.BoolVar(&f.committedUse, "committed-use", false, "use committed-use/reserved pricing")
	flags.StringVarP(&f.output, "output", "o", "table", "output format: table, json, or markdown")
	return cmd
}

// runEstimate executes the full pipeline: render → extract → pack → price →
// estimate → output. It is separated from cobra wiring for testability.
func runEstimate(w io.Writer, chartPath string, f *estimateFlags, cat *pricing.Catalog) error {
	if f.spot && f.committedUse {
		return fmt.Errorf("--spot and --committed-use are mutually exclusive")
	}
	format, err := output.ParseFormat(f.output)
	if err != nil {
		return err
	}
	regions, err := parsePairs(f.regions, "region")
	if err != nil {
		return err
	}
	machines, err := parsePairs(f.machines, "machine")
	if err != nil {
		return err
	}

	manifests, err := render.Render(render.Options{
		ChartPath:   chartPath,
		ValuesFiles: f.valuesFiles,
		SetValues:   f.setValues,
	})
	if err != nil {
		return err
	}
	m, err := extract.Extract(manifests)
	if err != nil {
		return err
	}

	res, err := estimate.Estimate(m, cat, estimate.Options{
		Clouds:       f.clouds,
		Regions:      regions,
		Machines:     machines,
		Spot:         f.spot,
		CommittedUse: f.committedUse,
		Overhead:     pack.DefaultOverhead(),
	})
	if err != nil {
		return err
	}
	return output.Render(w, res, format)
}

// parsePairs parses repeatable <cloud>=<value> flag values into a map.
func parsePairs(items []string, flag string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(items))
	for _, it := range items {
		k, v, ok := strings.Cut(it, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !ok || k == "" || v == "" {
			return nil, fmt.Errorf("invalid --%s %q, want <cloud>=<value>", flag, it)
		}
		out[k] = v
	}
	return out, nil
}
