# ADR-008: ConfigMap-based state machine for multi-step workflows

## Status

Accepted

## Context

UC-08 (cluster decommissioning) requires a multi-step lifecycle: import → audit → notify → backup → drain → delete → cleanup. Each step can fail or be interrupted, and the workflow must resume from the last completed step. The PoC needs a persistence mechanism that works today from the CLI and can be adopted by a Kubernetes controller later without changing the data model.

Options considered:

1. **ConfigMap in the hub cluster namespace** — labels identify the workflow, data keys hold phase and history
2. **Custom Resource Definition (CRD)** — purpose-built resource with status subresource
3. **Local file on the operator's workstation** — JSON/YAML state file

## Decision

Use a ConfigMap in the cluster's hub namespace (e.g., `<cluster>-decommission` in namespace `<cluster>`) with a label selector (`caas-poc/workflow=decommission`) for listing. The ConfigMap stores:

- `phase` — current lifecycle step (imported, audited, notified, backed-up, drained, deleted, cleaned)
- `history` — JSON array of `{phase, timestamp, message}` entries
- `owner` — cluster owner identifier
- `deadline` — ISO 8601 reclaim deadline
- `audit` — JSON audit report snapshot

## Consequences

- **Pro:** No CRD registration — works immediately on any hub cluster
- **Pro:** A future controller watches ConfigMaps with the workflow label — same data model, different trigger
- **Pro:** Standard RBAC — no extra API aggregation or webhook infrastructure
- **Pro:** Hub-native — lives alongside ACM resources in the same namespace
- **Pro:** `kubectl get cm -l caas-poc/workflow=decommission` lists all active workflows
- **Con:** No status subresource — phase transitions are not validated by the API server
- **Con:** ConfigMap data is limited to 1 MiB — sufficient for decommission state but not for large payloads
- **Con:** No schema enforcement — field names are conventions, not enforced by a CRD spec

## Migration Path

When graduating to a controller, the ConfigMap data model maps directly to a CRD:

```
ConfigMap.data.phase        → DecommissionRequest.status.phase
ConfigMap.data.deadline     → DecommissionRequest.spec.deadline
ConfigMap.data.owner        → DecommissionRequest.spec.owner
ConfigMap.data.history      → DecommissionRequest.status.history
```

The `internal/decommission` package functions remain unchanged — only the state read/write layer is swapped.
