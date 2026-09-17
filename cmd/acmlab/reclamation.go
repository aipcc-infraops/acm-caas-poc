package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/reclamation"
)

func reclamationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reclamation",
		Short: "Cluster TTL and reclamation management",
	}
	cmd.AddCommand(
		reclamationSetTTLCmd(),
		reclamationListTTLCmd(),
		reclamationCheckExpiredCmd(),
		reclamationExtendCmd(),
		reclamationReclaimCmd(),
	)
	return cmd
}

func reclamationSetTTLCmd() *cobra.Command {
	var ttlHours int
	cmd := &cobra.Command{
		Use:   "set-ttl <cluster-name>",
		Short: "Set TTL on a managed cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := reclamation.New(c, cfg, logger)
			if err := mgr.SetTTL(context.Background(), args[0], ttlHours); err != nil {
				return err
			}
			fmt.Printf("TTL set: %s expires in %d hours\n", args[0], ttlHours)
			return nil
		},
	}
	cmd.Flags().IntVar(&ttlHours, "hours", 48, "TTL in hours")
	return cmd
}

func reclamationListTTLCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "list-ttl",
		Short: "List all clusters with TTL labels",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := reclamation.New(c, cfg, logger)
			ttls, err := mgr.ListTTLs(context.Background())
			if err != nil {
				return err
			}
			if len(ttls) == 0 {
				fmt.Println("No clusters with TTL labels found")
				return nil
			}
			if outputJSON {
				data, _ := json.MarshalIndent(ttls, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("%-25s %-8s %-25s %-8s %s\n", "CLUSTER", "TTL(h)", "EXPIRY", "EXPIRED", "OWNER")
			for _, t := range ttls {
				fmt.Printf("%-25s %-8d %-25s %-8v %s\n", t.Name, t.TTLHours, t.ExpiryDate.Format("2006-01-02 15:04 UTC"), t.Expired, t.Owner)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func reclamationCheckExpiredCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "check-expired",
		Short: "List clusters whose TTL has expired",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := reclamation.New(c, cfg, logger)
			expired, err := mgr.CheckExpired(context.Background())
			if err != nil {
				return err
			}
			if len(expired) == 0 {
				fmt.Println("No expired clusters found")
				return nil
			}
			if outputJSON {
				data, _ := json.MarshalIndent(expired, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("%-25s %-8s %-25s %s\n", "CLUSTER", "TTL(h)", "EXPIRED AT", "OWNER")
			for _, t := range expired {
				fmt.Printf("%-25s %-8d %-25s %s\n", t.Name, t.TTLHours, t.ExpiryDate.Format("2006-01-02 15:04 UTC"), t.Owner)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func reclamationExtendCmd() *cobra.Command {
	var hours int
	var justification string
	cmd := &cobra.Command{
		Use:   "extend <cluster-name>",
		Short: "Extend a cluster's TTL with justification",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := reclamation.New(c, cfg, logger)
			if err := mgr.ExtendTTL(context.Background(), args[0], hours, justification); err != nil {
				return err
			}
			fmt.Printf("TTL extended: %s +%d hours (reason: %s)\n", args[0], hours, justification)
			return nil
		},
	}
	cmd.Flags().IntVar(&hours, "hours", 24, "Hours to extend")
	cmd.Flags().StringVar(&justification, "reason", "", "Justification for extension (required)")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func reclamationReclaimCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "reclaim <cluster-name>",
		Short: "Reclaim an expired cluster (hibernate Hive, detach imported)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := reclamation.New(c, cfg, logger)
			result, err := mgr.ReclaimCluster(context.Background(), args[0])
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Cluster %s: %s\n", result.Cluster, result.Action)
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
