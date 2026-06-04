# plimsoll

`plimsoll` estimates a Helm chart's **monthly cost across GCP, AWS, and Azure**
without a cluster. It renders the chart locally (via the Helm Go SDK), extracts
the workloads, volumes, and load balancers into a cloud-neutral model,
bin-packs the pods onto real node shapes, and prices the result from a bundled,
dated pricing snapshot — entirely offline.

```
render → extract → pack → price → estimate → output
```

## Install / build

```sh
make build            # builds bin/plimsoll and bin/pricing-gen
# or
go build -o bin/plimsoll ./cmd/plimsoll
```

## Usage

```sh
plimsoll estimate <chart> [flags]
```

Estimate the bundled sample chart across all three clouds:

```sh
plimsoll estimate testdata/charts/sample
```

```
Pricing snapshot: 2026-06-01

CLOUD  REGION       SHAPE            NODES  MONTHLY COST (min-max)
aws    us-east-1    m5.2xlarge       1      $373.75 - $373.75
azure  eastus       Standard_D2s_v5  2-4    $162.16 - $302.32
gcp    us-central1  e2-standard-2    2-4    $194.09 - $291.92

Breakdown (max monthly):
CLOUD  COMPUTE  STORAGE  LOAD BALANCER  CONTROL PLANE  TOTAL
aws    $280.32  $4.00    $16.43         $73.00         $373.75
azure  $280.32  $3.75    $18.25         $0.00          $302.32
gcp    $195.66  $5.00    $18.26         $73.00         $291.92
```

### Flags

| Flag | Description |
| --- | --- |
| `-f, --values <file>` | Values YAML file, applied in order (repeatable). |
| `--set <key=value>` | Inline value override (repeatable). |
| `--clouds <list>` | Comma-separated clouds to estimate. Default: `gcp,aws,azure`. |
| `--region <cloud>=<region>` | Region override per cloud (repeatable). |
| `--machine <cloud>=<type>` | Node-shape override per cloud (repeatable). |
| `--spot` | Use spot/preemptible pricing (**indicative**). |
| `--committed-use` | Use committed-use / reserved pricing. |
| `-o, --output <format>` | Output format: `table` (default), `json`, or `markdown`. |

`--spot` and `--committed-use` are mutually exclusive. A missing chart path or
an invalid flag value prints an error and exits non-zero.

### Chart sources

The `<chart>` argument accepts a local path or a remote reference. Remote
charts are materialized into a temporary directory and removed when the command
returns — estimation itself still runs entirely offline once the chart is local.

| Form | Example |
| --- | --- |
| Local directory | `./mychart` |
| Local packaged chart | `mychart-1.2.3.tgz` |
| Git repository | `git+https://github.com/org/repo.git#v1.4.0?path=charts/app` |
| Git (bare `.git` / SCP) | `https://github.com/org/repo.git`, `git@github.com:org/repo.git` |
| HTTP(S) archive | `https://example.com/charts/app-1.2.3.tgz` |
| OCI registry | `oci://registry.example.com/charts/app:1.2.3` |

For Git sources, `#<ref>` selects a branch, tag, or commit and `?path=<subdir>`
selects a sub-chart within the repo. Git sources require the `git` binary on
`PATH`; if it is missing, `plimsoll` reports a clear error — install git or
pre-clone the chart and pass a local path. A plain Helm-repository URL with no
chart (e.g. `https://charts.example.com`) is rejected; use an `oci://`
reference or a direct `.tgz` URL instead.

```sh
# Estimate a chart living in a Git repo sub-directory at a tag
plimsoll estimate git+https://github.com/org/repo.git#v1.4.0?path=charts/app --clouds gcp

# Estimate a chart pulled from an OCI registry
plimsoll estimate oci://registry.example.com/charts/app:1.2.3
```

Examples:

```sh
# JSON for a single cloud, custom region and machine type
plimsoll estimate ./mychart --clouds gcp --region gcp=europe-west1 \
  --machine gcp=e2-standard-4 -o json

# Markdown table to post on a pull request, using committed-use pricing
plimsoll estimate ./mychart --committed-use -o markdown
```

## Assumptions & methodology

These assumptions keep the estimate deterministic and comparable across clouds.
They are intentionally conservative; treat the output as a planning estimate,
not a billing guarantee.

- **Node bin-packing.** Pods are placed with a deterministic, overhead-aware
  first-fit-decreasing (FFD) algorithm. The cheapest feasible node shape per
  cloud is selected automatically; override it with `--machine`.
- **Per-node overhead.** Each node reserves **250m CPU** and **768Mi memory**
  for the kubelet, system daemons, and OS before pods are placed. DaemonSet
  pods are reserved on every node.
- **Replica range (min–max).** Workloads with an HPA produce a range from
  `minReplicas` to `maxReplicas`; the node shape is sized for peak (max) and the
  minimum is packed onto that same shape. Static workloads collapse to a single
  figure. Storage, load-balancer, and control-plane costs are held constant
  across the range; only compute varies.
- **Monthly normalization.** Hourly rates are multiplied by **730 hours/month**.
- **Cost categories (v1).** Compute (packed nodes × node rate) + PVC storage
  (per-GiB-month) + LoadBalancer (flat per-service monthly fee) + control plane
  (flat per-cluster fee). Network egress and managed add-ons are out of scope.
- **Pricing modes.** On-demand by default. `--committed-use` applies a
  committed-use rate; `--spot` uses the snapshot's last-known spot value and is
  always labelled **indicative — spot prices fluctuate**.
- **Snapshot freshness.** Prices come from a bundled, dated snapshot embedded in
  the binary. The snapshot date is **always shown** in every output format. No
  network request is made during estimation.

## Node-shape catalog

The bundled snapshot (dated `2026-06-01`) ships one region per cloud with these
candidate shapes (auto-selection picks the cheapest feasible one):

| Cloud | Default region | Node shapes | Control plane / mo |
| --- | --- | --- | --- |
| GCP | `us-central1` | `e2-standard-2`, `e2-standard-4`, `e2-standard-8` | $73.00 |
| AWS | `us-east-1` | `m5.large`, `m5.xlarge`, `m5.2xlarge` | $73.00 |
| Azure | `eastus` | `Standard_D2s_v5`, `Standard_D4s_v5`, `Standard_D8s_v5` | $0.00 (AKS free tier) |

## storageClass → disk-type mapping

Each PVC's `storageClass` is mapped to a cloud disk type and priced per
GiB-month. Unmapped classes fall back to the per-cloud default and emit a
warning. Defaults:

| storageClass | GCP | AWS | Azure |
| --- | --- | --- | --- |
| `standard` | `pd-balanced` | `gp3` | `StandardSSD_LRS` |
| `standard-rwo` | `pd-balanced` | — | — |
| `premium-rwo` | `pd-ssd` | — | — |
| `gp2` / `gp3` | — | `gp2` / `gp3` | — |
| `managed-premium` | — | — | `Premium_LRS` |
| `managed-csi` | — | — | `StandardSSD_LRS` |
| _default (unmapped)_ | `pd-balanced` | `gp3` | `Premium_LRS` |

Available disk types in the snapshot: GCP `pd-standard`/`pd-balanced`/`pd-ssd`,
AWS `gp2`/`gp3`/`io2`, Azure `StandardSSD_LRS`/`Premium_LRS`/`UltraSSD_LRS`.

## Refreshing pricing

Estimation never hits the network; prices live in `internal/pricing/data/<cloud>.json`,
embedded via `go:embed`. The separate `pricing-gen` tool regenerates them from
upstream sources (GCP Cloud Billing Catalog, AWS Price List, Azure Retail Prices):

```sh
pricing-gen --cloud azure --region eastus \
  --url "https://prices.azure.com/api/retail/prices?..." \
  --out internal/pricing/data
# or from a recorded source file:
pricing-gen --cloud aws --region us-east-1 --source aws_offer.json
```

`pricing-gen` validates the normalized snapshot against the schema and refuses
to write an invalid one. The `pricing-refresh` GitHub Actions workflow runs this
monthly and opens a pull request with the diff for review. See
`make refresh-pricing` for the wiring.

## Development

```sh
make test    # go test ./... -race -cover
make cover   # enforce the coverage gate (min 70%)
make lint    # gofmt check + go vet
```

The pipeline is split into small, independently tested packages:
`internal/chartsource`, `internal/render`, `internal/extract`, `internal/model`,
`internal/pack`, `internal/pricing`, `internal/estimate`, and `internal/output`.
