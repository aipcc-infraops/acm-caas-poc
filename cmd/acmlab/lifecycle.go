package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/lifecycle"
)

func lifecycleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lifecycle",
		Short: "Manage cluster lifecycle (hibernate/resume)",
		Long: `Manage cluster power state via Hive ClusterDeployment.

Only works with Hive-provisioned clusters. Imported clusters do not support
lifecycle operations.`,
	}
	cmd.AddCommand(hibernateCmd(), resumeCmd(), lifecycleStatusCmd(), lifecycleDiagnoseCmd(), lifecycleListCmd())
	return cmd
}

func hibernateCmd() *cobra.Command {
	var namespace string
	var doWait bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "hibernate <cluster-name>",
		Short: "Hibernate a cluster to save costs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			if namespace == "" {
				namespace = clusterName
			}

			c, err := buildClient()
			if err != nil {
				return err
			}

			m := lifecycle.New(c, cfg)
			ctx := context.Background()

			supported, err := m.ClusterSupportsLifecycle(ctx, namespace, clusterName)
			if err != nil {
				return fmt.Errorf("checking lifecycle support: %w", err)
			}
			if !supported {
				return fmt.Errorf("cluster %s/%s does not support lifecycle operations (no ClusterDeployment found — may be imported)", namespace, clusterName)
			}

			if err := m.Hibernate(ctx, namespace, clusterName); err != nil {
				return fmt.Errorf("hibernating cluster: %w", err)
			}

			fmt.Printf("Cluster %s/%s is hibernating\n", namespace, clusterName)

			if doWait {
				fmt.Printf("Waiting for cluster to hibernate (timeout: %v)...\n", timeout)
				if err := m.WaitForPowerState(ctx, namespace, clusterName, lifecycle.PowerStateHibernating, timeout); err != nil {
					return fmt.Errorf("waiting for hibernation: %w", err)
				}
				fmt.Println("Cluster successfully hibernated")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Cluster namespace (defaults to cluster name)")
	cmd.Flags().BoolVar(&doWait, "wait", false, "Wait for hibernation to complete")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Timeout for wait operation")

	return cmd
}

func resumeCmd() *cobra.Command {
	var namespace string
	var doWait bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "resume <cluster-name>",
		Short: "Resume a hibernated cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			if namespace == "" {
				namespace = clusterName
			}

			c, err := buildClient()
			if err != nil {
				return err
			}

			m := lifecycle.New(c, cfg)
			ctx := context.Background()

			supported, err := m.ClusterSupportsLifecycle(ctx, namespace, clusterName)
			if err != nil {
				return fmt.Errorf("checking lifecycle support: %w", err)
			}
			if !supported {
				return fmt.Errorf("cluster %s/%s does not support lifecycle operations (no ClusterDeployment found — may be imported)", namespace, clusterName)
			}

			if err := m.Resume(ctx, namespace, clusterName); err != nil {
				return fmt.Errorf("resuming cluster: %w", err)
			}

			fmt.Printf("Cluster %s/%s is resuming\n", namespace, clusterName)

			if doWait {
				fmt.Printf("Waiting for cluster to resume (timeout: %v)...\n", timeout)
				if err := m.WaitForPowerState(ctx, namespace, clusterName, lifecycle.PowerStateRunning, timeout); err != nil {
					return fmt.Errorf("waiting for resume: %w", err)
				}
				fmt.Println("Cluster successfully resumed")

				fmt.Println("Checking for expired kubelet certificates...")
				recovery, err := m.PostResumeRecovery(ctx, namespace, clusterName)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: certificate recovery failed: %v\n", err)
				} else {
					fmt.Println(recovery.Message)
					for _, name := range recovery.CSRNames {
						fmt.Printf("  - %s\n", name)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Cluster namespace (defaults to cluster name)")
	cmd.Flags().BoolVar(&doWait, "wait", false, "Wait for resume to complete")
	cmd.Flags().DurationVar(&timeout, "timeout", 15*time.Minute, "Timeout for wait operation")

	return cmd
}

func lifecycleStatusCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "status <cluster-name>",
		Short: "Get cluster power state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			if namespace == "" {
				namespace = clusterName
			}

			c, err := buildClient()
			if err != nil {
				return err
			}

			m := lifecycle.New(c, cfg)
			ctx := context.Background()

			supported, err := m.ClusterSupportsLifecycle(ctx, namespace, clusterName)
			if err != nil {
				return fmt.Errorf("checking lifecycle support: %w", err)
			}
			if !supported {
				return fmt.Errorf("cluster %s/%s does not support lifecycle operations (no ClusterDeployment found — may be imported)", namespace, clusterName)
			}

			specState, err := m.GetPowerState(ctx, namespace, clusterName)
			if err != nil {
				return fmt.Errorf("getting power state: %w", err)
			}

			statusState, err := m.GetPowerStateStatus(ctx, namespace, clusterName)
			if err != nil {
				return fmt.Errorf("getting power state status: %w", err)
			}

			fmt.Printf("Cluster: %s/%s\n", namespace, clusterName)
			fmt.Printf("Desired State (spec):  %s\n", specState)
			fmt.Printf("Actual State (status): %s\n", statusState)

			if specState != statusState {
				fmt.Println("\nNote: Power state transition in progress")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Cluster namespace (defaults to cluster name)")

	return cmd
}

func lifecycleDiagnoseCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "diagnose <cluster-name>",
		Short: "Diagnose cluster health by cross-referencing Hive and ACM state",
		Long: `Run diagnostic checks that compare ClusterDeployment (Hive) power state
with ManagedCluster (ACM) conditions. Detects inconsistencies like a cluster
that Hive reports as Running but ACM shows as unavailable (klusterlet issue).

Outputs actionable suggestions when problems are found.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			if namespace == "" {
				namespace = clusterName
			}

			c, err := buildClient()
			if err != nil {
				return err
			}

			m := lifecycle.New(c, cfg)
			ctx := context.Background()

			report, err := m.Diagnose(ctx, namespace, clusterName)
			if err != nil {
				return fmt.Errorf("diagnosing cluster: %w", err)
			}

			if outputJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Cluster: %s\n", report.Cluster)
			if report.Platform != "" {
				fmt.Printf("Platform: %s\n", report.Platform)
			}
			fmt.Printf("Hive Power (spec):   %s\n", report.HivePowerSpec)
			fmt.Printf("Hive Power (status): %s\n", report.HivePowerStatus)
			fmt.Printf("ACM Available: %s\n", report.ACMAvailable)
			fmt.Printf("ACM Joined:    %s\n", report.ACMJoined)
			fmt.Println()

			hasIssues := false
			for _, check := range report.Checks {
				switch check.Severity {
				case lifecycle.SeverityOK:
					fmt.Printf("  [OK]      %s\n", check.Message)
				case lifecycle.SeverityWarning:
					fmt.Fprintf(os.Stderr, "  [WARNING] %s\n", check.Message)
					hasIssues = true
				case lifecycle.SeverityError:
					fmt.Fprintf(os.Stderr, "  [ERROR]   %s\n", check.Message)
					hasIssues = true
				}
				if check.Detail != "" {
					fmt.Printf("            %s\n", check.Detail)
				}
			}

			if len(report.Suggestions) > 0 {
				fmt.Println()
				fmt.Println("Suggestions:")
				for _, s := range report.Suggestions {
					fmt.Printf("  - %s\n", s)
				}
			}

			if hasIssues {
				fmt.Println()
				fmt.Fprintln(os.Stderr, "Issues detected. Review suggestions above.")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Cluster namespace (defaults to cluster name)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output report as JSON")

	return cmd
}

func lifecycleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all clusters that support lifecycle operations",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}

			m := lifecycle.New(c, cfg)
			ctx := context.Background()

			clusters, err := m.ListClustersWithLifecycle(ctx)
			if err != nil {
				return fmt.Errorf("listing clusters: %w", err)
			}

			if len(clusters) == 0 {
				fmt.Println("No Hive-provisioned clusters found")
				return nil
			}

			fmt.Printf("Clusters with lifecycle support (%d):\n", len(clusters))
			for _, cluster := range clusters {
				fmt.Printf("  - %s\n", cluster)
			}

			return nil
		},
	}
}
