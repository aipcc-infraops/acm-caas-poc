package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/rollout"
)

func workorderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workorder",
		Short: "Manage ordered ManifestWork deployments (dependency-aware sequencing)",
	}
	cmd.AddCommand(
		workorderCreateCmd(),
		workorderGetCmd(),
		workorderListCmd(),
		workorderRemoveCmd(),
	)
	return cmd
}

func workorderCreateCmd() *cobra.Command {
	var cluster, manifestFile string
	cmd := &cobra.Command{
		Use:   "create-ordered <name>",
		Short: "Create a ManifestWork with ordinal-based manifest sequencing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			if manifestFile == "" {
				return fmt.Errorf("--manifests is required (JSON file with ordered manifests)")
			}
			data, err := os.ReadFile(manifestFile)
			if err != nil {
				return fmt.Errorf("reading manifests file: %w", err)
			}
			var manifests []rollout.OrderedManifest
			if err := json.Unmarshal(data, &manifests); err != nil {
				return fmt.Errorf("parsing manifests: %w", err)
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rollout.New(c, cfg, logger)
			if err := mgr.CreateOrderedWork(context.Background(), cluster, args[0], manifests); err != nil {
				return err
			}
			fmt.Printf("Ordered ManifestWork %s created on %s with %d manifests\n", args[0], cluster, len(manifests))
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster (required)")
	cmd.Flags().StringVar(&manifestFile, "manifests", "", "path to ordered manifests JSON file")
	return cmd
}

func workorderGetCmd() *cobra.Command {
	var cluster string
	cmd := &cobra.Command{
		Use:   "get-ordered <name>",
		Short: "Get status of an ordered ManifestWork",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rollout.New(c, cfg, logger)
			info, err := mgr.GetOrderedWork(context.Background(), cluster, args[0])
			if err != nil {
				return err
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster (required)")
	return cmd
}

func workorderListCmd() *cobra.Command {
	var cluster string
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "list-ordered",
		Short: "List ordered ManifestWorks on a cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rollout.New(c, cfg, logger)
			list, err := mgr.ListOrderedWork(context.Background(), cluster)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(list, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(list) == 0 {
				fmt.Printf("No ordered work found on %s\n", cluster)
				return nil
			}
			fmt.Printf("%-30s %-15s %s\n", "NAME", "CLUSTER", "MANIFESTS")
			for _, w := range list {
				fmt.Printf("%-30s %-15s %d\n", w.Name, w.Cluster, w.Manifests)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster (required)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func workorderRemoveCmd() *cobra.Command {
	var cluster string
	cmd := &cobra.Command{
		Use:   "remove-ordered <name>",
		Short: "Remove an ordered ManifestWork from a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rollout.New(c, cfg, logger)
			if err := mgr.RemoveOrderedWork(context.Background(), cluster, args[0]); err != nil {
				return err
			}
			fmt.Printf("Ordered work %s removed from %s\n", args[0], cluster)
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster (required)")
	return cmd
}
