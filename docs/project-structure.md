# Project Structure

```
acm-caas-poc/
│
├── cmd/acmlab/                    # CLI entry point
│   ├── main.go                    # Cobra root command, .env loading, global flags
│   ├── fleet.go                   # fleet list, fleet status <name>
│   ├── provision.go               # provision create/destroy/status/list/image-sets
│   ├── policy.go                  # policy list/apply/status/remove
│   ├── tenant.go                  # tenant deploy/status/list/remove
│   ├── monitor.go                 # monitor list/status/setup/teardown/obs-status
│   ├── lifecycle.go               # lifecycle hibernate/resume/status/diagnose/list
│   ├── import.go                  # import cluster/detach/status/list
│   ├── registry.go                # registry list-images/mirror-script/configure/status/remove
│   ├── scaling.go                 # scaling list/get/set/auto
│   ├── decommission.go            # decommission start/advance/status/list/cancel/audit
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
│   ├── scaling/                   # UC-10: Cluster scaling (add/remove workers)
│   │   ├── scaling.go             # Manager — GetMachinePool, SetReplicas, EnableAutoscaling, InitMachinePool
│   │   └── scaling_test.go
│   │
│   ├── registry/                  # UC-13: Registry mirror (ROKS, air-gapped)
│   │   ├── registry.go            # Manager — ListRequiredImages, ConfigureMirror, RemoveMirror, GenerateMirrorScript
│   │   └── registry_test.go
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
│   ├── batch/                     # Batch/parallel operations (shared CLI utility)
│   │   ├── batch.go               # Execute(), PrintSummary(), ToJSON()
│   │   ├── loader.go              # LoadFile(), NamesFromArgs(), ClusterItem
│   │   ├── batch_test.go
│   │   └── loader_test.go
│   │
│   └── mcp/
│       ├── server.go              # NewServer() — registers all MCP tools
│       └── server_test.go
│
├── features/                      # Gherkin .feature files (for godog)
│   ├── provisioning.feature       # UC-01 scenarios
│   ├── policy.feature             # UC-02 scenarios
│   ├── tenant.feature             # UC-03 scenarios
│   ├── fleet.feature              # UC-04 scenarios
│   ├── lifecycle.feature          # UC-05 scenarios
│   ├── monitoring.feature         # UC-06 scenarios
│   ├── importing.feature          # UC-07 scenarios
│   ├── scaling.feature            # UC-10 scenarios
│   └── registry.feature           # UC-13 scenarios
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
│   ├── demos/                      # Interactive demo scripts (phase-based for long ops)
│   │   ├── demo-uc01-provision.sh
│   │   ├── demo-uc02-policy.sh
│   │   ├── demo-uc03-tenant.sh
│   │   ├── demo-uc04-fleet.sh
│   │   ├── demo-uc05-lifecycle.sh
│   │   ├── demo-uc06-monitoring.sh
│   │   ├── demo-uc07-import.sh
│   │   ├── demo-uc10-scaling.sh
│   │   ├── demo-uc08-decommission.sh
│   │   └── demo-uc13-registry.sh
│   ├── manual/                     # Kubectl/curl manual reference scripts per UC
│   │   ├── uc-01-provision.sh
│   │   ├── uc-02-policy.sh
│   │   ├── uc-03-tenant.sh
│   │   ├── uc-04-fleet.sh
│   │   ├── uc-05-lifecycle.sh
│   │   ├── uc-06-monitoring.sh
│   │   ├── uc-07-import-detach.sh
│   │   ├── uc-10-scaling.sh
│   │   ├── uc-08-decommission.sh
│   │   └── uc-13-registry-mirror.sh
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
