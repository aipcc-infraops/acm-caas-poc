# ADR-011: CAPI hibernate via annotation-backed scale-to-zero

## Status

Accepted

## Context

Hive-provisioned OpenShift clusters support native hibernation via `ClusterDeployment.spec.powerState = Hibernating`. This stops all cloud VMs and resumes them later. CAPI-provisioned vanilla Kubernetes clusters have no equivalent power state mechanism.

UC-41 requires hibernate/resume for CAPI clusters to reduce cost for idle vanilla Kubernetes workloads.

## Decision

Hibernate CAPI clusters by patching all MachineDeployment replicas to 0. Before scaling down, store the current replica count in an annotation `acmlab.redhat.com/pre-hibernate-replicas` on each MachineDeployment. On resume, read the annotation and restore the original count. Default to 2 replicas if the annotation is missing.

The `Hibernate` and `Resume` methods in `internal/lifecycle/` try the Hive path first (ClusterDeployment). If no ClusterDeployment is found, they fall back to the CAPI path automatically.

## Consequences

- **Pro:** Unified CLI: `acmlab lifecycle hibernate` works for both Hive and CAPI clusters without the user specifying the type
- **Pro:** No new CRDs or controllers needed: uses existing MachineDeployment API
- **Pro:** ACM tracks the cluster as `Available=Unknown` when all workers are gone, consistent with Hive hibernation
- **Con:** Not a true hibernation: control plane may remain running (depends on CAPI provider), so cost savings are partial
- **Con:** If the annotation is lost (manual edit, resource recreation), resume defaults to 2 replicas instead of the original count
- **Con:** Scale-to-zero drains and deletes nodes rather than stopping VMs: resume provisions new nodes, which is slower than VM resume

## Failure Modes

- **Annotation lost:** Resume creates 2 workers. The operator should verify the expected count after resume.
- **MachineDeployment recreated externally:** The new MachineDeployment has no annotation. Hibernate works (scales to 0 and sets annotation), but prior state is lost.
- **Control plane cost:** Some CAPI providers keep control plane nodes running even with zero workers. This is provider-specific and outside ACM's control.

## PoC to Production

| PoC | Production |
|-----|------------|
| Scale-to-zero via MachineDeployment patch | Provider-native hibernate if CAPI providers add VM-stop support, or a dedicated `HibernationRequest` CRD |
| Annotation stores pre-hibernate replicas | CRD status field or etcd-backed state, resilient to resource recreation |
| Default 2 replicas on missing annotation | Controller validates and rejects resume without stored state |
| Sequential fallback (Hive then CAPI) | ComputeRequest controller knows cluster type from creation, no fallback needed |
