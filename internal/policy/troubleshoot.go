package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type ViolationDetail struct {
	Cluster   string `json:"cluster"`
	State     string `json:"state"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp,omitempty"`
}

type PolicyViolations struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Compliant  string            `json:"compliant"`
	Violations []ViolationDetail `json:"violations"`
}

type PolicyEvent struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type TroubleshootReport struct {
	Policy     string            `json:"policy"`
	Namespace  string            `json:"namespace"`
	Compliant  string            `json:"compliant"`
	Violations []ViolationDetail `json:"violations"`
	Events     []PolicyEvent     `json:"events"`
}

func (m *Manager) GetViolations(ctx context.Context, name, namespace string) (*PolicyViolations, error) {
	m.logger.Info("policy.GetViolations", "policy", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	obj, err := m.client.Get(ctx, client.GVRPolicy, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting policy %s: %w", name, err)
	}

	info := parsePolicyInfo(obj.Object)
	result := &PolicyViolations{
		Name:      info.Name,
		Namespace: info.Namespace,
		Compliant: info.Compliant,
	}

	status, _ := obj.Object["status"].(map[string]interface{})
	if status == nil {
		return result, nil
	}

	statusList, _ := status["status"].([]interface{})
	for _, raw := range statusList {
		cs, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		clusterName, _ := cs["clustername"].(string)
		compliant, _ := cs["compliant"].(string)

		if compliant != "NonCompliant" {
			continue
		}

		message := ""
		if conditions, ok := cs["clusterconditions"].([]interface{}); ok {
			for _, rawCond := range conditions {
				cond, ok := rawCond.(map[string]interface{})
				if !ok {
					continue
				}
				msg, _ := cond["message"].(string)
				if msg != "" {
					message = msg
				}
			}
		}

		result.Violations = append(result.Violations, ViolationDetail{
			Cluster: clusterName,
			State:   compliant,
			Message: message,
		})
	}

	return result, nil
}

func (m *Manager) Troubleshoot(ctx context.Context, name, namespace string) (*TroubleshootReport, error) {
	m.logger.Info("policy.Troubleshoot", "policy", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	violations, err := m.GetViolations(ctx, name, namespace)
	if err != nil {
		return nil, err
	}

	report := &TroubleshootReport{
		Policy:     violations.Name,
		Namespace:  violations.Namespace,
		Compliant:  violations.Compliant,
		Violations: violations.Violations,
	}

	events, err := m.getPolicyEvents(ctx, namespace)
	if err != nil {
		return report, nil
	}
	report.Events = events

	return report, nil
}

func (m *Manager) getPolicyEvents(ctx context.Context, namespace string) ([]PolicyEvent, error) {
	list, err := m.client.List(ctx, client.GVREvent, namespace, "")
	if err != nil {
		return nil, fmt.Errorf("listing events in %s: %w", namespace, err)
	}

	var events []PolicyEvent
	for _, item := range list.Items {
		obj := item.Object
		eventType, _ := obj["type"].(string)
		reason, _ := obj["reason"].(string)
		message, _ := obj["message"].(string)

		ts := ""
		if lastTS, ok := obj["lastTimestamp"].(string); ok && lastTS != "" {
			ts = lastTS
		} else if eventTime, ok := obj["eventTime"].(string); ok {
			ts = eventTime
		}

		events = append(events, PolicyEvent{
			Type:      eventType,
			Reason:    reason,
			Message:   message,
			Timestamp: ts,
		})
	}

	cutoff := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	var recent []PolicyEvent
	for _, e := range events {
		if e.Timestamp >= cutoff || e.Timestamp == "" {
			recent = append(recent, e)
		}
	}
	if len(recent) > 0 {
		return recent, nil
	}

	return events, nil
}
