// Package estimate orchestrates the cost pipeline: it expands the resource model
// into pods, packs them onto each cloud's node shapes, prices the result, and
// produces a per-cloud monthly cost range with a category breakdown.
package estimate

import (
	"fmt"
	"sort"

	"github.com/anomalyco/plimsoll/internal/model"
	"github.com/anomalyco/plimsoll/internal/pack"
	"github.com/anomalyco/plimsoll/internal/pricing"
)

// HoursPerMonth is the normalization constant used to convert hourly rates to a
// monthly figure.
const HoursPerMonth = 730.0

const gib = 1 << 30

// DiskMapping maps Kubernetes storageClass names to a cloud disk type, with a
// default for unknown classes.
type DiskMapping struct {
	ByClass map[string]string
	Default string
}

// Options configure an estimate run.
type Options struct {
	// Clouds to estimate; defaults to all clouds in the catalog when empty.
	Clouds []string
	// Regions overrides the region per cloud; defaults applied otherwise.
	Regions map[string]string
	// Machines overrides the node shape per cloud (the --machine flag).
	Machines map[string]string
	// Spot and CommittedUse select the pricing mode (mutually exclusive; Spot wins).
	Spot         bool
	CommittedUse bool
	// Overhead is the per-node reservation used during packing.
	Overhead pack.Overhead
	// DiskMappings overrides the default storageClass→disk-type mapping per cloud.
	DiskMappings map[string]DiskMapping
}

func (o Options) mode() pricing.Mode {
	switch {
	case o.Spot:
		return pricing.Spot
	case o.CommittedUse:
		return pricing.CommittedUse
	default:
		return pricing.OnDemand
	}
}

// Breakdown holds the monthly cost split across v1 categories.
type Breakdown struct {
	Compute      float64 `json:"compute"`
	Storage      float64 `json:"storage"`
	LoadBalancer float64 `json:"loadBalancer"`
	ControlPlane float64 `json:"controlPlane"`
}

// Total sums the breakdown categories.
func (b Breakdown) Total() float64 {
	return b.Compute + b.Storage + b.LoadBalancer + b.ControlPlane
}

// CloudEstimate is a single cloud's result.
type CloudEstimate struct {
	Cloud        string    `json:"cloud"`
	Region       string    `json:"region"`
	NodeShape    string    `json:"nodeShape"`
	NodeCountMin int       `json:"nodeCountMin"`
	NodeCountMax int       `json:"nodeCountMax"`
	SnapshotDate string    `json:"snapshotDate"`
	Min          Breakdown `json:"min"`
	Max          Breakdown `json:"max"`
}

// Result is the full multi-cloud estimate, shared with the output renderers.
type Result struct {
	Clouds   []CloudEstimate `json:"clouds"`
	Spot     bool            `json:"spot"`
	Warnings []string        `json:"warnings,omitempty"`
}

// defaultRegions are used when a cloud's region isn't specified.
var defaultRegions = map[string]string{
	"gcp":   "us-central1",
	"aws":   "us-east-1",
	"azure": "eastus",
}

// defaultDiskMappings map common storageClass names to each cloud's disk types.
var defaultDiskMappings = map[string]DiskMapping{
	"gcp": {
		ByClass: map[string]string{"standard": "pd-balanced", "standard-rwo": "pd-balanced", "premium-rwo": "pd-ssd"},
		Default: "pd-balanced",
	},
	"aws": {
		ByClass: map[string]string{"standard": "gp3", "gp2": "gp2", "gp3": "gp3"},
		Default: "gp3",
	},
	"azure": {
		ByClass: map[string]string{"standard": "StandardSSD_LRS", "managed-premium": "Premium_LRS", "managed-csi": "StandardSSD_LRS"},
		Default: "Premium_LRS",
	},
}

// Estimate runs the full pipeline and returns a per-cloud cost estimate.
func Estimate(m model.ResourceModel, cat *pricing.Catalog, opts Options) (Result, error) {
	clouds := opts.Clouds
	if len(clouds) == 0 {
		clouds = cat.Clouds()
		sort.Strings(clouds)
	}

	res := Result{Spot: opts.Spot}
	for _, cloud := range clouds {
		ce, warns, err := estimateCloud(m, cat, cloud, opts)
		if err != nil {
			return Result{}, fmt.Errorf("estimating %s: %w", cloud, err)
		}
		res.Clouds = append(res.Clouds, ce)
		res.Warnings = append(res.Warnings, warns...)
	}
	return res, nil
}

func estimateCloud(m model.ResourceModel, cat *pricing.Catalog, cloud string, opts Options) (CloudEstimate, []string, error) {
	snap, err := cat.Snapshot(cloud)
	if err != nil {
		return CloudEstimate{}, nil, err
	}
	region := opts.Regions[cloud]
	if region == "" {
		region = defaultRegions[cloud]
	}
	reg, err := snap.Region(region)
	if err != nil {
		return CloudEstimate{}, nil, err
	}

	mode := opts.mode()
	shapes := nodeShapes(reg, mode)

	maxReq := buildRequest(m, replicaMax, opts.Overhead)
	minReq := buildRequest(m, replicaMin, opts.Overhead)

	// Select the shape sized for peak (max replicas), then pack min onto the same shape.
	var selected pack.Result
	if machine := opts.Machines[cloud]; machine != "" {
		np, err := reg.Node(machine)
		if err != nil {
			return CloudEstimate{}, nil, err
		}
		selected, err = pack.SelectShape(maxReq, toShape(machine, np, mode))
		if err != nil {
			return CloudEstimate{}, nil, err
		}
	} else {
		selected, err = pack.Select(maxReq, shapes)
		if err != nil {
			return CloudEstimate{}, nil, err
		}
	}

	minPack := pack.Pack(minReq, selected.Shape)
	if !minPack.Feasible {
		// Should not happen if max is feasible, but guard anyway.
		minPack = selected
	}

	storage, lbCost, warns := nonComputeCosts(m, reg, cloud, opts)
	controlPlane := snap.ControlPlaneMonthly

	mkBreakdown := func(nodes int) Breakdown {
		return Breakdown{
			Compute:      float64(nodes) * selected.Shape.Hourly * HoursPerMonth,
			Storage:      storage,
			LoadBalancer: lbCost,
			ControlPlane: controlPlane,
		}
	}

	return CloudEstimate{
		Cloud:        cloud,
		Region:       region,
		NodeShape:    selected.Shape.Name,
		NodeCountMin: minPack.Nodes,
		NodeCountMax: selected.Nodes,
		SnapshotDate: snap.Date,
		Min:          mkBreakdown(minPack.Nodes),
		Max:          mkBreakdown(selected.Nodes),
	}, warns, nil
}

// replica selector functions.
func replicaMin(b model.ReplicaBounds) int { return b.Min }
func replicaMax(b model.ReplicaBounds) int { return b.Max }

// buildRequest expands the model into a pack.Request at the chosen replica bound.
func buildRequest(m model.ResourceModel, sel func(model.ReplicaBounds) int, overhead pack.Overhead) pack.Request {
	req := pack.Request{Overhead: overhead}
	for _, w := range m.Workloads {
		agg := pack.Pod{
			Name:     w.Name,
			CPUMilli: w.Pod.TotalCPURequest(),
			MemBytes: w.Pod.TotalMemRequest(),
		}
		if w.Kind == model.KindDaemonSet {
			req.PerNode = append(req.PerNode, agg)
			continue
		}
		n := sel(w.Replicas)
		for i := 0; i < n; i++ {
			p := agg
			p.Name = fmt.Sprintf("%s-%d", w.Name, i)
			req.Pods = append(req.Pods, p)
		}
	}
	return req
}

func nodeShapes(reg pricing.Region, mode pricing.Mode) []pack.NodeShape {
	shapes := make([]pack.NodeShape, 0, len(reg.Nodes))
	for name, np := range reg.Nodes {
		shapes = append(shapes, toShape(name, np, mode))
	}
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].Name < shapes[j].Name })
	return shapes
}

func toShape(name string, np pricing.NodePrice, mode pricing.Mode) pack.NodeShape {
	rate, _ := np.Hourly(mode)
	return pack.NodeShape{Name: name, VCPU: np.VCPU, MemBytes: np.MemBytes, Hourly: rate}
}

// nonComputeCosts prices storage and load balancers and collects warnings.
func nonComputeCosts(m model.ResourceModel, reg pricing.Region, cloud string, opts Options) (storage, lb float64, warns []string) {
	mapping := opts.DiskMappings[cloud]
	if mapping.Default == "" {
		mapping = defaultDiskMappings[cloud]
	}

	for _, v := range m.Volumes {
		diskType, ok := mapping.ByClass[v.StorageClass]
		if !ok {
			diskType = mapping.Default
			warns = append(warns, fmt.Sprintf("%s: storageClass %q not mapped; using default disk type %q", cloud, v.StorageClass, diskType))
		}
		dp, err := reg.Disk(diskType)
		if err != nil {
			warns = append(warns, fmt.Sprintf("%s: disk type %q not priced; skipping volume %q", cloud, diskType, v.Name))
			continue
		}
		gibs := float64(v.SizeBytes) / float64(gib)
		storage += gibs * dp.PerGiBMonthly
	}

	lb = float64(len(m.LoadBalancers)) * reg.LoadBalancerMonthly
	return storage, lb, warns
}
