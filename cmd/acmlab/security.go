package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/security"
)

func securityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "security",
		Short: "Manage security baselines via Gatekeeper/OPA on managed clusters",
	}
	cmd.AddCommand(securityApplyCmd(), securityStatusCmd(), securityListCmd(), securityRemoveCmd())
	return cmd
}

func securityApplyCmd() *cobra.Command {
	var cluster, clusterSet string

	cmd := &cobra.Command{
		Use:   "apply <level>",
		Short: "Apply a security baseline (e.g., cis-level1) to a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			fmt.Printf("Applying %s security baseline to %s...\n", args[0], cluster)
			if err := mgr.ApplyBaseline(context.Background(), cluster, args[0], clusterSet); err != nil {
				return err
			}
			fmt.Println("Security baseline applied. Gatekeeper constraints deployed via ManifestWork.")
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster name (required)")
	cmd.Flags().StringVar(&clusterSet, "cluster-set", "", "scope health policy to a ClusterSet")
	return cmd
}

func securityStatusCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "status <cluster>",
		Short: "Show security baseline status for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			status, err := mgr.GetStatus(context.Background(), args[0])
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Cluster:  %s\n", status.Cluster)
			fmt.Printf("Level:    %s\n", status.Level)
			fmt.Printf("Applied:  %v\n", status.Applied)
			if len(status.Conditions) > 0 {
				fmt.Println("Conditions:")
				for _, c := range status.Conditions {
					fmt.Printf("  - %s\n", c)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func securityListCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all security baselines across the fleet",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			baselines, err := mgr.ListBaselines(context.Background())
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(baselines, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(baselines) == 0 {
				fmt.Println("No security baselines found")
				return nil
			}
			fmt.Printf("%-25s %-15s %s\n", "CLUSTER", "LEVEL", "STATUS")
			for _, b := range baselines {
				fmt.Printf("%-25s %-15s %s\n", b.Cluster, b.Level, b.Status)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func securityRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <cluster>",
		Short: "Remove security baseline from a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			fmt.Printf("Removing security baseline from %s...\n", args[0])
			removed, err := mgr.RemoveBaseline(context.Background(), args[0])
			if err != nil {
				return err
			}
			if removed {
				fmt.Println("Security baseline removed. All resources cleaned up.")
			} else {
				fmt.Printf("No security baseline found on %s (nothing to remove)\n", args[0])
			}
			return nil
		},
	}
	return cmd
}
