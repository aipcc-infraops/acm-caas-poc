package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

func (m *Manager) AutoImport(ctx context.Context, clusterName, provider string) error {
	m.logger.Info("discovery.AutoImport", "cluster", clusterName, "provider", provider)

	runner := m.cmdRunner
	if runner == nil {
		runner = DefaultCmdRunner
	}

	var cluster *CloudCluster
	switch provider {
	case "aws":
		clusters, err := scanAWS(runner, "")
		if err != nil {
			return fmt.Errorf("scanning AWS: %w", err)
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
			return fmt.Errorf("scanning IBM Cloud: %w", err)
		}
		for _, c := range clusters {
			if c.Name == clusterName {
				cc := c
				cluster = &cc
				break
			}
		}
	default:
		return fmt.Errorf("unsupported provider %q (valid: aws, ibmcloud)", provider)
	}

	if cluster == nil {
		return fmt.Errorf("cluster %s not found in %s", clusterName, provider)
	}

	if err := m.ensureNamespace(ctx, clusterName); err != nil {
		return fmt.Errorf("creating namespace: %w", err)
	}

	mc := buildCloudManagedCluster(clusterName, provider, cluster.Type, cluster.Region)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedCluster, "", mc); err != nil {
		return fmt.Errorf("creating ManagedCluster: %w", err)
	}

	kac := buildKlusterletAddonConfig(clusterName)
	if err := m.client.CreateIfNotExists(ctx, client.GVRKlusterletAddonConfig, clusterName, kac); err != nil {
		return fmt.Errorf("creating KlusterletAddonConfig: %w", err)
	}

	return nil
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

func buildCloudManagedCluster(name, provider, clusterType, region string) *unstructured.Unstructured {
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
					"cluster.open-cluster-management.io/clusterset": "default",
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}
}
