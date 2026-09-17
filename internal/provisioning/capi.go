package provisioning

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type CAPIClusterOpts struct {
	Name                 string
	Namespace            string
	InfraProvider        string
	KubernetesVersion    string
	WorkerReplicas       int64
	ControlPlaneReplicas int64
	PullSecret           string
}

type CAPIClusterInfo struct {
	Name              string   `json:"name"`
	Namespace         string   `json:"namespace"`
	Phase             string   `json:"phase"`
	Ready             bool     `json:"ready"`
	KubernetesVersion string   `json:"kubernetesVersion,omitempty"`
	Conditions        []string `json:"conditions,omitempty"`
}

func (m *Manager) applyCAPIDefaults(opts *CAPIClusterOpts) {
	if opts.Namespace == "" {
		opts.Namespace = opts.Name
	}
	if opts.InfraProvider == "" {
		opts.InfraProvider = "docker"
	}
	if opts.KubernetesVersion == "" {
		opts.KubernetesVersion = "v1.30.0"
	}
	if opts.WorkerReplicas == 0 {
		opts.WorkerReplicas = 2
	}
	if opts.ControlPlaneReplicas == 0 {
		opts.ControlPlaneReplicas = 1
	}
}

func (m *Manager) CreateCAPI(ctx context.Context, opts CAPIClusterOpts) error {
	m.logger.Info("provisioning.CreateCAPI", "cluster", opts.Name)
	m.applyCAPIDefaults(&opts)

	if opts.Name == "" {
		return fmt.Errorf("cluster name is required")
	}

	ns := buildNamespace(opts.Namespace)
	if err := m.client.CreateIfNotExists(ctx, client.GVRNamespace, "", ns); err != nil {
		return fmt.Errorf("creating namespace %s: %w", opts.Namespace, err)
	}

	cluster := buildCAPICluster(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRCAPICluster, opts.Namespace, cluster); err != nil {
		return fmt.Errorf("creating CAPI Cluster %s: %w", opts.Name, err)
	}

	md := buildCAPIMachineDeployment(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRCAPIMachineDeployment, opts.Namespace, md); err != nil {
		return fmt.Errorf("creating CAPI MachineDeployment %s-workers: %w", opts.Name, err)
	}

	mc := buildCAPIManagedCluster(opts.Name)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedCluster, "", mc); err != nil {
		return fmt.Errorf("creating ManagedCluster %s: %w", opts.Name, err)
	}

	return nil
}

func (m *Manager) DestroyCAPI(ctx context.Context, name string) error {
	m.logger.Info("provisioning.DestroyCAPI", "cluster", name)

	err := m.client.DeleteIfExists(ctx, client.GVRCAPIMachineDeployment, name, name+"-workers")
	if err != nil {
		return fmt.Errorf("deleting CAPI MachineDeployment %s-workers: %w", name, err)
	}

	err = m.client.DeleteIfExists(ctx, client.GVRCAPICluster, name, name)
	if err != nil {
		return fmt.Errorf("deleting CAPI Cluster %s: %w", name, err)
	}

	err = m.client.DeleteIfExists(ctx, client.GVRManagedCluster, "", name)
	if err != nil {
		return fmt.Errorf("deleting ManagedCluster %s: %w", name, err)
	}

	return nil
}

func (m *Manager) StatusCAPI(ctx context.Context, name string) (*CAPIClusterInfo, error) {
	m.logger.Info("provisioning.StatusCAPI", "cluster", name)
	obj, err := m.client.Get(ctx, client.GVRCAPICluster, name, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("CAPI cluster %s not found", name)
		}
		return nil, fmt.Errorf("getting CAPI Cluster %s: %w", name, err)
	}
	return parseCAPIClusterInfo(obj.Object), nil
}

func (m *Manager) ListCAPI(ctx context.Context) ([]CAPIClusterInfo, error) {
	m.logger.Info("provisioning.ListCAPI")
	list, err := m.client.List(ctx, client.GVRCAPICluster, "", "acmlab.redhat.com/managed")
	if err != nil {
		return nil, fmt.Errorf("listing CAPI Clusters: %w", err)
	}
	clusters := make([]CAPIClusterInfo, 0, len(list.Items))
	for _, item := range list.Items {
		clusters = append(clusters, *parseCAPIClusterInfo(item.Object))
	}
	return clusters, nil
}
