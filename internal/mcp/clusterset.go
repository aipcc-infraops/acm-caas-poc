package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pablofelix/acm-caas-poc/internal/clusterset"
)

func registerClusterSetTools(s *server.MCPServer, cs *clusterset.Manager) {
	s.AddTool(
		mcp.NewTool("acm_list_clustersets",
			mcp.WithDescription("List all ManagedClusterSets with member clusters and counts."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sets, err := cs.List(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(sets, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_create_clusterset",
			mcp.WithDescription("Create a ManagedClusterSet with a binding in a team namespace. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("ClusterSet name")),
			mcp.WithString("namespace", mcp.Required(), mcp.Description("Team namespace for the ClusterSetBinding")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			ns, _ := req.RequireString("namespace")
			if err := cs.Create(ctx, name, ns); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("ClusterSet %s created with binding in %s", name, ns)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_remove_clusterset",
			mcp.WithDescription("Remove a ManagedClusterSet and its binding. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("ClusterSet name")),
			mcp.WithString("namespace", mcp.Required(), mcp.Description("Namespace of the ClusterSetBinding")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			ns, _ := req.RequireString("namespace")
			if err := cs.Remove(ctx, name, ns); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("ClusterSet %s removed", name)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_assign_cluster_to_set",
			mcp.WithDescription("Assign a managed cluster to a ClusterSet by updating its label."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Cluster name")),
			mcp.WithString("set", mcp.Required(), mcp.Description("Target ClusterSet name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster, _ := req.RequireString("cluster")
			set, _ := req.RequireString("set")
			if err := cs.Assign(ctx, cluster, set); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Cluster %s assigned to ClusterSet %s", cluster, set)), nil
		},
	)
}
