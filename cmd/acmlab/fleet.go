package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/batch"
	"github.com/pablofelix/acm-caas-poc/internal/fleet"
)

func fleetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Query managed cluster fleet",
	}
	cmd.AddCommand(fleetListCmd(), fleetStatusCmd())
	return cmd
}

func fleetListCmd() *cobra.Command {
	var outputJSON bool
	var labelSelector string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all managed clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			insp := fleet.New(c, cfg, logger)
			clusters, err := insp.ListClusters(context.Background(), labelSelector)
			if err != nil {
				return err
			}
			if len(clusters) == 0 {
				fmt.Println("No managed clusters found")
				return nil
			}
			if outputJSON {
				data, _ := json.MarshalIndent(clusters, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("%-30s %-10s %-10s %-10s %s\n", "NAME", "AVAILABLE", "JOINED", "ACCEPTED", "VERSION")
			for _, cl := range clusters {
				fmt.Printf("%-30s %-10v %-10v %-10v %s\n",
					cl.Name, cl.Available, cl.Joined, cl.Accepted, cl.Version)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	cmd.Flags().StringVarP(&labelSelector, "label-selector", "l", "", "filter clusters by label (e.g. vendor=OpenShift)")
	return cmd
}

func fleetStatusCmd() *cobra.Command {
	var outputJSON bool
	var fromFile string
	var concurrency int

	cmd := &cobra.Command{
		Use:   "status [name...]",
		Short: "Show status of one or more clusters",
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
			fi := fleet.New(c, cfg, logger)
			ctx := context.Background()

			// Single-cluster detailed output (original behaviour)
			if len(items) == 1 && fromFile == "" {
				info, err := fi.GetCluster(ctx, items[0].Name)
				if err != nil {
					return err
				}
				if outputJSON {
					data, _ := json.MarshalIndent(info, "", "  ")
					fmt.Println(string(data))
					return nil
				}
				fmt.Printf("Cluster: %s\n", info.Name)
				fmt.Printf("Version: %s\n", info.Version)
				fmt.Printf("Available: %v\n", info.Available)
				fmt.Printf("Joined: %v\n", info.Joined)
				fmt.Printf("Accepted: %v\n", info.Accepted)
				fmt.Printf("Labels:\n")
				for k, v := range info.Labels {
					fmt.Printf("  %s=%s\n", k, v)
				}
				fmt.Printf("Conditions:\n")
				for _, cond := range info.Conditions {
					fmt.Printf("  %-45s %-6s %s\n", cond.Type+":", cond.Status, cond.Message)
				}
				return nil
			}

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						s, err := fi.GetCluster(ctx, item.Name)
						if err != nil {
							return "", err
						}
						return fmt.Sprintf("Available=%v Joined=%v Version=%s", s.Available, s.Joined, s.Version), nil
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

	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "YAML file with cluster list")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Max parallel operations (max 20)")
	return cmd
}
