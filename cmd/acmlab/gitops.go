package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/gitops"
)

func gitopsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gitops",
		Short: "Manage GitOps fleet deployments via ApplicationSet",
	}
	cmd.AddCommand(gitopsCreateCmd(), gitopsGetCmd(), gitopsListCmd(), gitopsDeleteCmd(), gitopsSyncCmd(), gitopsEnableAgentCmd(), gitopsDisableAgentCmd(), gitopsAgentStatusCmd())
	return cmd
}

func gitopsCreateCmd() *cobra.Command {
	var repoURL, path, revision, generator, namespace string
	var labels []string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an ApplicationSet for fleet-wide GitOps deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoURL == "" {
				return fmt.Errorf("--repo is required")
			}
			if path == "" {
				return fmt.Errorf("--path is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gitops.New(c, cfg, logger)

			labelSelector := map[string]string{}
			for _, l := range labels {
				parts := strings.SplitN(l, "=", 2)
				if len(parts) == 2 {
					labelSelector[parts[0]] = parts[1]
				}
			}

			opts := gitops.AppSetOpts{
				Name:          args[0],
				Namespace:     namespace,
				RepoURL:       repoURL,
				Path:          path,
				Revision:      revision,
				Generator:     generator,
				LabelSelector: labelSelector,
			}
			fmt.Printf("Creating ApplicationSet %s...\n", args[0])
			if err := mgr.Create(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("ApplicationSet created. Argo CD will generate Application resources per matching cluster.")
			return nil
		},
	}
	cmd.Flags().StringVar(&repoURL, "repo", "", "Git repository URL (required)")
	cmd.Flags().StringVar(&path, "path", "", "Path in repository to deploy (required)")
	cmd.Flags().StringVar(&revision, "revision", "", "Git revision (default: main)")
	cmd.Flags().StringVar(&generator, "generator", "", "Generator type: placement, cluster (default: placement)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: openshift-gitops)")
	cmd.Flags().StringSliceVar(&labels, "label", nil, "Label selector (key=value, repeatable)")
	return cmd
}

func gitopsGetCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show ApplicationSet details and generated applications",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gitops.New(c, cfg, logger)
			info, err := mgr.Get(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Name:       %s\n", info.Name)
			fmt.Printf("Namespace:  %s\n", info.Namespace)
			fmt.Printf("Repo:       %s\n", info.RepoURL)
			fmt.Printf("Path:       %s\n", info.Path)
			fmt.Printf("Generator:  %s\n", info.Generator)
			fmt.Printf("Status:     %s\n", info.Status)
			fmt.Printf("Apps:       %d\n", info.AppCount)
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: openshift-gitops)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func gitopsListCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all ApplicationSets",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gitops.New(c, cfg, logger)
			infos, err := mgr.List(context.Background(), namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(infos, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(infos) == 0 {
				fmt.Println("No ApplicationSets found")
				return nil
			}
			fmt.Printf("%-25s %-12s %-8s %s\n", "NAME", "GENERATOR", "APPS", "STATUS")
			for _, i := range infos {
				fmt.Printf("%-25s %-12s %-8d %s\n", i.Name, i.Generator, i.AppCount, i.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: openshift-gitops)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func gitopsDeleteCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an ApplicationSet and its generated Applications",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gitops.New(c, cfg, logger)
			fmt.Printf("Deleting ApplicationSet %s...\n", args[0])
			removed, err := mgr.Delete(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if removed {
				fmt.Println("ApplicationSet deleted. Generated Applications will be cleaned up by Argo CD.")
			} else {
				fmt.Printf("ApplicationSet %s not found (nothing to delete)\n", args[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: openshift-gitops)")
	return cmd
}

func gitopsSyncCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "sync <name>",
		Short: "Trigger a sync refresh on an ApplicationSet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gitops.New(c, cfg, logger)
			fmt.Printf("Triggering sync on ApplicationSet %s...\n", args[0])
			if err := mgr.Sync(context.Background(), args[0], namespace); err != nil {
				return err
			}
			fmt.Println("Sync triggered. Argo CD will refresh generated Applications.")
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: openshift-gitops)")
	return cmd
}

func gitopsEnableAgentCmd() *cobra.Command {
	var repoURL, path, revision, namespace string
	var clusters []string

	cmd := &cobra.Command{
		Use:   "enable-agent <name>",
		Short: "Enable agent-mode GitOps for disconnected clusters",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoURL == "" {
				return fmt.Errorf("--repo is required")
			}
			if path == "" {
				return fmt.Errorf("--path is required")
			}
			if len(clusters) == 0 {
				return fmt.Errorf("--cluster is required (at least one)")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gitops.New(c, cfg, logger)
			opts := gitops.AgentModeOpts{
				Name:      args[0],
				Namespace: namespace,
				RepoURL:   repoURL,
				Path:      path,
				Revision:  revision,
				Clusters:  clusters,
			}
			fmt.Printf("Enabling agent-mode GitOps %s for %d cluster(s)...\n", args[0], len(clusters))
			if err := mgr.EnableAgentMode(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("Agent-mode ApplicationSet created with PullMode=true. Disconnected clusters will pull configurations.")
			return nil
		},
	}
	cmd.Flags().StringVar(&repoURL, "repo", "", "Git repository URL (required)")
	cmd.Flags().StringVar(&path, "path", "", "Path in repository (required)")
	cmd.Flags().StringVar(&revision, "revision", "", "Git revision (default: main)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: openshift-gitops)")
	cmd.Flags().StringSliceVar(&clusters, "cluster", nil, "Target cluster names (repeatable, required)")
	return cmd
}

func gitopsDisableAgentCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "disable-agent <name>",
		Short: "Disable agent-mode GitOps",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gitops.New(c, cfg, logger)
			removed, err := mgr.DisableAgentMode(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if removed {
				fmt.Printf("Agent-mode ApplicationSet %s deleted\n", args[0])
			} else {
				fmt.Printf("Agent-mode ApplicationSet %s not found\n", args[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: openshift-gitops)")
	return cmd
}

func gitopsAgentStatusCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "agent-status <name>",
		Short: "Show agent-mode GitOps status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gitops.New(c, cfg, logger)
			info, err := mgr.AgentModeStatus(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Name:       %s\n", info.Name)
			fmt.Printf("Namespace:  %s\n", info.Namespace)
			fmt.Printf("Repo:       %s\n", info.RepoURL)
			fmt.Printf("Path:       %s\n", info.Path)
			fmt.Printf("Mode:       %s\n", info.Mode)
			fmt.Printf("Status:     %s\n", info.Status)
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: openshift-gitops)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
