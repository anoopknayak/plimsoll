// Command pricing-gen fetches cloud pricing from the GCP Cloud Billing Catalog,
// AWS Price List, and Azure Retail Prices sources and writes normalized, dated
// snapshot files conforming to the bundled snapshot schema. It runs offline (in
// CI), never on the estimate hot path.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

func main() {
	if err := newGenCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func newGenCmd() *cobra.Command {
	var (
		cloud   string
		region  string
		source  string
		srcURL  string
		outDir  string
		dateStr string
	)
	cmd := &cobra.Command{
		Use:           "pricing-gen",
		Short:         "Generate a normalized, dated pricing snapshot for a cloud/region",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, ok := normalizers[cloud]; !ok {
				return fmt.Errorf("invalid --cloud %q (want gcp, aws, or azure)", cloud)
			}
			if region == "" {
				return fmt.Errorf("--region is required")
			}
			if dateStr == "" {
				dateStr = time.Now().UTC().Format("2006-01-02")
			}

			raw, err := readSource(source, srcURL)
			if err != nil {
				return err
			}
			path, err := generate(cloud, raw, region, dateStr, outDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&cloud, "cloud", "", "cloud to generate: gcp, aws, or azure")
	f.StringVar(&region, "region", "", "region identifier in the snapshot")
	f.StringVar(&source, "source", "", "path to a recorded source JSON file")
	f.StringVar(&srcURL, "url", "", "URL to fetch source pricing JSON from")
	f.StringVar(&outDir, "out", "internal/pricing/data", "output directory for snapshot files")
	f.StringVar(&dateStr, "date", "", "snapshot date (YYYY-MM-DD); defaults to today UTC")
	return cmd
}

// readSource loads source pricing data from a local file or, failing that, a URL.
func readSource(path, url string) ([]byte, error) {
	if path != "" {
		return os.ReadFile(path)
	}
	if url == "" {
		return nil, fmt.Errorf("one of --source or --url is required")
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: status %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
