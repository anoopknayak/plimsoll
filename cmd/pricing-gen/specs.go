package main

// This file holds the static, generator-owned metadata that the cloud pricing
// sources do not (reliably) provide: node-shape capacities, flat control-plane
// and load-balancer fees, and the deterministic discount multipliers used to
// derive committed-use and (where unavailable upstream) spot rates.

const (
	gib = int64(1) << 30

	// committedMultiplier models committed-use / reserved pricing as a stable
	// deterministic discount off on-demand (≈ 1-year commitment).
	committedMultiplier = 0.60
	// awsSpotMultiplier approximates spot pricing for AWS, whose bulk offer file
	// does not include spot rates. Documented as indicative.
	awsSpotMultiplier = 0.33
)

// nodeSpec is a node shape's static capacity.
type nodeSpec struct {
	vcpu     int
	memBytes int64
}

// cloudSpec bundles the generator-owned metadata for one cloud.
type cloudSpec struct {
	nodes               map[string]nodeSpec
	diskTypes           map[string]string // source disk identifier → snapshot disk name
	controlPlaneMonthly float64
	loadBalancerMonthly float64
}

// cloudSpecs holds the per-cloud static metadata used during normalization.
var cloudSpecs = map[string]cloudSpec{
	"gcp": {
		nodes: map[string]nodeSpec{
			"e2-standard-2": {vcpu: 2, memBytes: 8 * gib},
			"e2-standard-4": {vcpu: 4, memBytes: 16 * gib},
		},
		diskTypes: map[string]string{
			"PDStandard": "pd-balanced",
			"SSD":        "pd-ssd",
		},
		controlPlaneMonthly: 72.00,
		loadBalancerMonthly: 18.25,
	},
	"aws": {
		nodes: map[string]nodeSpec{
			"m5.large":  {vcpu: 2, memBytes: 8 * gib},
			"m5.xlarge": {vcpu: 4, memBytes: 16 * gib},
		},
		diskTypes: map[string]string{
			"gp3": "gp3",
			"gp2": "gp2",
		},
		controlPlaneMonthly: 73.00,
		loadBalancerMonthly: 16.43,
	},
	"azure": {
		nodes: map[string]nodeSpec{
			"Standard_D2s_v5": {vcpu: 2, memBytes: 8 * gib},
			"Standard_D4s_v5": {vcpu: 4, memBytes: 16 * gib},
		},
		diskTypes: map[string]string{
			"StandardSSD_LRS": "StandardSSD_LRS",
			"Premium_LRS":     "Premium_LRS",
		},
		controlPlaneMonthly: 0.00, // AKS free standard tier
		loadBalancerMonthly: 18.25,
	},
}

func memGiB(b int64) float64 { return float64(b) / float64(gib) }
