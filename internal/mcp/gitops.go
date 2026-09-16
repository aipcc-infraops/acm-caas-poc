package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pablofelix/acm-caas-poc/internal/gitops"
)

func registerGitOpsTools(s *server.MCPServer, mgr *gitops.Manager) {
	s.AddTool(
		mcp.NewTool("acm_create_appset",
			mcp.WithDescription("Create an ApplicationSet for fleet-wide GitOps deployment. Uses ACM Placement or Argo CD cluster generator to target spoke clusters by label. Deploys manifests from a Git repo path. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("ApplicationSet name")),
			mcp.WithString("repo_url", mcp.Required(), mcp.Description("Git repository URL")),
			mcp.WithString("path", mcp.Required(), mcp.Description("Path in repository to deploy")),
			mcp.WithString("revision", mcp.Description("Git revision (default: main)")),
			mcp.WithString("generator", mcp.Description("Generator type: placement, cluster (default: placement)")),
			mcp.WithString("labels", mcp.Description("Comma-separated label selectors (key=value,key=value)")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: openshift-gitops)")),
			mcp.WithString("project", mcp.Description("Argo CD project (default: default)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			repoURL, _ := req.RequireString("repo_url")
			path, _ := req.RequireString("path")
			revision, _ := req.GetArguments()["revision"].(string)
			generator, _ := req.GetArguments()["generator"].(string)
			namespace, _ := req.GetArguments()["namespace"].(string)
			project, _ := req.GetArguments()["project"].(string)

			labelSelector := map[string]string{}
			if ls, ok := req.GetArguments()["labels"].(string); ok && ls != "" {
				for _, pair := range splitTrim(ls) {
					parts := strings.SplitN(pair, "=", 2)
					if len(parts) == 2 {
						labelSelector[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
					}
				}
			}

			opts := gitops.AppSetOpts{
				Name:          name,
				Namespace:     namespace,
				RepoURL:       repoURL,
				Path:          path,
				Revision:      revision,
				Generator:     generator,
				LabelSelector: labelSelector,
				Project:       project,
			}
			if err := mgr.Create(ctx, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("ApplicationSet %s created (repo=%s, path=%s)", name, repoURL, path)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_get_appset",
			mcp.WithDescription("Get ApplicationSet details — repo, path, generator type, sync status, and generated app count."),
			mcp.WithString("name", mcp.Required(), mcp.Description("ApplicationSet name")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: openshift-gitops)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			namespace, _ := req.GetArguments()["namespace"].(string)
			info, err := mgr.Get(ctx, name, namespace)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_list_appsets",
			mcp.WithDescription("List all ApplicationSets with generator type, app count, and sync status."),
			mcp.WithString("namespace", mcp.Description("Namespace (default: openshift-gitops)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			namespace, _ := req.GetArguments()["namespace"].(string)
			infos, err := mgr.List(ctx, namespace)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(infos, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_delete_appset",
			mcp.WithDescription("Delete an ApplicationSet. Generated Applications are cleaned up by Argo CD. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("ApplicationSet name")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: openshift-gitops)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			namespace, _ := req.GetArguments()["namespace"].(string)
			removed, err := mgr.Delete(ctx, name, namespace)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !removed {
				return mcp.NewToolResultText(fmt.Sprintf("ApplicationSet %s not found (nothing to delete)", name)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("ApplicationSet %s deleted", name)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_sync_appset",
			mcp.WithDescription("Trigger a sync refresh on an ApplicationSet. Argo CD re-evaluates generators and refreshes generated Applications."),
			mcp.WithString("name", mcp.Required(), mcp.Description("ApplicationSet name")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: openshift-gitops)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			namespace, _ := req.GetArguments()["namespace"].(string)
			if err := mgr.Sync(ctx, name, namespace); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Sync triggered on ApplicationSet %s", name)), nil
		},
	)
}

