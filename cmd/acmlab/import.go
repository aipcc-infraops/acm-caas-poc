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

	cmd := &cobra.Command{
		Use:   "cluster <name>",
		Short: "Import an external cluster into ACM",
		Long: `Registers a cluster in ACM by creating a ManagedCluster, namespace,
and KlusterletAddonConfig.

Auto-import (recommended): provide the spoke kubeconfig via --kubeconfig-path
or --kubeconfig-context. ACM installs the klusterlet automatically.

Manual import: without kubeconfig flags, you must apply the import manifests
on the spoke yourself (instructions shown after creation).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			c, err := buildClient()
			if err != nil {
				return err
			}

			opts := importing.ImportOptions{
				Name:       name,
				ClusterSet: clusterSet,
			}

			if len(labels) > 0 {
				opts.Labels = make(map[string]string, len(labels))
				for _, l := range labels {
					parts := strings.SplitN(l, "=", 2)
					if len(parts) != 2 {
						return fmt.Errorf("invalid label format %q, expected key=value", l)
					}
					opts.Labels[parts[0]] = parts[1]
				}
			}

			if kubeconfigPath != "" {
				data, err := os.ReadFile(kubeconfigPath)
				if err != nil {
					return fmt.Errorf("reading kubeconfig %s: %w", kubeconfigPath, err)
				}
				opts.Kubeconfig = data
			} else if kubeconfigContext != "" {
				data, err := extractKubeconfigForContext(kubeconfigContext)
				if err != nil {
					return fmt.Errorf("extracting kubeconfig for context %s: %w", kubeconfigContext, err)
				}
				opts.Kubeconfig = data
			}

			m := importing.New(c, cfg)
			ctx := context.Background()

			result, err := m.Import(ctx, opts)
			if err != nil {
				return fmt.Errorf("importing cluster: %w", err)
			}

			fmt.Println(result.Message)

			if doWait && result.AutoImport {
				fmt.Printf("Waiting for cluster to become available (timeout: %v)...\n", timeout)
				if err := m.WaitForImport(ctx, name, timeout); err != nil {
					return fmt.Errorf("waiting for import: %w", err)
				}
				fmt.Println("Cluster successfully imported and available")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig-path", "", "Path to spoke cluster kubeconfig for auto-import")
	cmd.Flags().StringVar(&kubeconfigContext, "kubeconfig-context", "", "Context name in default kubeconfig to use for auto-import")
	cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "Labels for the ManagedCluster (key=value)")
	cmd.Flags().StringVar(&clusterSet, "cluster-set", "default", "ManagedClusterSet to assign")
	cmd.Flags().BoolVar(&doWait, "wait", false, "Wait for import to complete")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Timeout for wait operation")

	return cmd
}

func detachClusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detach <name>",
		Short: "Detach a cluster from ACM (does not destroy the cluster)",
		Long: `Removes a ManagedCluster from ACM management. The underlying
cluster continues to run — only the ACM registration is removed.
ACM's cleanup controllers handle removing the klusterlet from the spoke.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			c, err := buildClient()
			if err != nil {
				return err
			}

			m := importing.New(c, cfg)
			ctx := context.Background()

			if err := m.Detach(ctx, name); err != nil {
				return fmt.Errorf("detaching cluster: %w", err)
			}

			fmt.Printf("Cluster %s detached from ACM\n", name)
			return nil
		},
	}

	return cmd
}

func importStatusCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "status <name>",
		Short: "Show import status of a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			c, err := buildClient()
			if err != nil {
				return err
			}

			m := importing.New(c, cfg)
			ctx := context.Background()

			status, err := m.GetImportStatus(ctx, name)
			if err != nil {
				return fmt.Errorf("getting import status: %w", err)
			}

			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Cluster: %s\n", status.Name)
			fmt.Printf("Available: %s\n", status.Available)
			fmt.Printf("Joined: %s\n", status.Joined)
			if status.CreatedVia != "" {
				fmt.Printf("Created via: %s\n", status.CreatedVia)
			}
			fmt.Printf("Auto-import: %v\n", status.AutoImport)

			return nil
		},
	}

	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")

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

			m := importing.New(c, cfg)
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
