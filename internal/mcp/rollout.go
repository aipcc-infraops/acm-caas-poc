package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pablofelix/acm-caas-poc/internal/rollout"
)

func registerRolloutTools(s *server.MCPServer, mgr *rollout.Manager) {
	s.AddTool(
		mcp.NewTool("acm_create_rollout",
			mcp.WithDescription("Create a ManifestWorkReplicaSet for progressive fleet-wide rollout. Supports All, Progressive, and ProgressivePerGroup strategies. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Rollout name")),
			mcp.WithString("placement", mcp.Required(), mcp.Description("Placement name for cluster selection")),
			mcp.WithString("strategy", mcp.Description("Rollout strategy: All, Progressive, ProgressivePerGroup (default: All)")),
			mcp.WithNumber("max_concurrency", mcp.Description("Max clusters updated concurrently (default: 1)")),
			mcp.WithString("max_failures", mcp.Description("Max failures before stopping (e.g., 10%)")),
			mcp.WithString("progress_deadline", mcp.Description("Deadline per batch (e.g., 10m)")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: open-cluster-management)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			placement, _ := req.RequireString("placement")
			opts := rollout.RolloutOpts{
				Name:          name,
				PlacementName: placement,
			}
			if ns, ok := req.GetArguments()["namespace"].(string); ok {
				opts.Namespace = ns
			}
			if s, ok := req.GetArguments()["strategy"].(string); ok {
				opts.Strategy = s
			}
			if mc, ok := req.GetArguments()["max_concurrency"].(float64); ok && mc > 0 {
				opts.MaxConcurrency = int(mc)
			}
			if mf, ok := req.GetArguments()["max_failures"].(string); ok {
				opts.MaxFailures = mf
			}
			if pd, ok := req.GetArguments()["progress_deadline"].(string); ok {
				opts.ProgressDeadline = pd
			}
			if err := mgr.Create(ctx, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("ManifestWorkReplicaSet %s created", name)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_get_rollout",
			mcp.WithDescription("Get ManifestWorkReplicaSet status — strategy, applied/total/failed counts, and rollout state."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Rollout name")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: open-cluster-management)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			ns, _ := req.GetArguments()["namespace"].(string)
			info, err := mgr.Get(ctx, name, ns)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_list_rollouts",
			mcp.WithDescription("List all ManifestWorkReplicaSets with rollout strategy and progress."),
			mcp.WithString("namespace", mcp.Description("Namespace (default: open-cluster-management)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ns, _ := req.GetArguments()["namespace"].(string)
			infos, err := mgr.List(ctx, ns)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(infos, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_delete_rollout",
			mcp.WithDescription("Delete a ManifestWorkReplicaSet. Returns whether anything was removed."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Rollout name")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: open-cluster-management)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			ns, _ := req.GetArguments()["namespace"].(string)
			removed, err := mgr.Delete(ctx, name, ns)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !removed {
				return mcp.NewToolResultText(fmt.Sprintf("ManifestWorkReplicaSet %s not found (nothing to remove)", name)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("ManifestWorkReplicaSet %s deleted", name)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_update_rollout_strategy",
			mcp.WithDescription("Update the rollout strategy on an existing ManifestWorkReplicaSet. Change between All, Progressive, and ProgressivePerGroup."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Rollout name")),
			mcp.WithString("strategy", mcp.Required(), mcp.Description("New strategy: All, Progressive, ProgressivePerGroup")),
			mcp.WithNumber("max_concurrency", mcp.Description("Max clusters updated concurrently")),
			mcp.WithString("max_failures", mcp.Description("Max failures before stopping (e.g., 10%)")),
			mcp.WithString("progress_deadline", mcp.Description("Deadline per batch (e.g., 10m)")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: open-cluster-management)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			strategyType, _ := req.RequireString("strategy")
			ns, _ := req.GetArguments()["namespace"].(string)
			opts := rollout.StrategyOpts{Type: strategyType}
			if mc, ok := req.GetArguments()["max_concurrency"].(float64); ok && mc > 0 {
				opts.MaxConcurrency = int(mc)
			}
			if mf, ok := req.GetArguments()["max_failures"].(string); ok {
				opts.MaxFailures = mf
			}
			if pd, ok := req.GetArguments()["progress_deadline"].(string); ok {
				opts.ProgressDeadline = pd
			}
			if err := mgr.UpdateStrategy(ctx, name, ns, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("ManifestWorkReplicaSet %s strategy updated to %s", name, strategyType)), nil
		},
	)
}
