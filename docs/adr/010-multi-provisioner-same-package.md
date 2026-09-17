# ADR-010: Multi-provisioner architecture in a single package

## Status

Accepted

## Context

The PoC supports three cluster provisioning backends: Hive (OpenShift via ClusterDeployment), HyperShift (hosted control planes via HostedCluster), and CAPI (vanilla Kubernetes via cluster.x-k8s.io). Each backend uses different GVRs, different resource structures, and different lifecycle semantics.

Two approaches were considered: (a) a strategy interface with three implementations in separate packages, or (b) separate methods on the same Manager in `internal/provisioning/`.

## Decision

Keep all three backends in `internal/provisioning/` with distinct methods: `Create` (Hive), `CreateHyperShift`, `CreateCAPI`. Each backend has its own file pair (`provisioning.go`/`builder.go`, `hypershift.go`/`hypershift_builder.go`, `capi.go`/`capi_builder.go`).

## Consequences

- **Pro:** Single Manager, single client, single config: CLI commands share the `--type` flag to select the backend
- **Pro:** Shared helpers (namespace creation, pull secret, ManagedCluster registration) are reused without cross-package imports
- **Pro:** Each backend's logic is isolated in its own file, keeping files focused
- **Con:** The Manager grows three sets of methods instead of one interface: `Create`/`Destroy`/`Status`/`List` plus `CreateHyperShift`/`DestroyHyperShift`/`StatusHyperShift`/`ListHyperShift` plus `CreateCAPI`/`DestroyCAPI`/`StatusCAPI`/`ListCAPI`
- **Con:** No polymorphism: callers must know which backend to call

## Why Not a Strategy Interface

A `Provisioner` interface with `Create(opts)` would require a unified options struct that either uses empty fields (HyperShift has no InstallConfig, CAPI has no PullSecret requirement) or wraps backend-specific options in an `interface{}`. Both leak complexity. The backends share a Manager and config but not behaviour: Hive creates a ClusterDeployment, HyperShift creates a HostedCluster + NodePool, CAPI creates a Cluster + MachineDeployment. Forcing them into one signature hides more than it reveals.

When the code graduates to the ComputeRequest controller, a `Provisioner` interface may make sense because the controller reconciles a single CRD and dispatches based on `spec.type`. At the PoC stage, explicit methods are clearer.
