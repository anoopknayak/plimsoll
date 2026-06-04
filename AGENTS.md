# Project: plimsoll

A cluster-less CLI that estimates a Helm chart's monthly cost across GCP, AWS,
and Azure by bin-packing pods onto real node shapes using a bundled pricing
snapshot. Built in Go.

## Conventions to remember

### Version control
- **Do NOT check in OpenSpec material.** The `openspec/`, `.agent/`, `.opencode/`,
  and `.pi/` directories are intentionally gitignored and must stay out of version
  control. Planning/spec artifacts live locally only.
- **Atomic, sensible git history.** Never a single massive commit. Commit per
  logical unit (one commit per completed task, or a tight test+impl pair), each
  building cleanly with tests passing.
- **Conventional Commits**, scoped to the package, e.g. `feat(pack): ...`,
  `test(extract): ...`, `fix(pricing): ...`, `chore: ...`, `docs: ...`.

### Engineering
- **TDD from the start** — write failing tests first, then minimal code, then
  refactor. Table-driven tests; golden files for rendering/output; deterministic
  fixtures for pricing.
- Persona: senior Go engineer (10+ yrs) building high-performance, maintainable
  CLIs with deep multi-cloud + Kubernetes expertise. Idiomatic Go, small
  composable packages, clear interfaces, no premature abstraction.
