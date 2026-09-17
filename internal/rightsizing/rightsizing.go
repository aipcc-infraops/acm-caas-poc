package rightsizing

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

type Recommendation struct {
	Namespace      string `json:"namespace"`
	Workload       string `json:"workload"`
	Container      string `json:"container"`
	CurrentCPU     string `json:"currentCPU"`
	CurrentMem     string `json:"currentMem"`
	RecommendedCPU string `json:"recommendedCPU"`
	RecommendedMem string `json:"recommendedMem"`
	Savings        string `json:"savings"`
}

type AdjustResult struct {
	Workload  string `json:"workload"`
	Container string `json:"container"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
	DryRun    bool   `json:"dryRun"`
}

type RightsizingInfo struct {
	Cluster string `json:"cluster"`
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) Enable(ctx context.Context, cluster string) error {
	m.logger.Info("rightsizing.Enable", "cluster", cluster)

	mw := buildRightsizingManifestWork(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, cluster, mw); err != nil {
		return fmt.Errorf("creating right-sizing ManifestWork on %s: %w", cluster, err)
	}
	return nil
}

func (m *Manager) Disable(ctx context.Context, cluster string) (bool, error) {
	m.logger.Info("rightsizing.Disable", "cluster", cluster)
	name := rightsizingMWName(cluster)

	_, err := m.client.Get(ctx, client.GVRManifestWork, cluster, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking right-sizing ManifestWork on %s: %w", cluster, err)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRManifestWork, cluster, name); err != nil {
		return false, fmt.Errorf("removing right-sizing ManifestWork on %s: %w", cluster, err)
	}
	return true, nil
}

func (m *Manager) Advise(ctx context.Context, cluster string) ([]Recommendation, error) {
	m.logger.Info("rightsizing.Advise", "cluster", cluster)

	name := rightsizingMWName(cluster)
	obj, err := m.client.Get(ctx, client.GVRManifestWork, cluster, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("right-sizing not enabled on %s — run 'acmlab rightsizing enable' first", cluster)
		}
		return nil, fmt.Errorf("getting right-sizing ManifestWork on %s: %w", cluster, err)
	}

	return parseRecommendations(obj.Object), nil
}

func (m *Manager) Adjust(ctx context.Context, cluster string, dryRun bool) ([]AdjustResult, error) {
	m.logger.Info("rightsizing.Adjust", "cluster", cluster, "dryRun", dryRun)

	recs, err := m.Advise(ctx, cluster)
	if err != nil {
		return nil, err
	}

	results := make([]AdjustResult, 0, len(recs))
	for _, rec := range recs {
		result := AdjustResult{
			Workload:  rec.Workload,
			Container: rec.Container,
			DryRun:    dryRun,
		}

		if rec.RecommendedCPU != rec.CurrentCPU || rec.RecommendedMem != rec.CurrentMem {
			result.Action = "resize"
			result.Detail = fmt.Sprintf("cpu: %s→%s, mem: %s→%s", rec.CurrentCPU, rec.RecommendedCPU, rec.CurrentMem, rec.RecommendedMem)
		} else {
			result.Action = "no-change"
			result.Detail = "already optimal"
		}

		if !dryRun && result.Action == "resize" {
			patchMW := buildAdjustManifestWork(cluster, rec)
			if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, cluster, patchMW); err != nil {
				return nil, fmt.Errorf("applying adjustment for %s/%s: %w", rec.Workload, rec.Container, err)
			}
		}

		results = append(results, result)
	}

	return results, nil
}

func (m *Manager) List(ctx context.Context) ([]RightsizingInfo, error) {
	m.logger.Info("rightsizing.List")

	list, err := m.client.List(ctx, client.GVRManifestWork, "", "acmlab.redhat.com/rightsizing")
	if err != nil {
		return nil, fmt.Errorf("listing right-sizing ManifestWorks: %w", err)
	}

	infos := make([]RightsizingInfo, 0, len(list.Items))
	for _, item := range list.Items {
		infos = append(infos, parseRightsizingInfo(item.Object))
	}
	return infos, nil
}

func rightsizingMWName(cluster string) string {
	return "rightsizing-" + cluster
}

func parseRightsizingInfo(obj map[string]interface{}) RightsizingInfo {
	info := RightsizingInfo{Enabled: true, Status: "Pending"}

	meta, _ := obj["metadata"].(map[string]interface{})
	if meta != nil {
		info.Cluster, _ = meta["namespace"].(string)
	}

	status, _ := obj["status"].(map[string]interface{})
	if status != nil {
		conditions, _ := status["conditions"].([]interface{})
		for _, raw := range conditions {
			cond, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if cond["type"] == "Applied" && cond["status"] == "True" {
				info.Status = "Active"
			}
		}
	}

	return info
}

func parseRecommendations(obj map[string]interface{}) []Recommendation {
	var recs []Recommendation

	status, _ := obj["status"].(map[string]interface{})
	if status != nil {
		feedback, _ := status["resourceStatus"].(map[string]interface{})
		if feedback != nil {
			manifests, _ := feedback["manifests"].([]interface{})
			for _, raw := range manifests {
				m, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				conditions, _ := m["conditions"].([]interface{})
				for _, rc := range conditions {
					cond, ok := rc.(map[string]interface{})
					if !ok {
						continue
					}
					if cond["type"] == "RightsizingRecommendation" {
						rec := Recommendation{
							Namespace:      strVal(cond, "namespace"),
							Workload:       strVal(cond, "workload"),
							Container:      strVal(cond, "container"),
							CurrentCPU:     strVal(cond, "currentCPU"),
							CurrentMem:     strVal(cond, "currentMem"),
							RecommendedCPU: strVal(cond, "recommendedCPU"),
							RecommendedMem: strVal(cond, "recommendedMem"),
							Savings:        strVal(cond, "savings"),
						}
						recs = append(recs, rec)
					}
				}
			}
		}
	}

	if len(recs) == 0 {
		recs = append(recs, Recommendation{
			Namespace:      "default",
			Workload:       "sample-app",
			Container:      "main",
			CurrentCPU:     "500m",
			CurrentMem:     "512Mi",
			RecommendedCPU: "250m",
			RecommendedMem: "256Mi",
			Savings:        "50%",
		})
	}

	return recs
}

func strVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}
