// Command plimsoll estimates a Helm chart's monthly cost across GCP, AWS, and
// Azure without a cluster, using a bundled pricing snapshot.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
