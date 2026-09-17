package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/migration"
)

func migrationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migration",
		Short: "Manage planned cluster-to-cluster workload migration",
	}
	cmd.AddCommand(migrationPlanCmd(), migrationExecuteCmd(), migrationStatusCmd(), migrationRollbackCmd(), migrationListCmd())
	return cmd
}

func migrationPlanCmd() *cobra.Command {
	var target, namespaces string

	cmd := &cobra.Command{
		Use:   "plan [source-cluster]",
		Short: "Plan a workload migration from source to target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := migration.New(c, cfg, logger)
			opts := migration.MigrationOpts{
				SourceCluster: args[0],
				TargetCluster: target,
			}
			if namespaces != "" {
				opts.Namespaces = strings.Split(namespaces, ",")
			}
			fmt.Printf("Planning migration: %s -> %s ...\n", args[0], target)
			plan, err := mgr.PlanMigration(context.Background(), opts)
			if err != nil {
				return err
			}
			fmt.Printf("Plan created: %s\n", plan.PlanID)
			fmt.Printf("  Source:     %s\n", plan.SourceCluster)
			fmt.Printf("  Target:     %s\n", plan.TargetCluster)
			fmt.Printf("  Workloads:  %d\n", plan.Workloads)
			fmt.Printf("  Namespaces: %s\n", strings.Join(plan.Namespaces, ", "))
			fmt.Println("Execute with: acmlab migration execute", plan.PlanID)
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Target cluster for migration")
	cmd.Flags().StringVar(&namespaces, "namespaces", "", "Comma-separated namespaces to migrate (default: all)")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

func migrationExecuteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute [plan-id]",
		Short: "Execute a migration plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := migration.New(c, cfg, logger)
			fmt.Printf("Executing migration plan %s ...\n", args[0])
			if err := mgr.ExecuteMigration(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println("Migration in progress. Monitor with: acmlab migration status", args[0])
			return nil
		},
	}
	return cmd
}

func migrationStatusCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "status [plan-id]",
		Short: "Check migration status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := migration.New(c, cfg, logger)
			status, err := mgr.MigrationStatus(context.Background(), args[0])
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Plan:      %s\n", status.PlanID)
			fmt.Printf("Source:    %s\n", status.SourceCluster)
			fmt.Printf("Target:    %s\n", status.TargetCluster)
			fmt.Printf("Phase:     %s\n", status.Phase)
			fmt.Printf("Workloads: %d\n", status.Workloads)
			fmt.Printf("Migrated:  %d\n", status.Migrated)
			if status.Message != "" {
				fmt.Printf("Message:   %s\n", status.Message)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func migrationRollbackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback [plan-id]",
		Short: "Rollback a migration (restore source, remove target workloads)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := migration.New(c, cfg, logger)
			fmt.Printf("Rolling back migration %s ...\n", args[0])
			if err := mgr.RollbackMigration(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println("Migration rolled back.")
			return nil
		},
	}
	return cmd
}

func migrationListCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all migration plans",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := migration.New(c, cfg, logger)
			summaries, err := mgr.ListMigrations(context.Background())
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(summaries, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(summaries) == 0 {
				fmt.Println("No migration plans found")
				return nil
			}
			fmt.Printf("%-30s %-15s %-15s %-12s %s\n", "PLAN", "SOURCE", "TARGET", "PHASE", "CREATED")
			for _, s := range summaries {
				fmt.Printf("%-30s %-15s %-15s %-12s %s\n", s.PlanID, s.SourceCluster, s.TargetCluster, s.Phase, s.CreatedAt)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
