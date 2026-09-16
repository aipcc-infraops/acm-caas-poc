package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pablofelix/acm-caas-poc/internal/pool"
)

func registerPoolTools(s *server.MCPServer, mgr *pool.Manager) {
	s.AddTool(
		mcp.NewTool("acm_create_pool",
			mcp.WithDescription("Create a Hive ClusterPool for pre-warmed cluster access. Clusters are kept hibernated and ready to claim in seconds. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Pool name")),
			mcp.WithNumber("size", mcp.Description("Number of pre-warmed clusters (default: 3)")),
			mcp.WithString("platform", mcp.Description("Cloud platform: ibmcloud, aws, gcp, azure (default: ibmcloud)")),
			mcp.WithString("region", mcp.Description("Cloud region (default: us-south)")),
			mcp.WithString("image_set", mcp.Description("ClusterImageSet name")),
			mcp.WithString("base_domain", mcp.Description("Base domain (default: example.com)")),
			mcp.WithString("namespace", mcp.Description("Pool namespace (default: pool name)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			opts := pool.PoolOpts{Name: name}
			if ns, ok := req.GetArguments()["namespace"].(string); ok {
				opts.Namespace = ns
			}
			if s, ok := req.GetArguments()["size"].(float64); ok && s > 0 {
				opts.Size = int(s)
			} else {
				opts.Size = 3
			}
			if p, ok := req.GetArguments()["platform"].(string); ok {
				opts.Platform = p
			}
			if r, ok := req.GetArguments()["region"].(string); ok {
				opts.Region = r
			}
			if is, ok := req.GetArguments()["image_set"].(string); ok {
				opts.ImageSet = is
			}
			if bd, ok := req.GetArguments()["base_domain"].(string); ok {
				opts.BaseDomain = bd
			}
			if err := mgr.CreatePool(ctx, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("ClusterPool %s created (size=%d)", name, opts.Size)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_list_pools",
			mcp.WithDescription("List all Hive ClusterPools with ready/claimed/standby counts."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			pools, err := mgr.ListPools(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(pools, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_get_pool",
			mcp.WithDescription("Get ClusterPool details — size, ready, claimed, standby counts."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Pool name")),
			mcp.WithString("namespace", mcp.Description("Pool namespace (default: pool name)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			ns, _ := req.GetArguments()["namespace"].(string)
			info, err := mgr.GetPool(ctx, name, ns)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_delete_pool",
			mcp.WithDescription("Delete a ClusterPool. Hive destroys all pool clusters. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Pool name")),
			mcp.WithString("namespace", mcp.Description("Pool namespace (default: pool name)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			ns, _ := req.GetArguments()["namespace"].(string)
			if err := mgr.DeletePool(ctx, name, ns); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("ClusterPool %s deleted", name)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_create_claim",
			mcp.WithDescription("Claim a pre-warmed cluster from a pool. Returns instantly — Hive provisions a replacement to maintain pool size."),
			mcp.WithString("pool", mcp.Required(), mcp.Description("ClusterPool name")),
			mcp.WithString("name", mcp.Description("Claim name (default: <pool>-claim)")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: pool name)")),
			mcp.WithString("ttl", mcp.Description("Claim lifetime (e.g., 48h)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			poolName, _ := req.RequireString("pool")
			claimName, _ := req.GetArguments()["name"].(string)
			ns, _ := req.GetArguments()["namespace"].(string)
			ttl, _ := req.GetArguments()["ttl"].(string)
			info, err := mgr.Claim(ctx, poolName, ns, claimName, ttl)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_release_claim",
			mcp.WithDescription("Release a claimed cluster back to the pool. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Claim name")),
			mcp.WithString("namespace", mcp.Required(), mcp.Description("Claim namespace")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			ns, _ := req.RequireString("namespace")
			if err := mgr.ReleaseClaim(ctx, name, ns); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("ClusterClaim %s released", name)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_list_claims",
			mcp.WithDescription("List ClusterClaims with bound cluster and status info."),
			mcp.WithString("namespace", mcp.Description("Namespace to list claims from")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ns, _ := req.GetArguments()["namespace"].(string)
			claims, err := mgr.ListClaims(ctx, ns)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(claims, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}
