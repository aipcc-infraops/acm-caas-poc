package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/rollout"
)

func rolloutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollout",
		Short: "Manage ManifestWorkReplicaSet progressive rollouts",
	}
	cmd.AddCommand(rolloutCreateCmd(), rolloutGetCmd(), rolloutListCmd(), rolloutDeleteCmd(), rolloutUpdateStrategyCmd())
	return cmd
}

func rolloutCreateCmd() *cobra.Command {
	var placement, strategy, namespace, maxFailures, progressDeadline string
	var maxConcurrency int

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a ManifestWorkReplicaSet for fleet-wide rollout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rollout.New(c, cfg, logger)
			opts := rollout.RolloutOpts{
				Name:             args[0],
				Namespace:        namespace,
				PlacementName:    placement,
				Strategy:         strategy,
				MaxConcurrency:   maxConcurrency,
				MaxFailures:      maxFailures,
				ProgressDeadline: progressDeadline,
			}
			if err := mgr.Create(context.Background(), opts); err != nil {
				return err
			}
			fmt.Printf("ManifestWorkReplicaSet %s created (strategy=%s)\n", args[0], strategy)
			return nil
		},
	}
	cmd.Flags().StringVar(&placement, "placement", "", "Placement name for cluster selection")
	cmd.Flags().StringVar(&strategy, "strategy", "All", "Rollout strategy: All, Progressive, ProgressivePerGroup")
	cmd.Flags().IntVar(&maxConcurrency, "max-concurrency", 1, "Max clusters updated concurrently")
	cmd.Flags().StringVar(&maxFailures, "max-failures", "", "Max failures before stopping (e.g., 10%)")
	cmd.Flags().StringVar(&progressDeadline, "progress-deadline", "", "Deadline per batch (e.g., 10m)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: open-cluster-management)")
	_ = cmd.MarkFlagRequired("placement")
	return cmd
}

func rolloutGetCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get rollout status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rollout.New(c, cfg, logger)
			info, err := mgr.Get(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Name:      %s\n", info.Name)
			fmt.Printf("Strategy:  %s\n", info.Strategy)
			fmt.Printf("Applied:   %d/%d\n", info.Applied, info.Total)
			fmt.Printf("Failed:    %d\n", info.Failed)
			fmt.Printf("Status:    %s\n", info.Status)
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: open-cluster-management)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func rolloutListCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all ManifestWorkReplicaSets",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rollout.New(c, cfg, logger)
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
				fmt.Println("No rollouts found")
				return nil
			}

			fmt.Printf("%-25s  %-20s  %-8s  %-8s  %-8s  %-15s\n", "NAME", "STRATEGY", "APPLIED", "TOTAL", "FAILED", "STATUS")
			for _, r := range infos {
				fmt.Printf("%-25s  %-20s  %-8d  %-8d  %-8d  %-15s\n", r.Name, r.Strategy, r.Applied, r.Total, r.Failed, r.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: open-cluster-management)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func rolloutDeleteCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a ManifestWorkReplicaSet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rollout.New(c, cfg, logger)
			removed, err := mgr.Delete(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if !removed {
				fmt.Printf("ManifestWorkReplicaSet %s not found (nothing to remove)\n", args[0])
				return nil
			}
			fmt.Printf("ManifestWorkReplicaSet %s deleted\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: open-cluster-management)")
	return cmd
}

func rolloutUpdateStrategyCmd() *cobra.Command {
	var namespace, strategy, maxFailures, progressDeadline string
	var maxConcurrency int

	cmd := &cobra.Command{
		Use:   "update-strategy <name>",
		Short: "Update rollout strategy on an existing ManifestWorkReplicaSet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rollout.New(c, cfg, logger)
			opts := rollout.StrategyOpts{
				Type:             strategy,
				MaxConcurrency:   maxConcurrency,
				MaxFailures:      maxFailures,
				ProgressDeadline: progressDeadline,
			}
			if err := mgr.UpdateStrategy(context.Background(), args[0], namespace, opts); err != nil {
				return err
			}
			fmt.Printf("ManifestWorkReplicaSet %s strategy updated to %s\n", args[0], strategy)
			return nil
		},
	}
	cmd.Flags().StringVar(&strategy, "strategy", "Progressive", "Rollout strategy: All, Progressive, ProgressivePerGroup")
	cmd.Flags().IntVar(&maxConcurrency, "max-concurrency", 1, "Max clusters updated concurrently")
	cmd.Flags().StringVar(&maxFailures, "max-failures", "", "Max failures before stopping (e.g., 10%)")
	cmd.Flags().StringVar(&progressDeadline, "progress-deadline", "", "Deadline per batch (e.g., 10m)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: open-cluster-management)")
	return cmd
}
