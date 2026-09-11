package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/batch"
	"github.com/pablofelix/acm-caas-poc/internal/registry"
)

func registryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage image registry mirrors for restricted clusters",
		Long: `Configure image registry mirrors for clusters that cannot pull
directly from registry.redhat.io (ROKS, air-gapped, disconnected).

ACM's ManagedClusterImageRegistry CRD rewrites image references in
klusterlet manifests before applying them to the spoke — no changes
needed on the spoke itself.`,
	}
	cmd.AddCommand(
		registryListImagesCmd(),
		registryConfigureCmd(),
		registryRemoveCmd(),
		registryStatusCmd(),
		registryMirrorScriptCmd(),
	)
	return cmd
}

func registryListImagesCmd() *cobra.Command {
	var outputJSON bool
	var fromFile string
	var concurrency int

	cmd := &cobra.Command{
		Use:   "list-images [cluster...]",
		Short: "List images required by ACM on a spoke cluster",
		Long: `Extracts all container images from the ManifestWorks that ACM
created for the cluster. If the spoke cannot pull from the source
registries, these images must be mirrored.`,
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
			m := registry.New(c, cfg)
			ctx := context.Background()

			// Single-cluster detailed output (original behaviour)
			if len(items) == 1 && fromFile == "" {
				clusterName := items[0].Name
				images, err := m.ListRequiredImages(ctx, clusterName)
				if err != nil {
					return fmt.Errorf("listing required images: %w", err)
				}
				if len(images) == 0 {
					fmt.Println("No ManifestWork images found for cluster", clusterName)
					return nil
				}
				if outputJSON {
					data, _ := json.MarshalIndent(images, "", "  ")
					fmt.Println(string(data))
					return nil
				}
				fmt.Printf("Images required by ACM on %s (%d):\n\n", clusterName, len(images))
				for _, img := range images {
					fmt.Printf("  %s\n    ManifestWork: %s\n\n", img.Image, img.ManifestWork)
				}
				return nil
			}

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						images, err := m.ListRequiredImages(ctx, item.Name)
						if err != nil {
							return "", err
						}
						return fmt.Sprintf("%d images found", len(images)), nil
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

func registryConfigureCmd() *cobra.Command {
	var mirrorRegistry string
	var pullSecretPath string
	var registries []string
	var fromFile string
	var concurrency int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "configure [cluster...]",
		Short: "Configure image registry mirror for a cluster",
		Long: `Creates a ManagedClusterImageRegistry on the hub so that ACM
rewrites image references before applying them to the spoke.

Steps before running this command:
  1. Mirror the images (use 'registry mirror-script' to generate commands)
  2. Create a pull secret for the target registry if needed

Example:
  acmlab registry configure import-test \
    --mirror quay.io/myorg \
    --pull-secret ~/pull-secret.json`,
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
			m := registry.New(c, cfg)
			ctx := context.Background()

			// Single-cluster original behaviour
			if len(items) == 1 && fromFile == "" {
				clusterName := items[0].Name
				if mirrorRegistry == "" && len(registries) == 0 {
					return fmt.Errorf("provide --mirror or --registry flags")
				}
				opts := registry.MirrorConfig{
					ClusterName:    clusterName,
					MirrorRegistry: mirrorRegistry,
					PullSecretPath: pullSecretPath,
				}
				for _, r := range registries {
					parts := strings.SplitN(r, "=", 2)
					if len(parts) != 2 {
						return fmt.Errorf("invalid registry mapping %q, expected source=mirror", r)
					}
					opts.Registries = append(opts.Registries, registry.RegistryMapping{
						Source: parts[0],
						Mirror: parts[1],
					})
				}
				if err := m.ConfigureMirror(ctx, opts); err != nil {
					return fmt.Errorf("configuring mirror: %w", err)
				}
				fmt.Printf("Image registry mirror configured for cluster %s\n", clusterName)
				if mirrorRegistry != "" {
					fmt.Printf("Mirror registry: %s\n", mirrorRegistry)
				}
				return nil
			}

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						mirror := mirrorRegistry
						if item.MirrorRegistry != "" {
							mirror = item.MirrorRegistry
						}
						pullSecret := pullSecretPath
						if item.PullSecretPath != "" {
							pullSecret = item.PullSecretPath
						}
						if mirror == "" && len(registries) == 0 {
							return "", fmt.Errorf("provide --mirror or --registry")
						}
						opts := registry.MirrorConfig{
							ClusterName:    item.Name,
							MirrorRegistry: mirror,
							PullSecretPath: pullSecret,
						}
						for _, r := range registries {
							parts := strings.SplitN(r, "=", 2)
							if len(parts) != 2 {
								return "", fmt.Errorf("invalid mapping %q", r)
							}
							opts.Registries = append(opts.Registries, registry.RegistryMapping{Source: parts[0], Mirror: parts[1]})
						}
						if err := m.ConfigureMirror(ctx, opts); err != nil {
							return "", err
						}
						return fmt.Sprintf("mirror configured (%s)", mirror), nil
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

	cmd.Flags().StringVar(&mirrorRegistry, "mirror", "", "Mirror registry (e.g., quay.io/myorg)")
	cmd.Flags().StringVar(&pullSecretPath, "pull-secret", "", "Path to pull secret JSON for the mirror registry")
	cmd.Flags().StringSliceVar(&registries, "registry", nil, "Registry mappings (source=mirror, repeatable)")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "YAML file with cluster list")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Max parallel operations (max 20)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func registryRemoveCmd() *cobra.Command {
	var fromFile string
	var concurrency int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "remove [cluster...]",
		Short: "Remove image registry mirror configuration",
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
			m := registry.New(c, cfg)
			ctx := context.Background()

			// Single-cluster original behaviour
			if len(items) == 1 && fromFile == "" {
				clusterName := items[0].Name
				if err := m.RemoveMirror(ctx, clusterName); err != nil {
					return fmt.Errorf("removing mirror: %w", err)
				}
				fmt.Printf("Image registry mirror removed for cluster %s\n", clusterName)
				return nil
			}

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						if err := m.RemoveMirror(ctx, item.Name); err != nil {
							return "", err
						}
						return "mirror removed", nil
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
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func registryStatusCmd() *cobra.Command {
	var outputJSON bool
	var fromFile string
	var concurrency int

	cmd := &cobra.Command{
		Use:   "status [cluster...]",
		Short: "Show image registry mirror status",
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
			m := registry.New(c, cfg)
			ctx := context.Background()

			// Single-cluster original behaviour
			if len(items) == 1 && fromFile == "" {
				clusterName := items[0].Name
				status, err := m.GetMirrorStatus(ctx, clusterName)
				if err != nil {
					return fmt.Errorf("getting mirror status: %w", err)
				}
				if outputJSON {
					data, _ := json.MarshalIndent(status, "", "  ")
					fmt.Println(string(data))
					return nil
				}
				fmt.Printf("Cluster: %s\n", status.ClusterName)
				fmt.Printf("Mirror configured: %v\n", status.Configured)
				if status.Configured {
					fmt.Println("Registry mappings:")
					for _, r := range status.Registries {
						fmt.Printf("  %s → %s\n", r.Source, r.Mirror)
					}
				}
				return nil
			}

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						status, err := m.GetMirrorStatus(ctx, item.Name)
						if err != nil {
							return "", err
						}
						return fmt.Sprintf("configured=%v", status.Configured), nil
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

func registryMirrorScriptCmd() *cobra.Command {
	var targetRegistry string
	var fromFile string
	var concurrency int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "mirror-script [cluster...]",
		Short: "Generate a bash script to mirror images with skopeo",
		Long: `Generates a bash script with skopeo commands to copy all ACM
images from registry.redhat.io to a target registry. Run the script
before configuring the mirror.

Example:
  acmlab registry mirror-script import-test --target quay.io/myorg > mirror.sh
  chmod +x mirror.sh && ./mirror.sh
  acmlab registry configure import-test --mirror quay.io/myorg --pull-secret ~/pull-secret.json`,
		Args: cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if targetRegistry == "" {
				return fmt.Errorf("--target is required")
			}

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
			m := registry.New(c, cfg)
			ctx := context.Background()

			// Single-cluster original behaviour
			if len(items) == 1 && fromFile == "" {
				clusterName := items[0].Name
				images, err := m.ListRequiredImages(ctx, clusterName)
				if err != nil {
					return fmt.Errorf("listing required images: %w", err)
				}
				script := registry.GenerateMirrorScript(images, targetRegistry)
				fmt.Print(script)
				return nil
			}

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						images, err := m.ListRequiredImages(ctx, item.Name)
						if err != nil {
							return "", err
						}
						script := registry.GenerateMirrorScript(images, targetRegistry)
						return fmt.Sprintf("# === %s ===\n%s", item.Name, script), nil
					},
				}
			}

			results := batch.Execute(ctx, work, concurrency, os.Stdout)
			if outputJSON {
				data, _ := batch.ToJSON(results)
				fmt.Println(string(data))
			} else {
				for _, r := range results {
					if r.OK {
						fmt.Println(r.Message)
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&targetRegistry, "target", "", "Target mirror registry (e.g., quay.io/myorg)")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "YAML file with cluster list")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Max parallel operations (max 20)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output results as JSON array")
	return cmd
}
