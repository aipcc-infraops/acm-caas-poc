package decommission

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type Phase string

const (
	PhaseImported Phase = "imported"
	PhaseAudited  Phase = "audited"
	PhaseNotified Phase = "notified"
	PhaseBackedUp Phase = "backed-up"
	PhaseDrained  Phase = "drained"
	PhaseDeleted  Phase = "deleted"
	PhaseCleaned  Phase = "cleaned"
)

var phaseOrder = []Phase{
	PhaseImported, PhaseAudited, PhaseNotified,
	PhaseBackedUp, PhaseDrained, PhaseDeleted, PhaseCleaned,
}

func nextPhase(current Phase) Phase {
	for i, p := range phaseOrder {
		if p == current && i < len(phaseOrder)-1 {
			return phaseOrder[i+1]
		}
	}
	return current
}

type DecommissionState struct {
	ClusterName    string         `json:"clusterName"`
	Phase          Phase          `json:"phase"`
	Owner          string         `json:"owner,omitempty"`
	Deadline       string         `json:"deadline,omitempty"`
	NotifiedAt     string         `json:"notifiedAt,omitempty"`
	BackupPath     string         `json:"backupPath,omitempty"`
	KubeconfigPath string         `json:"kubeconfigPath,omitempty"`
	Audit          *AuditReport   `json:"audit,omitempty"`
	History        []HistoryEntry `json:"history"`
}

type HistoryEntry struct {
	Phase     Phase  `json:"phase"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

type AuditReport struct {
	NodeCount      int      `json:"nodeCount"`
	CPUCapacity    string   `json:"cpuCapacity"`
	MemoryCapacity string   `json:"memoryCapacity"`
	Namespaces     []string `json:"namespaces"`
	Owner          string   `json:"owner"`
	Platform       string   `json:"platform"`
	ClusterAge     string   `json:"clusterAge"`
}

type StartOpts struct {
	Owner          string
	Deadline       string
	KubeconfigPath string
}

func (s *DecommissionState) addHistory(phase Phase, message string) {
	s.History = append(s.History, HistoryEntry{
		Phase:     phase,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Message:   message,
	})
}

func configMapName(clusterName string) string {
	return clusterName + "-decommission"
}

func stateToConfigMap(s *DecommissionState) *unstructured.Unstructured {
	data := map[string]interface{}{
		"phase": string(s.Phase),
	}
	if s.Owner != "" {
		data["owner"] = s.Owner
	}
	if s.Deadline != "" {
		data["deadline"] = s.Deadline
	}
	if s.NotifiedAt != "" {
		data["notifiedAt"] = s.NotifiedAt
	}
	if s.BackupPath != "" {
		data["backupPath"] = s.BackupPath
	}
	if s.KubeconfigPath != "" {
		data["kubeconfigPath"] = s.KubeconfigPath
	}
	if s.Audit != nil {
		auditJSON, _ := json.Marshal(s.Audit)
		data["audit"] = string(auditJSON)
	}
	if len(s.History) > 0 {
		histJSON, _ := json.Marshal(s.History)
		data["history"] = string(histJSON)
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      configMapName(s.ClusterName),
				"namespace": s.ClusterName,
				"labels": map[string]interface{}{
					"caas-poc/workflow": "decommission",
					"caas-poc/cluster":  s.ClusterName,
				},
			},
			"data": data,
		},
	}
}

func stateFromConfigMap(obj *unstructured.Unstructured) (*DecommissionState, error) {
	data, ok, _ := unstructured.NestedStringMap(obj.Object, "data")
	if !ok {
		return nil, fmt.Errorf("ConfigMap has no data field")
	}

	labels, _, _ := unstructured.NestedStringMap(obj.Object, "metadata", "labels")
	clusterName := labels["caas-poc/cluster"]

	s := &DecommissionState{
		ClusterName:    clusterName,
		Phase:          Phase(data["phase"]),
		Owner:          data["owner"],
		Deadline:       data["deadline"],
		NotifiedAt:     data["notifiedAt"],
		BackupPath:     data["backupPath"],
		KubeconfigPath: data["kubeconfigPath"],
	}

	if auditStr, ok := data["audit"]; ok && auditStr != "" {
		var audit AuditReport
		if err := json.Unmarshal([]byte(auditStr), &audit); err != nil {
			return nil, fmt.Errorf("parsing audit: %w", err)
		}
		s.Audit = &audit
	}

	if histStr, ok := data["history"]; ok && histStr != "" {
		if err := json.Unmarshal([]byte(histStr), &s.History); err != nil {
			return nil, fmt.Errorf("parsing history: %w", err)
		}
	}

	return s, nil
}

func getState(ctx context.Context, c *client.Client, clusterName string) (*DecommissionState, error) {
	obj, err := c.Get(ctx, client.GVRConfigMap, clusterName, configMapName(clusterName))
	if err != nil {
		return nil, fmt.Errorf("decommission state not found for %s: %w", clusterName, err)
	}
	return stateFromConfigMap(obj)
}

func setState(ctx context.Context, c *client.Client, state *DecommissionState) error {
	cm := stateToConfigMap(state)
	_, err := c.Update(ctx, client.GVRConfigMap, state.ClusterName, cm)
	return err
}

func createState(ctx context.Context, c *client.Client, clusterName string, opts StartOpts) (*DecommissionState, error) {
	state := &DecommissionState{
		ClusterName:    clusterName,
		Phase:          PhaseImported,
		Owner:          opts.Owner,
		Deadline:       opts.Deadline,
		KubeconfigPath: opts.KubeconfigPath,
	}
	state.addHistory(PhaseImported, "Decommission workflow started")

	cm := stateToConfigMap(state)
	_, err := c.Create(ctx, client.GVRConfigMap, clusterName, cm)
	if err != nil {
		return nil, fmt.Errorf("creating decommission state: %w", err)
	}
	return state, nil
}

func deleteState(ctx context.Context, c *client.Client, clusterName string) error {
	return c.Delete(ctx, client.GVRConfigMap, clusterName, configMapName(clusterName))
}

func listStates(ctx context.Context, c *client.Client) ([]DecommissionState, error) {
	list, err := c.List(ctx, client.GVRConfigMap, "", "caas-poc/workflow=decommission")
	if err != nil {
		return nil, err
	}
	var states []DecommissionState
	for _, item := range list.Items {
		s, err := stateFromConfigMap(&item)
		if err != nil {
			continue
		}
		states = append(states, *s)
	}
	return states, nil
}
