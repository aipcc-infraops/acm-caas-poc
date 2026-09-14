package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/scaling"
)

// scalingErr handles ErrNoMachinePool with a clear actionable message.
func scalingErr(err error) error {
	var noMP *scaling.ErrNoMachinePool
	if errors.As(err, &noMP) {
		return fmt.Errorf("%s", noMP.Error())
	}
	return err
}

func scalingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaling",
		Short: "Manage cluster worker node scaling via Hive MachinePool",
		Long: `Scale cluster worker nodes by patching Hive MachinePool resources.

Supports fixed replica counts and autoscaling (min/max bounds).
Only works with Hive-provisioned clusters. For imported clusters, use
your cloud provider's native scaling tools.`,
	}
	cmd.AddCommand(scalingGetCmd(), scalingSetCmd(), scalingAutoCmd(), scalingListCmd(), scalingInitCmd())
	return cmd
}

func scalingGetCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "get <cluster>",
		Short: "Get MachinePool info for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			sc := scaling.New(c, cfg)
			ctx := context.Background()

			info, err := sc.GetMachinePool(ctx, args[0])
			if err != nil {
				return scalingErr(err)
			}

			if outputJSON {
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			printMachinePoolInfo(*info)
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func scalingSetCmd() *cobra.Command {
	var replicas int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "set <cluster>",
		Short: "Set fixed replica count on a cluster's MachinePool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			sc := scaling.New(c, cfg)
			ctx := context.Background()

			if err := sc.SetReplicas(ctx, args[0], replicas); err != nil {
				return scalingErr(err)
			}

			info, err := sc.GetMachinePool(ctx, args[0])
			if err != nil {
				return fmt.Errorf("reading updated MachinePool: %w", err)
			}

			if outputJSON {
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Cluster %s MachinePool replicas set to %d\n", args[0], replicas)
			printMachinePoolInfo(*info)
			return nil
		},
	}
	cmd.Flags().IntVar(&replicas, "replicas", 2, "Number of worker nodes")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func scalingAutoCmd() *cobra.Command {
	var min, max int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "auto <cluster>",
		Short: "Enable autoscaling on a cluster's MachinePool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if min >= max {
				return fmt.Errorf("--min (%d) must be less than --max (%d)", min, max)
			}

			c, err := buildClient()
			if err != nil {
				return err
			}
			sc := scaling.New(c, cfg)
			ctx := context.Background()

			if err := sc.EnableAutoscaling(ctx, args[0], min, max); err != nil {
				return scalingErr(err)
			}

			info, err := sc.GetMachinePool(ctx, args[0])
			if err != nil {
				return fmt.Errorf("reading updated MachinePool: %w", err)
			}

			if outputJSON {
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Cluster %s MachinePool autoscaling enabled (min=%d max=%d)\n", args[0], min, max)
			printMachinePoolInfo(*info)
			return nil
		},
	}
	cmd.Flags().IntVar(&min, "min", 1, "Minimum worker nodes")
	cmd.Flags().IntVar(&max, "max", 5, "Maximum worker nodes")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func scalingListCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all MachinePools across the fleet",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			sc := scaling.New(c, cfg)
			ctx := context.Background()

			pools, err := sc.ListMachinePools(ctx)
			if err != nil {
				return fmt.Errorf("listing MachinePools: %w", err)
			}

			if outputJSON {
				data, _ := json.MarshalIndent(pools, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(pools) == 0 {
				fmt.Println("No MachinePools found")
				return nil
			}

			fmt.Printf("MachinePools (%d):\n", len(pools))
			for _, p := range pools {
				printMachinePoolInfo(p)
				fmt.Fprintln(os.Stdout)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func scalingInitCmd() *cobra.Command {
	var workerType string
	var replicas int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "init <cluster>",
		Short: "Create a MachinePool for a Hive cluster that doesn't have one",
		Long: `Creates a Hive MachinePool for a cluster provisioned without one.

Auto-detects current worker count and instance type from ManagedClusterInfo.
If --replicas matches the detected worker count, no worker changes will be made.
If --replicas differs, Hive will add or remove workers to reach the desired count.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			c, err := buildClient()
			if err != nil {
				return err
			}
			sc := scaling.New(c, cfg)
			ctx := context.Background()

			// Auto-detect current workers if not provided
			detectedCount, detectedType, err := sc.GetCurrentWorkerInfo(ctx, clusterName)
			if err != nil {
				return fmt.Errorf("detecting worker info: %w", err)
			}

			if workerType == "" {
				workerType = detectedType
			}
			if !cmd.Flags().Changed("replicas") {
				replicas = detectedCount
			}

			// Warn if replicas differ from detected
			if replicas != detectedCount {
				fmt.Fprintf(os.Stderr, "Warning: cluster currently has %d worker(s) of type %s.\n", detectedCount, detectedType)
				fmt.Fprintf(os.Stderr, "Creating MachinePool with replicas=%d will %s worker(s).\n",
					replicas, func() string {
						if replicas > detectedCount {
							return fmt.Sprintf("provision %d additional", replicas-detectedCount)
						}
						return fmt.Sprintf("remove %d", detectedCount-replicas)
					}())
			} else {
				fmt.Printf("Creating MachinePool for %s (adopting %d existing worker(s) of type %s — no changes).\n",
					clusterName, detectedCount, workerType)
			}

			info, err := sc.InitMachinePool(ctx, clusterName, workerType, replicas)
			if err != nil {
				return scalingErr(err)
			}

			if outputJSON {
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("MachinePool created for cluster %s\n", clusterName)
			printMachinePoolInfo(*info)
			return nil
		},
	}

	cmd.Flags().StringVar(&workerType, "worker-type", "", "Worker instance type (auto-detected if not provided)")
	cmd.Flags().IntVar(&replicas, "replicas", 0, "Worker count (auto-detected if not provided)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func printMachinePoolInfo(info scaling.MachinePoolInfo) {
	fmt.Printf("MachinePool: %s/%s\n", info.Namespace, info.Name)
	if info.Platform != "" {
		fmt.Printf("  Platform:  %s\n", info.Platform)
	}
	if info.Replicas != nil {
		fmt.Printf("  Replicas:  %d (fixed)\n", *info.Replicas)
	} else if info.MinSize != nil && info.MaxSize != nil {
		fmt.Printf("  Autoscale: min=%d max=%d\n", *info.MinSize, *info.MaxSize)
	} else {
		fmt.Printf("  Replicas:  (unknown)\n")
	}
}
