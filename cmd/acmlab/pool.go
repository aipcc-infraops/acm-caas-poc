package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/pool"
)

func poolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pool",
		Short: "Manage Hive ClusterPools for pre-warmed cluster access",
	}
	cmd.AddCommand(poolCreateCmd(), poolListCmd(), poolGetCmd(), poolDeleteCmd())
	return cmd
}

func poolCreateCmd() *cobra.Command {
	var size int
	var platform, region, imageSet, baseDomain, namespace string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a ClusterPool with pre-warmed clusters",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := pool.New(c, cfg, logger)
			opts := pool.PoolOpts{
				Name:       args[0],
				Namespace:  namespace,
				Size:       size,
				Platform:   platform,
				Region:     region,
				ImageSet:   imageSet,
				BaseDomain: baseDomain,
			}
			if err := mgr.CreatePool(context.Background(), opts); err != nil {
				return err
			}
			fmt.Printf("ClusterPool %s created (size=%d)\n", args[0], size)
			return nil
		},
	}
	cmd.Flags().IntVar(&size, "size", 3, "Number of pre-warmed clusters")
	cmd.Flags().StringVar(&platform, "platform", "", "Cloud platform (default: ibmcloud)")
	cmd.Flags().StringVar(&region, "region", "", "Cloud region (default: us-south)")
	cmd.Flags().StringVar(&imageSet, "image-set", "", "ClusterImageSet name")
	cmd.Flags().StringVar(&baseDomain, "base-domain", "", "Base domain (default: example.com)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Pool namespace (default: pool name)")
	return cmd
}

func poolListCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all ClusterPools",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := pool.New(c, cfg, logger)
			pools, err := mgr.ListPools(context.Background())
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(pools, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(pools) == 0 {
				fmt.Println("No ClusterPools found")
				return nil
			}

			fmt.Printf("%-20s  %-5s  %-5s  %-7s  %-7s\n", "NAME", "SIZE", "READY", "CLAIMED", "STANDBY")
			for _, p := range pools {
				fmt.Printf("%-20s  %-5d  %-5d  %-7d  %-7d\n", p.Name, p.Size, p.Ready, p.Claimed, p.Standby)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func poolGetCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get ClusterPool details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := pool.New(c, cfg, logger)
			info, err := mgr.GetPool(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Pool:     %s\n", info.Name)
			fmt.Printf("Size:     %d\n", info.Size)
			fmt.Printf("Ready:    %d\n", info.Ready)
			fmt.Printf("Claimed:  %d\n", info.Claimed)
			fmt.Printf("Standby:  %d\n", info.Standby)
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Pool namespace (default: pool name)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func poolDeleteCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a ClusterPool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := pool.New(c, cfg, logger)
			if err := mgr.DeletePool(context.Background(), args[0], namespace); err != nil {
				return err
			}
			fmt.Printf("ClusterPool %s deleted\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Pool namespace (default: pool name)")
	return cmd
}

func claimCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claim",
		Short: "Manage ClusterClaims for instant cluster access",
	}
	cmd.AddCommand(claimCreateCmd(), claimReleaseCmd(), claimListCmd())
	return cmd
}

func claimCreateCmd() *cobra.Command {
	var claimName, namespace, ttl string

	cmd := &cobra.Command{
		Use:   "create <pool-name>",
		Short: "Claim a cluster from a pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := pool.New(c, cfg, logger)
			info, err := mgr.Claim(context.Background(), args[0], namespace, claimName, ttl)
			if err != nil {
				return err
			}

			fmt.Printf("ClusterClaim %s created from pool %s\n", info.Name, info.Pool)
			if info.Cluster != "" {
				fmt.Printf("Bound cluster: %s\n", info.Cluster)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&claimName, "name", "", "Claim name (default: <pool>-claim)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: pool name)")
	cmd.Flags().StringVar(&ttl, "ttl", "", "Claim lifetime (e.g., 48h)")
	return cmd
}

func claimReleaseCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "release <claim-name>",
		Short: "Release a claimed cluster back to the pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := pool.New(c, cfg, logger)
			if err := mgr.ReleaseClaim(context.Background(), args[0], namespace); err != nil {
				return err
			}
			fmt.Printf("ClusterClaim %s released\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Claim namespace (required)")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

func claimListCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ClusterClaims",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := pool.New(c, cfg, logger)
			claims, err := mgr.ListClaims(context.Background(), namespace)
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(claims, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(claims) == 0 {
				fmt.Println("No ClusterClaims found")
				return nil
			}

			fmt.Printf("%-20s  %-20s  %-20s  %-10s\n", "NAME", "POOL", "CLUSTER", "STATUS")
			for _, c := range claims {
				fmt.Printf("%-20s  %-20s  %-20s  %-10s\n", c.Name, c.Pool, c.Cluster, c.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace to list claims from")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
