package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/discovery"
)

func discoveryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discovery",
		Short: "Discover unmanaged clusters via OCM or cloud provider APIs",
	}
	cmd.AddCommand(
		discoveryEnableCmd(),
		discoveryDisableCmd(),
		discoveryListCmd(),
		discoveryImportCmd(),
		discoveryStatusCmd(),
		discoveryScanCmd(),
		discoveryAutoImportCmd(),
		discoveryScanKubeconfigsCmd(),
		discoveryAutoImportKubeconfigCmd(),
		discoveryListImportsCmd(),
		discoveryFixTLSCmd(),
	)
	return cmd
}

func discoveryEnableCmd() *cobra.Command {
	var namespace, token string
	var lastActive int
	var versions []string

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable cluster discovery in a namespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" {
				return fmt.Errorf("--namespace is required")
			}
			if token == "" {
				return fmt.Errorf("--token is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := discovery.New(c, cfg, logger)
			opts := discovery.EnableDiscoveryOpts{
				Namespace:  namespace,
				OCMToken:   token,
				LastActive: lastActive,
				Versions:   versions,
			}
			fmt.Printf("Enabling cluster discovery in %s...\n", namespace)
			if err := mgr.EnableDiscovery(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("Discovery enabled. Clusters will appear as DiscoveredCluster resources.")
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "open-cluster-management", "namespace for discovery resources")
	cmd.Flags().StringVar(&token, "token", "", "OpenShift Cluster Manager API token (required)")
	cmd.Flags().IntVar(&lastActive, "last-active", 7, "discover clusters active within N days")
	cmd.Flags().StringSliceVar(&versions, "versions", nil, "filter by OpenShift versions (e.g. 4.14,4.15)")
	return cmd
}

func discoveryDisableCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable cluster discovery in a namespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := discovery.New(c, cfg, logger)
			fmt.Printf("Disabling cluster discovery in %s...\n", namespace)
			if err := mgr.DisableDiscovery(context.Background(), namespace); err != nil {
				return err
			}
			fmt.Println("Discovery disabled.")
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "open-cluster-management", "namespace for discovery resources")
	return cmd
}

func discoveryListCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discovered clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := discovery.New(c, cfg, logger)
			clusters, err := mgr.ListDiscovered(context.Background(), namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(clusters, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(clusters) == 0 {
				fmt.Println("No discovered clusters found")
				return nil
			}
			fmt.Printf("%-25s %-10s %-12s %-15s %s\n", "NAME", "CLOUD", "VERSION", "REGION", "STATUS")
			for _, c := range clusters {
				fmt.Printf("%-25s %-10s %-12s %-15s %s\n", c.DisplayName, c.CloudProvider, c.OpenshiftVersion, c.Region, c.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "open-cluster-management", "namespace for discovery resources")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func discoveryImportCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "import <name>",
		Short: "Import a discovered cluster into ACM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := discovery.New(c, cfg, logger)
			fmt.Printf("Importing discovered cluster %s...\n", args[0])
			if err := mgr.ImportDiscovered(context.Background(), args[0], namespace); err != nil {
				return err
			}
			fmt.Printf("Cluster %s imported. Apply import manifests on the spoke to complete.\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "open-cluster-management", "namespace where cluster was discovered")
	return cmd
}

func discoveryScanCmd() *cobra.Command {
	var provider, region string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan cloud providers for unmanaged clusters (AWS EKS/ROSA, IBM Cloud IKS/ROKS)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := discovery.New(c, cfg, logger)
			opts := discovery.ScanOpts{
				Provider: provider,
				Region:   region,
			}
			clusters, err := mgr.ScanClusters(context.Background(), opts)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(clusters, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(clusters) == 0 {
				fmt.Println("No clusters found")
				return nil
			}
			fmt.Printf("%-25s %-10s %-8s %-15s %-12s %-10s %s\n", "NAME", "PROVIDER", "TYPE", "REGION", "VERSION", "STATUS", "MANAGED")
			for _, cl := range clusters {
				managed := ""
				if cl.Managed {
					managed = "yes"
				}
				fmt.Printf("%-25s %-10s %-8s %-15s %-12s %-10s %s\n", cl.Name, cl.Provider, cl.Type, cl.Region, cl.Version, cl.Status, managed)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "cloud provider: aws, ibmcloud (default: scan all)")
	cmd.Flags().StringVar(&region, "region", "", "filter by region")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func discoveryAutoImportCmd() *cobra.Command {
	var provider, clusterSet string
	var dryRun, allowInsecure, fixTLS bool

	cmd := &cobra.Command{
		Use:   "auto-import <name>",
		Short: "Import a cloud-discovered cluster into ACM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if provider == "" {
				return fmt.Errorf("--provider is required (aws or ibmcloud)")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := discovery.New(c, cfg, logger)
			opts := discovery.ImportOpts{DryRun: dryRun, ClusterSet: clusterSet, AllowInsecure: allowInsecure, FixTLS: fixTLS}
			if dryRun {
				fmt.Printf("[DRY-RUN] Previewing import of %s cluster %s...\n", provider, args[0])
			} else {
				fmt.Printf("Importing %s cluster %s...\n", provider, args[0])
			}
			preview, err := mgr.AutoImport(context.Background(), args[0], provider, opts)
			if err != nil {
				return err
			}
			if dryRun {
				fmt.Printf("Would create the following resources:\n")
				for _, r := range preview.Resources {
					fmt.Printf("  - %s\n", r)
				}
				return nil
			}
			if preview.AutoImported {
				fmt.Printf("Cluster %s imported into set %s. ACM will automatically install the klusterlet.\n", args[0], preview.ClusterSet)
			} else {
				fmt.Printf("Cluster %s imported into set %s. Auto-credentials not available; apply import manifests on the spoke manually.\n", args[0], preview.ClusterSet)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "cloud provider: aws, ibmcloud (required)")
	cmd.Flags().StringVar(&clusterSet, "cluster-set", "", "assign to a ManagedClusterSet (default: default)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview what would be created without creating resources")
	cmd.Flags().BoolVar(&allowInsecure, "allow-insecure", false, "allow importing clusters with insecure TLS (skip certificate verification)")
	cmd.Flags().BoolVar(&fixTLS, "fix-tls", false, "fetch server CA certificates and replace insecure-skip-tls-verify")
	return cmd
}

func discoveryStatusCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show discovery configuration status",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := discovery.New(c, cfg, logger)
			status, err := mgr.DiscoveryStatus(context.Background(), namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Namespace:  %s\n", status.Namespace)
			fmt.Printf("Credential: %s\n", status.Credential)
			fmt.Printf("Last Active: %d days\n", status.LastActive)
			fmt.Printf("Discovered: %d clusters\n", status.ClusterCount)
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "open-cluster-management", "namespace for discovery resources")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func discoveryScanKubeconfigsCmd() *cobra.Command {
	var dir string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "scan-kubeconfigs",
		Short: "Scan kubeconfig files to discover clusters not yet managed by ACM",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := discovery.New(c, cfg, logger)
			clusters, err := mgr.ScanKubeconfigs(cmd.Context(), dir)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(clusters, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(clusters) == 0 {
				fmt.Println("No clusters found in kubeconfig files")
				return nil
			}
			fmt.Printf("%-25s %-45s %-25s %-8s %s\n", "NAME", "SERVER", "CONTEXT", "MANAGED", "SOURCE")
			for _, cl := range clusters {
				managed := ""
				if cl.Managed {
					managed = "yes"
				}
				fmt.Printf("%-25s %-45s %-25s %-8s %s\n", cl.Name, cl.Server, cl.Context, managed, cl.Source)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "directory to scan for kubeconfig files (default: ~/.kube/)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func discoveryAutoImportKubeconfigCmd() *cobra.Command {
	var name, kubeconfigPath, clusterSet string
	var dryRun, allowInsecure, fixTLS bool

	cmd := &cobra.Command{
		Use:   "auto-import-kubeconfig",
		Short: "Import a cluster discovered via kubeconfig into ACM",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if kubeconfigPath == "" {
				return fmt.Errorf("--kubeconfig is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := discovery.New(c, cfg, logger)
			opts := discovery.ImportOpts{DryRun: dryRun, ClusterSet: clusterSet, AllowInsecure: allowInsecure, FixTLS: fixTLS}
			if dryRun {
				fmt.Printf("[DRY-RUN] Previewing import of cluster %s from %s...\n", name, kubeconfigPath)
			} else {
				fmt.Printf("Importing cluster %s from kubeconfig %s...\n", name, kubeconfigPath)
			}
			preview, err := mgr.AutoImportKubeconfig(cmd.Context(), name, kubeconfigPath, opts)
			if err != nil {
				return err
			}
			if dryRun {
				fmt.Printf("Would create the following resources:\n")
				for _, r := range preview.Resources {
					fmt.Printf("  - %s\n", r)
				}
				return nil
			}
			fmt.Printf("Cluster %s imported into set %s. ACM will use the kubeconfig to install the klusterlet.\n", name, preview.ClusterSet)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "cluster name for ACM (required)")
	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig", "", "path to the spoke cluster kubeconfig (required)")
	cmd.Flags().StringVar(&clusterSet, "cluster-set", "", "assign to a ManagedClusterSet (default: default)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview what would be created without creating resources")
	cmd.Flags().BoolVar(&allowInsecure, "allow-insecure", false, "allow importing with insecure TLS (skip certificate verification)")
	cmd.Flags().BoolVar(&fixTLS, "fix-tls", false, "fetch server CA certificates and replace insecure-skip-tls-verify")
	return cmd
}

func discoveryFixTLSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fix-tls <cluster-name>",
		Short: "Replace insecure-skip-tls-verify with server CA certificate in an imported cluster's auto-import-secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := discovery.New(c, cfg, logger)
			fmt.Printf("Fixing TLS for cluster %s...\n", args[0])
			result, err := mgr.FixTLS(context.Background(), args[0])
			if err != nil {
				return err
			}
			if result.Fixed {
				fmt.Printf("TLS fixed for %s: %s\n", result.ClusterName, result.Message)
			} else {
				fmt.Printf("No changes needed for %s: %s\n", result.ClusterName, result.Message)
			}
			return nil
		},
	}
	return cmd
}

func discoveryListImportsCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list-imports",
		Short: "List clusters imported via discovery (OCM, cloud, or kubeconfig)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := discovery.New(c, cfg, logger)
			imports, err := mgr.ListImports(context.Background())
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(imports, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(imports) == 0 {
				fmt.Println("No clusters imported via discovery")
				return nil
			}
			fmt.Printf("%-30s %-22s %-15s %-22s %s\n", "NAME", "CREATED-VIA", "CLUSTER-SET", "IMPORTED-AT", "STATUS")
			for _, imp := range imports {
				fmt.Printf("%-30s %-22s %-15s %-22s %s\n", imp.Name, imp.CreatedVia, imp.ClusterSet, imp.CreatedAt, imp.Status)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
