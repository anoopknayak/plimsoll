# Contributing to plimsoll

Thanks for your interest in improving plimsoll! This guide covers how to set up
the project, the workflow we follow, and the conventions we expect in pull
requests. For an overview of how the code fits together, read the
[architecture guide](docs/architecture.md).

## Prerequisites

- **Go** (a recent stable release; see the `go` directive in [`go.mod`](go.mod)).
- **make** — the developer entry point for build/test/lint.
- **git** on your `PATH` — required only to estimate charts from Git sources.

## Project setup

```sh
git clone https://github.com/anomalyco/plimsoll.git
cd plimsoll
make build      # compiles bin/plimsoll and bin/pricing-gen
```

Run the binary against the bundled sample chart to confirm everything works:

```sh
bin/plimsoll estimate testdata/charts/sample
```

## Everyday commands

All workflows go through `make`:

| Command      | What it does                                                                           |
| ------------ | -------------------------------------------------------------------------------------- |
| `make build` | Compile `bin/plimsoll` and `bin/pricing-gen`.                                          |
| `make test`  | Run the full suite with the race detector and coverage (`go test ./... -race -cover`). |
| `make cover` | Produce a coverage profile and **enforce the coverage gate** (min 70%).                |
| `make lint`  | `gofmt` check plus `go vet`.                                                           |
| `make tidy`  | Sync `go.mod` / `go.sum`.                                                              |
| `make all`   | `lint` + `test` + `build`.                                                             |
| `make clean` | Remove build and coverage artifacts.                                                   |

Before opening a pull request, run `make all` and `make cover` and make sure both
pass.

## Engineering workflow

We build this codebase as senior Go engineers would: idiomatic Go, small
composable packages, clear interfaces, no premature abstraction.

- **TDD from the start.** Write a failing test first, then the minimal code to
  pass, then refactor. Prefer table-driven tests; use golden files for
  rendering/output and deterministic fixtures for pricing.
- **Keep the coverage gate green.** The `make cover` gate enforces a 70% minimum.
- **Small packages.** The pipeline is split into independently tested packages:
  `internal/chartsource`, `internal/render`, `internal/extract`, `internal/model`,
  `internal/pack`, `internal/pricing`, `internal/estimate`, and
  `internal/output`. Keep cloud-specific knowledge in `pricing`; keep the rest
  reasoning over the cloud-neutral `model`.
- **Determinism matters.** Estimation must be reproducible — the same input
  always yields the same output. Avoid introducing nondeterminism (map iteration
  order, wall-clock, network calls) into the estimation path.

## Commit conventions

We use **[Conventional Commits](https://www.conventionalcommits.org/)**, scoped
to the package being changed:

```
feat(pack): select cheapest feasible node shape per cloud
test(extract): cover HPA-derived replica bounds
fix(pricing): reject snapshots failing schema validation
docs: add architecture guide
chore: tidy go.mod
```

Keep an **atomic, sensible git history** — never one massive commit. Commit per
logical unit (one commit per completed task, or a tight test+impl pair), each
building cleanly with tests passing.

## Pull requests

1. Branch from `main`.
2. Make your change following the workflow above.
3. Ensure `make all` and `make cover` pass.
4. Open a PR with a clear description of the _why_, not just the _what_.

If you're changing pricing data, prefer regenerating via `pricing-gen` rather
than hand-editing the embedded snapshots — see the
[reference](docs/reference.md#refreshing-pricing).
