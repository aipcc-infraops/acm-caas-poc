# ADR-015: ManifestWork as universal spoke delivery mechanism

## Status

Accepted

## Context

Multiple use cases need to deliver arbitrary Kubernetes resources to spoke clusters: VMs via KubeVirt CRDs (UC-51), GPU stacks with Kueue and Kyverno (UC-18), custom admission policies (UC-29), Velero backup configurations (UC-42), and observability ConfigMaps (UC-52). ACM provides three delivery mechanisms:

1. **ManifestWork** — wraps raw manifests, delivered directly by the klusterlet work agent
2. **Policy (ConfigurationPolicy)** — desired-state governance with audit loop and compliance reporting
3. **ApplicationSet** — GitOps-driven delivery via Argo CD, requires a Git repository as source of truth

## Decision

Use ManifestWork as the primary mechanism for delivering spoke-bound resources across all use cases. Resources are constructed programmatically in `builder.go` files and wrapped in ManifestWork objects created in the spoke's namespace on the hub.

## Consequences

- **Pro:** Direct delivery with no intermediary controller — resources appear on the spoke within seconds
- **Pro:** Status feedback via ManifestWork conditions — the hub knows whether resources were applied successfully
- **Pro:** Engine-agnostic — works for any CRD (KubeVirt, Kueue, Kyverno, Velero, core resources) without additional dependencies
- **Pro:** No Git repository required — resources are constructed at runtime from CLI parameters
- **Con:** No continuous drift detection — if someone modifies or deletes the resource on the spoke, ManifestWork does not reconcile unless `updateStrategy` is configured
- **Con:** No compliance reporting — Policy provides audit trails and fleet-wide compliance status; ManifestWork does not
- **Con:** Resources are coupled to the ManifestWork lifecycle — deleting the ManifestWork removes all contained resources from the spoke

## When to use Policy instead

Policy is used in this PoC for governance-oriented use cases where compliance state matters: image registry restrictions (UC-02), resource quota enforcement (UC-15), operator version pinning (UC-27), certificate expiry detection (UC-28). The distinction is intent: ManifestWork delivers, Policy governs.

## PoC to Production

| PoC | Production |
|-----|------------|
| ManifestWork per resource per cluster | ManifestWorkReplicaSet for fleet-wide delivery with rollout strategy |
| No drift detection | `updateStrategy: ServerSideApply` with `fieldManager` for continuous reconciliation |
| Builder constructs resources in Go maps | Controller reconciles from a declarative CRD spec |
| CLI creates ManifestWork directly | ComputeRequest controller manages ManifestWork lifecycle with garbage collection |
