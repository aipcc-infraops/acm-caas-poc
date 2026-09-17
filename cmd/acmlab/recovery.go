package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/recovery"
)

func recoveryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recovery",
		Short: "Manage workload disaster recovery (Velero + GitOps)",
	}
	cmd.AddCommand(enableDRCmd(), triggerFailoverCmd(), failoverStatusCmd(), disableDRCmd(), listDRCmd())
	return cmd
}

func enableDRCmd() *cobra.Command {
	var target, pairName, schedule, ttl string

	cmd := &cobra.Command{
		Use:   "enable-dr [source-cluster]",
		Short: "Enable disaster recovery for a cluster pair",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := recovery.New(c, cfg, logger)
			opts := recovery.DROpts{
				SourceCluster: args[0],
				TargetCluster: target,
				PairName:      pairName,
				Schedule:      schedule,
				TTL:           ttl,
			}
			fmt.Printf("Enabling DR: %s -> %s ...\n", args[0], target)
			if err := mgr.EnableDR(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("DR enabled. Velero backups will run on the source cluster.")
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Target (standby) cluster")
	cmd.Flags().StringVar(&pairName, "pair-name", "", "DR pair name (default: source-target)")
	cmd.Flags().StringVar(&schedule, "schedule", "", "Backup schedule (default: every 4 hours)")
	cmd.Flags().StringVar(&ttl, "ttl", "", "Backup TTL (default: 720h)")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

func triggerFailoverCmd() *cobra.Command {
	var target string

	cmd := &cobra.Command{
		Use:   "failover [source-cluster]",
		Short: "Trigger failover from source to target cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := recovery.New(c, cfg, logger)
			fmt.Printf("Triggering failover: %s -> %s ...\n", args[0], target)
			if err := mgr.TriggerFailover(context.Background(), args[0], target); err != nil {
				return err
			}
			fmt.Println("Failover initiated. Monitor with 'acmlab recovery failover-status'.")
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Target cluster for failover")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

func failoverStatusCmd() *cobra.Command {
	var target string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "failover-status [source-cluster]",
		Short: "Check failover status between cluster pair",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := recovery.New(c, cfg, logger)
			status, err := mgr.FailoverStatus(context.Background(), args[0], target)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Source:        %s\n", status.SourceCluster)
			fmt.Printf("Target:        %s\n", status.TargetCluster)
			fmt.Printf("Pair:          %s\n", status.PairName)
			fmt.Printf("Phase:         %s\n", status.Phase)
			fmt.Printf("Restore:       %s\n", status.RestorePhase)
			fmt.Printf("GitOps:        %s\n", status.GitOpsStatus)
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Target cluster")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

func disableDRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable-dr [cluster]",
		Short: "Disable disaster recovery for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := recovery.New(c, cfg, logger)
			fmt.Printf("Disabling DR for %s ...\n", args[0])
			if err := mgr.DisableDR(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println("DR disabled.")
			return nil
		},
	}
	return cmd
}

func listDRCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list-dr",
		Short: "List all DR pairs",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := recovery.New(c, cfg, logger)
			pairs, err := mgr.ListDRPairs(context.Background())
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(pairs, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(pairs) == 0 {
				fmt.Println("No DR pairs configured")
				return nil
			}
			fmt.Printf("%-25s %-20s %-20s %s\n", "PAIR", "SOURCE", "TARGET", "STATUS")
			for _, p := range pairs {
				fmt.Printf("%-25s %-20s %-20s %s\n", p.Name, p.SourceCluster, p.TargetCluster, p.Status)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
