package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/idp"
)

func idpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "idp",
		Short: "Manage Identity Providers across clusters via ManifestWork",
	}
	cmd.AddCommand(
		idpConfigureCmd(),
		idpConfigureUniqueCmd(),
		idpRemoveCmd(),
		idpListCmd(),
		idpRotateCmd(),
		idpEnforceSSOCmd(),
	)
	return cmd
}

func idpConfigureCmd() *cobra.Command {
	var (
		cluster       string
		idpType       string
		clientID      string
		clientSecret  string
		organizations string
		issuerURL     string
		users         string
		ldapURL       string
		bindDN        string
		bindPassword  string
		insecure      bool
	)
	cmd := &cobra.Command{
		Use:   "configure <name>",
		Short: "Configure an Identity Provider on a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			if idpType == "" {
				return fmt.Errorf("--type is required (github, google, htpasswd, ldap, oidc)")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := idp.New(c, cfg, logger)

			opts := idp.IdPOpts{
				Name:         args[0],
				Cluster:      cluster,
				Type:         idp.IdPType(idpType),
				ClientID:     clientID,
				ClientSecret: clientSecret,
				IssuerURL:    issuerURL,
				LDAPURL:      ldapURL,
				BindDN:       bindDN,
				BindPassword: bindPassword,
				Insecure:     insecure,
			}
			if organizations != "" {
				for _, o := range strings.Split(organizations, ",") {
					o = strings.TrimSpace(o)
					if o != "" {
						opts.Organizations = append(opts.Organizations, o)
					}
				}
			}
			if users != "" {
				opts.Users = parseUsers(users)
			}

			if err := mgr.Configure(context.Background(), opts); err != nil {
				return err
			}
			fmt.Printf("IdP %s (%s) configured on cluster %s\n", args[0], idpType, cluster)
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster (required)")
	cmd.Flags().StringVar(&idpType, "type", "", "IdP type: github, google, htpasswd, ldap, oidc (required)")
	cmd.Flags().StringVar(&clientID, "client-id", "", "OAuth client ID (github, google, oidc)")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "OAuth client secret (github, google, oidc)")
	cmd.Flags().StringVar(&organizations, "organizations", "", "GitHub organizations (comma-separated)")
	cmd.Flags().StringVar(&issuerURL, "issuer-url", "", "OIDC issuer URL (oidc)")
	cmd.Flags().StringVar(&users, "users", "", "htpasswd users (user1:pass1,user2:pass2)")
	cmd.Flags().StringVar(&ldapURL, "ldap-url", "", "LDAP server URL")
	cmd.Flags().StringVar(&bindDN, "bind-dn", "", "LDAP bind DN")
	cmd.Flags().StringVar(&bindPassword, "bind-password", "", "LDAP bind password")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "LDAP insecure connection")
	return cmd
}

func idpRemoveCmd() *cobra.Command {
	var cluster string
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an Identity Provider from a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := idp.New(c, cfg, logger)
			if err := mgr.Remove(context.Background(), args[0], cluster); err != nil {
				return err
			}
			fmt.Printf("IdP %s removed from cluster %s\n", args[0], cluster)
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster (required)")
	return cmd
}

func idpListCmd() *cobra.Command {
	var (
		cluster    string
		outputJSON bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Identity Providers on a cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := idp.New(c, cfg, logger)
			infos, err := mgr.List(context.Background(), cluster)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(infos, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(infos) == 0 {
				fmt.Printf("No IdPs configured on cluster %s\n", cluster)
				return nil
			}
			fmt.Printf("%-20s %-10s %-10s\n", "NAME", "TYPE", "STATUS")
			for _, i := range infos {
				fmt.Printf("%-20s %-10s %-10s\n", i.Name, i.Type, i.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster (required)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func idpRotateCmd() *cobra.Command {
	var (
		cluster      string
		clientID     string
		clientSecret string
		users        string
		bindPassword string
	)
	cmd := &cobra.Command{
		Use:   "rotate <name>",
		Short: "Rotate credentials for an Identity Provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := idp.New(c, cfg, logger)

			opts := idp.IdPOpts{
				ClientID:     clientID,
				ClientSecret: clientSecret,
				BindPassword: bindPassword,
			}
			if users != "" {
				opts.Users = parseUsers(users)
			}

			if err := mgr.Rotate(context.Background(), args[0], cluster, opts); err != nil {
				return err
			}
			fmt.Printf("IdP %s credentials rotated on cluster %s\n", args[0], cluster)
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster (required)")
	cmd.Flags().StringVar(&clientID, "client-id", "", "new OAuth client ID")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "new OAuth client secret")
	cmd.Flags().StringVar(&users, "users", "", "new htpasswd users (user1:pass1,user2:pass2)")
	cmd.Flags().StringVar(&bindPassword, "bind-password", "", "new LDAP bind password")
	return cmd
}

func idpConfigureUniqueCmd() *cobra.Command {
	var (
		cluster   string
		adminUser string
	)
	cmd := &cobra.Command{
		Use:   "configure-unique",
		Short: "Deploy unique emergency htpasswd credentials to a cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			if adminUser == "" {
				adminUser = "cluster-admin"
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := idp.New(c, cfg, logger)
			password, err := mgr.ConfigureUnique(context.Background(), cluster, adminUser)
			if err != nil {
				return err
			}
			fmt.Printf("Unique emergency IdP deployed to cluster %s\n", cluster)
			fmt.Printf("  User:     %s\n", adminUser)
			fmt.Printf("  Password: %s\n", password)
			fmt.Println("  (save this password — it will not be shown again)")
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster (required)")
	cmd.Flags().StringVar(&adminUser, "admin-user", "cluster-admin", "admin username")
	return cmd
}

func idpEnforceSSOCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "enforce-sso",
		Short: "Create fleet-wide SSO compliance policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := idp.New(c, cfg, logger)
			if err := mgr.EnforceSSO(context.Background(), namespace); err != nil {
				return err
			}
			fmt.Println("SSO enforcement policy created — clusters without OpenID IdP will be marked NonCompliant")
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "policy namespace (default: global-set)")
	return cmd
}

func parseUsers(s string) map[string]string {
	users := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			users[parts[0]] = parts[1]
		}
	}
	return users
}
