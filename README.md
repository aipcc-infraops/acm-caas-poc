# acm-caas-poc

PoC that validates Red Hat ACM 2.17 capabilities for a Containers-as-a-Service platform. The code is structured to graduate into a ComputeRequest controller.

## Use Cases

Full use case documentation with Gherkin scenarios: [docs/acm-use-cases-caas-poc.md](docs/acm-use-cases-caas-poc.md)

| UC | Description | Status | Package |
|----|-------------|--------|---------|
| UC-01 | Cluster provisioning via ClusterDeployment (multi-platform) | ✅ Implemented | `internal/provisioning/` |
| UC-02 | Governance policy management (image registry) | ✅ Implemented | `internal/policy/` |
| UC-03 | Tenant RBAC isolation via ManifestWork | ✅ Implemented | `internal/tenant/` |
| UC-04 | Fleet status and cross-cluster search | ✅ Implemented | `internal/fleet/` |
| UC-05 | Hibernate/resume lifecycle (Hive-only) | ✅ Implemented | `internal/lifecycle/` |
| UC-06 | Cluster resource monitoring & observability (Thanos) | ✅ Implemented | `internal/monitoring/`, `internal/observability/` |
| UC-07 | External cluster import | ✅ Implemented | `internal/importing/` |
| UC-13 | Registry mirror for restricted clusters (ROKS, air-gap) | ✅ Implemented | `internal/registry/` |
| UC-08 | Legacy cluster decommissioning | 📋 Planned | `internal/decommission/` |
| UC-09 | Cluster upgrades (Day-2 operations) | 📋 Planned | `internal/upgrades/` |
| UC-10 | Cluster scaling (add/remove workers) | 📋 Planned | `internal/scaling/` |
| UC-11 | Cost tracking and chargeback | 📋 Planned | `internal/cost/` |
| UC-12 | Identity Provider management (GitHub IdP, htpasswd rotation) | 📋 Planned | `internal/idp/` |
| UC-14 | Automatic cluster reclamation (idle/expired) | 📋 Planned | `internal/reclamation/` |
| UC-15 | Resource quota gates via governance policy | 📋 Planned | `internal/quota/` |
| UC-16 | Unique IdP per cluster (security hardening) | 📋 Planned | `internal/idp/` |
| UC-17 | Cost center attribution and budget alerting | 📋 Planned | `internal/cost/` |
| UC-18 | GPU sharing stack deployment (Kueue + Kyverno) | 📋 Planned | `internal/gpusharing/` |
| UC-19 | Multi-cluster GPU workload routing via Placement | 📋 Planned | `internal/gpurouting/` |
| UC-20 | OpenShift AI version fleet segregation | 📋 Planned | `internal/gpurouting/` |
| UC-21 | Elastic GPU capacity (auto-provision on saturation) | 📋 Planned | `internal/gpuelastic/` |
| UC-22 | ClusterSet management — team isolation | 📋 Planned | `internal/clusterset/` |
| UC-23 | Multi-architecture cluster matrix (QA) | 📋 Planned | `internal/matrix/` |
| UC-24 | Per-team compliance reporting | 📋 Planned | `internal/policy/` |
| UC-25 | ClusterPool + ClusterClaim (pre-warmed clusters) | 📋 Planned | `internal/pool/` |
| UC-26 | Multi-cluster networking (Submariner) | 📋 Planned | `internal/submariner/` |

### PoC Priority Areas

The PoC validates ACM value in 4 areas:

1. **Provisioning clusters** (UC-01, UC-10) ✅
2. **Managing imported clusters** (UC-07, UC-08, UC-09, UC-13) 🔄 UC-07, UC-13 done
3. **Policy management & enforcement** (UC-02, UC-12) ⚠️ UC-12 needed
4. **Monitor and alert** (UC-04, UC-06) ✅

## Quick Start

```bash
# 1. Copy and fill environment config
cp .env.example .env
# Edit .env with your kubeconfig path, context, and credentials

# 2. Build
go build -o bin/acmlab ./cmd/acmlab/

# 3. List managed clusters
bin/acmlab fleet list

# 4. Get cluster details
bin/acmlab fleet status spoke1

# 5. Monitor cluster resources
bin/acmlab monitor list

# 6. Manage governance policies
bin/acmlab policy list
bin/acmlab policy apply my-policy --remediation enforce --registries "registry.redhat.io,quay.io"
bin/acmlab policy status my-policy
bin/acmlab policy remove my-policy

# 7. Deploy tenant isolation to a spoke
bin/acmlab tenant deploy team-alpha --cluster spoke1 --team alpha-devs --cpu 8 --memory 16Gi
bin/acmlab tenant status team-alpha --cluster spoke1
bin/acmlab tenant list --cluster spoke1
bin/acmlab tenant remove team-alpha --cluster spoke1

# 8. Provision a spoke cluster (IBM Cloud, AWS, GCP, Azure)
bin/acmlab provision image-sets
bin/acmlab provision create spoke1 --pull-secret ~/pull-secret.json --region us-south
bin/acmlab provision create spoke2 --platform aws --pull-secret ~/pull-secret.json --region us-east-1
bin/acmlab provision status spoke1
bin/acmlab provision list
bin/acmlab provision destroy spoke1

# Batch provision
bin/acmlab provision destroy spoke1 spoke2 spoke3
bin/acmlab provision status spoke1 spoke2 --json

# 9. Cluster lifecycle (hibernate/resume)
bin/acmlab lifecycle list
bin/acmlab lifecycle status spoke2
bin/acmlab lifecycle hibernate spoke2 --wait
bin/acmlab lifecycle resume spoke2 --wait      # auto-recovers expired kubelet certs
bin/acmlab lifecycle diagnose spoke2
bin/acmlab lifecycle diagnose spoke2 --json

# Batch lifecycle
bin/acmlab lifecycle hibernate spoke1 spoke2 --wait
bin/acmlab lifecycle resume --from-file clusters.yaml

# 10. Import external clusters
bin/acmlab import cluster my-roks --kubeconfig-path /tmp/roks.kubeconfig --label cloud=IBM --wait
bin/acmlab import status my-roks
bin/acmlab import list
bin/acmlab import detach my-roks

# Batch import
bin/acmlab import cluster c1 c2 c3 --wait
bin/acmlab import cluster --from-file clusters.yaml
bin/acmlab import detach c1 c2 c3
bin/acmlab import status c1 c2 c3 --json

# 11. Registry mirror (for ROKS, air-gapped clusters)
bin/acmlab registry list-images import-test
bin/acmlab registry mirror-script import-test --target quay.io/myorg > mirror.sh
bin/acmlab registry configure import-test --mirror quay.io/myorg --pull-secret ~/pull-secret.json
bin/acmlab registry status import-test
bin/acmlab registry remove import-test

# Batch registry
bin/acmlab registry configure c1 c2 --mirror quay.io/myorg --pull-secret ~/pull-secret.json
bin/acmlab registry status c1 c2 c3 --json

# 12. Fleet batch
bin/acmlab fleet status spoke1 spoke2 spoke3 --json
```

## MCP Server

The MCP server exposes ACM operations as tools for Claude Code.

```bash
# Start the MCP server (stdio transport)
bin/acmlab mcp serve
```

Register in Claude Code's MCP config:

```json
{
  "mcpServers": {
    "acmlab": {
      "command": "/path/to/bin/acmlab",
      "args": ["mcp", "serve"]
    }
  }
}
```

Available tools: `acm_fleet_status`, `acm_list_managed_clusters`, `acm_get_managed_cluster`, `acm_hub_health`, `acm_list_cluster_resources`, `acm_cluster_resources`, `acm_list_policies`, `acm_get_policy`, `acm_apply_policy`, `acm_remove_policy`, `acm_set_policy_remediation`, `acm_provision_create`, `acm_provision_destroy`, `acm_provision_status`, `acm_provision_list`, `acm_list_image_sets`, `acm_hibernate_cluster`, `acm_resume_cluster`, `acm_lifecycle_status`, `acm_lifecycle_diagnose`, `acm_lifecycle_recover_certs`, `acm_list_lifecycle_clusters`, `acm_import_cluster`, `acm_detach_cluster`, `acm_import_status`, `acm_list_imported_clusters`, `acm_registry_list_images`, `acm_registry_configure_mirror`, `acm_registry_mirror_status`, `acm_registry_generate_mirror_script`.

See [docs/acmlab-commands.md](docs/acmlab-commands.md) for the full command and tool reference.

## Testing

```bash
# Unit tests only
go test ./...

# Integration tests (requires live hub connection)
go test ./... -tags=integration

# Full lifecycle tests (slow, requires provisioning access)
go test ./... -tags=slow
```

## Architecture

Three consumers share the same `internal/<uc>/` packages:

```
CLI (cmd/acmlab) ──┐
MCP server ────────┤── internal/fleet, provisioning, policy, ...
Controller (future)┘
```

All hub interaction uses `k8s.io/client-go/dynamic` — no typed ACM imports. See [ADR-001](docs/adr/001-dynamic-client-over-typed.md).

## Documentation

- [Design spec](docs/specs/2026-08-19-acm-caas-poc-design.md)
- [Project structure](docs/project-structure.md)
- [CLI & MCP commands](docs/acmlab-commands.md)
- [ADRs](docs/adr/)
