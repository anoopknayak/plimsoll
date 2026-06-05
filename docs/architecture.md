# Architecture

plimsoll turns a Helm chart into a multi-cloud monthly cost estimate **without a
cluster** and **without network access** at estimation time. It does this by
rendering the chart locally, reducing it to a small cloud-neutral model, and
running that model through a deterministic costing pipeline.

```
chartsource → render → extract → model → pack → pricing → estimate → output
```

Each stage is a small, independently tested package under `internal/`. The
`model` package sits at the center as the shared contract: extraction produces a
`model.ResourceModel`; packing, pricing, and estimation consume it.

## The pipeline

```
 <chart ref>
     │
     ▼
┌──────────────┐   local path (dir or .tgz), temp-dir lifecycle
│ chartsource  │   resolves local/Git/HTTP/OCI references
└──────┬───────┘
       ▼
┌──────────────┐   Helm Go SDK templating against a pinned kubeVersion
│   render     │   → rendered Kubernetes manifests (YAML)
└──────┬───────┘
       ▼
┌──────────────┐   parse manifests, pull out cost-driving resources
│   extract    │   → model.ResourceModel
└──────┬───────┘
       ▼
┌──────────────┐   provider-agnostic: Workloads, Volumes, LoadBalancers
│    model     │   the central contract every later stage reads
└──────┬───────┘
       ▼
┌──────────────┐   FFD bin-packing of pods onto candidate node shapes,
│    pack      │   overhead-aware, cheapest feasible shape per cloud
└──────┬───────┘
       ▼
┌──────────────┐   bundled, dated price snapshot embedded via go:embed
│   pricing    │   node hourly rates, disk $/GiB-mo, LB + control-plane fees
└──────┬───────┘
       ▼
┌──────────────┐   compose compute + storage + LB + control plane into a
│   estimate   │   per-cloud monthly min–max range with a breakdown
└──────┬───────┘
       ▼
┌──────────────┐   table (default), json, or markdown
│   output     │   always stamped with the pricing-snapshot date
└──────────────┘
```

## Packages

### `internal/chartsource`
Resolves the `<chart>` argument into a local path the renderer can load. A
`Resolver` owns the temporary-directory lifecycle and dispatches remote kinds
(Git, HTTP archive, OCI, Helm repo) to per-kind fetchers behind a small
`fetcher` interface. Local paths pass through untouched; remote charts are
materialized into a temp dir that is removed when the command returns. After
this stage everything is local, so the rest of the pipeline runs offline.

### `internal/render`
Templates the chart with the Helm Go SDK against a pinned, recent-stable
Kubernetes version (so charts that declare a `kubeVersion` floor render), and
returns the rendered manifests. The version is overridable via `--kube-version`.

### `internal/extract`
Parses the rendered manifests and reduces them to the cost-driving essentials,
producing a `model.ResourceModel`. Controllers become workloads, PVCs become
volumes, and `type: LoadBalancer` services become load balancers. Everything
that doesn't affect cost is dropped here.

### `internal/model`
The provider-agnostic contract at the heart of plimsoll. A `ResourceModel`
holds `Workloads` (compute), `Volumes` (storage), and `LoadBalancers`
(networking). A `Workload` carries its `ReplicaBounds` (min/max — from an HPA, or
the static replica count when there's no HPA) and a pod template. Keeping this
provider-neutral lets the costing logic be written once and parameterized by
cloud.

### `internal/pack`
Simulates scheduling. Pods are placed onto candidate `NodeShape`s with a
deterministic, overhead-aware **first-fit-decreasing (FFD)** algorithm, and the
cheapest feasible shape per cloud is selected. A per-node `Overhead`
(default **250m CPU / 768Mi memory**) is reserved for kube-reserved and system
DaemonSets before pods are placed; DaemonSet pods are reserved on every node.
Determinism is a hard requirement — the same input always yields the same
packing.

### `internal/pricing`
Owns the bundled price data. Snapshots live in
`internal/pricing/data/<cloud>.json` and are compiled into the binary via
`go:embed`, so **no network request is made during estimation**. Each snapshot
is dated, and that date is surfaced in every output. The data is regenerated
out-of-band by the separate `pricing-gen` tool (see below).

### `internal/estimate`
Orchestrates the costing. It expands the `ResourceModel` into concrete pods,
packs them onto each cloud's node shapes, prices the result, and composes the
four v1 cost categories — **compute** (packed nodes × node rate), **storage**
(PVC GiB × disk $/GiB-month), **load balancer** (flat per-service fee), and
**control plane** (flat per-cluster fee) — into a per-cloud monthly min–max
range with a breakdown. Hourly rates are normalized with `HoursPerMonth = 730`.

### `internal/output`
Renders the estimate as a `table` (default), `json`, or `markdown`, always
stamping the pricing-snapshot date so a reader knows how fresh the numbers are.

## Key design decisions

- **Cloud-neutral model.** All cloud-specific knowledge lives in `pricing` and
  the node-shape catalog; `extract → pack → estimate` reason over the neutral
  `model`. Adding a cloud is mostly a pricing-data concern.
- **Determinism over cleverness.** FFD bin-packing with fixed sort order makes
  estimates reproducible and diffable in CI, which matters more than squeezing
  out the theoretical optimum packing.
- **Offline by construction.** Pricing is embedded at build time. The only
  network access is optional chart fetching in `chartsource`; estimation never
  touches the network.
- **Conservative, documented assumptions.** Per-node overhead, the 730-hour
  month, and the v1 cost categories are explicit so the output is a defensible
  planning estimate, not a black box. See the
  [reference](reference.md#assumptions--methodology).

## The `pricing-gen` tool

`cmd/pricing-gen` is a separate binary that regenerates the embedded snapshots
from upstream sources (GCP Cloud Billing Catalog, AWS Price List, Azure Retail
Prices). It normalizes each source into the snapshot schema and refuses to write
an invalid snapshot. The `pricing-refresh` GitHub Actions workflow runs it
monthly and opens a pull request with the diff for review. Estimation and price
generation are intentionally decoupled: the `plimsoll` binary only ever reads the
committed snapshot. See the [reference](reference.md#refreshing-pricing) for
invocation details.
