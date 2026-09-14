# UC-08: Legacy Cluster Decommissioning — Design Spec

## Overview

A governed, multi-step workflow to safely retire clusters from the CaaS platform. The workflow audits usage, notifies owners, backs up state, drains workloads, destroys infrastructure, and cleans up ACM resources — with full idempotency and resumability.

ACM provides atomic primitives (delete ManagedCluster, delete ClusterDeployment) but no orchestrated decommission lifecycle. This design adds that orchestration layer on top.

## Goals

1. Safe, auditable cluster retirement with owner notification and deadline enforcement
2. Idempotent — can resume from any interrupted step without side effects
3. CLI-driven today, controller-driven tomorrow — same business logic (see ADR-005, ADR-008)
4. Hub-cluster-native state persistence via ConfigMap (see ADR-008)

## Non-Goals

- Email/Slack integration for notifications (notification is recorded in ConfigMap; delivery is out of scope)
- Automated approval workflows (owner responds out-of-band; operator advances the workflow)
- Cross-hub decommissioning (single hub cluster only)

---

## Lifecycle Phases

```
imported → audited → notified → backed-up → drained → deleted → cleaned
```

Each phase is a function in `internal/decommission/` that:
- Reads current state from the ConfigMap
- Executes the phase's operations (idempotently)
- Writes the new phase and a history entry to the ConfigMap

### Phase 1: Import (reuses UC-07)

If the cluster is not yet managed by ACM, import it using `internal/importing`. Skip if already a ManagedCluster.

### Phase 2: Audit

Collect cluster usage data from ACM resources on the hub:

| Data | Source | ACM Resource |
|------|--------|--------------|
| Node count, CPU, memory | ManagedClusterInfo | `status.nodeList` |
| Non-system namespaces | ManifestWork (audit job) or spoke kubeconfig | Direct query |
| Owner | ManagedCluster labels/annotations | `metadata.labels["caas-poc/owner"]` |
| Cluster age | ManagedCluster | `metadata.creationTimestamp` |
| Platform | ClusterDeployment (if Hive) or ManagedCluster labels | `spec.platform` or `cloud` label |

The audit report is stored as JSON in `ConfigMap.data.audit`. Clusters below a configurable utilisation threshold are flagged as decommission candidates.

### Phase 3: Notify

Record notification in the ConfigMap:
- `data.owner` — who was notified
- `data.deadline` — ISO 8601 reclaim deadline
- `data.notifiedAt` — timestamp

The actual notification delivery (email, Slack, ticket) is out of scope for the PoC. The ConfigMap serves as the record-of-notification. The operator is responsible for delivering the message using the audit data.

If the deadline passes with no owner response, the operator advances to the next phase.

### Phase 4: Backup

Export cluster state before destruction:

- Non-system namespace resources (Deployments, Services, ConfigMaps, Secrets metadata)
- PersistentVolume inventory (storage class, size, bound claims)
- Cluster-scoped resources (ClusterRoles, CRDs, OAuth config)

Backup is written to a local directory (`./decommission-backups/<cluster>/`) as YAML files. The ConfigMap records the backup path and resource count.

For Hive-provisioned clusters, backup uses the spoke kubeconfig extracted from the hub secret. For imported clusters, the original kubeconfig path (from `data.kubeconfigPath` in the ConfigMap or CLI flag) is used.

### Phase 5: Drain

Cordon and drain all worker nodes:

1. Mark nodes as unschedulable (`spec.unschedulable: true`)
2. Evict all pods (respecting PodDisruptionBudgets)
3. Wait for eviction to complete (configurable timeout, default 5m per node)

Drain uses the spoke kubeconfig. If drain times out, the phase records the timeout but does not block — the operator decides whether to force-proceed.

### Phase 6: Delete

Destroy the cluster infrastructure:

- **Hive-provisioned clusters**: Delete the ClusterDeployment — Hive handles infrastructure teardown
- **Imported clusters**: Delete the ManagedCluster — ACM removes the klusterlet. Infrastructure destruction is the cloud provider's responsibility (out of scope)

This phase calls `provisioning.Manager.DeleteCluster()` for Hive clusters or `importing.Manager.DetachCluster()` for imported clusters.

### Phase 7: Cleanup

Remove all hub-side resources associated with the cluster:

- ManagedCluster (if not already deleted in phase 6)
- Namespace (and all contained resources: Secrets, ManifestWorks, ConfigMaps)
- PlacementBindings, Placements referencing the cluster
- The decommission ConfigMap itself (final step)

---

## Package Structure

```
internal/decommission/
├── manager.go       # Manager struct — orchestrates the lifecycle
├── state.go         # ConfigMap CRUD: GetState, SetPhase, AddHistory
├── audit.go         # Audit() — collects usage data from ACM resources
├── backup.go        # Backup() — exports cluster state to YAML
├── drain.go         # Drain() — cordon + evict via spoke kubeconfig
├── cleanup.go       # Cleanup() — removes hub-side resources
└── decommission_test.go
```

### Key Types

```go
type Phase string

const (
    PhaseImported Phase = "imported"
    PhaseAudited  Phase = "audited"
    PhaseNotified Phase = "notified"
    PhaseBackedUp Phase = "backed-up"
    PhaseDrained  Phase = "drained"
    PhaseDeleted  Phase = "deleted"
    PhaseCleaned  Phase = "cleaned"
)

type DecommissionState struct {
    ClusterName string        `json:"clusterName"`
    Phase       Phase         `json:"phase"`
    Owner       string        `json:"owner,omitempty"`
    Deadline    string        `json:"deadline,omitempty"`
    NotifiedAt  string        `json:"notifiedAt,omitempty"`
    BackupPath  string        `json:"backupPath,omitempty"`
    Audit       *AuditReport  `json:"audit,omitempty"`
    History     []HistoryEntry `json:"history"`
}

type HistoryEntry struct {
    Phase     Phase  `json:"phase"`
    Timestamp string `json:"timestamp"`
    Message   string `json:"message"`
}

type AuditReport struct {
    NodeCount      int      `json:"nodeCount"`
    CPUCapacity    string   `json:"cpuCapacity"`
    MemoryCapacity string   `json:"memoryCapacity"`
    Namespaces     []string `json:"namespaces"`
    Owner          string   `json:"owner"`
    Platform       string   `json:"platform"`
    ClusterAge     string   `json:"clusterAge"`
}
```

### Manager Interface

```go
type Manager struct {
    client     *client.Client
    cfg        config.Config
    importer   *importing.Manager
    provisioner *provisioning.Manager
}

func New(c *client.Client, cfg config.Config) *Manager

func (m *Manager) Start(ctx context.Context, clusterName string, opts StartOpts) (*DecommissionState, error)
func (m *Manager) Advance(ctx context.Context, clusterName string) (*DecommissionState, error)
func (m *Manager) GetState(ctx context.Context, clusterName string) (*DecommissionState, error)
func (m *Manager) List(ctx context.Context) ([]DecommissionState, error)
func (m *Manager) Cancel(ctx context.Context, clusterName string) error

// Individual phase functions (called by Advance, also callable directly)
func (m *Manager) Audit(ctx context.Context, clusterName string) (*AuditReport, error)
func (m *Manager) Notify(ctx context.Context, clusterName string, owner, deadline string) error
func (m *Manager) Backup(ctx context.Context, clusterName string, outputDir string) error
func (m *Manager) Drain(ctx context.Context, clusterName string, timeout time.Duration) error
func (m *Manager) Delete(ctx context.Context, clusterName string) error
func (m *Manager) Cleanup(ctx context.Context, clusterName string) error
```

---

## CLI Commands

```
acmlab decommission start <cluster>     # Import (if needed) + audit → creates ConfigMap
acmlab decommission advance <cluster>   # Execute next phase
acmlab decommission status <cluster>    # Show current phase, audit, history
acmlab decommission list                # List all active decommissions
acmlab decommission cancel <cluster>    # Remove ConfigMap, keep cluster
acmlab decommission audit <cluster>     # Run audit without starting decommission
```

### Options for `start`

- `--kubeconfig-path` — spoke kubeconfig for imported clusters
- `--owner` — cluster owner (overrides label detection)
- `--deadline` — reclaim deadline in ISO 8601 (default: 14 days from now)

### Options for `advance`

- `--force` — skip deadline check for notify→backup transition
- `--backup-dir` — output directory for backup (default: `./decommission-backups/<cluster>/`)
- `--drain-timeout` — per-node drain timeout (default: 5m)

---

## MCP Tools

| Tool | Description |
|------|-------------|
| `acm_decommission_start` | Start decommission workflow for a cluster |
| `acm_decommission_advance` | Advance to next phase |
| `acm_decommission_status` | Get current decommission state |
| `acm_decommission_list` | List all active decommissions |
| `acm_decommission_cancel` | Cancel a decommission workflow |
| `acm_decommission_audit` | Run standalone audit |

---

## ConfigMap Layout

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: spoke1-decommission
  namespace: spoke1
  labels:
    caas-poc/workflow: decommission
    caas-poc/cluster: spoke1
data:
  phase: "notified"
  owner: "team-alpha@example.com"
  deadline: "2026-09-28T00:00:00Z"
  notifiedAt: "2026-09-14T10:30:00Z"
  backupPath: ""
  kubeconfigPath: "/tmp/spoke1.kubeconfig"
  audit: |
    {
      "nodeCount": 3,
      "cpuCapacity": "24",
      "memoryCapacity": "96Gi",
      "namespaces": ["app-a", "app-b", "monitoring"],
      "owner": "team-alpha@example.com",
      "platform": "ibmcloud",
      "clusterAge": "342d"
    }
  history: |
    [
      {"phase": "imported", "timestamp": "2026-09-14T10:00:00Z", "message": "Cluster imported into ACM"},
      {"phase": "audited", "timestamp": "2026-09-14T10:15:00Z", "message": "3 nodes, 24 CPU, 96Gi memory, 3 namespaces"},
      {"phase": "notified", "timestamp": "2026-09-14T10:30:00Z", "message": "Owner team-alpha@example.com notified, deadline 2026-09-28"}
    ]
```

---

## Idempotency

Each phase checks preconditions before acting:

| Phase | Idempotency check |
|-------|-------------------|
| Import | Skip if ManagedCluster exists |
| Audit | Re-run always (data may have changed) |
| Notify | Skip if `notifiedAt` is already set |
| Backup | Skip if backup directory exists and resource count matches |
| Drain | Skip nodes already cordoned; re-evict only pending pods |
| Delete | Skip if ClusterDeployment/ManagedCluster already gone |
| Cleanup | Skip resources that don't exist |

---

## Testing Strategy

- **Unit tests** (90% target): Mock `client.Client` with `dynamicfake`. Test each phase function independently. Test state machine transitions.
- **Integration tests** (`//go:build integration`): Test against a live hub with a disposable imported cluster. Full lifecycle: start → advance through all phases.

---

## Dependencies

- Reuses `internal/importing` (UC-07) for the import phase
- Reuses `internal/provisioning` (UC-01) for Hive cluster deletion
- Reuses `internal/client` for all hub-side operations
- Spoke kubeconfig access required for backup and drain phases

---

## ACM Resources Used

| Resource | GVR | Operation |
|----------|-----|-----------|
| ManagedCluster | `cluster.open-cluster-management.io/v1` | Read (audit), Delete (cleanup) |
| ManagedClusterInfo | `internal.open-cluster-management.io/v1beta1` | Read (audit: nodes, CPU, memory) |
| ClusterDeployment | `hive.openshift.io/v1` | Read (platform detection), Delete (destroy) |
| ManifestWork | `work.open-cluster-management.io/v1` | Delete (cleanup) |
| ConfigMap | `v1` | CRUD (state machine) |
