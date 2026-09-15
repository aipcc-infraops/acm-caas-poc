package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/clusterset"
)

func clustersetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clusterset",
		Short: "Manage ManagedClusterSets for team-based fleet isolation",
	}
	cmd.AddCommand(
		clustersetCreateCmd(),
		clustersetRemoveCmd(),
		clustersetListCmd(),
		clustersetAssignCmd(),
	)
	return cmd
}

func clustersetCreateCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a ClusterSet with binding in a team namespace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" {
				return fmt.Errorf("--namespace is required (team namespace for the binding)")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			if err := mgr.Create(context.Background(), args[0], namespace); err != nil {
				return err
			}
			fmt.Printf("ClusterSet %s created with binding in namespace %s\n", args[0], namespace)
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "team namespace for the ClusterSetBinding (required)")
	return cmd
}

func clustersetRemoveCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a ClusterSet and its binding",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" {
				return fmt.Errorf("--namespace is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			if err := mgr.Remove(context.Background(), args[0], namespace); err != nil {
				return err
			}
			fmt.Printf("ClusterSet %s removed\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "namespace of the ClusterSetBinding (required)")
	return cmd
}

func clustersetListCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all ClusterSets with member counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			sets, err := mgr.List(context.Background())
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(sets, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(sets) == 0 {
				fmt.Println("No ClusterSets found")
				return nil
			}
			fmt.Printf("%-30s %-8s %s\n", "NAME", "COUNT", "MEMBERS")
			for _, s := range sets {
				members := ""
				for i, m := range s.Members {
					if i > 0 {
						members += ", "
					}
					members += m
				}
				fmt.Printf("%-30s %-8d %s\n", s.Name, s.Count, members)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func clustersetAssignCmd() *cobra.Command {
	var setName string
	cmd := &cobra.Command{
		Use:   "assign <cluster>",
		Short: "Assign a cluster to a ClusterSet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if setName == "" {
				return fmt.Errorf("--to is required (target ClusterSet name)")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			if err := mgr.Assign(context.Background(), args[0], setName); err != nil {
				return err
			}
			fmt.Printf("Cluster %s assigned to ClusterSet %s\n", args[0], setName)
			return nil
		},
	}
	cmd.Flags().StringVar(&setName, "to", "", "target ClusterSet name (required)")
	return cmd
}
