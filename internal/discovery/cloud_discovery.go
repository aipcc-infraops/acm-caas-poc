package discovery

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type CloudCluster struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Type     string `json:"type"`
	Region   string `json:"region"`
	Version  string `json:"version"`
	Status   string `json:"status"`
	Managed  bool   `json:"managed"`
}

type ScanOpts struct {
	Provider string
	Region   string
}

type CmdRunner func(name string, args ...string) ([]byte, error)

type ImportOpts struct {
	DryRun     bool
	ClusterSet string
}

func (o ImportOpts) resolvedSet() string {
	if o.ClusterSet != "" {
		return o.ClusterSet
	}
	return "default"
}

type ImportPreview struct {
	ClusterName string `json:"clusterName"`
	ClusterSet  string `json:"clusterSet"`
	Provider    string `json:"provider,omitempty"`
	Type        string `json:"type,omitempty"`
	Region      string `json:"region,omitempty"`
	Resources   []string `json:"resources"`
}

type ImportedCluster struct {
	Name       string `json:"name"`
	CreatedVia string `json:"createdVia"`
	ClusterSet string `json:"clusterSet"`
	CreatedAt  string `json:"createdAt"`
	Status     string `json:"status"`
}

func DefaultCmdRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func (m *Manager) ScanClusters(ctx context.Context, opts ScanOpts) ([]CloudCluster, error) {
	m.logger.Info("discovery.ScanClusters", "provider", opts.Provider, "region", opts.Region)

	runner := m.cmdRunner
	if runner == nil {
		runner = DefaultCmdRunner
	}

	var all []CloudCluster

	if opts.Provider == "" || opts.Provider == "aws" {
		clusters, err := scanAWS(runner, opts.Region)
		if err != nil {
			m.logger.Warn("AWS scan skipped", "error", err)
		} else {
			all = append(all, clusters...)
		}
	}

	if opts.Provider == "" || opts.Provider == "ibmcloud" {
		clusters, err := scanIBMCloud(runner, opts.Region)
		if err != nil {
			m.logger.Warn("IBM Cloud scan skipped", "error", err)
		} else {
			all = append(all, clusters...)
		}
	}

	if opts.Provider != "" && opts.Provider != "aws" && opts.Provider != "ibmcloud" {
		return nil, fmt.Errorf("unsupported provider %q (valid: aws, ibmcloud)", opts.Provider)
	}

	managed, err := m.listManagedNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing managed clusters: %w", err)
	}
	for i := range all {
		if managed[all[i].Name] {
			all[i].Managed = true
		}
	}

	return all, nil
}

func (m *Manager) AutoImport(ctx context.Context, clusterName, provider string, opts ImportOpts) (*ImportPreview, error) {
	m.logger.Info("discovery.AutoImport", "cluster", clusterName, "provider", provider, "dryRun", opts.DryRun)

	runner := m.cmdRunner
	if runner == nil {
		runner = DefaultCmdRunner
	}

	var cluster *CloudCluster
	switch provider {
	case "aws":
		clusters, err := scanAWS(runner, "")
		if err != nil {
			return nil, fmt.Errorf("scanning AWS: %w", err)
		}
		for _, c := range clusters {
			if c.Name == clusterName {
				cc := c
				cluster = &cc
				break
			}
		}
	case "ibmcloud":
		clusters, err := scanIBMCloud(runner, "")
		if err != nil {
			return nil, fmt.Errorf("scanning IBM Cloud: %w", err)
		}
		for _, c := range clusters {
			if c.Name == clusterName {
				cc := c
				cluster = &cc
				break
			}
		}
	default:
		return nil, fmt.Errorf("unsupported provider %q (valid: aws, ibmcloud)", provider)
	}

	if cluster == nil {
		return nil, fmt.Errorf("cluster %s not found in %s", clusterName, provider)
	}

	preview := &ImportPreview{
		ClusterName: clusterName,
		ClusterSet:  opts.resolvedSet(),
		Provider:    provider,
		Type:        cluster.Type,
		Region:      cluster.Region,
		Resources: []string{
			fmt.Sprintf("Namespace/%s", clusterName),
			fmt.Sprintf("ManagedCluster/%s (set=%s)", clusterName, opts.resolvedSet()),
			fmt.Sprintf("KlusterletAddonConfig/%s", clusterName),
		},
	}

	if opts.DryRun {
		return preview, nil
	}

	if err := m.ensureNamespace(ctx, clusterName); err != nil {
		return nil, fmt.Errorf("creating namespace: %w", err)
	}

	mc := buildCloudManagedCluster(clusterName, provider, cluster.Type, cluster.Region, opts.resolvedSet())
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedCluster, "", mc); err != nil {
		return nil, fmt.Errorf("creating ManagedCluster: %w", err)
	}

	kac := buildKlusterletAddonConfig(clusterName)
	if err := m.client.CreateIfNotExists(ctx, client.GVRKlusterletAddonConfig, clusterName, kac); err != nil {
		return nil, fmt.Errorf("creating KlusterletAddonConfig: %w", err)
	}

	return preview, nil
}

func (m *Manager) listManagedNames(ctx context.Context) (map[string]bool, error) {
	list, err := m.client.List(ctx, client.GVRManagedCluster, "", "")
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool, len(list.Items))
	for _, item := range list.Items {
		names[item.GetName()] = true
	}
	return names, nil
}

type eksListOutput struct {
	Clusters []string `json:"clusters"`
}

type eksDescribeOutput struct {
	Cluster struct {
		Name     string `json:"name"`
		Version  string `json:"version"`
		Status   string `json:"status"`
		Arn      string `json:"arn"`
		Endpoint string `json:"endpoint"`
	} `json:"cluster"`
}

type rosaCluster struct {
	Name    string       `json:"name"`
	ID      string       `json:"id"`
	Region  string       `json:"region,omitempty"`
	Status  string       `json:"status,omitempty"`
	Version rosaVersion  `json:"version,omitempty"`
	AWS     rosaAWS      `json:"aws,omitempty"`
	State   string       `json:"state,omitempty"`
	OpenshiftVersion string `json:"openshift_version,omitempty"`
}

type rosaVersion struct {
	ID string `json:"id"`
}

type rosaAWS struct {
	Region string `json:"region"`
}

type ibmCluster struct {
	Name         string `json:"name"`
	ID           string `json:"id"`
	Region       string `json:"region"`
	State        string `json:"state"`
	Type         string `json:"type"`
	MasterKubeVersion string `json:"masterKubeVersion"`
}

func scanAWS(runner CmdRunner, region string) ([]CloudCluster, error) {
	var all []CloudCluster

	eksArgs := []string{"eks", "list-clusters", "--output", "json"}
	if region != "" {
		eksArgs = append(eksArgs, "--region", region)
	}
	out, err := runner("aws", eksArgs...)
	if err == nil {
		var eksResult eksListOutput
		if json.Unmarshal(out, &eksResult) == nil {
			for _, name := range eksResult.Clusters {
				descArgs := []string{"eks", "describe-cluster", "--name", name, "--output", "json"}
				if region != "" {
					descArgs = append(descArgs, "--region", region)
				}
				descOut, descErr := runner("aws", descArgs...)
				cc := CloudCluster{
					Name:     name,
					Provider: "aws",
					Type:     "eks",
				}
				if descErr == nil {
					var desc eksDescribeOutput
					if json.Unmarshal(descOut, &desc) == nil {
						cc.Version = desc.Cluster.Version
						cc.Status = desc.Cluster.Status
					}
				}
				if region != "" {
					cc.Region = region
				}
				all = append(all, cc)
			}
		}
	}

	rosaOut, rosaErr := runner("rosa", "list", "clusters", "--output", "json")
	if rosaErr == nil {
		var rosaClusters []rosaCluster
		if json.Unmarshal(rosaOut, &rosaClusters) == nil {
			for _, rc := range rosaClusters {
				cc := CloudCluster{
					Name:     rc.Name,
					Provider: "aws",
					Type:     "rosa",
					Status:   rc.State,
				}
				if rc.Version.ID != "" {
					cc.Version = rc.Version.ID
				} else if rc.OpenshiftVersion != "" {
					cc.Version = rc.OpenshiftVersion
				}
				if rc.AWS.Region != "" {
					cc.Region = rc.AWS.Region
				} else if rc.Region != "" {
					cc.Region = rc.Region
				}
				if region != "" && cc.Region != region {
					continue
				}
				all = append(all, cc)
			}
		}
	}

	if err != nil && rosaErr != nil {
		return nil, fmt.Errorf("neither aws nor rosa CLI available")
	}

	return all, nil
}

func scanIBMCloud(runner CmdRunner, region string) ([]CloudCluster, error) {
	out, err := runner("ibmcloud", "ks", "cluster", "ls", "--output", "json")
	if err != nil {
		return nil, fmt.Errorf("ibmcloud CLI not available: %w", err)
	}

	var clusters []ibmCluster
	if err := json.Unmarshal(out, &clusters); err != nil {
		return nil, fmt.Errorf("parsing ibmcloud output: %w", err)
	}

	var result []CloudCluster
	for _, c := range clusters {
		if region != "" && c.Region != region {
			continue
		}
		clusterType := "iks"
		if c.Type == "openshift" {
			clusterType = "roks"
		}
		result = append(result, CloudCluster{
			Name:     c.Name,
			Provider: "ibmcloud",
			Type:     clusterType,
			Region:   c.Region,
			Version:  c.MasterKubeVersion,
			Status:   c.State,
		})
	}
	return result, nil
}

type KubeconfigCluster struct {
	Name    string `json:"name"`
	Server  string `json:"server"`
	Context string `json:"context"`
	Managed bool   `json:"managed"`
	Source  string `json:"source"`
}

func (m *Manager) ScanKubeconfigs(ctx context.Context, dir string) ([]KubeconfigCluster, error) {
	m.logger.Info("discovery.ScanKubeconfigs", "dir", dir)

	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolving home directory: %w", err)
		}
		dir = filepath.Join(home, ".kube")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var all []KubeconfigCluster
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		cfg, err := clientcmd.LoadFromFile(path)
		if err != nil {
			continue
		}
		for ctxName, ctxVal := range cfg.Contexts {
			clusterInfo, ok := cfg.Clusters[ctxVal.Cluster]
			if !ok {
				continue
			}
			all = append(all, KubeconfigCluster{
				Name:    ctxVal.Cluster,
				Server:  clusterInfo.Server,
				Context: ctxName,
				Source:  path,
			})
		}
	}

	managed, err := m.listManagedWithURLs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing managed clusters: %w", err)
	}
	for i := range all {
		if managed[all[i].Name] || managed[all[i].Server] {
			all[i].Managed = true
		}
	}

	return all, nil
}

func (m *Manager) listManagedWithURLs(ctx context.Context) (map[string]bool, error) {
	list, err := m.client.List(ctx, client.GVRManagedCluster, "", "")
	if err != nil {
		return nil, err
	}
	lookup := make(map[string]bool, len(list.Items)*2)
	for _, item := range list.Items {
		lookup[item.GetName()] = true
		urls, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "managedClusterClientConfigs")
		if len(urls) == 0 {
			url, _, _ := unstructured.NestedString(item.Object, "spec", "managedClusterClientConfigs", "url")
			if url != "" {
				lookup[url] = true
			}
		}
		statusURL, _, _ := unstructured.NestedString(item.Object, "status", "apiServerURL")
		if statusURL != "" {
			lookup[statusURL] = true
		}
	}
	return lookup, nil
}

func (m *Manager) AutoImportKubeconfig(ctx context.Context, name, kubeconfigPath string, opts ImportOpts) (*ImportPreview, error) {
	m.logger.Info("discovery.AutoImportKubeconfig", "name", name, "kubeconfig", kubeconfigPath, "dryRun", opts.DryRun)

	data, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("reading kubeconfig %s: %w", kubeconfigPath, err)
	}

	if _, err := clientcmd.Load(data); err != nil {
		return nil, fmt.Errorf("invalid kubeconfig %s: %w", kubeconfigPath, err)
	}

	preview := &ImportPreview{
		ClusterName: name,
		ClusterSet:  opts.resolvedSet(),
		Resources: []string{
			fmt.Sprintf("Namespace/%s", name),
			fmt.Sprintf("ManagedCluster/%s (set=%s)", name, opts.resolvedSet()),
			fmt.Sprintf("KlusterletAddonConfig/%s", name),
			fmt.Sprintf("Secret/%s/auto-import-secret", name),
		},
	}

	if opts.DryRun {
		return preview, nil
	}

	if err := m.ensureNamespace(ctx, name); err != nil {
		return nil, fmt.Errorf("creating namespace: %w", err)
	}

	mc := buildKubeconfigManagedCluster(name, opts.resolvedSet())
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedCluster, "", mc); err != nil {
		return nil, fmt.Errorf("creating ManagedCluster: %w", err)
	}

	kac := buildKlusterletAddonConfig(name)
	if err := m.client.CreateIfNotExists(ctx, client.GVRKlusterletAddonConfig, name, kac); err != nil {
		return nil, fmt.Errorf("creating KlusterletAddonConfig: %w", err)
	}

	secret := buildAutoImportSecret(name, data)
	if err := m.client.CreateIfNotExists(ctx, client.GVRSecret, name, secret); err != nil {
		return nil, fmt.Errorf("creating auto-import secret: %w", err)
	}

	return preview, nil
}

func buildKubeconfigManagedCluster(name, clusterSet string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"name":                        name,
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/discovery": "true",
					"created-via":                 "kubeconfig-discovery",
					"cluster.open-cluster-management.io/clusterset": clusterSet,
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}
}

func buildAutoImportSecret(name string, kubeconfig []byte) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "auto-import-secret",
				"namespace": name,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"type": "Opaque",
			"data": map[string]interface{}{
				"kubeconfig": base64.StdEncoding.EncodeToString(kubeconfig),
			},
		},
	}
}

func buildCloudManagedCluster(name, provider, clusterType, region, clusterSet string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"name":                          name,
					"acmlab.redhat.com/managed":     "true",
					"acmlab.redhat.com/discovered-from": provider,
					"cloud":                         provider,
					"cluster-type":                  clusterType,
					"region":                        region,
					"created-via":                   "cloud-discovery",
					"cluster.open-cluster-management.io/clusterset": clusterSet,
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}
}

func (m *Manager) ListImports(ctx context.Context) ([]ImportedCluster, error) {
	m.logger.Info("discovery.ListImports")

	list, err := m.client.List(ctx, client.GVRManagedCluster, "", "acmlab.redhat.com/discovery=true")
	if err != nil {
		return nil, fmt.Errorf("listing imported clusters: %w", err)
	}

	result := make([]ImportedCluster, 0, len(list.Items))
	for _, item := range list.Items {
		labels := item.GetLabels()
		createdVia := labels["created-via"]
		clusterSet := labels["cluster.open-cluster-management.io/clusterset"]
		createdAt := item.GetCreationTimestamp().Format("2006-01-02T15:04:05Z")

		status := "Unknown"
		conditions, found, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
		if found {
			for _, cond := range conditions {
				c, ok := cond.(map[string]interface{})
				if !ok {
					continue
				}
				t, _, _ := unstructured.NestedString(c, "type")
				s, _, _ := unstructured.NestedString(c, "status")
				if t == "ManagedClusterConditionAvailable" {
					if s == "True" {
						status = "Available"
					} else {
						status = "Unavailable"
					}
					break
				}
			}
		}

		result = append(result, ImportedCluster{
			Name:       item.GetName(),
			CreatedVia: createdVia,
			ClusterSet: clusterSet,
			CreatedAt:  createdAt,
			Status:     status,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt > result[j].CreatedAt
	})

	return result, nil
}
