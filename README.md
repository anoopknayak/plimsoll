# plimsoll

**Know what a Helm chart will cost on GCP, AWS, and Azure — before you deploy it,
without a cluster, and without going online.**

[![CI](https://github.com/anomalyco/plimsoll/actions/workflows/ci.yml/badge.svg)](https://github.com/anomalyco/plimsoll/actions/workflows/ci.yml)

```
render → extract → pack → price → estimate → output
```

## Why plimsoll?

"How much will this run me?" is one of the first questions you ask about a
workload — and one of the hardest to answer early. The usual options are to
deploy it and wait for a bill, or to model it by hand in a spreadsheet across
three different cloud pricing pages. Both are slow, and neither lets you compare
clouds apples-to-apples.

plimsoll answers the question from the chart itself. Point it at a Helm chart and
it renders the chart locally, works out the pods/volumes/load balancers,
bin-packs the pods onto real node shapes, and prices the result against a
bundled, dated price snapshot — **entirely offline**. You get a per-cloud monthly
estimate with a cost breakdown in seconds, so cost becomes something you can
check in code review instead of discover on an invoice.

- **No cluster required** — it renders and reasons locally.
- **Offline & deterministic** — prices are embedded; the same input always gives
  the same answer (great for CI and PR comments).
- **Multi-cloud, side by side** — GCP, AWS, and Azure in one run.
- **Honest about assumptions** — every methodology choice is documented.

## Install

```sh
git clone https://github.com/anomalyco/plimsoll.git
cd plimsoll
make build            # builds bin/plimsoll (and bin/pricing-gen)
# or
go build -o bin/plimsoll ./cmd/plimsoll
```

## Quickstart

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
```

The `<chart>` argument also accepts remote references — Git repos, HTTP `.tgz`
archives, and `oci://` registries:

```sh
plimsoll estimate oci://registry.example.com/charts/app:1.2.3 --clouds gcp -o markdown
```

For the full flag list, every chart-source form, the assumptions behind the
numbers, the node-shape catalog, and how to refresh pricing, see the
**[reference](docs/reference.md)**.

## Documentation

- **[Reference](docs/reference.md)** — flags, chart sources, methodology, node
  catalog, storage mapping, pricing refresh.
- **[Architecture](docs/architecture.md)** — how the pipeline and packages fit
  together.
- **[Contributing](CONTRIBUTING.md)** — setup, TDD workflow, commit conventions,
  and `make` targets.

## Inspirations

- **[Helm](https://helm.sh) & the Helm Go SDK** — plimsoll renders charts the
  same way Helm does, locally, so estimates match what you'd actually deploy.
- **Cloud cost tools like [Infracost](https://www.infracost.io) and
  [OpenCost](https://www.opencost.io)/kubecost** — for the idea of bringing cost
  visibility into the developer workflow instead of the monthly invoice.
- **Classic bin-packing (first-fit-decreasing)** — the well-worn scheduling
  heuristic plimsoll uses to place pods onto node shapes deterministically.

## Star the project

If plimsoll saves you a spreadsheet or a surprise bill, please **give it a ⭐ on
GitHub** — it helps others find the project and tells us it's worth investing in.

## Development

```sh
make test    # go test ./... -race -cover
make cover   # enforce the coverage gate (min 70%)
make lint    # gofmt check + go vet
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full workflow.
