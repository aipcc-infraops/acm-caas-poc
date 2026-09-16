package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pablofelix/acm-caas-poc/internal/access"
)

func registerAccessTools(s *server.MCPServer, mgr *access.Manager) {
	s.AddTool(
		mcp.NewTool("acm_enable_access",
			mcp.WithDescription("Enable credential-free hub-to-spoke access via ManagedServiceAccount and cluster-proxy. Creates auto-rotated token — no static kubeconfig needed. Idempotent."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Target spoke cluster")),
			mcp.WithString("ttl", mcp.Description("Token rotation interval (default: 720h)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster, _ := req.RequireString("cluster")
			opts := access.AccessOpts{}
			if ttl, ok := req.GetArguments()["ttl"].(string); ok && ttl != "" {
				opts.TTL = ttl
			}
			if err := mgr.Enable(ctx, cluster, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Managed access enabled on %s", cluster)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_disable_access",
			mcp.WithDescription("Disable credential-free access on a spoke cluster. Removes ManagedServiceAccount and addons. Idempotent."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Target spoke cluster")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster, _ := req.RequireString("cluster")
			removed, err := mgr.Disable(ctx, cluster)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !removed {
				return mcp.NewToolResultText(fmt.Sprintf("Managed access not found on %s (nothing to remove)", cluster)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Managed access disabled on %s", cluster)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_access_status",
			mcp.WithDescription("Check managed access status for a cluster — token availability, rotation interval, and addon health."),
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
		mcp.NewTool("acm_list_access",
			mcp.WithDescription("List all clusters with managed access (ManagedServiceAccount) enabled."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			infos, err := mgr.List(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(infos, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}
