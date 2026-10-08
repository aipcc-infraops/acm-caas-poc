package submariner

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type RepairAction struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type RepairResult struct {
	ClusterSet string         `json:"clusterSet"`
	Actions    []RepairAction `json:"actions"`
}

func (r *RepairResult) addAction(name, status, message string) {
	r.Actions = append(r.Actions, RepairAction{Name: name, Status: status, Message: message})
}

func (m *Manager) Repair(ctx context.Context, clusterSet string, dryRun bool) (*RepairResult, error) {
	m.logger.Info("submariner.Repair", "clusterSet", clusterSet, "dryRun", dryRun)

	result := &RepairResult{ClusterSet: clusterSet}

	clusters, err := m.clustersInSet(ctx, clusterSet)
	if err != nil {
		return nil, err
	}

	m.repairStuckAddons(ctx, clusters, dryRun, result)
	m.repairBrokerCR(ctx, clusterSet, dryRun, result)
	m.repairDuplicateFinalizers(ctx, clusterSet, dryRun, result)

	return result, nil
}

func (m *Manager) repairStuckAddons(ctx context.Context, clusters []string, dryRun bool, result *RepairResult) {
	for _, name := range clusters {
		addon, err := m.client.Get(ctx, client.GVRManagedClusterAddOn, name, "submariner")
		if err != nil {
			continue
		}
		ts, _, _ := unstructured.NestedString(addon.Object, "metadata", "deletionTimestamp")
		if ts == "" {
			continue
		}

		checkName := fmt.Sprintf("stuck-addon/%s", name)
		if dryRun {
			result.addAction(checkName, "would-fix",
				fmt.Sprintf("ManagedClusterAddOn/submariner on %s has deletionTimestamp %s with stuck finalizers — would remove finalizers", name, ts))
			continue
		}

		patch, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": nil,
			},
		})
		if _, err := m.client.Patch(ctx, client.GVRManagedClusterAddOn, name, "submariner", types.MergePatchType, patch); err != nil {
			result.addAction(checkName, "failed",
				fmt.Sprintf("Could not remove finalizers from ManagedClusterAddOn/submariner on %s: %v", name, err))
			continue
		}
		result.addAction(checkName, "fixed",
			fmt.Sprintf("Removed stuck finalizers from ManagedClusterAddOn/submariner on %s", name))
	}
}

func (m *Manager) repairBrokerCR(ctx context.Context, clusterSet string, dryRun bool, result *RepairResult) {
	brokerNS := clusterSet + "-broker"
	_, err := m.client.Get(ctx, client.GVRSubmarinerBroker, brokerNS, "submariner-broker")
	if err == nil {
		result.addAction("broker-cr", "ok",
			fmt.Sprintf("Broker CR exists in %s", brokerNS))
		return
	}

	if dryRun {
		result.addAction("broker-cr", "would-fix",
			fmt.Sprintf("Broker CR missing in %s — would create submariner-broker with service-discovery and connectivity", brokerNS))
		return
	}

	if err := m.ensureBrokerCR(ctx, clusterSet); err != nil {
		result.addAction("broker-cr", "failed",
			fmt.Sprintf("Could not create Broker CR in %s: %v", brokerNS, err))
		return
	}
	result.addAction("broker-cr", "fixed",
		fmt.Sprintf("Created Broker CR in %s", brokerNS))
}

func (m *Manager) repairDuplicateFinalizers(ctx context.Context, clusterSet string, dryRun bool, result *RepairResult) {
	cs, err := m.client.Get(ctx, client.GVRManagedClusterSet, "", clusterSet)
	if err != nil {
		return
	}

	finalizers, _, _ := unstructured.NestedStringSlice(cs.Object, "metadata", "finalizers")
	seen := map[string]bool{}
	var deduped []string
	hasDupes := false
	for _, f := range finalizers {
		if seen[f] {
			hasDupes = true
			continue
		}
		seen[f] = true
		deduped = append(deduped, f)
	}

	if !hasDupes {
		return
	}

	checkName := "duplicate-finalizers/" + clusterSet
	if dryRun {
		result.addAction(checkName, "would-fix",
			fmt.Sprintf("ManagedClusterSet %s has duplicate finalizers — would deduplicate from %d to %d", clusterSet, len(finalizers), len(deduped)))
		return
	}

	finalizerList := make([]interface{}, len(deduped))
	for i, f := range deduped {
		finalizerList[i] = f
	}
	patch, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"finalizers": finalizerList,
		},
	})
	if _, err := m.client.Patch(ctx, client.GVRManagedClusterSet, "", clusterSet, types.MergePatchType, patch); err != nil {
		result.addAction(checkName, "failed",
			fmt.Sprintf("Could not deduplicate finalizers on ManagedClusterSet %s: %v", clusterSet, err))
		return
	}
	result.addAction(checkName, "fixed",
		fmt.Sprintf("Deduplicated finalizers on ManagedClusterSet %s (%d → %d)", clusterSet, len(finalizers), len(deduped)))
}
