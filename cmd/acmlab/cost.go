package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/cost"
)

func costCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Cost tracking and budget management",
	}
	cmd.AddCommand(
		costClusterCmd(),
		costReportCmd(),
		costStampCmd(),
		costByCenterCmd(),
		costCheckBudgetsCmd(),
		costCreateBudgetCmd(),
		costRemoveBudgetCmd(),
	)
	return cmd
}

func costClusterCmd() *cobra.Command {
	var days int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "cluster <name>",
		Short: "Show estimated cost for a single cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := cost.New(c, cfg, logger)

			result, err := mgr.GetClusterCost(context.Background(), args[0], days)
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Cluster:        %s\n", result.Name)
			fmt.Printf("Nodes:          %d\n", result.Nodes)
			fmt.Printf("CPU Cores:      %d\n", result.CPUCores)
			fmt.Printf("Memory (GiB):   %.1f\n", result.MemoryGiB)
			fmt.Printf("Instance Type:  %s\n", result.InstanceType)
			fmt.Printf("Daily Estimate: $%.2f\n", result.DailyEstimate)
			fmt.Printf("%d-Day Estimate: $%.2f\n", result.Days, result.PeriodEstimate)
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "Cost estimation period in days")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func costReportCmd() *cobra.Command {
	var days int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate cost report for all managed clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := cost.New(c, cfg, logger)

			report, err := mgr.GenerateReport(context.Background(), days)
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("%-20s %-6s %-6s %-10s %-12s %-12s\n", "CLUSTER", "NODES", "CPU", "MEMORY", "DAILY", fmt.Sprintf("%dD TOTAL", days))
			for _, cl := range report.Clusters {
				fmt.Printf("%-20s %-6d %-6d %-10.1f $%-11.2f $%-11.2f\n",
					cl.Name, cl.Nodes, cl.CPUCores, cl.MemoryGiB, cl.DailyEstimate, cl.PeriodEstimate)
			}
			fmt.Printf("\nTotal %d-day estimate: $%.2f\n", days, report.TotalCost)
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "Cost estimation period in days")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func costStampCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stamp <cluster> <cost-center>",
		Short: "Stamp a cost center label on a managed cluster",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := cost.New(c, cfg, logger)

			if err := mgr.StampCostCenter(context.Background(), args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Cost center %q stamped on cluster %s\n", args[1], args[0])
			return nil
		},
	}
}

func costByCenterCmd() *cobra.Command {
	var days int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "by-center",
		Short: "Show costs grouped by cost center",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := cost.New(c, cfg, logger)

			centers, err := mgr.GetCostByCenter(context.Background(), days)
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(centers, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			for _, center := range centers {
				fmt.Printf("\nCost Center: %s\n", center.CostCenter)
				fmt.Printf("  Clusters: %d\n", len(center.Clusters))
				fmt.Printf("  %d-Day Estimate: $%.2f\n", days, center.TotalEstimate)
				for _, cl := range center.Clusters {
					fmt.Printf("    - %s: $%.2f (%d nodes)\n", cl.Name, cl.PeriodEstimate, cl.Nodes)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "Cost estimation period in days")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func costCheckBudgetsCmd() *cobra.Command {
	var days int
	var budgetFlags []string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "check-budgets",
		Short: "Check cost centers against budget limits",
		Long:  "Check cost centers against budget limits. Pass budgets as --budget center=limit pairs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			budgets := make(map[string]float64)
			for _, b := range budgetFlags {
				parts := strings.SplitN(b, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid budget format %q, use center=limit", b)
				}
				limit, err := strconv.ParseFloat(parts[1], 64)
				if err != nil {
					return fmt.Errorf("invalid budget limit %q: %w", parts[1], err)
				}
				budgets[parts[0]] = limit
			}

			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := cost.New(c, cfg, logger)

			alerts, err := mgr.CheckBudgets(context.Background(), days, budgets)
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(alerts, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(alerts) == 0 {
				fmt.Println("All cost centers within budget.")
				return nil
			}
			fmt.Printf("%-20s %-12s %-12s %-12s\n", "COST CENTER", "ESTIMATE", "BUDGET", "OVERAGE")
			for _, a := range alerts {
				fmt.Printf("%-20s $%-11.2f $%-11.2f $%-11.2f\n",
					a.CostCenter, a.TotalEstimate, a.BudgetLimit, a.Overage)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "Cost estimation period in days")
	cmd.Flags().StringArrayVar(&budgetFlags, "budget", nil, "Budget limit as center=amount (repeatable)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func costCreateBudgetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create-budget <cost-center> <limit>",
		Short: "Create a budget policy for a cost center",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, err := strconv.ParseFloat(args[1], 64)
			if err != nil {
				return fmt.Errorf("invalid budget limit %q: %w", args[1], err)
			}

			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := cost.New(c, cfg, logger)

			if err := mgr.CreateBudgetPolicy(context.Background(), args[0], limit); err != nil {
				return err
			}
			fmt.Printf("Budget policy created for cost center %q with limit $%.2f\n", args[0], limit)
			return nil
		},
	}
}

func costRemoveBudgetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-budget <cost-center>",
		Short: "Remove a budget policy for a cost center",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := cost.New(c, cfg, logger)

			removed, err := mgr.RemoveBudgetPolicy(context.Background(), args[0])
			if err != nil {
				return err
			}
			if removed {
				fmt.Printf("Budget policy removed for cost center %q\n", args[0])
			} else {
				fmt.Printf("No budget policy found for cost center %q\n", args[0])
			}
			return nil
		},
	}
}
