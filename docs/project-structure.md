# Project Structure

```
acm-caas-poc/
│
├── cmd/acmlab/                    # CLI entry point
│   ├── main.go                    # Cobra root command, .env loading, global flags
│   ├── fleet.go                   # fleet list, fleet status <name>
│   ├── provision.go               # provision create/destroy/status/list/image-sets
│   ├── policy.go                  # policy list/apply/status/remove/apply-quota/quota-status/automate/automation-status/list-automations/remove-automation/set-automation-mode
│   ├── tenant.go                  # tenant deploy/status/list/remove
│   ├── monitor.go                 # monitor list/status/setup/teardown/obs-status
│   ├── lifecycle.go               # lifecycle hibernate/resume/status/diagnose/list
│   ├── import.go                  # import cluster/detach/status/list
│   ├── registry.go                # registry list-images/mirror-script/configure/status/remove
│   ├── scaling.go                 # scaling list/get/set/auto/set-flavor/init
│   ├── decommission.go            # decommission start/advance/status/list/cancel/audit
│   ├── upgrade.go                 # upgrade status/list/set-channel/start/history
│   ├── clusterset.go              # clusterset create/list/assign/remove
│   ├── idp.go                     # idp configure/list/rotate/remove/configure-unique/enforce-sso
│   ├── pool.go                    # pool create/list/get/delete, claim create/list/release (UC-25)
│   ├── security.go                # security apply/status/list/remove (UC-29 Gatekeeper)
│   ├── rollout.go                 # rollout create/get/list/delete/update-strategy (UC-33)
│   ├── access.go                  # access enable/disable/status/list (UC-35)
│   ├── backup.go                  # backup enable/disable/status/list/restore (UC-36)
│   ├── gitops.go                  # gitops create/get/list/delete/sync (UC-32)
│   └── mcp.go                     # mcp serve (MCP server on stdio)
│
├── internal/
│   ├── config/
│   │   ├── config.go              # Config struct, LoadFromEnv() with defaults
│   │   └── config_test.go         # Env parsing, defaults, validation errors
│   │
│   ├── client/
│   │   ├── client.go              # Dynamic k8s client wrapper (Create/Get/List/Delete/Patch/Watch)
│   │   ├── client_test.go         # CRUD operations with dynamicfake
│   │   ├── client_integration_test.go
│   │   ├── clustertype.go          # ClusterType detection from ManagedClusterInfo
│   │   └── gvr.go                 # GVR constants for all ACM resources
│   │
│   ├── fleet/                     # UC-04: Fleet observability
│   │   ├── fleet.go               # Inspector — ListClusters, GetCluster
│   │   └── fleet_test.go
│   │
│   ├── provisioning/              # UC-01: Cluster provisioning (multi-platform)
│   │   ├── provisioning.go        # Manager — Create, Destroy, Status, List, WaitForProvision
│   │   ├── builder.go             # Builds ClusterDeployment, ManagedCluster, KlusterletAddonConfig, Secrets
│   │   ├── credentials.go         # IBM Cloud CredentialsRequest definitions (5 components)
│   │   ├── ibmiam.go              # IBM Cloud IAM REST client (Service IDs, Policies, API Keys)
│   │   ├── ibmcreds.go            # Orchestrates IAM credential generation and cleanup
│   │   └── provisioning_test.go
│   │
│   ├── policy/                    # UC-02: Governance policy management
│   │   ├── policy.go              # Manager — Apply, Remove, Status, List, SetRemediation
│   │   ├── builder.go             # Builds Policy + PlacementRule + PlacementBinding
│   │   └── policy_test.go
│   │
│   ├── tenant/                    # UC-03: Tenant RBAC isolation
│   │   ├── tenant.go              # Manager — Deploy, Remove, Status, List
│   │   ├── builder.go             # Builds ManifestWork with Namespace/RoleBinding/NetworkPolicy/ResourceQuota
│   │   └── tenant_test.go
│   │
│   ├── monitoring/                # UC-06: Cluster resource monitoring
│   │   ├── monitoring.go          # Monitor — ListClusterResources, GetClusterResources
│   │   └── monitoring_test.go
│   │
│   ├── observability/             # UC-06: Thanos-based observability
│   │   ├── observability.go       # Manager — Enable, Disable, Status
│   │   ├── builder.go             # Builds MultiClusterObservability + object storage Secret
│   │   └── observability_test.go
│   │
│   ├── lifecycle/                 # UC-05: Cluster lifecycle (hibernate/resume)
│   │   ├── lifecycle.go           # Manager — Hibernate, Resume, GetPowerState, WaitForPowerState
│   │   ├── recovery.go            # PostResumeRecovery — approves expired kubelet CSRs
│   │   └── lifecycle_test.go
│   │
│   ├── importing/                 # UC-07: External cluster import
│   │   ├── importing.go           # Manager — Import, Detach, WaitForImport, GetImportStatus, ListImported
│   │   └── importing_test.go
│   │
│   ├── scaling/                   # UC-10, UC-37: Cluster scaling + flavor change
│   │   ├── scaling.go             # Manager — GetMachinePool, SetReplicas, EnableAutoscaling, SetFlavor
│   │   └── scaling_test.go
│   │
│   ├── registry/                  # UC-13: Registry mirror (ROKS, air-gapped)
│   │   ├── registry.go            # Manager — ListRequiredImages, ConfigureMirror, RemoveMirror, GetMirrorStatus
│   │   └── registry_test.go
│   │
│   ├── upgrade/                   # UC-09: Cluster version upgrades (Day-2 OCP)
│   │   ├── upgrade.go             # Manager — GetUpgradeStatus, ListUpgradeable, SetChannel, StartUpgrade, GetHistory
│   │   ├── builder.go             # Builds ManifestWork for ClusterVersion channel/upgrade patches
│   │   └── upgrade_test.go
│   │
│   ├── decommission/              # UC-08: Legacy cluster decommissioning
│   │   ├── manager.go             # Manager — Start, Advance, Cancel, List
│   │   ├── state.go               # ConfigMap-based state machine (see ADR-008)
│   │   ├── audit.go               # Audit() — collects usage data from ACM resources
│   │   ├── backup.go              # Backup() — exports cluster state to YAML
│   │   ├── drain.go               # Drain() — cordon + evict via spoke kubeconfig
│   │   ├── cleanup.go             # Cleanup() — removes hub-side resources
│   │   ├── state_test.go          # State machine unit tests
│   │   └── decommission_test.go   # Manager, audit, backup, drain, cleanup tests
│   │
│   ├── idp/                       # UC-12, UC-16: Identity Provider management
│   │   ├── idp.go                 # Manager — Configure, Remove, List, Rotate, ConfigureUnique, EnforceSSO
│   │   ├── builder.go             # Builds ManifestWork with OAuth CR + Secrets (htpasswd, GitHub, OIDC)
│   │   └── idp_test.go
│   │
│   ├── clusterset/                # UC-22, UC-24: ClusterSet management + compliance
│   │   ├── clusterset.go          # Manager — Create, List, Assign, Remove, ComplianceReport
│   │   └── clusterset_test.go
│   │
│   ├── pool/                      # UC-25: ClusterPool + ClusterClaim
│   │   ├── pool.go                # Manager — CreatePool, ListPools, GetPool, DeletePool, Claim, ReleaseClaim
│   │   └── pool_test.go
│   │
│   ├── security/                  # UC-29: Gatekeeper/OPA security baseline
│   │   ├── security.go            # Manager — ApplyBaseline, GetStatus, ListBaselines, RemoveBaseline
│   │   ├── builder.go             # Builds ManifestWork with ConstraintTemplates + Constraints
│   │   └── security_test.go
│   │
│   ├── rollout/                   # UC-33: ManifestWorkReplicaSet progressive rollout
│   │   ├── rollout.go             # Manager — Create, Get, List, Delete, UpdateStrategy
│   │   ├── builder.go             # Builds ManifestWorkReplicaSet with rollout strategies
│   │   └── rollout_test.go
│   │
│   ├── access/                    # UC-35: ManagedServiceAccount + cluster-proxy
│   │   ├── access.go              # Manager — Enable, Disable, GetStatus, List
│   │   ├── builder.go             # Builds ManagedServiceAccount + ManagedClusterAddOn
│   │   └── access_test.go
│   │
│   ├── automation/                # UC-30: PolicyAutomation (Ansible auto-remediation)
│   │   ├── automation.go          # Manager — Create, Get, List, Delete, UpdateMode
│   │   ├── builder.go             # Builds PolicyAutomation CR
│   │   └── automation_test.go
│   │
│   ├── gitops/                    # UC-32: GitOps ApplicationSet integration
│   │   ├── gitops.go              # Manager — Create, Get, List, Delete, Sync
│   │   ├── builder.go             # Builds ApplicationSet with clusterDecisionResource generator
│   │   └── gitops_test.go
│   │
│   ├── backup/                    # UC-36: Hub backup and restore
│   │   ├── backup.go              # Manager — Enable, Disable, Status, List, Restore
│   │   ├── builder.go             # Builds BackupSchedule + Restore CRs
│   │   └── backup_test.go
│   │
│   ├── batch/                     # Batch/parallel operations (shared CLI utility)
│   │   ├── batch.go               # Execute(), PrintSummary(), ToJSON()
│   │   ├── loader.go              # LoadFile(), NamesFromArgs(), ClusterItem
│   │   ├── batch_test.go
│   │   └── loader_test.go
│   │
│   └── mcp/
│       ├── server.go              # NewServer() — registers all MCP tools
│       ├── access.go              # UC-35 access MCP tools
│       ├── automation.go          # UC-30 policy automation MCP tools
│       ├── backup.go              # UC-36 hub backup MCP tools
│       ├── clusterset.go          # UC-22/24 ClusterSet MCP tools
│       ├── gitops.go              # UC-32 GitOps MCP tools
│       ├── idp.go                 # UC-12/16 IdP MCP tools
│       ├── pool.go                # UC-25 pool/claim MCP tools
│       ├── rollout.go             # UC-33 rollout MCP tools
│       ├── security.go            # UC-29 security MCP tools
│       ├── upgrade.go             # UC-09 upgrade MCP tools
│       ├── server_test.go
│       ├── server_automation_gitops_backup_test.go
│       ├── server_import_registry_scaling_test.go
│       └── server_security_rollout_access_test.go
│
├── features/                      # Gherkin .feature files (for godog)
│   ├── provisioning.feature       # UC-01 scenarios
│   ├── policy.feature             # UC-02 scenarios
│   ├── tenant.feature             # UC-03 scenarios
│   ├── fleet.feature              # UC-04 scenarios
│   ├── lifecycle.feature          # UC-05 scenarios
│   ├── monitoring.feature         # UC-06 scenarios
│   ├── importing.feature          # UC-07 scenarios
│   ├── decommission.feature       # UC-08 scenarios
│   ├── upgrade.feature            # UC-09 scenarios
│   ├── scaling.feature            # UC-10 scenarios
│   ├── registry.feature           # UC-13 scenarios
│   ├── idp-management.feature     # UC-12 scenarios
│   ├── resource-quota.feature     # UC-15 scenarios
│   ├── unique-idp.feature         # UC-16 scenarios
│   ├── clusterset.feature         # UC-22 scenarios
│   ├── compliance-report.feature  # UC-24 scenarios
│   ├── cluster-pool.feature       # UC-25 scenarios
│   ├── operator-policy.feature    # UC-27 scenarios
│   ├── certificate-policy.feature # UC-28 scenarios
│   ├── security-baseline.feature  # UC-29 scenarios
│   ├── policy-automation.feature  # UC-30 scenarios
│   ├── gitops-appset.feature      # UC-32 scenarios
│   ├── progressive-rollout.feature # UC-33 scenarios
│   ├── managed-access.feature     # UC-35 scenarios
│   ├── hub-backup.feature         # UC-36 scenarios
│   └── worker-flavor.feature      # UC-37 scenarios
│
├── integration/                   # Godog step definitions (//go:build integration)
│   └── *_steps.go                 # Step defs per UC + test runner
│
├── docs/
│   ├── specs/
│   │   ├── 2026-08-19-acm-caas-poc-design.md   # Design spec (7 UCs)
│   │   └── 2026-09-14-uc08-decommission-design.md  # UC-08 decommission design
│   ├── adr/
│   │   ├── 001-dynamic-client-over-typed.md
│   │   ├── 002-mcp-server-for-interactive-testing.md
│   │   ├── 003-gherkin-driven-with-godog.md
│   │   ├── 004-env-based-configuration.md
│   │   ├── 005-uc-packages-as-controller-foundation.md
│   │   ├── 006-idempotent-operations.md
│   │   ├── 007-minio-for-observability-object-storage.md
│   │   └── 008-configmap-state-machine-for-workflows.md
│   ├── demos/                      # Interactive demo scripts using acmlab CLI
│   │   ├── demo-uc01-provision.sh
│   │   ├── demo-uc02-policy.sh
│   │   ├── demo-uc03-tenant.sh
│   │   ├── demo-uc04-fleet.sh
│   │   ├── demo-uc05-lifecycle.sh
│   │   ├── demo-uc06-monitoring.sh
│   │   ├── demo-uc07-import.sh
│   │   ├── demo-uc08-decommission.sh
│   │   ├── demo-uc09-upgrade.sh
│   │   ├── demo-uc10-scaling.sh
│   │   ├── demo-uc12-idp.sh
│   │   ├── demo-uc13-registry.sh
│   │   ├── demo-uc15-resource-quota.sh
│   │   ├── demo-uc16-unique-idp.sh
│   │   ├── demo-uc22-clusterset.sh
│   │   ├── demo-uc24-compliance-report.sh
│   │   ├── demo-uc25-cluster-pool.sh
│   │   ├── demo-uc27-operator-pin.sh
│   │   ├── demo-uc28-cert-expiry.sh
│   │   ├── demo-uc29-security-baseline.sh
│   │   ├── demo-uc30-policy-automation.sh
│   │   ├── demo-uc32-gitops-appset.sh
│   │   ├── demo-uc33-progressive-rollout.sh
│   │   ├── demo-uc35-managed-access.sh
│   │   ├── demo-uc36-hub-backup.sh
│   │   └── demo-uc37-flavor-change.sh
│   ├── manual/                     # Raw oc/kubectl reference scripts per UC
│   │   └── uc-{01..37}-*.sh        # 26 scripts for all implemented UCs
│   ├── project-structure.md        # This file
│   └── acmlab-commands.md          # CLI + MCP command reference
│
├── .env.example                    # Template for environment variables
├── .gitignore                      # .env, style.md, vendor/, bin/, docs/superpowers/
├── go.mod
└── go.sum
```

## Design Principles

- **Three consumers, one codebase:** CLI, MCP server, and (future) ComputeRequest controller all use the same `internal/<uc>/` packages
- **Dynamic client only:** `k8s.io/client-go/dynamic` — no typed ACM/Hive imports (see ADR-001)
- **Resource construction in Go maps:** `builder.go` files build unstructured resources — no YAML templates
- **Configuration from environment:** `.env` + `godotenv` (see ADR-004)
- **Idempotent operations:** All create/apply operations use createIfNotExists or update-or-create (see ADR-006)
- **Multi-platform provisioning:** Platform-specific logic (IBM Cloud IAM) isolated in dedicated files; shared flow (ManagedCluster, KlusterletAddonConfig) applies to all clouds
- **Build tags for test scope:** `go test ./...` = unit tests only. `integration` and `slow` tags for live hub tests (see ADR-003)
