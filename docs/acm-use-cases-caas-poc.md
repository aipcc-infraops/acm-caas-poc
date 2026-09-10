# ACM Use Cases — ACM CaaS PoC

> AIPCC-29876 — Epic: AIPCC-29817
>
> Use cases to validate ACM capabilities for the CaaS platform.
> Each use case exercises ACM's Go API (`open-cluster-management.io/api`,
> `github.com/openshift/hive/apis`) and maps directly to what the
> ComputeRequest controller will do in the final solution.
>
> Implementation approach: each use case becomes a Go package that
> uses `k8s.io/client-go` + ACM typed clients to create, watch, and
> assert on ACM resources. The same code graduates into the
> ComputeRequest controller's reconcile logic.

## UC-01: Provision a spoke cluster on-demand

**Feature**: Cluster provisioning via ACM Go API

As a platform operator  
I want to provision spoke clusters programmatically via the ACM API  
So that the ComputeRequest controller can automate this

### Scenario: Create a ClusterDeployment and wait for provisioning

**Given** I have a typed client for `hive.openshift.io/v1`  
**And** cloud credentials exist as a Secret in the spoke namespace  
**And** a ClusterImageSet for the target OCP version exists  
**When** I create a ClusterDeployment resource via the Go API  
**Then** the ClusterDeployment is accepted by Hive  
**And** I can watch the `status.conditions` until `Provisioned = True`  
**And** a ManagedCluster resource is automatically created on the hub

### Scenario: Delete a ClusterDeployment and verify cleanup

**Given** a ClusterDeployment exists with status `Provisioned = True`  
**When** I delete the ClusterDeployment via the Go API  
**Then** Hive deprovisions the cloud infrastructure  
**And** the ManagedCluster is removed from the hub

### ACM Go types

```go
github.com/openshift/hive/apis/hive/v1.ClusterDeployment
github.com/openshift/hive/apis/hive/v1.ClusterImageSet
open-cluster-management.io/api/cluster/v1.ManagedCluster
```

### ComputeRequest controller equivalent

```
ComputeRequest.spec.platform + spec.capacity
  -> controller builds ClusterDeployment
  -> controller watches until Provisioned = True
  -> controller updates ComputeRequest.status.phase = Ready
```

---

## UC-02: Enforce image registry restrictions on managed clusters

**Feature**: Image registry policy enforcement via ACM Go API

As a platform operator  
I want to restrict container images to approved registries  
So that the ComputeRequest controller can enforce supply-chain security

### Scenario: Create an image registry restriction policy targeting dev clusters

**Given** I have a typed client for `policy.open-cluster-management.io/v1`  
**And** a spoke cluster exists with label `env=dev`  
**When** I create a Policy that only allows images from approved registries  
**And** I bind it to clusters with label `env=dev` via PlacementRule  
**Then** the policy is distributed to matching clusters  
**And** the Policy status shows compliance state per cluster

### Scenario: Detect non-compliant image usage

**Given** an image registry policy is distributed to a spoke  
**When** a pod running an image from an unapproved registry exists  
**Then** `Policy.status.compliant = NonCompliant`  
**And** the per-cluster status identifies the violating namespace and pod

### Scenario: Enforce approved registries only

**Given** an image registry policy with `remediationAction = enforce`  
**When** a user tries to deploy a pod with image from docker.io  
**Then** the admission controller blocks the pod  
**And** the Policy status remains Compliant

### Approved registries (example)

- `registry.redhat.io`
- `quay.io/your-org`
- `us.icr.io/your-namespace` (IBM Cloud Container Registry)

### ACM Go types

```go
open-cluster-management.io/governance-policy-propagator/api/v1.Policy
open-cluster-management.io/api/cluster/v1beta1.Placement
```

### ComputeRequest controller equivalent

```
ComputeRequest.spec.security.allowedRegistries = ["registry.redhat.io", "quay.io/myorg"]
  -> controller creates Policy with ConfigurationPolicy enforcing AllowedContainerImagesPolicy
  -> controller creates PlacementBinding targeting the spoke
  -> controller watches Policy.status.compliant
  -> controller updates ComputeRequest.status.conditions[PolicyCompliant]
```

---

## UC-03: Deploy tenant isolation to spokes from the hub

**Feature**: Tenant RBAC isolation deployment via ManifestWork

As a platform operator  
I want to prepare spoke clusters with tenant namespaces, RBAC, and network policies  
So that the ComputeRequest controller can isolate teams on shared clusters

### Scenario: Create a ManifestWork with tenant isolation resources

**Given** I have a typed client for `work.open-cluster-management.io/v1`  
**And** a spoke cluster is registered and accepted  
**When** I create a ManifestWork in the spoke's namespace containing:

| Resource       | Name                 | Purpose                              |
|----------------|----------------------|--------------------------------------|
| Namespace      | team-alpha           | Isolated tenant namespace             |
| RoleBinding    | team-alpha-admin     | Grants edit role to team-alpha group  |
| NetworkPolicy  | deny-cross-namespace | Blocks traffic from other namespaces  |
| ResourceQuota  | team-alpha-quota     | Limits CPU/memory per tenant          |

**Then** the ManifestWork is synced to the spoke  
**And** `status.conditions` shows `Applied = True`  
**And** `status.resourceStatus` lists each manifest's apply result

### Scenario: Update tenant resource limits via ManifestWork

**Given** a tenant isolation ManifestWork exists with ResourceQuota `cpu=4, memory=8Gi`  
**When** I update the ManifestWork to set ResourceQuota `cpu=8, memory=16Gi`  
**Then** the spoke ResourceQuota is updated to the new limits  
**And** the ManifestWork status reflects the updated state

### Scenario: Onboard a new team to an existing spoke

**Given** a spoke cluster already has tenant "team-alpha" deployed  
**When** I create a second ManifestWork with isolation for "team-beta"  
**Then** both tenants coexist on the spoke with independent RBAC and quotas  
**And** NetworkPolicies prevent cross-tenant traffic

### Manifests deployed per tenant

- `Namespace` — isolated workspace for the team
- `RoleBinding` — binds `edit` ClusterRole to the team's group (e.g., LDAP/OIDC group)
- `NetworkPolicy` — default-deny ingress from other namespaces, allow only within tenant
- `ResourceQuota` — CPU, memory, and pod limits per tenant

### ACM Go types

```go
open-cluster-management.io/api/work/v1.ManifestWork
open-cluster-management.io/api/work/v1.ManifestWorkReplicaSet
```

### ComputeRequest controller equivalent

```
ComputeRequest.spec.tenants[0].name = "team-alpha"
ComputeRequest.spec.tenants[0].group = "cn=team-alpha,ou=groups,dc=company"
ComputeRequest.spec.tenants[0].quota = {cpu: "8", memory: "16Gi"}
  -> controller builds ManifestWork with Namespace + RoleBinding + NetworkPolicy + ResourceQuota
  -> controller creates ManifestWork in spoke namespace
  -> controller watches Applied condition
  -> controller updates ComputeRequest.status.tenants[0].ready = true

ComputeRequest.spec.features = ["monitoring", "pipelines"]
  -> controller looks up ManifestWork templates for each feature
  -> controller creates additional ManifestWork in spoke namespace
  -> controller updates ComputeRequest.status with feature readiness
```

---

## UC-04: Query fleet status and search resources across clusters

**Feature**: ManagedCluster status and cross-cluster search via ACM Go API

As a platform operator  
I want to query cluster health and search resources across the fleet  
So that the ComputeRequest controller can reflect spoke state and find resources

### Scenario: List managed clusters and read their conditions

**Given** I have a typed client for `cluster.open-cluster-management.io/v1`  
**When** I list ManagedCluster resources on the hub  
**Then** each cluster has conditions: `ManagedClusterConditionAvailable`, `HubAcceptedManagedCluster`, `ManagedClusterJoined`  
**And** I can read labels (cloud, region, gpu-count) from each cluster

### Scenario: Watch for cluster health changes

**Given** a spoke cluster is `Available = True`  
**When** I set up a watch on ManagedCluster resources  
**And** the spoke loses connectivity  
**Then** the watch receives an event with `Available = False`

### Scenario: Search resources across all managed clusters

**Given** ACM Search is enabled on the hub (search-collector addon active)  
**When** I query the Search API for pods with label `app=my-workload`  
**Then** I receive results from all spoke clusters where matching pods exist  
**And** each result includes cluster name, namespace, pod name, and status

### Scenario: Search for resources in CrashLoopBackOff across the fleet

**Given** workloads are deployed across multiple spoke clusters  
**When** I query Search for pods with `status.phase != Running`  
**Then** I can identify failing pods across the entire fleet  
**And** correlate them with the cluster and tenant they belong to

### ACM Go types

```go
open-cluster-management.io/api/cluster/v1.ManagedCluster
open-cluster-management.io/api/cluster/v1.ManagedClusterStatus
```

### ACM Search

- Search API endpoint: `https://<hub>/searchapi/graphql`
- Indexed by the `search-collector` addon on each spoke
- Supports queries by kind, namespace, label, cluster, status
- Provides cross-cluster resource visibility without direct spoke access

### ComputeRequest controller equivalent

```
ComputeRequest.status.binding.clusterName = "spoke-1"
  -> controller watches ManagedCluster "spoke-1"
  -> controller mirrors conditions into ComputeRequest.status.conditions
  -> controller sets ComputeRequest.status.phase = Degraded if Available = False

ComputeRequest.status.workloads
  -> controller uses Search API to find resources matching tenant labels
  -> controller aggregates workload status across clusters
  -> controller updates ComputeRequest.status.workloads[].healthy
```

---

## UC-05: Manage cluster lifecycle (hibernate/resume)

**Feature**: Cluster power management via Hive Go API

As a platform operator  
I want to hibernate and resume clusters programmatically  
So that the ComputeRequest controller can implement idle reclamation

### Scenario: Hibernate a Hive-provisioned cluster by patching powerState

**Given** a ClusterDeployment exists with `powerState = Running`  
**When** I patch `spec.powerState` to `Hibernating` via the Go API  
**Then** Hive stops the compute instances  
**And** `ClusterDeployment.status.powerState = Hibernating`  
**And** the ManagedCluster condition `Available` transitions to `Unknown`

### Scenario: Resume a hibernated cluster

**Given** a ClusterDeployment has `powerState = Hibernating`  
**When** I patch `spec.powerState` to `Running`  
**Then** Hive starts the compute instances  
**And** the spoke reconnects to the hub  
**And** `ManagedCluster Available = True`

### Scenario: Verify lifecycle limitations on imported clusters

**Given** a ManagedCluster "imported-cluster" was imported via UC-07 (no Hive ClusterDeployment)  
**When** I attempt to hibernate the imported cluster  
**Then** the operation fails because no ClusterDeployment exists  
**And** the error is reported clearly to the caller

### Important

Hibernate/resume relies on Hive's `ClusterDeployment.spec.powerState`, which only exists for ACM-provisioned clusters. Imported/registered clusters (UC-07) do not have a Hive-managed ClusterDeployment, so lifecycle operations are not available.

The ComputeRequest controller must distinguish between provisioned and imported clusters and offer lifecycle management only where supported.

### ACM Go types

```go
github.com/openshift/hive/apis/hive/v1.ClusterDeployment (spec.powerState)
```

### ComputeRequest controller equivalent

```
ComputeRequest.spec.utilization.policy = reclaimable
ComputeRequest.spec.utilization.idleTimeout = 2h
  -> controller checks if cluster was provisioned by Hive (ClusterDeployment exists)
  -> if yes: patches ClusterDeployment.spec.powerState = Hibernating
  -> if no (imported): skips lifecycle, sets condition LifecycleNotSupported
  -> controller updates ComputeRequest.status.phase accordingly

User accesses cluster again:
  -> controller patches powerState = Running (Hive-provisioned only)
  -> controller waits for Available = True
  -> controller updates ComputeRequest.status.phase = Ready
```

---

## UC-06: Monitor cluster resources from the hub

**Feature**: Cluster resource monitoring via ACM Go API

As a platform operator  
I want to query cluster resource usage and node status programmatically  
So that the ComputeRequest controller can make capacity-aware scheduling decisions

### Scenario: Read node count and resource capacity for a managed cluster

**Given** I have a typed client for `internal.open-cluster-management.io/v1beta1`  
**And** a ManagedCluster "spoke-1" is joined and available  
**When** I get the ManagedClusterInfo for "spoke-1"  
**Then** I can read the node list with roles (master, worker)  
**And** I can read CPU and memory capacity per node  
**And** I can read CPU and memory allocatable per node

### Scenario: Compare resource usage across the fleet

**Given** multiple spoke clusters are joined and available  
**When** I list ManagedClusterInfo resources on the hub  
**Then** I can aggregate total CPU capacity across the fleet  
**And** I can identify clusters with available capacity  
**And** I can rank clusters by utilization percentage

### Scenario: Detect resource pressure on a spoke

**Given** a spoke cluster is running workloads near capacity  
**When** I read the ManagedClusterInfo resource conditions  
**Then** I can detect nodes with MemoryPressure or DiskPressure  
**And** I can identify clusters approaching resource limits

### Scenario: Enable Thanos-based observability on the hub

**Given** ACM Observability operator is installed on the hub  
**When** I deploy MinIO as object storage in the observability namespace  
**And** I create a Thanos object storage secret with S3 credentials  
**And** I create a MultiClusterObservability CR with minimal instance size  
**Then** the observability stack (Thanos Querier, metrics collector) is deployed  
**And** spoke clusters begin sending metrics to the hub  
**And** the MCO status shows `Ready = True`

### Scenario: Query real-time CPU and memory usage via Thanos

**Given** MultiClusterObservability is Ready on the hub  
**And** spoke clusters are sending metrics  
**When** I query the Thanos API for `cluster:capacity_cpu_cores:sum`  
**Then** I receive real-time CPU usage (not just capacity) per cluster  
**And** I can compare usage vs capacity to calculate utilization percentage

### Scenario: Clean teardown of observability stack

**Given** MultiClusterObservability and MinIO are deployed  
**When** I run the observability teardown  
**Then** the MCO CR is deleted  
**And** MinIO (Deployment, Service, PVC, Secret) is removed  
**And** the observability namespace is deleted  
**And** no leftover resources remain on the hub

### Two data sources for monitoring

| Source  |  Data  |  Setup required |
|---------|--------|-----------------|
| ManagedClusterInfo  |  Node count, CPU/memory capacity, instance type, region, zone, OCP version  |  None — works out of the box |
| Thanos (Observability)  |  Real-time CPU/memory usage, API server latency, etcd health, pod metrics  |  MinIO + MultiClusterObservability CR |

### ACM Go types

```go
internal.open-cluster-management.io/v1beta1.ManagedClusterInfo
cluster.open-cluster-management.io/v1.ManagedCluster (conditions)
observability.open-cluster-management.io/v1beta2.MultiClusterObservability
```

### Key fields in ManagedClusterInfo

- `status.nodeList[].name` — node names
- `status.nodeList[].labels` — node roles and topology
- `status.nodeList[].capacity` — CPU, memory, pods
- `status.nodeList[].conditions` — Ready, MemoryPressure, DiskPressure
- `status.distributionInfo` — OCP version, channel, upgrade status

### Observability setup (idempotent)

- MinIO deployment with PVC for Thanos object storage
- Thanos secret with S3 endpoint pointing to MinIO
- MultiClusterObservability CR with `instanceSize: minimal`
- Teardown removes all resources — no leftovers

### ComputeRequest controller equivalent

```
ComputeRequest.spec.capacity.cpu = "16"
ComputeRequest.spec.capacity.memory = "64Gi"
  -> controller queries ManagedClusterInfo across fleet (capacity)
  -> controller queries Thanos for real-time usage (if observability enabled)
  -> controller finds spoke with sufficient allocatable resources
  -> controller updates ComputeRequest.status.binding.clusterName = best-fit
  -> controller monitors spoke resource pressure
  -> controller sets ComputeRequest.status.conditions[CapacitySufficient]
```

---

## UC-07: Import an existing cluster into ACM

**Feature**: External cluster import via ACM Go API

As a platform operator  
I want to import clusters not provisioned by ACM  
So that the ComputeRequest controller can manage heterogeneous fleets

### Scenario: Import a cluster using auto-import secret

**Given** I have admin access to an external cluster  
**And** I have the external cluster's kubeconfig  
**When** I create a ManagedCluster resource on the hub with appropriate labels  
**And** I create a KlusterletAddonConfig in the cluster's namespace  
**And** I create an auto-import Secret with the external kubeconfig  
**Then** the klusterlet agent is deployed on the external cluster  
**And** `ManagedCluster.status.conditions` shows `ManagedClusterJoined = True`  
**And** `ManagedCluster.status.conditions` shows `ManagedClusterConditionAvailable = True`

### Scenario: Import a cluster with specific addon configuration

**Given** an external cluster needs monitoring and policy addons  
**When** I create a KlusterletAddonConfig with enabled addons  
**Then** the specified addons are deployed on the imported cluster  
**And** addon status is reported back to the hub

### Scenario: Detach an imported cluster

**Given** an imported ManagedCluster "external-1" exists  
**When** I delete the ManagedCluster resource from the hub  
**Then** the klusterlet agent is removed from the external cluster  
**And** the cluster operates independently without ACM management

### ACM Go types

```go
cluster.open-cluster-management.io/v1.ManagedCluster
agent.open-cluster-management.io/v1.KlusterletAddonConfig
v1.Secret (auto-import secret with kubeconfig)
```

### Auto-import secret format

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: auto-import-secret
  namespace: <cluster-name>
type: Opaque
stringData:
  kubeconfig: |
    <external-cluster-kubeconfig>
```

### KlusterletAddonConfig addons

- `applicationManager` — application lifecycle
- `policyController` — policy enforcement on spoke
- `searchCollector` — search indexing
- `certPolicyController` — certificate policy
- `iamPolicyController` — IAM policy

### ComputeRequest controller equivalent

```
ComputeRequest.spec.import.kubeconfig = <secret-ref>
ComputeRequest.spec.import.labels = {cloud: "on-prem", region: "dc-1"}
  -> controller creates ManagedCluster with labels
  -> controller creates KlusterletAddonConfig with required addons
  -> controller creates auto-import Secret with kubeconfig
  -> controller watches ManagedClusterJoined + Available conditions
  -> controller updates ComputeRequest.status.phase = Ready
```

---

## UC-08: Legacy cluster decommissioning lifecycle

**Feature**: Legacy cluster decommissioning via ACM Go API

As a platform operator  
I want to audit, notify, and safely decommission inherited clusters  
So that the CaaS platform can reclaim resources and reduce cost

### Scenario: Import a legacy cluster and audit its usage

**Given** an existing cluster not yet managed by ACM  
**And** I have its kubeconfig  
**When** I import the cluster into ACM via auto-import secret  
**And** I query its ManagedClusterInfo for node count, CPU, and memory  
**And** I list all non-system namespaces and their workload counts  
**Then** I get an audit report with: owner (from labels/annotations), last workload activity, resource utilization  
**And** clusters with utilization below threshold are flagged as decommission candidates

### Scenario: Notify cluster owner before decommissioning

**Given** a legacy cluster is flagged as a decommission candidate  
**When** I send a notification to the cluster owner with a reclaim deadline  
**Then** the notification includes: cluster name, current usage summary, deadline date, action required  
**And** the notification is tracked in the audit log

### Scenario: Backup cluster state before deletion

**Given** a legacy cluster is approved for decommissioning  
**When** I export all non-system namespace resources as YAML  
**And** I list all PersistentVolumes with storage class, size, and bound claims  
**And** I export cluster-scoped resources (ClusterRoles, CRDs, OAuth config)  
**Then** the backup is stored in a designated location  
**And** the backup manifest lists all exported resources with their sizes

### Scenario: Decommission the cluster

**Given** a legacy cluster has been backed up and the owner notified  
**And** the reclaim deadline has passed with no response  
**When** I drain all worker nodes  
**And** I delete the cluster via cloud provider API or Hive ClusterDeployment  
**And** I remove the ManagedCluster, ManifestWorks, and policies from ACM  
**Then** the cluster no longer appears in the fleet  
**And** the decommission event is recorded in the audit log

### Scenario: Decommission is idempotent and handles partial state

**Given** a decommission was interrupted midway (e.g., cluster deleted but ACM not cleaned)  
**When** I run decommission again  
**Then** it completes the remaining cleanup steps without errors  
**And** no resources are left behind in ACM

### Decommission workflow steps

1. **Import** into ACM (reuses UC-07)
2. **Audit** — workloads, utilization, owner identification
3. **Notify** — owner notification with deadline
4. **Backup** — namespace resources, PVs, cluster-scoped config
5. **Drain** — cordon + drain nodes
6. **Delete** — destroy cluster via cloud API
7. **Cleanup** — remove from ACM (ManagedCluster, ManifestWorks, policies)

### ACM Go types

```go
cluster.open-cluster-management.io/v1.ManagedCluster — detach/delete
internal.open-cluster-management.io/v1beta1.ManagedClusterInfo — usage audit
work.open-cluster-management.io/v1.ManifestWork — cleanup
```

### ComputeRequest controller equivalent

```
ComputeRequest.spec.lifecycle.decommission = true
ComputeRequest.spec.lifecycle.decommissionDeadline = "2026-09-15"
  -> controller audits cluster usage and identifies owner
  -> controller sends notification (webhook/email)
  -> controller waits for deadline
  -> controller exports backup manifest
  -> controller drains and deletes cluster
  -> controller cleans up ACM resources
  -> controller updates ComputeRequest.status.phase = Decommissioned
```

---

## UC-09: Cluster upgrades (Day-2 operations)

**Feature**: Managed cluster OCP upgrades via ACM Go API

As a platform operator  
I want to orchestrate OCP version upgrades across the fleet from the hub  
So that the CaaS platform keeps clusters on supported versions

### Scenario: Check available upgrades for a managed cluster

**Given** a managed cluster is running OCP 4.21.x on channel stable-4.21  
**When** I query the cluster's ManagedClusterInfo for distributionInfo  
**And** I check the available upgrade versions from the channel  
**Then** I get the current version, channel, and list of available target versions

### Scenario: Upgrade a single cluster via ClusterCurator

**Given** a managed cluster is running OCP 4.21.28  
**And** OCP 4.21.29 is available in the stable-4.21 channel  
**When** I create a ClusterCurator CR with `desiredUpdate = 4.21.29`  
**Then** the ClusterCurator orchestrates the upgrade on the spoke  
**And** the curator status shows progress (pre-hook, upgrade, post-hook)  
**And** ManagedClusterInfo.distributionInfo reflects the new version when complete

### Scenario: Batch upgrade clusters by label

**Given** multiple managed clusters have label `upgrade-group=batch-1`  
**When** I create ClusterCurator CRs for all clusters in batch-1  
**Then** upgrades proceed in parallel across the batch  
**And** I can monitor per-cluster progress from the hub  
**And** clusters that fail upgrade are flagged without blocking others

### Scenario: Upgrade with pre and post hooks

**Given** I need to run health checks before and after upgrade  
**When** I create a ClusterCurator with prehook and posthook Ansible jobs  
**Then** the prehook runs before the upgrade starts  
**And** the posthook runs after the upgrade completes  
**And** if the prehook fails, the upgrade is aborted

### ACM Go types

```go
cluster.open-cluster-management.io/v1.ManagedCluster — cluster selection
internal.open-cluster-management.io/v1beta1.ManagedClusterInfo — version info
cluster.open-cluster-management.io/v1beta1.ClusterCurator — upgrade orchestration
```

### ComputeRequest controller equivalent

```
ComputeRequest.spec.version.desired = "4.21.29"
ComputeRequest.spec.version.channel = "stable-4.21"
  -> controller checks current version via ManagedClusterInfo
  -> controller creates ClusterCurator with desiredUpdate
  -> controller monitors curator status conditions
  -> controller updates ComputeRequest.status.version.current
```

---

## UC-10: Cluster scaling (add/remove workers)

**Feature**: Managed cluster worker node scaling via ACM Go API

As a platform operator  
I want to scale worker nodes in managed clusters from the hub  
So that the CaaS platform can adjust capacity to tenant demand

### Scenario: Query current node pool configuration

**Given** a managed cluster was provisioned via Hive with a MachinePool  
**When** I query the MachinePool for the cluster  
**Then** I get the current replica count, instance type, and autoscaling config

### Scenario: Scale up workers by increasing MachinePool replicas

**Given** a managed cluster has a MachinePool with 3 replicas  
**When** I patch the MachinePool to set `replicas = 5`  
**Then** 2 new worker nodes are provisioned  
**And** ManagedClusterInfo.nodeList shows 5 worker nodes when scaling completes

### Scenario: Scale down workers

**Given** a managed cluster has a MachinePool with 5 replicas  
**And** cluster utilization is below 30%  
**When** I patch the MachinePool to set `replicas = 3`  
**Then** 2 worker nodes are drained and removed  
**And** workloads are redistributed to remaining nodes

### Scenario: Enable autoscaling on a MachinePool

**Given** a managed cluster has a MachinePool with fixed replicas  
**When** I patch the MachinePool to enable autoscaling with `min=3, max=10`  
**Then** the MachinePool switches from fixed replicas to autoscaling  
**And** the cluster scales automatically based on pod scheduling pressure

### Scenario: Scale a cluster without Hive (imported cluster)

**Given** a managed cluster was imported (not provisioned by Hive)  
**When** I try to scale its workers from the hub  
**Then** the operation reports that scaling is not available for imported clusters  
**And** suggests using the cluster's native scaling mechanism

### ACM/Hive Go types

```go
hive.openshift.io/v1.MachinePool — replica count, instance type, autoscaling
internal.open-cluster-management.io/v1beta1.ManagedClusterInfo — node verification
```

### ComputeRequest controller equivalent

```
ComputeRequest.spec.capacity.workers = 5
ComputeRequest.spec.capacity.autoscaling = {min: 3, max: 10}
  -> controller patches MachinePool replicas or autoscaling config
  -> controller monitors ManagedClusterInfo.nodeList for convergence
  -> controller updates ComputeRequest.status.capacity.currentWorkers
```

---

## UC-11: Cost tracking and chargeback

**Feature**: Cluster and tenant cost tracking via ACM Go API

As a platform operator  
I want to track resource consumption per cluster and tenant  
So that the CaaS platform can charge teams for actual usage

### Scenario: Calculate CPU-hours per cluster over a time period

**Given** Thanos metrics are available via the MCO observability stack  
**When** I query total CPU usage for cluster infraops1 over the last 7 days  
**Then** I get the aggregate CPU-hours consumed  
**And** the result is broken down by day

### Scenario: Calculate resource usage per tenant namespace

**Given** tenant team-alpha has a namespace on cluster infraops1  
**When** I query CPU and memory usage for namespace team-alpha over the last 30 days  
**Then** I get CPU-hours and memory-GiB-hours for the tenant  
**And** I can compare usage against the tenant's ResourceQuota limits

### Scenario: Generate a fleet-wide cost report

**Given** multiple clusters and tenants are being tracked  
**When** I generate a cost report for the current billing period  
**Then** the report lists per-cluster and per-tenant resource consumption  
**And** each entry includes: CPU-hours, memory-GiB-hours, storage-GiB, estimated cost  
**And** the report can be exported as CSV or JSON

### Scenario: Identify idle tenants for cleanup

**Given** cost tracking data is available for all tenants  
**When** I query tenants with zero CPU usage in the last 14 days  
**Then** I get a list of idle tenants with their last activity timestamp  
**And** these tenants are flagged as candidates for decommissioning (links to UC-08)

### Data sources

- Thanos/Prometheus via MCO (CPU, memory metrics over time)
- ManagedClusterInfo (node capacity, instance types for cost calculation)
- ResourceQuota per tenant (allocated vs actual usage)

### ComputeRequest controller equivalent

```
ComputeRequest.status.cost.cpuHours = 1234.5
ComputeRequest.status.cost.memoryGiBHours = 5678.9
ComputeRequest.status.cost.lastUpdated = "2026-08-24T00:00:00Z"
  -> controller queries Thanos for usage metrics periodically
  -> controller aggregates by cluster and tenant namespace
  -> controller updates ComputeRequest.status.cost
  -> external billing system reads status.cost for invoicing
```

---

## UC-12: Manage Identity Providers to all clusters in ACM

**Feature**: Identity Provider configuration and credential rotation via ACM Go API

As a platform operator  
I want to configure and rotate identity providers across all clusters  
So that the CaaS platform can enforce authentication policies and respond to security incidents

### Scenario: Configure GitHub Identity Provider on each cluster

**Given** a cluster is created or imported into ACM  
**When** I create a ManifestWork containing an OAuth configuration with GitHub IdP  
**Then** the GitHub IdP is configured on the cluster  
**And** users can authenticate via GitHub  
**And** the configuration is applied to all current and future clusters

### Scenario: Rotate htpasswd credentials after a leak

**Given** clusters have htpasswd Identity Provider configured  
**And** a credential leak has been detected  
**When** I generate new htpasswd credentials  
**And** I create/update ManifestWork with the new htpasswd Secret  
**And** I bind the ManifestWork to all affected clusters via Placement  
**Then** the htpasswd Secret is updated on all clusters  
**And** old credentials are invalidated  
**And** the rotation is tracked in an audit log

### Scenario: Apply IdP policy to clusters by label

**Given** multiple clusters exist with different environments (dev, staging, prod)  
**When** I create a Policy requiring GitHub IdP for prod clusters  
**And** I bind the policy to clusters with label `env=prod`  
**Then** all prod clusters must have GitHub IdP configured  
**And** non-compliant clusters are flagged  
**And** policy status shows per-cluster compliance

### ACM Go types

```go
work.open-cluster-management.io/v1.ManifestWork — OAuth config deployment
policy.open-cluster-management.io/v1.Policy — IdP requirement enforcement
cluster.open-cluster-management.io/v1beta1.Placement — cluster targeting
v1.Secret — htpasswd credentials
```

### OAuth configuration manifest (GitHub IdP)

```yaml
apiVersion: config.openshift.io/v1
kind: OAuth
metadata:
  name: cluster
spec:
  identityProviders:
  - name: github
    type: GitHub
    mappingMethod: claim
    github:
      clientID: <github-client-id>
      clientSecret:
        name: github-client-secret
      organizations:
      - your-org
```

### htpasswd Secret manifest

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: htpasswd-secret
  namespace: openshift-config
type: Opaque
data:
  htpasswd: <base64-encoded-htpasswd-file>
```

### ComputeRequest controller equivalent

```
ComputeRequest.spec.identityProvider.type = "github"
ComputeRequest.spec.identityProvider.github.clientID = "..."
ComputeRequest.spec.identityProvider.github.organizations = ["your-org"]
  -> controller creates ManifestWork with OAuth config + Secret
  -> controller creates ManifestWork in cluster namespace
  -> controller watches Applied condition
  -> controller updates ComputeRequest.status.identityProvider.configured = true

Security incident (credential rotation):
ComputeRequest.spec.identityProvider.rotateCredentials = true
  -> controller generates new htpasswd file
  -> controller updates ManifestWork with new Secret
  -> controller tracks rotation timestamp in status
  -> controller logs rotation event for audit
```

---

## UC-13: Registry mirror for restricted-registry clusters

**Feature**: Image registry mirror for clusters that cannot pull from registry.redhat.io (ROKS, air-gapped, disconnected)

As a platform operator  
I want to configure image registry mirrors for clusters with restricted network access  
So that ACM can import and manage clusters that cannot reach public registries

### Scenario: Identify required images for a cluster

**Given** a ManagedCluster has been registered but klusterlet pods are in ImagePullBackOff  
**When** I run `acmlab registry list-images <cluster>`  
**Then** I see all images extracted from the cluster's ManifestWorks  
**And** I know exactly which images must be available on the spoke

### Scenario: Mirror images to a reachable registry

**Given** I have a target registry accessible from the spoke (e.g., us.icr.io, Quay.io)  
**When** I run `acmlab registry mirror-script <cluster> --target <registry>`  
**Then** I get a bash script with `skopeo copy` commands for each required image  
**And** after running the script, all ACM images are available in the target registry

### Scenario: Configure ManagedClusterImageRegistry on the hub

**Given** ACM images are mirrored to a reachable registry  
**And** I have a pull secret for the target registry  
**When** I run `acmlab registry configure <cluster> --mirror <registry> --pull-secret <path>`  
**Then** a ManagedClusterImageRegistry CR is created on the hub  
**And** ACM rewrites image references in klusterlet ManifestWorks to use the mirror  
**And** no changes are required on the spoke

### Scenario: Chicken-and-egg on unavailable clusters

**Given** a cluster is not yet imported (Available=Unknown) and images are blocked  
**When** I configure the registry mirror before the cluster becomes available  
**Then** the Placement uses tolerations to select unavailable clusters  
**And** the MCIR takes effect before the klusterlet finishes its first import

### Scenario: Remove registry mirror configuration

**Given** a ManagedClusterImageRegistry is configured for a cluster  
**When** I run `acmlab registry remove <cluster>`  
**Then** the ManagedClusterImageRegistry, Placement, and pull secret are deleted  
**And** ACM reverts to using original image references

### ROKS findings (PoC)

Attempting to import a ROKS cluster revealed a fundamental network restriction: ROKS workers have no outbound access to external registries — not `registry.redhat.io`, not `quay.io`. All image pulls are intercepted and routed through `us.icr.io/armada-extensions/` (IBM's mirror), which does not include ACM/MCE images.

`ManagedClusterImageRegistry` can rewrite references to point at `us.icr.io`, but the IBM Cloud Container Registry Free plan (512 MB/month) is insufficient for the 6 required images (~300 MB amd64-only, ~750 MB all-arch). Workaround: ICR Standard plan or custom ROKS network configuration.

**Conclusion**: Importing ROKS into an external ACM hub is possible in principle but requires either ICR Standard plan or network changes to allow external registry access from ROKS workers.

### ACM Go types

`imageregistry.open-cluster-management.io/v1alpha1.ManagedClusterImageRegistry`  
`cluster.open-cluster-management.io/v1beta1.Placement`  
`cluster.open-cluster-management.io/v1beta2.ManagedClusterSetBinding`  
`v1.Secret` (pull secret for mirror registry)

### CLI commands

```
acmlab registry list-images <cluster>
acmlab registry mirror-script <cluster> --target <registry>
acmlab registry configure <cluster> --mirror <registry> [--pull-secret <path>]
acmlab registry status <cluster>
acmlab registry remove <cluster>
```

### MCP tools

`acm_registry_list_images`, `acm_registry_configure_mirror`, `acm_registry_mirror_status`, `acm_registry_generate_mirror_script`

### ComputeRequest controller equivalent

```
ComputeRequest.spec.import.restrictedRegistry = true
ComputeRequest.spec.import.mirrorRegistry = "us.icr.io/acm-mirror"
ComputeRequest.spec.import.pullSecretRef = "mirror-pull-secret"
  -> controller calls ListRequiredImages to identify needed images
  -> controller creates ManagedClusterSetBinding in cluster namespace
  -> controller creates Placement with tolerations for unavailable clusters
  -> controller creates ManagedClusterImageRegistry with source→mirror mappings
  -> controller waits for ClustersUpdated=True on MCIR status
  -> controller imports cluster via UC-07 flow
```

---

## Summary

| UC  |  What it validates  |  ACM Go module  |  ComputeRequest field |
|-----|---------------------|-----------------|----------------------|
| UC-01  |  Programmatic cluster provisioning  |  `hive/apis/hive/v1`  |  spec.platform, spec.capacity |
| UC-02  |  Image registry policy enforcement  |  `governance-policy-propagator/api/v1`  |  spec.security.allowedRegistries |
| UC-03  |  Tenant isolation via ManifestWork  |  `api/work/v1`  |  spec.tenants, spec.features |
| UC-04  |  Fleet health queries + cross-cluster search  |  `api/cluster/v1` + Search API  |  status.conditions, status.workloads |
| UC-05  |  Programmatic lifecycle management  |  `hive/apis/hive/v1`  |  spec.lifecycle, spec.utilization |
| UC-06  |  Cluster monitoring (capacity + real-time usage via Thanos)  |  `ManagedClusterInfo` + `MultiClusterObservability`  |  status.capacity, scheduling |
| UC-07  |  External cluster import  |  `api/cluster/v1` + `agent/v1`  |  spec.import |
| UC-08  |  Legacy cluster decommissioning  |  `api/cluster/v1` + `ManagedClusterInfo`  |  spec.lifecycle.decommission |
| UC-09  |  Cluster upgrades (Day-2)  |  `ClusterCurator` + `ManagedClusterInfo`  |  spec.version.desired |
| UC-10  |  Cluster scaling (workers)  |  `hive/v1.MachinePool` + `ManagedClusterInfo`  |  spec.capacity.workers |
| UC-11  |  Cost tracking / chargeback  |  `MCO/Thanos` + `ManagedClusterInfo`  |  status.cost |
| UC-12  |  Identity Provider management  |  `api/work/v1` + `policy/v1`  |  spec.identityProvider |
| UC-13  |  Registry mirror for restricted clusters (ROKS, air-gapped)  |  `imageregistry.open-cluster-management.io/v1alpha1`  |  spec.import.mirrorRegistry |

## Go Dependencies (for the lab repo)

```go
// go.mod — key dependencies
// Note: using k8s.io/client-go/dynamic only — no typed ACM/Hive imports
require (
    k8s.io/client-go             v0.30.x   // dynamic client, kubeconfig loading
    k8s.io/apimachinery          v0.30.x   // unstructured, GVR, watch
    github.com/spf13/cobra       v1.10.x   // CLI framework
    github.com/cucumber/godog    v0.15.x   // Gherkin test runner
    github.com/mark3labs/mcp-go  v0.28.x   // MCP server (stdio)
    github.com/joho/godotenv     v1.5.x    // .env loading
)
```
