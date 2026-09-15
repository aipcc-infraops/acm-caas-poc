package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/batch"
	"github.com/pablofelix/acm-caas-poc/internal/provisioning"
)

func provisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provision",
		Short: "Provision and manage spoke clusters via Hive ClusterDeployment",
	}
	cmd.AddCommand(
		provisionCreateCmd(),
		provisionDestroyCmd(),
		provisionStatusCmd(),
		provisionListCmd(),
		provisionImageSetsCmd(),
	)
	return cmd
}

func provisionCreateCmd() *cobra.Command {
	var platform, region, imageSet, workerType, masterType, sshKeyFile, sshPrivateKeyFile, pullSecretFile, manifestsDir string
	var workers, masters int64
	cmd := &cobra.Command{
		Use:   "create <cluster-name>",
		Short: "Create a spoke cluster on IBM Cloud via Hive (idempotent)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := provisioning.New(c, cfg, logger)

			opts := provisioning.ClusterOpts{
				Name:           args[0],
				Platform:       platform,
				Region:         region,
				ImageSet:       imageSet,
				WorkerType:     workerType,
				MasterType:     masterType,
				WorkerReplicas: workers,
				MasterReplicas: masters,
				ManifestsDir:   manifestsDir,
			}

			if pullSecretFile != "" {
				data, err := os.ReadFile(pullSecretFile)
				if err != nil {
					return fmt.Errorf("reading pull secret: %w", err)
				}
				opts.PullSecret = string(data)
			}

			if sshKeyFile != "" {
				data, err := os.ReadFile(sshKeyFile)
				if err != nil {
					return fmt.Errorf("reading SSH public key: %w", err)
				}
				opts.SSHKey = string(data)
			}

			if sshPrivateKeyFile != "" {
				data, err := os.ReadFile(sshPrivateKeyFile)
				if err != nil {
					return fmt.Errorf("reading SSH private key: %w", err)
				}
				opts.SSHPrivateKey = string(data)
			}

			fmt.Printf("Creating cluster %s in %s...\n", args[0], opts.Region)
			if err := mgr.Create(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("ClusterDeployment created. Hive will now provision the cluster.")
			fmt.Println("Use 'acmlab provision status' to monitor progress.")
			return nil
		},
	}
	cmd.Flags().StringVar(&platform, "platform", "", "cloud platform: ibmcloud, aws, gcp, azure (default: from env)")
	cmd.Flags().StringVar(&region, "region", "", "cloud region (default: from env)")
	cmd.Flags().StringVar(&imageSet, "image-set", "", "ClusterImageSet name (default: from env)")
	cmd.Flags().StringVar(&workerType, "worker-type", "", "worker instance type (default: bx2-4x16)")
	cmd.Flags().StringVar(&masterType, "master-type", "", "master instance type (default: bx2-8x32)")
	cmd.Flags().Int64Var(&workers, "workers", 0, "number of worker nodes (default: 2)")
	cmd.Flags().Int64Var(&masters, "masters", 0, "number of master nodes (default: 3)")
	cmd.Flags().StringVar(&pullSecretFile, "pull-secret", "", "path to pull secret file (required)")
	cmd.Flags().StringVar(&sshKeyFile, "ssh-key", "", "path to SSH public key file")
	cmd.Flags().StringVar(&sshPrivateKeyFile, "ssh-private-key", "", "path to SSH private key file")
	cmd.Flags().StringVar(&manifestsDir, "manifests-dir", "", "path to ccoctl-generated manifests directory (optional — auto-generated via IBM Cloud IAM API if omitted)")
	return cmd
}

func provisionDestroyCmd() *cobra.Command {
	var fromFile string
	var concurrency int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "destroy [name...]",
		Short: "Destroy one or more provisioned clusters",
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
			mgr := provisioning.New(c, cfg, logger)
			ctx := context.Background()

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						if err := mgr.Destroy(ctx, item.Name); err != nil {
							return "", err
						}
						return "destruction initiated", nil
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
	cmd.Flags().StringVar(&fromFile, "from-file", "", "YAML file with cluster list")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Max parallel operations (max 20)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output results as JSON array")
	return cmd
}

func provisionStatusCmd() *cobra.Command {
	var fromFile string
	var concurrency int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "status [name...]",
		Short: "Show provisioning status of one or more clusters",
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
			mgr := provisioning.New(c, cfg, logger)
			ctx := context.Background()

			// single-item: preserve existing detailed output
			if len(items) == 1 && fromFile == "" {
				info, err := mgr.Status(ctx, items[0].Name)
				if err != nil {
					return err
				}
				if outputJSON {
					data, _ := json.MarshalIndent(info, "", "  ")
					fmt.Println(string(data))
					return nil
				}
				fmt.Printf("Cluster:      %s\n", info.Name)
				fmt.Printf("BaseDomain:   %s\n", info.BaseDomain)
				fmt.Printf("Region:       %s\n", info.Region)
				fmt.Printf("ImageSet:     %s\n", info.ImageSet)
				fmt.Printf("Installed:    %v\n", info.Installed)
				fmt.Printf("Provisioned:  %v\n", info.Provisioned)
				if info.FailureReason != "" {
					fmt.Printf("Failure:      %s\n", info.FailureReason)
				}
				if len(info.Conditions) > 0 {
					fmt.Printf("Conditions:   %v\n", info.Conditions)
				}
				return nil
			}

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						s, err := mgr.Status(ctx, item.Name)
						if err != nil {
							return "", err
						}
						return fmt.Sprintf("Installed=%v Provisioned=%v", s.Installed, s.Provisioned), nil
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
	cmd.Flags().StringVar(&fromFile, "from-file", "", "YAML file with cluster list")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Max parallel operations (max 20)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func provisionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List clusters provisioned via acmlab",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := provisioning.New(c, cfg, logger)
			clusters, err := mgr.List(context.Background())
			if err != nil {
				return err
			}
			if len(clusters) == 0 {
				fmt.Println("No clusters provisioned via acmlab")
				return nil
			}
			fmt.Printf("%-20s %-25s %-12s %-10s %s\n", "NAME", "DOMAIN", "REGION", "INSTALLED", "IMAGE SET")
			for _, c := range clusters {
				fmt.Printf("%-20s %-25s %-12s %-10v %s\n", c.Name, c.BaseDomain, c.Region, c.Installed, c.ImageSet)
			}
			return nil
		},
	}
}

func provisionImageSetsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "image-sets",
		Short: "List available ClusterImageSets",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := provisioning.New(c, cfg, logger)
			sets, err := mgr.ListImageSets(context.Background())
			if err != nil {
				return err
			}
			if len(sets) == 0 {
				fmt.Println("No ClusterImageSets found")
				return nil
			}
			fmt.Printf("%-35s %s\n", "NAME", "RELEASE IMAGE")
			for _, s := range sets {
				img := s.ReleaseImage
				if len(img) > 60 {
					img = img[:57] + "..."
				}
				fmt.Printf("%-35s %s\n", s.Name, img)
			}
			return nil
		},
	}
}
