package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/batch"
	"github.com/pablofelix/acm-caas-poc/internal/lifecycle"
)

// lifecycleSupportErr returns a clear, actionable error when a cluster does not
// support the requested lifecycle operation. Returns nil when support is full.
// Keeping the logic here (not in the lifecycle package) avoids coupling the
// package to CLI-specific formatting.
func lifecycleSupportErr(clusterName, op string, r *lifecycle.LifecycleSupportReason) error {
	switch r.Support {
	case lifecycle.LifecycleFull:
		return nil
	case lifecycle.LifecycleNotYetImplemented:
		msg := fmt.Sprintf(
			"cluster %s is a Kubernetes cluster — %s is not yet implemented for Kubernetes.\n"+
				"To save costs, scale workers to 0 instead:\n"+
				"  %s",
			clusterName, op, r.Alternative,
		)
		return fmt.Errorf("%s", msg)
	default:
		return fmt.Errorf(
			"cluster %s does not support lifecycle operations (no ClusterDeployment found — may be an imported cluster)",
			clusterName,
		)
	}
}

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
	var fromFile string
	var concurrency int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "hibernate [name...]",
		Short: "Hibernate one or more clusters to save costs",
		Args:  cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			var fileItems []batch.ClusterItem
			if fromFile != "" {
				var err error
				fileItems, err = batch.LoadFile(fromFile)
				if err != nil {
					return err
				}
			}
			items, err := batch.NamesFromArgs(args, fileItems)
			if err != nil {
				return err
			}

			c, err := buildClient()
			if err != nil {
				return err
			}
			m := lifecycle.New(c, cfg, logger)
			ctx := context.Background()

			// single-item: preserve existing behavior with namespace flag
			if len(items) == 1 && fromFile == "" {
				clusterName := items[0].Name
				ns := namespace
				if ns == "" {
					ns = clusterName
				}

				support, err := m.CheckLifecycleSupport(ctx, ns, clusterName)
				if err != nil {
					return fmt.Errorf("checking lifecycle support: %w", err)
				}
				if err := lifecycleSupportErr(clusterName, "hibernate", support); err != nil {
					return err
				}

				if err := m.Hibernate(ctx, ns, clusterName); err != nil {
					return fmt.Errorf("hibernating cluster: %w", err)
				}

				fmt.Printf("Cluster %s/%s is hibernating\n", ns, clusterName)

				if doWait {
					fmt.Printf("Waiting for cluster to hibernate (timeout: %v)...\n", timeout)
					if err := m.WaitForPowerState(ctx, ns, clusterName, lifecycle.PowerStateHibernating, timeout); err != nil {
						return fmt.Errorf("waiting for hibernation: %w", err)
					}
					fmt.Println("Cluster successfully hibernated")
				}

				return nil
			}

			// batch path: namespace = cluster name (Hive convention)
			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						ns := item.Name
						support, err := m.CheckLifecycleSupport(ctx, ns, item.Name)
						if err != nil {
							return "", err
						}
						if err2 := lifecycleSupportErr(item.Name, "hibernate", support); err2 != nil {
							return "", err2
						}
						if err := m.Hibernate(ctx, ns, item.Name); err != nil {
							return "", err
						}
						if doWait {
							if err := m.WaitForPowerState(ctx, ns, item.Name, lifecycle.PowerStateHibernating, timeout); err != nil {
								return "", fmt.Errorf("waiting: %w", err)
							}
							return "hibernating", nil
						}
						return "hibernation initiated", nil
					},
				}
			}

			results := batch.Execute(ctx, work, concurrency, os.Stdout)
			if outputJSON {
				data, _ := batch.ToJSON(results)
				fmt.Println(string(data))
			} else {
				batch.PrintSummary(results, os.Stdout)
			}
			for _, r := range results {
				if !r.OK {
					os.Exit(1)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Cluster namespace (defaults to cluster name, single-cluster only)")
	cmd.Flags().BoolVar(&doWait, "wait", false, "Wait for hibernation to complete")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Timeout for wait operation")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "YAML file with cluster list")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Max parallel operations (max 20)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output results as JSON array")

	return cmd
}

func resumeCmd() *cobra.Command {
	var namespace string
	var doWait bool
	var timeout time.Duration
	var fromFile string
	var concurrency int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "resume [name...]",
		Short: "Resume one or more hibernated clusters",
		Args:  cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			var fileItems []batch.ClusterItem
			if fromFile != "" {
				var err error
				fileItems, err = batch.LoadFile(fromFile)
				if err != nil {
					return err
				}
			}
			items, err := batch.NamesFromArgs(args, fileItems)
			if err != nil {
				return err
			}

			c, err := buildClient()
			if err != nil {
				return err
			}
			m := lifecycle.New(c, cfg, logger)
			ctx := context.Background()

			// single-item: preserve existing behavior with namespace flag
			if len(items) == 1 && fromFile == "" {
				clusterName := items[0].Name
				ns := namespace
				if ns == "" {
					ns = clusterName
				}

				support, err := m.CheckLifecycleSupport(ctx, ns, clusterName)
				if err != nil {
					return fmt.Errorf("checking lifecycle support: %w", err)
				}
				if err := lifecycleSupportErr(clusterName, "resume", support); err != nil {
					return err
				}

				if err := m.Resume(ctx, ns, clusterName); err != nil {
					return fmt.Errorf("resuming cluster: %w", err)
				}

				fmt.Printf("Cluster %s/%s is resuming\n", ns, clusterName)

				if doWait {
					fmt.Printf("Waiting for cluster to resume (timeout: %v)...\n", timeout)
					if err := m.WaitForPowerState(ctx, ns, clusterName, lifecycle.PowerStateRunning, timeout); err != nil {
						return fmt.Errorf("waiting for resume: %w", err)
					}
					fmt.Println("Cluster successfully resumed")

					fmt.Println("Checking for expired kubelet certificates...")
					recovery, err := m.PostResumeRecovery(ctx, ns, clusterName)
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
			}

			// batch path: namespace = cluster name (Hive convention)
			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						ns := item.Name
						support, err := m.CheckLifecycleSupport(ctx, ns, item.Name)
						if err != nil {
							return "", err
						}
						if err2 := lifecycleSupportErr(item.Name, "hibernate", support); err2 != nil {
							return "", err2
						}
						if err := m.Resume(ctx, ns, item.Name); err != nil {
							return "", err
						}
						if doWait {
							if err := m.WaitForPowerState(ctx, ns, item.Name, lifecycle.PowerStateRunning, timeout); err != nil {
								return "", fmt.Errorf("waiting: %w", err)
							}
							return "running", nil
						}
						return "resume initiated", nil
					},
				}
			}

			results := batch.Execute(ctx, work, concurrency, os.Stdout)
			if outputJSON {
				data, _ := batch.ToJSON(results)
				fmt.Println(string(data))
			} else {
				batch.PrintSummary(results, os.Stdout)
			}
			for _, r := range results {
				if !r.OK {
					os.Exit(1)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Cluster namespace (defaults to cluster name, single-cluster only)")
	cmd.Flags().BoolVar(&doWait, "wait", false, "Wait for resume to complete")
	cmd.Flags().DurationVar(&timeout, "timeout", 15*time.Minute, "Timeout for wait operation")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "YAML file with cluster list")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Max parallel operations (max 20)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output results as JSON array")

	return cmd
}

func lifecycleStatusCmd() *cobra.Command {
	var namespace string
	var fromFile string
	var concurrency int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "status [name...]",
		Short: "Get cluster power state",
		Args:  cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			var fileItems []batch.ClusterItem
			if fromFile != "" {
				var err error
				fileItems, err = batch.LoadFile(fromFile)
				if err != nil {
					return err
				}
			}
			items, err := batch.NamesFromArgs(args, fileItems)
			if err != nil {
				return err
			}

			c, err := buildClient()
			if err != nil {
				return err
			}
			m := lifecycle.New(c, cfg, logger)
			ctx := context.Background()

			// single-item: preserve existing detailed output
			if len(items) == 1 && fromFile == "" {
				clusterName := items[0].Name
				ns := namespace
				if ns == "" {
					ns = clusterName
				}

				support, err := m.CheckLifecycleSupport(ctx, ns, clusterName)
				if err != nil {
					return fmt.Errorf("checking lifecycle support: %w", err)
				}
				if err := lifecycleSupportErr(clusterName, "status", support); err != nil {
					return err
				}

				specState, err := m.GetPowerState(ctx, ns, clusterName)
				if err != nil {
					return fmt.Errorf("getting power state: %w", err)
				}

				statusState, err := m.GetPowerStateStatus(ctx, ns, clusterName)
				if err != nil {
					return fmt.Errorf("getting power state status: %w", err)
				}

				fmt.Printf("Cluster: %s/%s\n", ns, clusterName)
				fmt.Printf("Desired State (spec):  %s\n", specState)
				fmt.Printf("Actual State (status): %s\n", statusState)

				if specState != statusState {
					fmt.Println("\nNote: Power state transition in progress")
				}

				return nil
			}

			// batch path: namespace = cluster name
			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						ns := item.Name
						support, err := m.CheckLifecycleSupport(ctx, ns, item.Name)
						if err != nil {
							return "", err
						}
						if err2 := lifecycleSupportErr(item.Name, "hibernate", support); err2 != nil {
							return "no ClusterDeployment — imported cluster", nil
						}
						specState, err := m.GetPowerState(ctx, ns, item.Name)
						if err != nil {
							return "", err
						}
						statusState, err := m.GetPowerStateStatus(ctx, ns, item.Name)
						if err != nil {
							return "", err
						}
						return fmt.Sprintf("powerState=%s available=%s", specState, statusState), nil
					},
				}
			}
			results := batch.Execute(ctx, work, concurrency, os.Stdout)
			if outputJSON {
				data, _ := batch.ToJSON(results)
				fmt.Println(string(data))
			} else {
				batch.PrintSummary(results, os.Stdout)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Cluster namespace (defaults to cluster name, single-cluster only)")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "YAML file with cluster list")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Max parallel operations (max 20)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")

	return cmd
}

func lifecycleDiagnoseCmd() *cobra.Command {
	var namespace string
	var outputJSON bool
	var fromFile string
	var concurrency int

	cmd := &cobra.Command{
		Use:   "diagnose [name...]",
		Short: "Diagnose cluster health by cross-referencing Hive and ACM state",
		Long: `Run diagnostic checks that compare ClusterDeployment (Hive) power state
with ManagedCluster (ACM) conditions. Detects inconsistencies like a cluster
that Hive reports as Running but ACM shows as unavailable (klusterlet issue).

Outputs actionable suggestions when problems are found.`,
		Args: cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			var fileItems []batch.ClusterItem
			if fromFile != "" {
				var err error
				fileItems, err = batch.LoadFile(fromFile)
				if err != nil {
					return err
				}
			}
			items, err := batch.NamesFromArgs(args, fileItems)
			if err != nil {
				return err
			}

			c, err := buildClient()
			if err != nil {
				return err
			}
			m := lifecycle.New(c, cfg, logger)
			ctx := context.Background()

			// single-item: preserve existing detailed output
			if len(items) == 1 && fromFile == "" {
				clusterName := items[0].Name
				ns := namespace
				if ns == "" {
					ns = clusterName
				}

				report, err := m.Diagnose(ctx, ns, clusterName)
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
			}

			// batch path: namespace = cluster name, read-only, never sets exit 1
			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						ns := item.Name
						report, err := m.Diagnose(ctx, ns, item.Name)
						if err != nil {
							return "", err
						}
						hasIssues := false
						for _, check := range report.Checks {
							if check.Severity == lifecycle.SeverityWarning || check.Severity == lifecycle.SeverityError {
								hasIssues = true
								break
							}
						}
						if hasIssues {
							return fmt.Sprintf("spec=%s status=%s acm-available=%s — issues detected", report.HivePowerSpec, report.HivePowerStatus, report.ACMAvailable), nil
						}
						return "ok", nil
					},
				}
			}
			results := batch.Execute(ctx, work, concurrency, os.Stdout)
			if outputJSON {
				data, _ := batch.ToJSON(results)
				fmt.Println(string(data))
			} else {
				batch.PrintSummary(results, os.Stdout)
			}
			// diagnose is read-only: never sets exit 1
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Cluster namespace (defaults to cluster name, single-cluster only)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output report as JSON")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "YAML file with cluster list")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Max parallel operations (max 20)")

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

			m := lifecycle.New(c, cfg, logger)
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
