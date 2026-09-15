package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pablofelix/acm-caas-poc/internal/upgrade"
)

func registerUpgradeTools(s *server.MCPServer, mgr *upgrade.Manager) {
	s.AddTool(
		mcp.NewTool("acm_upgrade_status",
			mcp.WithDescription("Get upgrade status for a cluster — current version, channel, available updates, upgrade method (hive/manifestwork/report-only), and whether an upgrade is in progress."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Name of the managed cluster")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster := req.GetArguments()["cluster"].(string)
			status, err := mgr.GetUpgradeStatus(ctx, cluster)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(status, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_upgrade_list",
			mcp.WithDescription("List all clusters that have available OCP upgrades. Excludes vanilla Kubernetes clusters (report-only) and clusters already at latest version."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			clusters, err := mgr.ListUpgradeable(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(clusters, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_upgrade_set_channel",
			mcp.WithDescription("Set the OCP update channel for a cluster (e.g., stable-4.16, fast-4.16, candidate-4.16). Creates a ManifestWork to patch the spoke ClusterVersion."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Name of the managed cluster")),
			mcp.WithString("channel", mcp.Required(), mcp.Description("Update channel (e.g., stable-4.16, fast-4.16)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster := req.GetArguments()["cluster"].(string)
			channel := req.GetArguments()["channel"].(string)
			if err := mgr.SetChannel(ctx, cluster, channel); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Channel set to %s for cluster %s", channel, cluster)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_upgrade_start",
			mcp.WithDescription("Start an OCP version upgrade for a cluster. Creates a ManifestWork to patch the spoke ClusterVersion desiredUpdate."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Name of the managed cluster")),
			mcp.WithString("version", mcp.Required(), mcp.Description("Target OCP version (e.g., 4.16.6)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster := req.GetArguments()["cluster"].(string)
			version := req.GetArguments()["version"].(string)
			if err := mgr.StartUpgrade(ctx, cluster, version); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Upgrade to %s started for cluster %s", version, cluster)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_upgrade_history",
			mcp.WithDescription("Get the OCP version upgrade history for a cluster — past versions with state, start time, and completion time."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Name of the managed cluster")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster := req.GetArguments()["cluster"].(string)
			entries, err := mgr.GetHistory(ctx, cluster)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(entries, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}
