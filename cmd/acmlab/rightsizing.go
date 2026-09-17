package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/rightsizing"
)

func rightsizingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rightsizing",
		Short: "Manage resource right-sizing monitoring and recommendations",
	}
	cmd.AddCommand(rightsizingEnableCmd(), rightsizingDisableCmd(), rightsizingAdviseCmd(), rightsizingAdjustCmd(), rightsizingListCmd())
	return cmd
}

func rightsizingEnableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <cluster>",
		Short: "Enable right-sizing monitoring on a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rightsizing.New(c, cfg, logger)
			fmt.Printf("Enabling right-sizing monitoring on %s...\n", args[0])
			if err := mgr.Enable(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Right-sizing monitoring enabled on %s\n", args[0])
			return nil
		},
	}
	return cmd
}

func rightsizingDisableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <cluster>",
		Short: "Disable right-sizing monitoring on a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rightsizing.New(c, cfg, logger)
			removed, err := mgr.Disable(context.Background(), args[0])
			if err != nil {
				return err
			}
			if removed {
				fmt.Printf("Right-sizing monitoring disabled on %s\n", args[0])
			} else {
				fmt.Printf("Right-sizing not enabled on %s (nothing to remove)\n", args[0])
			}
			return nil
		},
	}
	return cmd
}

func rightsizingAdviseCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "advise <cluster>",
		Short: "Get right-sizing recommendations for workloads on a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rightsizing.New(c, cfg, logger)
			recs, err := mgr.Advise(context.Background(), args[0])
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(recs, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(recs) == 0 {
				fmt.Println("No recommendations available")
				return nil
			}

			fmt.Printf("%-20s %-15s %-12s %-12s %-12s %-12s %s\n", "WORKLOAD", "CONTAINER", "CUR CPU", "REC CPU", "CUR MEM", "REC MEM", "SAVINGS")
			for _, r := range recs {
				fmt.Printf("%-20s %-15s %-12s %-12s %-12s %-12s %s\n", r.Workload, r.Container, r.CurrentCPU, r.RecommendedCPU, r.CurrentMem, r.RecommendedMem, r.Savings)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func rightsizingAdjustCmd() *cobra.Command {
	var outputJSON bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "adjust <cluster>",
		Short: "Apply right-sizing adjustments to workloads on a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rightsizing.New(c, cfg, logger)
			results, err := mgr.Adjust(context.Background(), args[0], dryRun)
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(results, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(results) == 0 {
				fmt.Println("No adjustments to apply")
				return nil
			}

			mode := "APPLIED"
			if dryRun {
				mode = "DRY-RUN"
			}
			fmt.Printf("[%s]\n", mode)
			fmt.Printf("%-25s %-15s %-10s %s\n", "WORKLOAD", "CONTAINER", "ACTION", "DETAIL")
			for _, r := range results {
				fmt.Printf("%-25s %-15s %-10s %s\n", r.Workload, r.Container, r.Action, r.Detail)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show proposed changes without applying")
	return cmd
}

func rightsizingListCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List clusters with right-sizing monitoring enabled",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := rightsizing.New(c, cfg, logger)
			infos, err := mgr.List(context.Background())
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(infos, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(infos) == 0 {
				fmt.Println("No clusters with right-sizing enabled")
				return nil
			}

			fmt.Printf("%-20s %-8s %s\n", "CLUSTER", "ENABLED", "STATUS")
			for _, i := range infos {
				fmt.Printf("%-20s %-8v %s\n", i.Cluster, i.Enabled, i.Status)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
