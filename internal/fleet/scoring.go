package fleet

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type ScoringOpts struct {
	Name         string
	Namespace    string
	Prioritizers []string
	ClusterSet   string
	Labels       map[string]string
}

type ScoringDecision struct {
	Cluster string `json:"cluster"`
	Score   int64  `json:"score"`
	Reason  string `json:"reason"`
}

type ScoringStatus struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Decisions []ScoringDecision `json:"decisions"`
}

type ScoringInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Clusters  int    `json:"clusters"`
}

func (i *Inspector) ConfigureScoring(ctx context.Context, opts ScoringOpts) error {
	i.logger.Info("fleet.ConfigureScoring", "name", opts.Name)
	ns := opts.Namespace
	if ns == "" {
		ns = "open-cluster-management"
	}
	if len(opts.Prioritizers) == 0 {
		opts.Prioritizers = []string{"ResourceAllocatableCPU", "ResourceAllocatableMemory"}
	}

	placement := buildScoringPlacement(opts.Name, ns, opts.Prioritizers, opts.ClusterSet, opts.Labels)
	if err := i.client.CreateIfNotExists(ctx, client.GVRPlacement, ns, placement); err != nil {
		return fmt.Errorf("creating scoring placement: %w", err)
	}
	return nil
}

func (i *Inspector) GetScoring(ctx context.Context, name, namespace string) (*ScoringStatus, error) {
	i.logger.Info("fleet.GetScoring", "name", name)
	if namespace == "" {
		namespace = "open-cluster-management"
	}

	_, err := i.client.Get(ctx, client.GVRPlacement, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting scoring placement %s: %w", name, err)
	}

	status := &ScoringStatus{Name: name, Namespace: namespace}

	decisions, err := i.client.List(ctx, client.GVRPlacementDecision, namespace, "cluster.open-cluster-management.io/placement="+name)
	if err != nil {
		return status, nil
	}

	for _, d := range decisions.Items {
		statusField, _ := d.Object["status"].(map[string]interface{})
		if statusField == nil {
			continue
		}
		decs, _ := statusField["decisions"].([]interface{})
		for _, raw := range decs {
			dec, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			cluster, _ := dec["clusterName"].(string)
			reason, _ := dec["reason"].(string)
			status.Decisions = append(status.Decisions, ScoringDecision{
				Cluster: cluster,
				Reason:  reason,
			})
		}
	}

	return status, nil
}

func (i *Inspector) RemoveScoring(ctx context.Context, name, namespace string) error {
	i.logger.Info("fleet.RemoveScoring", "name", name)
	if namespace == "" {
		namespace = "open-cluster-management"
	}

	_, err := i.client.Get(ctx, client.GVRPlacement, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("scoring placement %s not found", name)
		}
		return fmt.Errorf("checking scoring placement: %w", err)
	}

	labels, _ := i.client.Get(ctx, client.GVRPlacement, namespace, name)
	if labels != nil {
		l := labels.GetLabels()
		if l == nil || l["acmlab.redhat.com/scoring"] != "true" {
			return fmt.Errorf("placement %s is not a scoring placement", name)
		}
	}

	return i.client.DeleteIfExists(ctx, client.GVRPlacement, namespace, name)
}

func (i *Inspector) ListScoring(ctx context.Context, namespace string) ([]ScoringInfo, error) {
	i.logger.Info("fleet.ListScoring")
	if namespace == "" {
		namespace = "open-cluster-management"
	}

	list, err := i.client.List(ctx, client.GVRPlacement, namespace, "acmlab.redhat.com/scoring")
	if err != nil {
		return nil, fmt.Errorf("listing scoring placements: %w", err)
	}

	infos := make([]ScoringInfo, 0, len(list.Items))
	for _, item := range list.Items {
		info := ScoringInfo{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
		}
		infos = append(infos, info)
	}
	return infos, nil
}
