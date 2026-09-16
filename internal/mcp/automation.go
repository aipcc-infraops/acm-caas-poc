package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pablofelix/acm-caas-poc/internal/automation"
)

func registerAutomationTools(s *server.MCPServer, mgr *automation.Manager) {
	s.AddTool(
		mcp.NewTool("acm_create_automation",
			mcp.WithDescription("Create a PolicyAutomation CR linking an ACM governance policy to an Ansible job template. When the policy detects violations, Ansible auto-remediates. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("PolicyAutomation name")),
			mcp.WithString("policy", mcp.Required(), mcp.Description("Referenced policy name")),
			mcp.WithString("tower_secret", mcp.Required(), mcp.Description("Secret name with Ansible Tower credentials")),
			mcp.WithString("job_template", mcp.Required(), mcp.Description("Ansible job template name")),
			mcp.WithString("mode", mcp.Description("Automation mode: scan, once, disabled (default: scan)")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: open-cluster-management-policies)")),
			mcp.WithString("tower_url", mcp.Description("Ansible Tower URL")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			policyName, _ := req.RequireString("policy")
			towerSecret, _ := req.RequireString("tower_secret")
			jobTemplate, _ := req.RequireString("job_template")
			mode, _ := req.GetArguments()["mode"].(string)
			namespace, _ := req.GetArguments()["namespace"].(string)
			towerURL, _ := req.GetArguments()["tower_url"].(string)

			opts := automation.AutomationOpts{
				Name:        name,
				Namespace:   namespace,
				PolicyName:  policyName,
				Mode:        mode,
				TowerURL:    towerURL,
				TowerSecret: towerSecret,
				JobTemplate: jobTemplate,
			}
			if err := mgr.Create(ctx, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("PolicyAutomation %s created for policy %s", name, policyName)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_get_automation",
			mcp.WithDescription("Get PolicyAutomation status — mode, linked policy, readiness, and last run time."),
			mcp.WithString("name", mcp.Required(), mcp.Description("PolicyAutomation name")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: open-cluster-management-policies)")),
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
		mcp.NewTool("acm_list_automations",
			mcp.WithDescription("List all PolicyAutomations with their linked policies, modes, and status."),
			mcp.WithString("namespace", mcp.Description("Namespace (default: open-cluster-management-policies)")),
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
		mcp.NewTool("acm_delete_automation",
			mcp.WithDescription("Delete a PolicyAutomation. Idempotent — returns success if already absent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("PolicyAutomation name")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: open-cluster-management-policies)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			namespace, _ := req.GetArguments()["namespace"].(string)
			removed, err := mgr.Delete(ctx, name, namespace)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !removed {
				return mcp.NewToolResultText(fmt.Sprintf("PolicyAutomation %s not found (nothing to delete)", name)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("PolicyAutomation %s deleted", name)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_set_automation_mode",
			mcp.WithDescription("Update a PolicyAutomation's mode — scan (continuous), once (one-shot), or disabled."),
			mcp.WithString("name", mcp.Required(), mcp.Description("PolicyAutomation name")),
			mcp.WithString("mode", mcp.Required(), mcp.Description("New mode: scan, once, disabled")),
			mcp.WithString("namespace", mcp.Description("Namespace (default: open-cluster-management-policies)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			mode, _ := req.RequireString("mode")
			namespace, _ := req.GetArguments()["namespace"].(string)
			if err := mgr.UpdateMode(ctx, name, namespace, mode); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("PolicyAutomation %s mode set to %s", name, mode)), nil
		},
	)
}
