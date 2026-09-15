package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/pablofelix/acm-caas-poc/internal/batch"
	"github.com/pablofelix/acm-caas-poc/internal/importing"
)

func importCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import and manage external clusters",
		Long: `Import external clusters into ACM management. Unlike provisioned
clusters (created via Hive), imported clusters already exist and are
registered by installing the klusterlet agent on them.`,
	}
	cmd.AddCommand(importClusterCmd(), detachClusterCmd(), importStatusCmd(), importListCmd())
	return cmd
}

func importClusterCmd() *cobra.Command {
	var kubeconfigPath string
	var kubeconfigContext string
	var labels []string
	var clusterSet string
	var doWait bool
	var timeout time.Duration
	var fromFile string
	var concurrency int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "cluster [name...]",
		Short: "Import one or more external clusters into ACM",
		Long: `Registers clusters in ACM. Provide names as arguments or via --from-file.
Global --kubeconfig-path / --kubeconfig-context apply to all clusters when
no per-item kubeconfig is set in the file.`,
		Args: cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			labelMap := make(map[string]string, len(labels))
			for _, l := range labels {
				parts := strings.SplitN(l, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid label format %q, expected key=value", l)
				}
				labelMap[parts[0]] = parts[1]
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
			m := importing.New(c, cfg, logger)
			ctx := context.Background()

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						opts := importing.ImportOptions{
							Name:       item.Name,
							ClusterSet: clusterSet,
							Labels:     labelMap,
						}
						if item.ClusterSet != "" {
							opts.ClusterSet = item.ClusterSet
						}
						if item.Labels != nil {
							opts.Labels = item.Labels
						}
						// resolve kubeconfig: per-item overrides global flags
						switch {
						case item.KubeconfigPath != "":
							data, err := os.ReadFile(item.KubeconfigPath)
							if err != nil {
								return "", fmt.Errorf("reading kubeconfig: %w", err)
							}
							opts.Kubeconfig = data
						case item.KubeconfigContext != "":
							data, err := extractKubeconfigForContext(item.KubeconfigContext)
							if err != nil {
								return "", err
							}
							opts.Kubeconfig = data
						case kubeconfigPath != "":
							data, err := os.ReadFile(kubeconfigPath)
							if err != nil {
								return "", fmt.Errorf("reading kubeconfig: %w", err)
							}
							opts.Kubeconfig = data
						case kubeconfigContext != "":
							data, err := extractKubeconfigForContext(kubeconfigContext)
							if err != nil {
								return "", err
							}
							opts.Kubeconfig = data
						}

						result, err := m.Import(ctx, opts)
						if err != nil {
							return "", err
						}
						msg := result.Message
						if doWait && result.AutoImport {
							if err := m.WaitForImport(ctx, item.Name, timeout); err != nil {
								return "", fmt.Errorf("waiting: %w", err)
							}
							msg = "imported and available"
						}
						return msg, nil
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

	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig-path", "", "Path to spoke kubeconfig (applies to all)")
	cmd.Flags().StringVar(&kubeconfigContext, "kubeconfig-context", "", "Context name in default kubeconfig (applies to all)")
	cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "Labels for ManagedCluster (key=value, applies to all)")
	cmd.Flags().StringVar(&clusterSet, "cluster-set", "default", "ManagedClusterSet to assign")
	cmd.Flags().BoolVar(&doWait, "wait", false, "Wait for import to complete")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Timeout for --wait")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "YAML file with cluster list")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Max parallel operations (max 20)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output results as JSON array")
	return cmd
}

func detachClusterCmd() *cobra.Command {
	var fromFile string
	var concurrency int
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "detach [name...]",
		Short: "Detach one or more clusters from ACM (does not destroy the clusters)",
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
			m := importing.New(c, cfg, logger)
			ctx := context.Background()

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						if err := m.Detach(ctx, item.Name); err != nil {
							return "", err
						}
						return "detached from ACM", nil
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

func importStatusCmd() *cobra.Command {
	var outputJSON bool
	var fromFile string
	var concurrency int

	cmd := &cobra.Command{
		Use:   "status [name...]",
		Short: "Show import status of one or more clusters",
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
			m := importing.New(c, cfg, logger)
			ctx := context.Background()

			// single cluster with no file: keep existing detailed output
			if len(items) == 1 && fromFile == "" {
				status, err := m.GetImportStatus(ctx, items[0].Name)
				if err != nil {
					return fmt.Errorf("getting import status: %w", err)
				}
				if outputJSON {
					data, _ := json.MarshalIndent(status, "", "  ")
					fmt.Println(string(data))
					return nil
				}
				fmt.Printf("Cluster: %s\nAvailable: %s\nJoined: %s\nAuto-import: %v\n",
					status.Name, status.Available, status.Joined, status.AutoImport)
				return nil
			}

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						s, err := m.GetImportStatus(ctx, item.Name)
						if err != nil {
							return "", err
						}
						return fmt.Sprintf("Available=%s Joined=%s", s.Available, s.Joined), nil
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

func importListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List imported (non-Hive) clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}

			m := importing.New(c, cfg, logger)
			ctx := context.Background()

			clusters, err := m.ListImported(ctx)
			if err != nil {
				return fmt.Errorf("listing imported clusters: %w", err)
			}

			if len(clusters) == 0 {
				fmt.Println("No imported clusters found")
				return nil
			}

			fmt.Printf("Imported clusters (%d):\n", len(clusters))
			for _, cl := range clusters {
				fmt.Printf("  - %s  Available=%s  Joined=%s\n", cl.Name, cl.Available, cl.Joined)
			}

			return nil
		},
	}

	return cmd
}

func extractKubeconfigForContext(contextName string) ([]byte, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	ctx, ok := config.Contexts[contextName]
	if !ok {
		return nil, fmt.Errorf("context %q not found in kubeconfig", contextName)
	}

	cluster, ok := config.Clusters[ctx.Cluster]
	if !ok {
		return nil, fmt.Errorf("cluster %q referenced by context %q not found", ctx.Cluster, contextName)
	}

	authInfo, ok := config.AuthInfos[ctx.AuthInfo]
	if !ok {
		return nil, fmt.Errorf("user %q referenced by context %q not found", ctx.AuthInfo, contextName)
	}

	if err := embedClusterCertData(cluster); err != nil {
		return nil, fmt.Errorf("embedding cluster cert data: %w", err)
	}
	if err := embedAuthInfoCertData(authInfo); err != nil {
		return nil, fmt.Errorf("embedding auth cert data: %w", err)
	}

	minConfig := api.NewConfig()
	minConfig.Clusters[ctx.Cluster] = cluster
	minConfig.AuthInfos[ctx.AuthInfo] = authInfo
	minConfig.Contexts[contextName] = ctx
	minConfig.CurrentContext = contextName

	return clientcmd.Write(*minConfig)
}

func embedClusterCertData(cluster *api.Cluster) error {
	if cluster.CertificateAuthority != "" && len(cluster.CertificateAuthorityData) == 0 {
		data, err := os.ReadFile(cluster.CertificateAuthority)
		if err != nil {
			return fmt.Errorf("reading CA %s: %w", cluster.CertificateAuthority, err)
		}
		cluster.CertificateAuthorityData = data
		cluster.CertificateAuthority = ""
	}
	return nil
}

func embedAuthInfoCertData(authInfo *api.AuthInfo) error {
	if authInfo.ClientCertificate != "" && len(authInfo.ClientCertificateData) == 0 {
		data, err := os.ReadFile(authInfo.ClientCertificate)
		if err != nil {
			return fmt.Errorf("reading client cert %s: %w", authInfo.ClientCertificate, err)
		}
		authInfo.ClientCertificateData = data
		authInfo.ClientCertificate = ""
	}
	if authInfo.ClientKey != "" && len(authInfo.ClientKeyData) == 0 {
		data, err := os.ReadFile(authInfo.ClientKey)
		if err != nil {
			return fmt.Errorf("reading client key %s: %w", authInfo.ClientKey, err)
		}
		authInfo.ClientKeyData = data
		authInfo.ClientKey = ""
	}
	return nil
}
