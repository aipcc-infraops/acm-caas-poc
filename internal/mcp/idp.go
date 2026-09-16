package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pablofelix/acm-caas-poc/internal/idp"
)

func registerIdPTools(s *server.MCPServer, mgr *idp.Manager) {
	s.AddTool(
		mcp.NewTool("acm_configure_idp",
			mcp.WithDescription("Configure an Identity Provider on a cluster via ManifestWork. Supports GitHub, Google, htpasswd, LDAP, and OIDC (Keycloak). Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("IdP name")),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Target cluster")),
			mcp.WithString("type", mcp.Required(), mcp.Description("IdP type: github, google, htpasswd, ldap, oidc")),
			mcp.WithString("client_id", mcp.Description("OAuth client ID (github, google, oidc)")),
			mcp.WithString("client_secret", mcp.Description("OAuth client secret (github, google, oidc)")),
			mcp.WithString("organizations", mcp.Description("GitHub organizations (comma-separated)")),
			mcp.WithString("issuer_url", mcp.Description("OIDC issuer URL")),
			mcp.WithString("users", mcp.Description("htpasswd users as JSON object: {\"user1\":\"pass1\",\"user2\":\"pass2\"}")),
			mcp.WithString("ldap_url", mcp.Description("LDAP server URL")),
			mcp.WithString("bind_dn", mcp.Description("LDAP bind DN")),
			mcp.WithString("bind_password", mcp.Description("LDAP bind password")),
			mcp.WithBoolean("insecure", mcp.Description("LDAP insecure connection")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			cluster, _ := req.RequireString("cluster")
			idpType, _ := req.RequireString("type")

			opts := idp.IdPOpts{
				Name:    name,
				Cluster: cluster,
				Type:    idp.IdPType(idpType),
			}
			if v, ok := req.GetArguments()["client_id"].(string); ok {
				opts.ClientID = v
			}
			if v, ok := req.GetArguments()["client_secret"].(string); ok {
				opts.ClientSecret = v
			}
			if v, ok := req.GetArguments()["issuer_url"].(string); ok {
				opts.IssuerURL = v
			}
			if v, ok := req.GetArguments()["ldap_url"].(string); ok {
				opts.LDAPURL = v
			}
			if v, ok := req.GetArguments()["bind_dn"].(string); ok {
				opts.BindDN = v
			}
			if v, ok := req.GetArguments()["bind_password"].(string); ok {
				opts.BindPassword = v
			}
			if v, ok := req.GetArguments()["insecure"].(bool); ok {
				opts.Insecure = v
			}
			if orgs, ok := req.GetArguments()["organizations"].(string); ok && orgs != "" {
				for _, o := range strings.Split(orgs, ",") {
					o = strings.TrimSpace(o)
					if o != "" {
						opts.Organizations = append(opts.Organizations, o)
					}
				}
			}
			if usersJSON, ok := req.GetArguments()["users"].(string); ok && usersJSON != "" {
				users := map[string]string{}
				if err := json.Unmarshal([]byte(usersJSON), &users); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("parsing users JSON: %v", err)), nil
				}
				opts.Users = users
			}

			if err := mgr.Configure(ctx, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("IdP %s (%s) configured on cluster %s", name, idpType, cluster)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_configure_unique_idp",
			mcp.WithDescription("Deploy unique emergency htpasswd credentials to a cluster. Generates a random password, deploys via ManifestWork. Each cluster gets different credentials — a leak on one cluster does not affect the fleet."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Target cluster")),
			mcp.WithString("admin_user", mcp.Description("Admin username (default: cluster-admin)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster, _ := req.RequireString("cluster")
			adminUser := "cluster-admin"
			if v, ok := req.GetArguments()["admin_user"].(string); ok && v != "" {
				adminUser = v
			}
			password, err := mgr.ConfigureUnique(ctx, cluster, adminUser)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			result := map[string]interface{}{
				"cluster":  cluster,
				"user":     adminUser,
				"password": password,
				"message":  "Unique emergency IdP deployed. Save this password — it will not be shown again.",
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_enforce_sso",
			mcp.WithDescription("Create a fleet-wide SSO compliance policy. Clusters without an OpenID (SSO) Identity Provider will be marked NonCompliant."),
			mcp.WithString("namespace", mcp.Description("Policy namespace (default: global-set)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ns, _ := req.GetArguments()["namespace"].(string)
			if err := mgr.EnforceSSO(ctx, ns); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("SSO enforcement policy created — clusters without OpenID IdP will be marked NonCompliant"), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_remove_idp",
			mcp.WithDescription("Remove an Identity Provider from a cluster. Idempotent."),
			mcp.WithString("name", mcp.Required(), mcp.Description("IdP name")),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Target cluster")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			cluster, _ := req.RequireString("cluster")
			if err := mgr.Remove(ctx, name, cluster); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("IdP %s removed from cluster %s", name, cluster)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_list_idps",
			mcp.WithDescription("List Identity Providers configured on a cluster."),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Target cluster")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cluster, _ := req.RequireString("cluster")
			infos, err := mgr.List(ctx, cluster)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(infos, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_rotate_idp",
			mcp.WithDescription("Rotate credentials for an Identity Provider. Updates the ManifestWork Secret."),
			mcp.WithString("name", mcp.Required(), mcp.Description("IdP name")),
			mcp.WithString("cluster", mcp.Required(), mcp.Description("Target cluster")),
			mcp.WithString("client_id", mcp.Description("New OAuth client ID")),
			mcp.WithString("client_secret", mcp.Description("New OAuth client secret")),
			mcp.WithString("users", mcp.Description("New htpasswd users as JSON")),
			mcp.WithString("bind_password", mcp.Description("New LDAP bind password")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.RequireString("name")
			cluster, _ := req.RequireString("cluster")
			opts := idp.IdPOpts{}
			if v, ok := req.GetArguments()["client_id"].(string); ok {
				opts.ClientID = v
			}
			if v, ok := req.GetArguments()["client_secret"].(string); ok {
				opts.ClientSecret = v
			}
			if v, ok := req.GetArguments()["bind_password"].(string); ok {
				opts.BindPassword = v
			}
			if usersJSON, ok := req.GetArguments()["users"].(string); ok && usersJSON != "" {
				users := map[string]string{}
				if err := json.Unmarshal([]byte(usersJSON), &users); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("parsing users JSON: %v", err)), nil
				}
				opts.Users = users
			}
			if err := mgr.Rotate(ctx, name, cluster, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("IdP %s credentials rotated on cluster %s", name, cluster)), nil
		},
	)
}
