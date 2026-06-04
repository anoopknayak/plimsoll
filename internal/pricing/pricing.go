// Package pricing defines the bundled pricing snapshot schema and provides an
// offline loader and lookups. Snapshots are embedded in the binary via go:embed
// so estimation never performs a network request.
package pricing

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed data/*.json
var seedFS embed.FS

// ErrNotFound is returned (wrapped) when a requested cloud, region, or SKU is
// absent from the snapshot. Callers must treat this as an error rather than a
// zero price.
var ErrNotFound = errors.New("not found in pricing snapshot")

// Mode selects which rate to use from a node's price record.
type Mode int

const (
	// OnDemand is the default pay-as-you-go rate.
	OnDemand Mode = iota
	// Spot is the snapshot's last-known spot rate; treated as indicative.
	Spot
	// CommittedUse is the committed-use / reserved rate.
	CommittedUse
)

// Snapshot is a single cloud's dated pricing data.
type Snapshot struct {
	Cloud               string            `json:"cloud"`
	Date                string            `json:"date"`
	Currency            string            `json:"currency"`
	ControlPlaneMonthly float64           `json:"controlPlaneMonthly"`
	Regions             map[string]Region `json:"regions"`
}

// Region holds per-region node, disk, and load-balancer prices.
type Region struct {
	LoadBalancerMonthly float64              `json:"loadBalancerMonthly"`
	Nodes               map[string]NodePrice `json:"nodes"`
	Disks               map[string]DiskPrice `json:"disks"`
}

// NodePrice is a node shape's capacity and hourly rates.
type NodePrice struct {
	VCPU            int     `json:"vcpu"`
	MemBytes        int64   `json:"memBytes"`
	OnDemandHourly  float64 `json:"onDemandHourly"`
	SpotHourly      float64 `json:"spotHourly"`
	CommittedHourly float64 `json:"committedHourly"`
}

// DiskPrice is a disk type's per-GiB monthly rate.
type DiskPrice struct {
	PerGiBMonthly float64 `json:"perGiBMonthly"`
}

// Hourly returns the rate for the given mode and whether it should be treated as
// indicative (true only for spot).
func (n NodePrice) Hourly(mode Mode) (rate float64, indicative bool) {
	switch mode {
	case Spot:
		return n.SpotHourly, true
	case CommittedUse:
		return n.CommittedHourly, false
	default:
		return n.OnDemandHourly, false
	}
}

// Region looks up a region within a snapshot.
func (s Snapshot) Region(name string) (Region, error) {
	r, ok := s.Regions[name]
	if !ok {
		return Region{}, fmt.Errorf("region %q for cloud %q: %w", name, s.Cloud, ErrNotFound)
	}
	return r, nil
}

// Node looks up a node shape within a region.
func (r Region) Node(shape string) (NodePrice, error) {
	n, ok := r.Nodes[shape]
	if !ok {
		return NodePrice{}, fmt.Errorf("node shape %q: %w", shape, ErrNotFound)
	}
	return n, nil
}

// Disk looks up a disk type within a region.
func (r Region) Disk(diskType string) (DiskPrice, error) {
	d, ok := r.Disks[diskType]
	if !ok {
		return DiskPrice{}, fmt.Errorf("disk type %q: %w", diskType, ErrNotFound)
	}
	return d, nil
}

// Catalog holds the loaded snapshots, keyed by cloud.
type Catalog struct {
	snapshots map[string]Snapshot
}

// Load reads the embedded seed snapshots bundled in the binary.
func Load() (*Catalog, error) {
	return LoadFS(seedFS, "data")
}

// LoadFS loads all *.json snapshots from dir within fsys. It is used by Load and
// by tests with alternate fixtures.
func LoadFS(fsys fs.FS, dir string) (*Catalog, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot dir %q: %w", dir, err)
	}
	c := &Catalog{snapshots: map[string]Snapshot{}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %q: %w", e.Name(), err)
		}
		var s Snapshot
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("parsing %q: %w", e.Name(), err)
		}
		if s.Cloud == "" {
			return nil, fmt.Errorf("snapshot %q missing cloud field", e.Name())
		}
		c.snapshots[s.Cloud] = s
	}
	if len(c.snapshots) == 0 {
		return nil, fmt.Errorf("no snapshots found in %q", dir)
	}
	return c, nil
}

// Snapshot returns a cloud's snapshot.
func (c *Catalog) Snapshot(cloud string) (Snapshot, error) {
	s, ok := c.snapshots[cloud]
	if !ok {
		return Snapshot{}, fmt.Errorf("cloud %q: %w", cloud, ErrNotFound)
	}
	return s, nil
}

// Clouds returns the sorted list of clouds present in the catalog.
func (c *Catalog) Clouds() []string {
	out := make([]string, 0, len(c.snapshots))
	for k := range c.snapshots {
		out = append(out, k)
	}
	return out
}
