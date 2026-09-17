# ADR-014: Right-sizing dual-mode pattern (advise vs adjust)

## Status

Accepted

## Context

UC-45 provides fleet right-sizing recommendations based on MCOA metrics (CPU/memory utilisation collected via PrometheusRules on spokes). The question is whether to expose a single command that both reports and applies recommendations, or to separate the read-only and mutating paths.

## Decision

Split right-sizing into two distinct modes:

- **`advise`** — read-only. Collects metrics, computes recommendations, and prints them. No side effects. Safe to run in CI pipelines, cron jobs, or during incident triage.
- **`adjust`** — mutating. Applies the recommendations by patching resource requests/limits on the spoke via ManifestWork. Supports `--dry-run` to preview changes without applying them.

## Consequences

- **Pro:** Clear separation of intent — operators cannot accidentally mutate production workloads when they only wanted a report
- **Pro:** `advise` can run in automated pipelines (dashboards, Slack alerts) with no risk
- **Pro:** `adjust --dry-run` provides an intermediate safety step before full mutation
- **Con:** Two commands where one might suffice — users must learn both
- **Con:** The pattern is unique in this codebase, which may confuse contributors expecting a single `apply` command

## PoC to Production

| PoC | Production |
|-----|------------|
| `advise` prints to stdout | Controller emits recommendations as `PlacementScore` or custom CRD status |
| `adjust` patches via ManifestWork | Controller applies recommendations via VPA integration or direct resource patching with rollback |
| `--dry-run` previews locally | Admission webhook validates right-sizing changes against fleet policy |
