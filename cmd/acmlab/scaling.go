package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/scaling"
)

func scalingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaling",
		Short: "Manage cluster worker node scaling via Hive MachinePool",
		Long: `Scale cluster worker nodes by patching Hive MachinePool resources.

Supports fixed replica counts and autoscaling (min/max bounds).
Only works with Hive-provisioned clusters.`,
	}
	cmd.AddCommand(scalingGetCmd(), scalingSetCmd(), scalingAutoCmd(), scalingListCmd())
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
				return fmt.Errorf("getting MachinePool: %w", err)
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
				return fmt.Errorf("setting replicas: %w", err)
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
				return fmt.Errorf("enabling autoscaling: %w", err)
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
