# ADR-012: Scaling resource auto-detection fallback chain

## Status

Accepted

## Context

A CaaS fleet is heterogeneous: Hive-provisioned clusters have MachinePools, HyperShift clusters have NodePools, CAPI clusters have MachineDeployments, and imported clusters may have none. The `acmlab scaling set` command needs to scale workers regardless of cluster type.

Two approaches were considered: (a) require the user to specify `--type machinepool|nodepool|machinedeployment`, or (b) auto-detect which scaling resource exists for the cluster.

## Decision

Implement `SetReplicasAuto` with a fallback chain: try MachinePool first, then NodePool, then CAPI MachineDeployment. The first resource found is used for the scaling operation. If none is found, return `ErrNoMachinePool` with guidance on the cluster type.

The fallback order reflects provisioning prevalence: most clusters in the PoC are Hive-provisioned (MachinePool), then HyperShift (NodePool), then CAPI (MachineDeployment).

## Consequences

- **Pro:** Single command scales any cluster type: `acmlab scaling set spoke1 --replicas 4` works for Hive, HyperShift, and CAPI
- **Pro:** MCP tools and automation code do not need cluster-type awareness
- **Pro:** New scaling backends can be added to the chain without changing callers
- **Con:** Up to 3 API calls per scaling operation when the first two resource types are not found
- **Con:** If a cluster has both a MachinePool and a NodePool (unlikely but possible during migration), MachinePool always wins
- **Con:** Error messages from the final fallback may not clearly indicate which resource types were tried

## PoC to Production

| PoC | Production |
|-----|------------|
| Runtime fallback chain (MachinePool → NodePool → MachineDeployment) | ComputeRequest controller knows cluster type from `spec.type`, scales the correct resource directly |
| Up to 3 API calls on miss | Single API call: controller stores scaling resource reference in status |
| `ErrNoMachinePool` on final fallback | Controller rejects scaling for imported clusters at admission time |
| Fixed fallback order | No fallback needed: cluster type is explicit metadata |

## Performance Note

The fallback cost is negligible: each "miss" is a single List call that returns zero items. The happy path (MachinePool found on first try) has no overhead. For fleets where most clusters are HyperShift, the order could be reconfigured, but this is not worth the complexity for the PoC.
