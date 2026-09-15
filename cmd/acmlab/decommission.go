package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/decommission"
)

func decommissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decommission",
		Short: "Manage cluster decommissioning lifecycle",
		Long: `Multi-step workflow to safely retire clusters: audit, notify,
backup, drain, delete, and clean up ACM resources.`,
	}
	cmd.AddCommand(
		decommissionStartCmd(),
		decommissionAdvanceCmd(),
		decommissionStatusCmd(),
		decommissionListCmd(),
		decommissionCancelCmd(),
		decommissionAuditCmd(),
	)
	return cmd
}

func decommissionStartCmd() *cobra.Command {
	var owner, deadline, kubeconfigPath string

	cmd := &cobra.Command{
		Use:   "start <cluster>",
		Short: "Start decommission workflow for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			m := decommission.New(c, cfg, logger)

			if deadline == "" {
				deadline = time.Now().AddDate(0, 0, 14).UTC().Format(time.RFC3339)
			}

			state, err := m.Start(context.Background(), args[0], decommission.StartOpts{
				Owner:          owner,
				Deadline:       deadline,
				KubeconfigPath: kubeconfigPath,
			})
			if err != nil {
				return err
			}

			data, _ := json.MarshalIndent(state, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&owner, "owner", "", "Cluster owner (overrides label detection)")
	cmd.Flags().StringVar(&deadline, "deadline", "", "Reclaim deadline ISO 8601 (default: 14 days)")
	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig-path", "", "Spoke kubeconfig for imported clusters")
	return cmd
}

func decommissionAdvanceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "advance <cluster>",
		Short: "Execute the next decommission phase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			m := decommission.New(c, cfg, logger)

			state, err := m.Advance(context.Background(), args[0])
			if err != nil {
				return err
			}

			data, _ := json.MarshalIndent(state, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}
}

func decommissionStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <cluster>",
		Short: "Show current decommission phase and history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			m := decommission.New(c, cfg, logger)

			state, err := m.GetState(context.Background(), args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Cluster:  %s\n", state.ClusterName)
			fmt.Printf("Phase:    %s\n", state.Phase)
			fmt.Printf("Owner:    %s\n", state.Owner)
			fmt.Printf("Deadline: %s\n", state.Deadline)
			if state.Audit != nil {
				fmt.Printf("Nodes:    %d\n", state.Audit.NodeCount)
				fmt.Printf("CPU:      %s\n", state.Audit.CPUCapacity)
				fmt.Printf("Memory:   %s\n", state.Audit.MemoryCapacity)
				fmt.Printf("Platform: %s\n", state.Audit.Platform)
			}
			if len(state.History) > 0 {
				fmt.Println("\nHistory:")
				for _, h := range state.History {
					fmt.Printf("  [%s] %s — %s\n", h.Timestamp, h.Phase, h.Message)
				}
			}
			return nil
		},
	}
}

func decommissionListCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all active decommission workflows",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			m := decommission.New(c, cfg, logger)

			states, err := m.List(context.Background())
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(states, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(states) == 0 {
				fmt.Println("No active decommission workflows")
				return nil
			}

			fmt.Printf("%-20s %-12s %-30s %s\n", "CLUSTER", "PHASE", "OWNER", "DEADLINE")
			for _, s := range states {
				fmt.Printf("%-20s %-12s %-30s %s\n", s.ClusterName, s.Phase, s.Owner, s.Deadline)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func decommissionCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <cluster>",
		Short: "Cancel decommission workflow (keeps cluster)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			m := decommission.New(c, cfg, logger)

			if err := m.Cancel(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Decommission cancelled for %s\n", args[0])
			return nil
		},
	}
}

func decommissionAuditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "audit <cluster>",
		Short: "Run standalone audit without starting decommission",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			m := decommission.New(c, cfg, logger)

			report, err := m.Audit(context.Background(), args[0])
			if err != nil {
				return err
			}

			data, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}
}
