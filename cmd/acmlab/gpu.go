package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/gpu"
)

func gpuCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gpu",
		Short: "GPU fleet management (stack deployment, routing, version segregation, elastic capacity)",
	}
	cmd.AddCommand(
		gpuDeployStackCmd(),
		gpuRemoveStackCmd(),
		gpuDriftStatusCmd(),
		gpuCreateQueuesCmd(),
		gpuRouteCmd(),
		gpuBestClusterCmd(),
		gpuMarkSaturatedCmd(),
		gpuRouteVersionCmd(),
		gpuEnforceVersionCmd(),
		gpuListVersionsCmd(),
		gpuDetectSaturationCmd(),
		gpuProvisionOnDemandCmd(),
		gpuHibernateIdleCmd(),
		gpuListElasticCmd(),
	)
	return cmd
}

func gpuDeployStackCmd() *cobra.Command {
	var clusterSet string

	cmd := &cobra.Command{
		Use:   "deploy-stack <cluster>",
		Short: "Deploy Kueue + Kyverno GPU sharing stack to a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			fmt.Printf("Deploying GPU sharing stack to %s...\n", args[0])
			if err := mgr.DeployStack(context.Background(), args[0], clusterSet); err != nil {
				return err
			}
			fmt.Println("GPU sharing stack deployed. Kueue + Kyverno ManifestWorks created.")
			return nil
		},
	}
	cmd.Flags().StringVar(&clusterSet, "cluster-set", "", "scope placement to a ClusterSet")
	return cmd
}

func gpuRemoveStackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-stack <cluster>",
		Short: "Remove GPU sharing stack from a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			fmt.Printf("Removing GPU sharing stack from %s...\n", args[0])
			if err := mgr.RemoveStack(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println("GPU sharing stack removed.")
			return nil
		},
	}
}

func gpuDriftStatusCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "drift-status <cluster>",
		Short: "Check GPU stack health and drift status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			status, err := mgr.DetectDrift(context.Background(), args[0])
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Cluster:   %s\n", status.Cluster)
			fmt.Printf("Compliant: %v\n", status.Compliant)
			if len(status.Degraded) > 0 {
				fmt.Printf("Degraded:  %s\n", strings.Join(status.Degraded, ", "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func gpuCreateQueuesCmd() *cobra.Command {
	var gpuTypes string

	cmd := &cobra.Command{
		Use:   "create-queues <cluster>",
		Short: "Create ClusterQueues per GPU type on a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			types := strings.Split(gpuTypes, ",")
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			fmt.Printf("Creating ClusterQueues for GPU types %s on %s...\n", gpuTypes, args[0])
			if err := mgr.CreateClusterQueues(context.Background(), args[0], types); err != nil {
				return err
			}
			fmt.Printf("%d ClusterQueues created.\n", len(types))
			return nil
		},
	}
	cmd.Flags().StringVar(&gpuTypes, "gpu-types", "h100,l4", "comma-separated GPU types")
	return cmd
}

func gpuRouteCmd() *cobra.Command {
	var gpuType, region string

	cmd := &cobra.Command{
		Use:   "route",
		Short: "Create a Placement for GPU workload routing by type and region",
		RunE: func(cmd *cobra.Command, args []string) error {
			if gpuType == "" {
				return fmt.Errorf("--gpu-type is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			name := "gpu-route-" + strings.ToLower(gpuType)
			if region != "" {
				name += "-" + region
			}
			fmt.Printf("Creating GPU Placement %s (type=%s, region=%s)...\n", name, gpuType, region)
			if err := mgr.CreateGPUPlacement(context.Background(), name, gpuType, region); err != nil {
				return err
			}
			fmt.Println("GPU Placement created. Check PlacementDecisions for selected cluster.")
			return nil
		},
	}
	cmd.Flags().StringVar(&gpuType, "gpu-type", "", "GPU type to route (e.g. H100, L4, A100)")
	cmd.Flags().StringVar(&region, "region", "", "preferred region (optional)")
	return cmd
}

func gpuBestClusterCmd() *cobra.Command {
	var gpuType string

	cmd := &cobra.Command{
		Use:   "best-cluster",
		Short: "Find the best available cluster for a GPU type",
		RunE: func(cmd *cobra.Command, args []string) error {
			if gpuType == "" {
				return fmt.Errorf("--gpu-type is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			cluster, err := mgr.BestCluster(context.Background(), gpuType)
			if err != nil {
				return err
			}
			fmt.Printf("Best cluster for %s: %s\n", gpuType, cluster)
			return nil
		},
	}
	cmd.Flags().StringVar(&gpuType, "gpu-type", "", "GPU type to search for")
	return cmd
}

func gpuMarkSaturatedCmd() *cobra.Command {
	var clear bool

	cmd := &cobra.Command{
		Use:   "mark-saturated <cluster>",
		Short: "Mark or clear GPU cluster saturation status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			saturated := !clear
			if err := mgr.MarkSaturated(context.Background(), args[0], saturated); err != nil {
				return err
			}
			if saturated {
				fmt.Printf("Cluster %s marked as saturated (gpu-available=false)\n", args[0])
			} else {
				fmt.Printf("Cluster %s saturation cleared (gpu-available=true)\n", args[0])
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&clear, "clear", false, "Clear saturation status instead of setting it")
	return cmd
}

func gpuRouteVersionCmd() *cobra.Command {
	var version string

	cmd := &cobra.Command{
		Use:   "route-version",
		Short: "Find a cluster running a specific AI platform operator version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if version == "" {
				return fmt.Errorf("--version is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			cluster, err := mgr.RouteByVersion(context.Background(), version)
			if err != nil {
				return err
			}
			fmt.Printf("Cluster with operator version %s: %s\n", version, cluster)
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "AI platform operator version (e.g. 2.17, 2.18)")
	return cmd
}

func gpuEnforceVersionCmd() *cobra.Command {
	var version string

	cmd := &cobra.Command{
		Use:   "enforce-version <cluster>",
		Short: "Enforce a single AI platform operator version on a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if version == "" {
				return fmt.Errorf("--version is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			fmt.Printf("Enforcing operator version %s on %s...\n", version, args[0])
			if err := mgr.EnforceVersionPolicy(context.Background(), args[0], version); err != nil {
				return err
			}
			fmt.Println("Version enforcement policy created.")
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "AI platform operator version to enforce")
	return cmd
}

func gpuListVersionsCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list-versions",
		Short: "List GPU clusters grouped by AI platform operator version",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			clusters, err := mgr.ListVersionClusters(context.Background())
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(clusters, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(clusters) == 0 {
				fmt.Println("No GPU clusters with operator version labels found")
				return nil
			}
			fmt.Printf("%-20s %-12s %-12s %-10s %s\n", "CLUSTER", "VERSION", "CHANNEL", "BUILD", "AVAILABLE")
			for _, cl := range clusters {
				fmt.Printf("%-20s %-12s %-12s %-10s %v\n", cl.Name, cl.Version, cl.Channel, cl.Build, cl.Available)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func gpuDetectSaturationCmd() *cobra.Command {
	var threshold float64
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "detect-saturation <cluster>",
		Short: "Check GPU utilization and saturation status for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			status, err := mgr.DetectSaturation(context.Background(), args[0], threshold)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Cluster:     %s\n", status.Cluster)
			fmt.Printf("GPU Type:    %s\n", status.GPUType)
			fmt.Printf("Utilization: %.1f%%\n", status.Utilization)
			fmt.Printf("Threshold:   %.1f%%\n", status.Threshold)
			fmt.Printf("Saturated:   %v\n", status.Saturated)
			return nil
		},
	}
	cmd.Flags().Float64Var(&threshold, "threshold", 85.0, "Saturation threshold percentage")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func gpuProvisionOnDemandCmd() *cobra.Command {
	var gpuType string

	cmd := &cobra.Command{
		Use:   "provision-ondemand <cluster>",
		Short: "Label a cluster as on-demand elastic GPU capacity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if gpuType == "" {
				return fmt.Errorf("--gpu-type is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			fmt.Printf("Provisioning on-demand GPU capacity on %s (type=%s)...\n", args[0], gpuType)
			if err := mgr.ProvisionOnDemand(context.Background(), gpu.OnDemandOpts{
				Cluster: args[0],
				GPUType: gpuType,
			}); err != nil {
				return err
			}
			fmt.Println("Cluster labelled as elastic GPU capacity. ManifestWork deployed.")
			return nil
		},
	}
	cmd.Flags().StringVar(&gpuType, "gpu-type", "", "GPU type (e.g. H100, L4, A100)")
	return cmd
}

func gpuHibernateIdleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hibernate-idle <cluster>",
		Short: "Mark an idle elastic GPU cluster as hibernated",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			fmt.Printf("Marking %s as hibernated...\n", args[0])
			if err := mgr.HibernateIdle(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println("Cluster marked as hibernated (gpu-available=false, gpu-elastic-state=hibernated).")
			return nil
		},
	}
}

func gpuListElasticCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list-elastic",
		Short: "List elastic (on-demand/spot) GPU clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := gpu.New(c, cfg, logger)
			clusters, err := mgr.ListElasticClusters(context.Background())
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(clusters, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(clusters) == 0 {
				fmt.Println("No elastic GPU clusters found")
				return nil
			}
			fmt.Printf("%-20s %-10s %-12s %s\n", "CLUSTER", "GPU TYPE", "COST TIER", "AVAILABLE")
			for _, cl := range clusters {
				fmt.Printf("%-20s %-10s %-12s %v\n", cl.Name, cl.GPUType, cl.CostTier, cl.Available)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
