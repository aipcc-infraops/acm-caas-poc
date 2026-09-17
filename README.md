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
| UC-08 | Legacy cluster decommissioning | ✅ Implemented | `internal/decommission/` |
| UC-09 | Cluster upgrades (Day-2 operations) | ✅ Implemented | `internal/upgrade/` |
| UC-10 | Cluster scaling (add/remove workers) | ✅ Implemented | `internal/scaling/` |
| UC-11 | Cost tracking and chargeback | ✅ Implemented | `internal/cost/` |
| UC-12 | Identity Provider management (GitHub, Google, htpasswd, LDAP, OIDC) | ✅ Implemented | `internal/idp/` |
| UC-14 | Automatic cluster reclamation (idle/expired) | ✅ Implemented | `internal/reclamation/` |
| UC-15 | Resource quota gates via governance policy | ✅ Implemented | `internal/policy/` |
| UC-16 | Unique IdP per cluster (security hardening) | ✅ Implemented | `internal/idp/` |
| UC-17 | Cost center attribution and budget alerting | ✅ Implemented | `internal/cost/` |
| UC-18 | GPU sharing stack deployment (Kueue + Kyverno) | ✅ Implemented | `internal/gpu/` |
| UC-19 | Multi-cluster GPU workload routing via Placement | ✅ Implemented | `internal/gpu/` |
| UC-20 | AI platform operator version fleet segregation | ✅ Implemented | `internal/gpu/` |
| UC-21 | Elastic GPU capacity (auto-provision on saturation) | ✅ Implemented | `internal/gpu/` |
| UC-22 | ClusterSet management — team isolation | ✅ Implemented | `internal/clusterset/` |
| UC-23 | Multi-architecture cluster matrix (QA) | ✅ Implemented | `internal/matrix/` |
| UC-24 | Per-team compliance reporting | ✅ Implemented | `internal/policy/` |
| UC-25 | ClusterPool + ClusterClaim (pre-warmed clusters) | ✅ Implemented | `internal/pool/` |
| UC-26 | Multi-cluster networking (Submariner) | ✅ Implemented | `internal/submariner/` |
| UC-27 | Operator version pinning (OperatorPolicy) | ✅ Implemented | `internal/policy/` |
| UC-28 | Certificate expiry detection fleet-wide | ✅ Implemented | `internal/policy/` |
| UC-29 | Security baseline enforcement (Gatekeeper/OPA) | ✅ Implemented | `internal/security/` |
| UC-30 | Policy automation — Ansible auto-remediation | ✅ Implemented | `internal/automation/` |
| UC-31 | SCAP scanning via Compliance Operator | ✅ Implemented | `internal/security/` |
| UC-32 | GitOps fleet deployment via ApplicationSet | ✅ Implemented | `internal/gitops/` |
| UC-33 | ManifestWorkReplicaSet progressive rollout | ✅ Implemented | `internal/rollout/` |
| UC-35 | Credential-free spoke access (ManagedServiceAccount) | ✅ Implemented | `internal/access/` |
| UC-36 | Hub backup and restore | ✅ Implemented | `internal/backup/` |
| UC-37 | Worker node flavor change (rolling replacement) | ✅ Implemented | `internal/scaling/` |
| UC-38 | HyperShift (HostedCluster) provisioning | ✅ Implemented | `internal/provisioning/` |
| UC-39 | Cloud-provider native scaling for imported clusters | ✅ Implemented | `internal/scaling/` |
| UC-40 | Cluster API (CAPI) provisioning for vanilla Kubernetes | ✅ Implemented | `internal/provisioning/` |
| UC-41 | Kubernetes cluster hibernate via CAPI scale-to-zero | ✅ Implemented | `internal/lifecycle/` |
| UC-42 | Workload disaster recovery (Velero + GitOps) | ✅ Implemented | `internal/recovery/` |
| UC-43 | Cluster relocation (planned workload migration) | ✅ Implemented | `internal/migration/` |
| UC-44 | Disconnected cluster GitOps (Argo CD Agent pull-based) | 📋 Planned | `internal/gitops/` |
| UC-45 | Fleet right-sizing recommendations (MCOA) | 📋 Planned | `internal/observability/` |
| UC-46 | Cluster Proxy (spoke service exposure to hub) | 📋 Planned | `internal/access/` |
| UC-47 | Add-on lifecycle management (AddOnDeploymentConfig) | 📋 Planned | `internal/addon/` |
| UC-48 | Placement scoring (resource-based workload scheduling) | 📋 Planned | `internal/fleet/` |
| UC-49 | PolicySet compliance profiles (golden config) | 📋 Planned | `internal/policy/` |
| UC-50 | ClusterCurator day-2 automation hooks | 📋 Planned | `internal/lifecycle/` |

### PoC Priority Areas

The PoC validates ACM value in 4 areas:

1. **Provisioning clusters** (UC-01, UC-10) ✅
2. **Managing imported clusters** (UC-07, UC-08, UC-09, UC-13) ✅
3. **Policy management & enforcement** (UC-02, UC-12) ✅
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

# Scaling
bin/acmlab scaling list
bin/acmlab scaling get spoke2
bin/acmlab scaling set spoke2 --replicas 4
bin/acmlab scaling auto spoke2 --min 2 --max 8

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

# 13. Resource quota enforcement
bin/acmlab policy apply-quota --cluster spoke1 --max-workers 5 --max-gpus 1
bin/acmlab policy quota-status spoke1

# 14. Unique IdP per cluster
bin/acmlab idp configure-unique --cluster spoke1 --admin-user cluster-admin
bin/acmlab idp enforce-sso

# 15. ClusterPool & ClusterClaim
bin/acmlab pool create amd64-419 --size 3 --image-set img4.19-multi --platform ibmcloud --region us-south --base-domain example.com
bin/acmlab pool list
bin/acmlab pool get amd64-419
bin/acmlab claim create amd64-419 --name my-claim --ttl 48h
bin/acmlab claim list
bin/acmlab claim release my-claim
bin/acmlab pool delete amd64-419

# 16. Security baseline (Gatekeeper/OPA)
bin/acmlab security deploy-baseline --cluster spoke1 --baseline cis-k8s
bin/acmlab security status spoke1
bin/acmlab security list-baselines
bin/acmlab security remove-baseline --cluster spoke1

# 17. Progressive rollout
bin/acmlab rollout create my-rollout --manifest configmap.yaml --placement prod-clusters --strategy rolling --max-concurrency 25%
bin/acmlab rollout get my-rollout
bin/acmlab rollout list
bin/acmlab rollout update-strategy my-rollout --strategy all --max-concurrency 100%
bin/acmlab rollout delete my-rollout

# 18. Managed cluster access
bin/acmlab access enable spoke1
bin/acmlab access status spoke1
bin/acmlab access list
bin/acmlab access disable spoke1

# 19. ClusterSet grouping
bin/acmlab clusterset create prod-set --namespace prod-team
bin/acmlab clusterset list
bin/acmlab clusterset assign spoke1 --set prod-set
bin/acmlab clusterset remove prod-set

# 20. Policy automation (Ansible)
bin/acmlab policy automate image-registry-policy --secret ansible-creds --job-template remediate-registry --mode scan
bin/acmlab policy automation-status image-registry-policy
bin/acmlab policy list-automations
bin/acmlab policy set-automation-mode image-registry-policy --mode once
bin/acmlab policy remove-automation image-registry-policy

# 21. GitOps (ApplicationSet)
bin/acmlab gitops create training-stack --repo https://github.com/org/configs --path teams/training
bin/acmlab gitops get training-stack
bin/acmlab gitops list
bin/acmlab gitops sync training-stack
bin/acmlab gitops delete training-stack

# 22. Hub backup and restore
bin/acmlab backup enable --schedule "0 */6 * * *" --ttl 720h
bin/acmlab backup status
bin/acmlab backup list
bin/acmlab backup restore --backup-name acm-backup-2026-09-17-06-00-00
bin/acmlab backup disable
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

Available tools: `acm_fleet_status`, `acm_list_managed_clusters`, `acm_get_managed_cluster`, `acm_hub_health`, `acm_list_cluster_resources`, `acm_cluster_resources`, `acm_deploy_tenant`, `acm_remove_tenant`, `acm_list_tenants`, `acm_tenant_status`, `acm_list_policies`, `acm_get_policy`, `acm_apply_policy`, `acm_remove_policy`, `acm_set_policy_remediation`, `acm_apply_quota_policy`, `acm_quota_status`, `acm_provision_create`, `acm_provision_destroy`, `acm_provision_status`, `acm_provision_list`, `acm_list_image_sets`, `acm_hibernate_cluster`, `acm_resume_cluster`, `acm_lifecycle_status`, `acm_lifecycle_diagnose`, `acm_lifecycle_recover_certs`, `acm_list_lifecycle_clusters`, `acm_import_cluster`, `acm_detach_cluster`, `acm_import_status`, `acm_list_imported_clusters`, `acm_registry_list_images`, `acm_registry_configure_mirror`, `acm_registry_mirror_status`, `acm_registry_generate_mirror_script`, `acm_configure_idp`, `acm_remove_idp`, `acm_list_idps`, `acm_rotate_idp`, `acm_configure_unique_idp`, `acm_enforce_sso`, `acm_scaling_set_flavor`, `acm_create_pool`, `acm_list_pools`, `acm_get_pool`, `acm_delete_pool`, `acm_create_claim`, `acm_release_claim`, `acm_list_claims`, `acm_list_clustersets`, `acm_create_clusterset`, `acm_remove_clusterset`, `acm_assign_cluster_to_set`, `acm_create_automation`, `acm_get_automation`, `acm_list_automations`, `acm_delete_automation`, `acm_set_automation_mode`, `acm_create_appset`, `acm_get_appset`, `acm_list_appsets`, `acm_delete_appset`, `acm_sync_appset`, `acm_enable_backup`, `acm_disable_backup`, `acm_backup_status`, `acm_list_backups`, `acm_restore_backup`, `acm_apply_security_baseline`, `acm_security_status`, `acm_list_security_baselines`, `acm_remove_security_baseline`, `acm_create_rollout`, `acm_get_rollout`, `acm_list_rollouts`, `acm_delete_rollout`, `acm_update_rollout_strategy`, `acm_enable_access`, `acm_disable_access`, `acm_access_status`, `acm_list_access`, `acm_scaling_get`, `acm_scaling_set`, `acm_scaling_auto`, `acm_scaling_list`, `acm_upgrade_status`, `acm_upgrade_list`, `acm_upgrade_set_channel`, `acm_upgrade_start`, `acm_upgrade_history`, `acm_decommission_start`, `acm_decommission_advance`, `acm_decommission_status`, `acm_decommission_list`, `acm_decommission_cancel`, `acm_decommission_audit`.

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
