// Package output renders an estimate.Result in human- and machine-readable
// formats: a side-by-side table (default), JSON, and a Markdown table suitable
// for posting as a pull-request comment.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/anomalyco/plimsoll/internal/estimate"
)

// Format selects an output renderer.
type Format string

const (
	// Table is the default human-readable side-by-side layout.
	Table Format = "table"
	// JSON is a machine-readable structured document.
	JSON Format = "json"
	// Markdown is a PR-comment-friendly table.
	Markdown Format = "markdown"
)

// ParseFormat resolves a user-supplied format string (case-insensitive, with a
// "md" alias for Markdown).
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "table":
		return Table, nil
	case "json":
		return JSON, nil
	case "markdown", "md":
		return Markdown, nil
	default:
		return "", fmt.Errorf("unknown output format %q (want table, json, or markdown)", s)
	}
}

// Render writes res to w in the requested format.
func Render(w io.Writer, res estimate.Result, format Format) error {
	switch format {
	case JSON:
		return renderJSON(w, res)
	case Markdown:
		return renderMarkdown(w, res)
	case Table, "":
		return renderTable(w, res)
	default:
		return fmt.Errorf("unknown output format %q", format)
	}
}

func renderJSON(w io.Writer, res estimate.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(res)
}

// snapshotDates returns the distinct, sorted snapshot dates across clouds.
func snapshotDates(res estimate.Result) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range res.Clouds {
		if c.SnapshotDate != "" && !seen[c.SnapshotDate] {
			seen[c.SnapshotDate] = true
			out = append(out, c.SnapshotDate)
		}
	}
	sort.Strings(out)
	return out
}

func money(v float64) string { return fmt.Sprintf("$%.2f", v) }

func costRange(min, max float64) string {
	return fmt.Sprintf("%s - %s", money(min), money(max))
}

func nodeRange(min, max int) string {
	if min == max {
		return fmt.Sprintf("%d", min)
	}
	return fmt.Sprintf("%d-%d", min, max)
}

func renderTable(w io.Writer, res estimate.Result) error {
	fmt.Fprintf(w, "Pricing snapshot: %s\n", strings.Join(snapshotDates(res), ", "))
	if res.Spot {
		fmt.Fprintln(w, "Pricing mode: spot (indicative — spot prices fluctuate)")
	}
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CLOUD\tREGION\tSHAPE\tNODES\tMONTHLY COST (min-max)")
	for _, c := range res.Clouds {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			c.Cloud, c.Region, c.NodeShape,
			nodeRange(c.NodeCountMin, c.NodeCountMax),
			costRange(c.Min.Total(), c.Max.Total()))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Breakdown (max monthly):")
	bw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(bw, "CLOUD\tCOMPUTE\tSTORAGE\tLOAD BALANCER\tCONTROL PLANE\tTOTAL")
	for _, c := range res.Clouds {
		fmt.Fprintf(bw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			c.Cloud, money(c.Max.Compute), money(c.Max.Storage),
			money(c.Max.LoadBalancer), money(c.Max.ControlPlane), money(c.Max.Total()))
	}
	if err := bw.Flush(); err != nil {
		return err
	}

	if len(res.Warnings) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Warnings:")
		for _, msg := range res.Warnings {
			fmt.Fprintf(w, "  - %s\n", msg)
		}
	}
	return nil
}

func renderMarkdown(w io.Writer, res estimate.Result) error {
	fmt.Fprintln(w, "## plimsoll cost estimate")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "_Pricing snapshot: %s_\n", strings.Join(snapshotDates(res), ", "))
	if res.Spot {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "> **Spot pricing is indicative** — spot prices fluctuate and are not guaranteed.")
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "| Cloud | Region | Node shape | Nodes (min-max) | Monthly cost (min-max) |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- |")
	for _, c := range res.Clouds {
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n",
			c.Cloud, c.Region, c.NodeShape,
			nodeRange(c.NodeCountMin, c.NodeCountMax),
			costRange(c.Min.Total(), c.Max.Total()))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "**Breakdown (max monthly):**")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Cloud | Compute | Storage | Load balancer | Control plane | Total |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- | --- |")
	for _, c := range res.Clouds {
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s |\n",
			c.Cloud, money(c.Max.Compute), money(c.Max.Storage),
			money(c.Max.LoadBalancer), money(c.Max.ControlPlane), money(c.Max.Total()))
	}

	if len(res.Warnings) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "**Warnings:**")
		fmt.Fprintln(w)
		for _, msg := range res.Warnings {
			fmt.Fprintf(w, "- %s\n", msg)
		}
	}
	return nil
}
