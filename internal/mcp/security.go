package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pablofelix/acm-caas-poc/internal/security"
)

func registerSecurityTools(s *server.MCPServer, mgr *security.Manager) {
	s.AddTool(
		mcp.NewTool("acm_apply_security_baseline",
			mcp.WithDescription("Deploy Gatekeeper/OPA security baseline to a cluster via ManifestWork. Deploys ConstraintTemplates (privileged containers, hostPID, resource limits, allowed repos) and a ConfigurationPolicy to verify Gatekeeper health. Idempotent."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Target cluster name")),
			mcp.WithString("level", mcp.Description("Security level: cis-level1 (default: cis-level1)")),
			mcp.WithString("cluster_set", mcp.Description("Scope health policy to a ClusterSet")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster, _ := req.RequireString("cluster")
			level, _ := req.GetArguments()["level"].(string)
			clusterSet, _ := req.GetArguments()["cluster_set"].(string)
			if err := mgr.ApplyBaseline(ctx, cluster, level, clusterSet); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Security baseline applied to %s (level=%s)", cluster, level)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_security_status",
			mcp.WithDescription("Get security baseline status for a cluster — level, applied state, and conditions."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Cluster name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster, _ := req.RequireString("cluster")
			status, err := mgr.GetStatus(ctx, cluster)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(status, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_list_security_baselines",
			mcp.WithDescription("List all security baselines across the fleet with level and status."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			baselines, err := mgr.ListBaselines(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(baselines, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_remove_security_baseline",
			mcp.WithDescription("Remove security baseline from a cluster. Deletes ManifestWork and health policy resources. Idempotent."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Cluster name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster, _ := req.RequireString("cluster")
			removed, err := mgr.RemoveBaseline(ctx, cluster)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !removed {
				return mcp.NewToolResultText(fmt.Sprintf("No security baseline found on %s (nothing to remove)", cluster)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Security baseline removed from %s", cluster)), nil
		},
	)
}
