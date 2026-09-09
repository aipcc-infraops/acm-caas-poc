package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
	"github.com/pablofelix/acm-caas-poc/internal/fleet"
	"github.com/pablofelix/acm-caas-poc/internal/lifecycle"
	"github.com/pablofelix/acm-caas-poc/internal/monitoring"
	"github.com/pablofelix/acm-caas-poc/internal/policy"
	"github.com/pablofelix/acm-caas-poc/internal/provisioning"
	"github.com/pablofelix/acm-caas-poc/internal/tenant"
)

func NewServer(c *client.Client, cfg config.Config) *server.MCPServer {
	s := server.NewMCPServer(
		"acmlab",
		"0.1.0",
		server.WithToolCapabilities(false),
	)

	fleetInsp := fleet.New(c, cfg)
	registerFleetTools(s, fleetInsp)
	registerHealthTool(s, c)

	mon := monitoring.New(c, cfg)
	registerMonitoringTools(s, mon)

	pol := policy.New(c, cfg)
	registerPolicyTools(s, pol)

	ten := tenant.New(c, cfg)
	registerTenantTools(s, ten)

	prov := provisioning.New(c, cfg)
	registerProvisioningTools(s, prov, cfg)

	lc := lifecycle.New(c, cfg)
	registerLifecycleTools(s, lc)

	return s
}

func registerFleetTools(s *server.MCPServer, fi *fleet.Inspector) {
	s.AddTool(
		mcp.NewTool("acm_fleet_status",
			mcp.WithDescription("Get complete fleet status — all managed clusters with health, labels, version, and conditions. Returns a summary with total/healthy/degraded counts."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			clusters, err := fi.ListClusters(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			healthy := 0
			for _, c := range clusters {
				if c.Available {
					healthy++
				}
			}
			summary := map[string]interface{}{
				"total":    len(clusters),
				"healthy":  healthy,
				"degraded": len(clusters) - healthy,
				"clusters": clusters,
			}
			data, _ := json.MarshalIndent(summary, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_list_managed_clusters",
			mcp.WithDescription("List all ManagedCluster resources on the ACM hub with status, labels, and conditions."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			clusters, err := fi.ListClusters(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(clusters, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_get_managed_cluster",
			mcp.WithDescription("Get detailed info for a specific ManagedCluster including labels, conditions, version, and health."),
			mcp.WithString("name", mcp.Required(), mcp.Description("ManagedCluster name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			info, err := fi.GetCluster(ctx, name)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

func registerMonitoringTools(s *server.MCPServer, mon *monitoring.Monitor) {
	s.AddTool(
		mcp.NewTool("acm_list_cluster_resources",
			mcp.WithDescription("List resource summaries for all clusters — node count, CPU capacity, OCP version, and channel."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			results, err := mon.ListClusterResources(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(results, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_cluster_resources",
			mcp.WithDescription("Get detailed resource info for a specific cluster — per-node CPU, memory, instance type, region, zone, and readiness."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Cluster name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			cr, err := mon.GetClusterResources(ctx, name)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(cr, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

func registerPolicyTools(s *server.MCPServer, pol *policy.Manager) {
	s.AddTool(
		mcp.NewTool("acm_list_policies",
			mcp.WithDescription("List governance policies with compliance status."),
			mcp.WithString("namespace", mcp.Description("Policy namespace (default: global-set)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ns, _ := req.GetArguments()["namespace"].(string)
			policies, err := pol.List(ctx, ns)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(policies, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_get_policy",
			mcp.WithDescription("Get detailed policy info with per-cluster compliance status."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Policy name")),
			mcp.WithString("namespace", mcp.Description("Policy namespace (default: global-set)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			ns, _ := req.GetArguments()["namespace"].(string)
			info, err := pol.Get(ctx, name, ns)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_apply_policy",
			mcp.WithDescription("Create a governance policy with placement targeting clusters by label. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Policy name")),
			mcp.WithString("namespace", mcp.Description("Policy namespace (default: global-set)")),
			mcp.WithString("remediation", mcp.Description("inform or enforce (default: inform)")),
			mcp.WithString("registries", mcp.Description("Comma-separated list of allowed container registries")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			ns, _ := req.GetArguments()["namespace"].(string)
			remediation, _ := req.GetArguments()["remediation"].(string)
			registriesStr, _ := req.GetArguments()["registries"].(string)

			opts := policy.PolicyOpts{
				Name:              name,
				Namespace:         ns,
				RemediationAction: remediation,
			}
			if registriesStr != "" {
				for _, r := range splitTrim(registriesStr) {
					opts.AllowedRegistries = append(opts.AllowedRegistries, r)
				}
			}
			if err := pol.Apply(ctx, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Policy %s applied successfully", name)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_remove_policy",
			mcp.WithDescription("Remove a policy and its placement resources. Idempotent, no leftovers."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Policy name")),
			mcp.WithString("namespace", mcp.Description("Policy namespace (default: global-set)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			ns, _ := req.GetArguments()["namespace"].(string)
			if err := pol.Remove(ctx, name, ns); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Policy %s removed successfully", name)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_set_policy_remediation",
			mcp.WithDescription("Change a policy's remediation action between inform and enforce."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Policy name")),
			mcp.WithString("action", mcp.Required(), mcp.Description("inform or enforce")),
			mcp.WithString("namespace", mcp.Description("Policy namespace (default: global-set)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			action, _ := req.RequireString("action")
			ns, _ := req.GetArguments()["namespace"].(string)
			if err := pol.SetRemediation(ctx, name, ns, action); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Policy %s remediation set to %s", name, action)), nil
		},
	)
}

func splitTrim(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func registerTenantTools(s *server.MCPServer, ten *tenant.Manager) {
	s.AddTool(
		mcp.NewTool("acm_deploy_tenant",
			mcp.WithDescription("Deploy tenant isolation (namespace, RBAC, network policy, quota) to a spoke cluster via ManifestWork. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Tenant name")),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Target spoke cluster")),
			mcp.WithString("team", mcp.Description("Team/group for RBAC (default: tenant name)")),
			mcp.WithString("cpu", mcp.Description("CPU request limit (default: 4)")),
			mcp.WithString("memory", mcp.Description("Memory request limit (default: 8Gi)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			cluster, _ := req.RequireString("cluster")
			team, _ := req.GetArguments()["team"].(string)
			cpu, _ := req.GetArguments()["cpu"].(string)
			mem, _ := req.GetArguments()["memory"].(string)
			opts := tenant.TenantOpts{
				Name:        name,
				Cluster:     cluster,
				Team:        team,
				CPULimit:    cpu,
				MemoryLimit: mem,
			}
			if err := ten.Deploy(ctx, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Tenant %s deployed to %s", name, cluster)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_remove_tenant",
			mcp.WithDescription("Remove tenant isolation from a spoke cluster. Idempotent, no leftovers."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Tenant name")),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Target spoke cluster")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			cluster, _ := req.RequireString("cluster")
			if err := ten.Remove(ctx, name, cluster); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Tenant %s removed from %s", name, cluster)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_list_tenants",
			mcp.WithDescription("List tenants deployed to a spoke cluster with sync status."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Spoke cluster")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster, _ := req.RequireString("cluster")
			tenants, err := ten.List(ctx, cluster)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(tenants, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_tenant_status",
			mcp.WithDescription("Get detailed tenant ManifestWork sync status with per-resource results."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Tenant name")),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Spoke cluster")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			cluster, _ := req.RequireString("cluster")
			ms, err := ten.Status(ctx, name, cluster)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(ms, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

func registerProvisioningTools(s *server.MCPServer, prov *provisioning.Manager, cfg config.Config) {
	s.AddTool(
		mcp.NewTool("acm_provision_create",
			mcp.WithDescription("Create a spoke cluster via Hive ClusterDeployment. Requires pull secret. Idempotent. For IBM Cloud, IAM credentials are auto-generated."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Cluster name")),
			mcp.WithString("platform", mcp.Description("Cloud platform: ibmcloud, aws, gcp, azure (default: from config)")),
			mcp.WithString("region", mcp.Description("Cloud region (default: from config)")),
			mcp.WithString("image_set", mcp.Description("ClusterImageSet name (default: from config)")),
			mcp.WithString("worker_type", mcp.Description("Worker instance type (default: from config)")),
			mcp.WithString("workers", mcp.Description("Number of worker nodes (default: 2)")),
			mcp.WithString("pull_secret", mcp.Required(), mcp.Description("Pull secret JSON content")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			pullSecret, _ := req.RequireString("pull_secret")
			platform, _ := req.GetArguments()["platform"].(string)
			region, _ := req.GetArguments()["region"].(string)
			imageSet, _ := req.GetArguments()["image_set"].(string)
			workerType, _ := req.GetArguments()["worker_type"].(string)

			opts := provisioning.ClusterOpts{
				Name:       name,
				Platform:   platform,
				Region:     region,
				ImageSet:   imageSet,
				WorkerType: workerType,
				PullSecret: pullSecret,
			}
			if err := prov.Create(ctx, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Cluster %s creation initiated. Hive will provision on IBM Cloud.", name)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_provision_destroy",
			mcp.WithDescription("Destroy a spoke cluster — deletes ClusterDeployment, Hive deprovisions infrastructure. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Cluster name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			if err := prov.Destroy(ctx, name); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Cluster %s destruction initiated", name)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_provision_status",
			mcp.WithDescription("Get ClusterDeployment provisioning status with conditions and failure info."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Cluster name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			info, err := prov.Status(ctx, name)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_provision_list",
			mcp.WithDescription("List clusters provisioned via acmlab with status."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			clusters, err := prov.List(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(clusters, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_list_image_sets",
			mcp.WithDescription("List available ClusterImageSets for provisioning."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sets, err := prov.ListImageSets(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(sets, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

func registerHealthTool(s *server.MCPServer, c *client.Client) {
	s.AddTool(
		mcp.NewTool("acm_hub_health",
			mcp.WithDescription("Check ACM hub connectivity and verify that ACM CRDs are installed. Use this before any other tool to confirm the hub is reachable."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			checks := map[string]string{}

			_, err := c.List(ctx, client.GVRManagedCluster, "", "")
			if err != nil {
				checks["ManagedCluster"] = fmt.Sprintf("FAIL: %v", err)
			} else {
				checks["ManagedCluster"] = "OK"
			}

			_, err = c.List(ctx, client.GVRClusterDeployment, "", "")
			if err != nil {
				checks["ClusterDeployment"] = fmt.Sprintf("FAIL: %v", err)
			} else {
				checks["ClusterDeployment"] = "OK"
			}

			_, err = c.List(ctx, client.GVRManifestWork, "", "")
			if err != nil {
				checks["ManifestWork"] = fmt.Sprintf("FAIL: %v", err)
			} else {
				checks["ManifestWork"] = "OK"
			}

			allOK := true
			for _, v := range checks {
				if v != "OK" {
					allOK = false
					break
				}
			}

			result := map[string]interface{}{
				"healthy": allOK,
				"checks":  checks,
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}

func registerLifecycleTools(s *server.MCPServer, lc *lifecycle.Manager) {
	s.AddTool(
		mcp.NewTool("acm_hibernate_cluster",
			mcp.WithDescription("Hibernate a Hive-provisioned cluster to save costs. Sets ClusterDeployment powerState to Hibernating. Returns immediately — poll with acm_lifecycle_status to track progress. Only works with Hive-provisioned clusters, not imported ones."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Cluster name")),
			mcp.WithString("namespace", mcp.Description("Cluster namespace (defaults to cluster name)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			namespace := name
			if ns, err := req.RequireString("namespace"); err == nil && ns != "" {
				namespace = ns
			}

			supported, err := lc.ClusterSupportsLifecycle(ctx, namespace, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("checking lifecycle support: %v", err)), nil
			}
			if !supported {
				return mcp.NewToolResultError(fmt.Sprintf("cluster %s/%s does not support lifecycle operations (no ClusterDeployment found — may be imported)", namespace, name)), nil
			}

			if err := lc.Hibernate(ctx, namespace, name); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("hibernating cluster: %v", err)), nil
			}

			state, _ := lc.GetPowerState(ctx, namespace, name)
			result := map[string]interface{}{
				"cluster":   fmt.Sprintf("%s/%s", namespace, name),
				"action":    "hibernate",
				"status":    "initiated",
				"powerState": string(state),
				"next":      "Use acm_lifecycle_status to track progress",
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_resume_cluster",
			mcp.WithDescription("Resume a hibernated Hive-provisioned cluster. Sets ClusterDeployment powerState to Running. Returns immediately — poll with acm_lifecycle_status to track progress. After resume completes, use acm_lifecycle_recover_certs to approve any expired kubelet certificates."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Cluster name")),
			mcp.WithString("namespace", mcp.Description("Cluster namespace (defaults to cluster name)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			namespace := name
			if ns, err := req.RequireString("namespace"); err == nil && ns != "" {
				namespace = ns
			}

			supported, err := lc.ClusterSupportsLifecycle(ctx, namespace, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("checking lifecycle support: %v", err)), nil
			}
			if !supported {
				return mcp.NewToolResultError(fmt.Sprintf("cluster %s/%s does not support lifecycle operations (no ClusterDeployment found — may be imported)", namespace, name)), nil
			}

			if err := lc.Resume(ctx, namespace, name); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("resuming cluster: %v", err)), nil
			}

			state, _ := lc.GetPowerState(ctx, namespace, name)
			result := map[string]interface{}{
				"cluster":   fmt.Sprintf("%s/%s", namespace, name),
				"action":    "resume",
				"status":    "initiated",
				"powerState": string(state),
				"next":      "Use acm_lifecycle_status to track progress",
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_lifecycle_status",
			mcp.WithDescription("Get the power state of a cluster (spec and status). Shows desired state vs actual state to track transitions."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Cluster name")),
			mcp.WithString("namespace", mcp.Description("Cluster namespace (defaults to cluster name)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			namespace := name
			if ns, err := req.RequireString("namespace"); err == nil && ns != "" {
				namespace = ns
			}

			supported, err := lc.ClusterSupportsLifecycle(ctx, namespace, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("checking lifecycle support: %v", err)), nil
			}
			if !supported {
				return mcp.NewToolResultError(fmt.Sprintf("cluster %s/%s does not support lifecycle operations (no ClusterDeployment found — may be imported)", namespace, name)), nil
			}

			specState, err := lc.GetPowerState(ctx, namespace, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("getting power state: %v", err)), nil
			}

			statusState, err := lc.GetPowerStateStatus(ctx, namespace, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("getting power state status: %v", err)), nil
			}

			transitioning := specState != statusState
			result := map[string]interface{}{
				"cluster":       fmt.Sprintf("%s/%s", namespace, name),
				"desiredState":  string(specState),
				"actualState":   string(statusState),
				"transitioning": transitioning,
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_lifecycle_diagnose",
			mcp.WithDescription("Diagnose cluster health by cross-referencing Hive ClusterDeployment state with ACM ManagedCluster conditions. Detects inconsistencies (e.g., Hive says Running but ACM reports unavailable), checks for problem conditions, and returns actionable suggestions."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Cluster name")),
			mcp.WithString("namespace", mcp.Description("Cluster namespace (defaults to cluster name)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			namespace := name
			if ns, err := req.RequireString("namespace"); err == nil && ns != "" {
				namespace = ns
			}

			report, err := lc.Diagnose(ctx, namespace, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("diagnosing cluster: %v", err)), nil
			}

			data, _ := json.MarshalIndent(report, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_lifecycle_recover_certs",
			mcp.WithDescription("Approve expired kubelet certificates on a spoke cluster after resume from hibernation. Kubelet certs rotate every ~24h in OpenShift — if the cluster was hibernated during rotation, certs expire and nodes cannot start pods until CSRs are approved. This tool connects to the spoke via its admin kubeconfig and approves pending kubelet CSRs automatically."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Cluster name")),
			mcp.WithString("namespace", mcp.Description("Cluster namespace (defaults to cluster name)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			namespace := name
			if ns, err := req.RequireString("namespace"); err == nil && ns != "" {
				namespace = ns
			}

			result, err := lc.PostResumeRecovery(ctx, namespace, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("recovering certificates: %v", err)), nil
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_list_lifecycle_clusters",
			mcp.WithDescription("List all Hive-provisioned clusters that support lifecycle operations (hibernate/resume). Imported clusters are excluded."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			clusters, err := lc.ListClustersWithLifecycle(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("listing clusters: %v", err)), nil
			}

			result := map[string]interface{}{
				"count":    len(clusters),
				"clusters": clusters,
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}
