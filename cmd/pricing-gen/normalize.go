package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anomalyco/plimsoll/internal/pricing"
)

// ---------------------------------------------------------------------------
// GCP — Cloud Billing Catalog (services.skus.list)
// ---------------------------------------------------------------------------

type gcpCatalog struct {
	Skus []gcpSku `json:"skus"`
}

type gcpSku struct {
	Description string `json:"description"`
	Category    struct {
		ResourceFamily string `json:"resourceFamily"`
		ResourceGroup  string `json:"resourceGroup"`
		UsageType      string `json:"usageType"`
	} `json:"category"`
	ServiceRegions []string `json:"serviceRegions"`
	PricingInfo    []struct {
		PricingExpression struct {
			UsageUnit   string `json:"usageUnit"`
			TieredRates []struct {
				UnitPrice struct {
					Units string `json:"units"`
					Nanos int64  `json:"nanos"`
				} `json:"unitPrice"`
			} `json:"tieredRates"`
		} `json:"pricingExpression"`
	} `json:"pricingInfo"`
}

func (s gcpSku) unitPrice() (float64, bool) {
	for _, pi := range s.PricingInfo {
		for _, tr := range pi.PricingExpression.TieredRates {
			units, _ := strconv.ParseFloat(tr.UnitPrice.Units, 64)
			return units + float64(tr.UnitPrice.Nanos)/1e9, true
		}
	}
	return 0, false
}

// normalizeGCP assembles per-shape node prices from the catalog's separate
// vCPU-hour and RAM-hour SKUs, plus per-GiB-month disk SKUs.
func normalizeGCP(raw []byte, region, date string) (pricing.Snapshot, error) {
	var cat gcpCatalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		return pricing.Snapshot{}, fmt.Errorf("parsing gcp catalog: %w", err)
	}
	spec := cloudSpecs["gcp"]

	var coreOD, coreSpot, ramOD, ramSpot float64
	disks := map[string]pricing.DiskPrice{}
	for _, sku := range cat.Skus {
		if !contains(sku.ServiceRegions, region) {
			continue
		}
		price, ok := sku.unitPrice()
		if !ok {
			continue
		}
		switch {
		case sku.Category.ResourceFamily == "Compute" && sku.Category.ResourceGroup == "CPU":
			if sku.Category.UsageType == "Preemptible" {
				coreSpot = price
			} else if sku.Category.UsageType == "OnDemand" {
				coreOD = price
			}
		case sku.Category.ResourceFamily == "Compute" && sku.Category.ResourceGroup == "RAM":
			if sku.Category.UsageType == "Preemptible" {
				ramSpot = price
			} else if sku.Category.UsageType == "OnDemand" {
				ramOD = price
			}
		case sku.Category.ResourceFamily == "Storage":
			if name, ok := spec.diskTypes[sku.Category.ResourceGroup]; ok {
				disks[name] = pricing.DiskPrice{PerGiBMonthly: round4(price)}
			}
		}
	}

	if coreOD == 0 || ramOD == 0 {
		return pricing.Snapshot{}, fmt.Errorf("gcp: missing on-demand core/ram rates for region %q", region)
	}

	nodes := map[string]pricing.NodePrice{}
	for name, ns := range spec.nodes {
		od := float64(ns.vcpu)*coreOD + memGiB(ns.memBytes)*ramOD
		spot := float64(ns.vcpu)*coreSpot + memGiB(ns.memBytes)*ramSpot
		nodes[name] = pricing.NodePrice{
			VCPU:            ns.vcpu,
			MemBytes:        ns.memBytes,
			OnDemandHourly:  round4(od),
			SpotHourly:      round4(spot),
			CommittedHourly: round4(od * committedMultiplier),
		}
	}
	return assemble("gcp", region, date, nodes, disks), nil
}

// ---------------------------------------------------------------------------
// AWS — Price List bulk JSON (offer file)
// ---------------------------------------------------------------------------

type awsOffer struct {
	Products map[string]awsProduct                    `json:"products"`
	Terms    map[string]map[string]map[string]awsTerm `json:"terms"`
}

type awsProduct struct {
	SKU           string            `json:"sku"`
	ProductFamily string            `json:"productFamily"`
	Attributes    map[string]string `json:"attributes"`
}

type awsTerm struct {
	PriceDimensions map[string]struct {
		Unit         string            `json:"unit"`
		PricePerUnit map[string]string `json:"pricePerUnit"`
	} `json:"priceDimensions"`
}

func (o awsOffer) onDemandUSD(sku string) (float64, bool) {
	for _, term := range o.Terms["OnDemand"][sku] {
		for _, pd := range term.PriceDimensions {
			if usd, ok := pd.PricePerUnit["USD"]; ok {
				v, err := strconv.ParseFloat(usd, 64)
				if err == nil {
					return v, true
				}
			}
		}
	}
	return 0, false
}

// normalizeAWS maps Compute Instance and Storage products to snapshot prices.
// Spot is derived from on-demand via a documented multiplier, since the bulk
// offer file does not include spot rates.
func normalizeAWS(raw []byte, region, date string) (pricing.Snapshot, error) {
	var offer awsOffer
	if err := json.Unmarshal(raw, &offer); err != nil {
		return pricing.Snapshot{}, fmt.Errorf("parsing aws offer: %w", err)
	}
	spec := cloudSpecs["aws"]

	nodes := map[string]pricing.NodePrice{}
	disks := map[string]pricing.DiskPrice{}
	for _, p := range offer.Products {
		a := p.Attributes
		if a["regionCode"] != region {
			continue
		}
		switch p.ProductFamily {
		case "Compute Instance":
			ns, ok := spec.nodes[a["instanceType"]]
			if !ok || a["operatingSystem"] != "Linux" || a["tenancy"] != "Shared" ||
				a["preInstalledSw"] != "NA" || a["capacitystatus"] != "Used" {
				continue
			}
			od, ok := offer.onDemandUSD(p.SKU)
			if !ok {
				continue
			}
			nodes[a["instanceType"]] = pricing.NodePrice{
				VCPU:            ns.vcpu,
				MemBytes:        ns.memBytes,
				OnDemandHourly:  round4(od),
				SpotHourly:      round4(od * awsSpotMultiplier),
				CommittedHourly: round4(od * committedMultiplier),
			}
		case "Storage":
			name, ok := spec.diskTypes[a["volumeApiName"]]
			if !ok {
				continue
			}
			od, ok := offer.onDemandUSD(p.SKU)
			if !ok {
				continue
			}
			disks[name] = pricing.DiskPrice{PerGiBMonthly: round4(od)}
		}
	}
	return assemble("aws", region, date, nodes, disks), nil
}

// ---------------------------------------------------------------------------
// Azure — Retail Prices API
// ---------------------------------------------------------------------------

type azureRetail struct {
	Items []azureItem `json:"Items"`
}

type azureItem struct {
	ArmRegionName string  `json:"armRegionName"`
	ArmSkuName    string  `json:"armSkuName"`
	RetailPrice   float64 `json:"retailPrice"`
	UnitOfMeasure string  `json:"unitOfMeasure"`
	Type          string  `json:"type"`
	ServiceName   string  `json:"serviceName"`
	SkuName       string  `json:"skuName"`
	MeterName     string  `json:"meterName"`
}

// normalizeAzure reads VM consumption (on-demand and Spot) and per-GiB/month
// managed-disk meters.
func normalizeAzure(raw []byte, region, date string) (pricing.Snapshot, error) {
	var retail azureRetail
	if err := json.Unmarshal(raw, &retail); err != nil {
		return pricing.Snapshot{}, fmt.Errorf("parsing azure retail: %w", err)
	}
	spec := cloudSpecs["azure"]

	nodes := map[string]pricing.NodePrice{}
	disks := map[string]pricing.DiskPrice{}

	// First pass: on-demand and spot hourly rates per arm SKU.
	type rates struct{ od, spot float64 }
	vm := map[string]*rates{}
	get := func(sku string) *rates {
		if vm[sku] == nil {
			vm[sku] = &rates{}
		}
		return vm[sku]
	}
	for _, it := range retail.Items {
		if it.ArmRegionName != region {
			continue
		}
		switch {
		case it.ServiceName == "Virtual Machines" && it.Type == "Consumption":
			if _, ok := spec.nodes[it.ArmSkuName]; !ok {
				continue
			}
			if strings.Contains(strings.ToLower(it.SkuName), "spot") {
				get(it.ArmSkuName).spot = it.RetailPrice
			} else {
				get(it.ArmSkuName).od = it.RetailPrice
			}
		case it.ServiceName == "Storage" && strings.Contains(it.UnitOfMeasure, "GB/Month"):
			if name, ok := spec.diskTypes[it.ArmSkuName]; ok {
				disks[name] = pricing.DiskPrice{PerGiBMonthly: round4(it.RetailPrice)}
			}
		}
	}
	for armSku, r := range vm {
		ns := spec.nodes[armSku]
		nodes[armSku] = pricing.NodePrice{
			VCPU:            ns.vcpu,
			MemBytes:        ns.memBytes,
			OnDemandHourly:  round4(r.od),
			SpotHourly:      round4(r.spot),
			CommittedHourly: round4(r.od * committedMultiplier),
		}
	}
	return assemble("azure", region, date, nodes, disks), nil
}

// ---------------------------------------------------------------------------
// Shared assembly, validation, and writer
// ---------------------------------------------------------------------------

// normalizers dispatches to the per-cloud normalizer.
var normalizers = map[string]func(raw []byte, region, date string) (pricing.Snapshot, error){
	"gcp":   normalizeGCP,
	"aws":   normalizeAWS,
	"azure": normalizeAzure,
}

func assemble(cloud, region, date string, nodes map[string]pricing.NodePrice, disks map[string]pricing.DiskPrice) pricing.Snapshot {
	spec := cloudSpecs[cloud]
	return pricing.Snapshot{
		Cloud:               cloud,
		Date:                date,
		Currency:            "USD",
		ControlPlaneMonthly: spec.controlPlaneMonthly,
		Regions: map[string]pricing.Region{
			region: {
				LoadBalancerMonthly: spec.loadBalancerMonthly,
				Nodes:               nodes,
				Disks:               disks,
			},
		},
	}
}

// validateSnapshot enforces the snapshot schema invariants so a malformed source
// can never be written as a snapshot.
func validateSnapshot(s pricing.Snapshot) error {
	if s.Cloud == "" {
		return fmt.Errorf("snapshot missing cloud")
	}
	if s.Date == "" {
		return fmt.Errorf("%s: snapshot missing date", s.Cloud)
	}
	if len(s.Regions) == 0 {
		return fmt.Errorf("%s: snapshot has no regions", s.Cloud)
	}
	for rn, reg := range s.Regions {
		if len(reg.Nodes) == 0 {
			return fmt.Errorf("%s/%s: no node shapes priced", s.Cloud, rn)
		}
		for name, n := range reg.Nodes {
			if n.VCPU <= 0 || n.MemBytes <= 0 {
				return fmt.Errorf("%s/%s/%s: non-positive capacity", s.Cloud, rn, name)
			}
			if n.OnDemandHourly <= 0 {
				return fmt.Errorf("%s/%s/%s: non-positive on-demand price", s.Cloud, rn, name)
			}
		}
		for name, d := range reg.Disks {
			if d.PerGiBMonthly <= 0 {
				return fmt.Errorf("%s/%s/%s: non-positive disk price", s.Cloud, rn, name)
			}
		}
	}
	return nil
}

// generate normalizes a source, validates it, and only on success writes a
// dated snapshot file to outDir, returning the written path.
func generate(cloud string, raw []byte, region, date, outDir string) (string, error) {
	norm, ok := normalizers[cloud]
	if !ok {
		return "", fmt.Errorf("unknown cloud %q", cloud)
	}
	snap, err := norm(raw, region, date)
	if err != nil {
		return "", err
	}
	if err := validateSnapshot(snap); err != nil {
		return "", fmt.Errorf("schema validation failed: %w", err)
	}
	path := filepath.Join(outDir, cloud+".json")
	if err := writeSnapshot(path, snap); err != nil {
		return "", err
	}
	return path, nil
}

func writeSnapshot(path string, snap pricing.Snapshot) error {
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling snapshot: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func round4(v float64) float64 {
	return float64(int64(v*1e4+0.5)) / 1e4
}
