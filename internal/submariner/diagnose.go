package submariner

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type DiagnoseCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type DiagnoseResult struct {
	ClusterSet string          `json:"clusterSet"`
	Healthy    bool            `json:"healthy"`
	Checks     []DiagnoseCheck `json:"checks"`
}

func (r *DiagnoseResult) addCheck(name, status, message string) {
	r.Checks = append(r.Checks, DiagnoseCheck{Name: name, Status: status, Message: message})
	if status == "fail" {
		r.Healthy = false
	}
}

func (m *Manager) Diagnose(ctx context.Context, clusterSet string) (*DiagnoseResult, error) {
	m.logger.Info("submariner.Diagnose", "clusterSet", clusterSet)

	clusters, err := m.clustersInSet(ctx, clusterSet)
	if err != nil {
		return nil, err
	}

	result := &DiagnoseResult{ClusterSet: clusterSet, Healthy: true}

	if len(clusters) == 0 {
		result.addCheck("cluster-set", "fail",
			fmt.Sprintf("No clusters found in ClusterSet %q.", clusterSet))
		return result, nil
	}

	m.checkClusterSetMembers(clusters, result)
	m.checkDefaultClusterSet(clusterSet, result)

	for _, name := range clusters {
		m.checkClusterAddOn(ctx, name, result)
		m.checkClusterConfig(ctx, name, result)
	}

	m.checkGlobalnetRecommendation(ctx, clusters, result)

	return result, nil
}

func (m *Manager) checkClusterSetMembers(clusters []string, result *DiagnoseResult) {
	var warnings []string
	for _, name := range clusters {
		if name == "local-cluster" {
			warnings = append(warnings, "local-cluster (hub) is in this set — use a dedicated ClusterSet for Submariner pairs")
		}
	}

	if len(clusters) < 2 {
		result.addCheck("cluster-set", "fail",
			fmt.Sprintf("ClusterSet has %d cluster(s); Submariner requires at least 2.", len(clusters)))
		return
	}

	if len(warnings) > 0 {
		for _, w := range warnings {
			result.addCheck("cluster-set", "warn", w)
		}
		return
	}

	result.addCheck("cluster-set", "pass",
		fmt.Sprintf("ClusterSet has %d clusters.", len(clusters)))
}

func (m *Manager) checkClusterAddOn(ctx context.Context, cluster string, result *DiagnoseResult) {
	checkName := fmt.Sprintf("addon/%s", cluster)
	addOn, err := m.client.Get(ctx, client.GVRManagedClusterAddOn, cluster, "submariner")
	if err != nil {
		result.addCheck(checkName, "fail",
			fmt.Sprintf("No ManagedClusterAddOn/submariner on %s. Run 'acmlab submariner enable <cluster-set>'.", cluster))
		return
	}

	conditions := extractConditions(addOn.Object)

	m.checkCondition(conditions, checkName+"/available", cluster,
		"Available", true,
		"Submariner add-on is available",
		"Submariner add-on is not available on %s.", result)

	m.checkCondition(conditions, checkName+"/gateway", cluster,
		"SubmarinerGatewayNodesLabeled", true,
		"Gateway node is labelled and ready",
		"No gateway node is ready/labelled on %s.", result)

	m.checkCondition(conditions, checkName+"/agent", cluster,
		"SubmarinerAgentDegraded", false,
		"Submariner agent is healthy",
		"Submariner agent is degraded on %s.", result)

	m.checkCondition(conditions, checkName+"/route-agent", cluster,
		"RouteAgentConnectionDegraded", false,
		"Route agent connections are healthy",
		"Route agent connection is degraded on %s.", result)

	m.checkCondition(conditions, checkName+"/broker", cluster,
		"SubmarinerBrokerConfigApplied", true,
		"Broker config applied",
		"Broker config not applied on %s.", result)

	m.checkCondition(conditions, checkName+"/manifest", cluster,
		"ManifestApplied", true,
		"Manifests applied",
		"Manifests not applied on %s.", result)

	m.checkConnectionDegraded(conditions, checkName, cluster, result)
}

func (m *Manager) checkConnectionDegraded(conditions []condition, checkName, cluster string, result *DiagnoseResult) {
	name := checkName + "/connections"
	for _, c := range conditions {
		if c.condType == "SubmarinerConnectionDegraded" {
			if c.status == "True" {
				msg := fmt.Sprintf("No gateway connections established on %s.", cluster)
				if c.reason == "ConnectionsNotEstablished" {
					msg = fmt.Sprintf("No gateway connections established on %s. Check CIDR overlap, Globalnet, firewall/NAT-T/IPSec ports, and broker endpoints.", cluster)
				}
				if c.message != "" {
					msg += " Detail: " + c.message
				}
				result.addCheck(name, "fail", msg)
				return
			}
			result.addCheck(name, "pass", fmt.Sprintf("Gateway connections healthy on %s.", cluster))
			return
		}
	}
	result.addCheck(name, "warn",
		fmt.Sprintf("SubmarinerConnectionDegraded condition not found on %s; add-on may still be initializing.", cluster))
}

func (m *Manager) checkCondition(conditions []condition, checkName, cluster, condType string, wantTrue bool, passMsg, failFmt string, result *DiagnoseResult) {
	for _, c := range conditions {
		if c.condType == condType {
			isTrue := c.status == "True"
			if isTrue == wantTrue {
				result.addCheck(checkName, "pass", passMsg)
			} else {
				msg := fmt.Sprintf(failFmt, cluster)
				if c.reason != "" {
					msg += " Reason: " + c.reason
				}
				if c.message != "" {
					msg += " Detail: " + c.message
				}
				result.addCheck(checkName, "fail", msg)
			}
			return
		}
	}
}

func (m *Manager) checkClusterConfig(ctx context.Context, cluster string, result *DiagnoseResult) {
	checkName := fmt.Sprintf("config/%s", cluster)
	cfg, err := m.client.Get(ctx, client.GVRSubmarinerConfig, cluster, "submariner")
	if err != nil {
		result.addCheck(checkName, "warn",
			fmt.Sprintf("No SubmarinerConfig on %s. Submariner may use defaults.", cluster))
		return
	}
	result.addCheck(checkName, "pass",
		fmt.Sprintf("SubmarinerConfig present on %s.", cluster))

	credsName, found, _ := unstructured.NestedString(cfg.Object, "spec", "credentialsSecret", "name")
	credsCheckName := fmt.Sprintf("config/%s/credentials", cluster)
	if !found || credsName == "" {
		result.addCheck(credsCheckName, "fail",
			fmt.Sprintf("SubmarinerConfig on %s has no credentialsSecret. The add-on controller needs cloud credentials to label gateway nodes and configure the broker.", cluster))
		return
	}
	_, err = m.client.Get(ctx, client.GVRSecret, cluster, credsName)
	if err != nil {
		result.addCheck(credsCheckName, "fail",
			fmt.Sprintf("Credentials secret %q not found in namespace %s. The add-on controller cannot configure the broker or label gateway nodes without valid cloud credentials.", credsName, cluster))
		return
	}
	result.addCheck(credsCheckName, "pass",
		fmt.Sprintf("Credentials secret %q exists in %s.", credsName, cluster))
}

func (m *Manager) checkDefaultClusterSet(clusterSet string, result *DiagnoseResult) {
	if clusterSet == "default" {
		result.addCheck("cluster-set/naming", "warn",
			"Using the 'default' ClusterSet. Create a dedicated ClusterSet with 'acmlab submariner create-test-set' to avoid affecting unrelated placements.")
	}
}

func (m *Manager) checkGlobalnetRecommendation(ctx context.Context, clusters []string, result *DiagnoseResult) {
	hasConnectionFail := false
	for _, ch := range result.Checks {
		if strings.Contains(ch.Name, "/connections") && ch.Status == "fail" {
			hasConnectionFail = true
			break
		}
	}
	if !hasConnectionFail {
		return
	}

	hasGlobalnet := false
	for _, cluster := range clusters {
		cfg, err := m.client.Get(ctx, client.GVRSubmarinerConfig, cluster, "submariner")
		if err != nil {
			continue
		}
		globalCIDR, _, _ := unstructured.NestedString(cfg.Object, "spec", "globalCIDR")
		if globalCIDR != "" {
			hasGlobalnet = true
			break
		}
	}
	if !hasGlobalnet {
		result.addCheck("globalnet", "warn",
			"Globalnet is not enabled. If clusters have overlapping Pod/Service CIDRs, re-enable with 'acmlab submariner enable <set> --globalnet' or reprovision clusters with non-overlapping CIDRs.")
	}
}

type condition struct {
	condType string
	status   string
	reason   string
	message  string
}

func conditionIsTrue(conditions []condition, condType string) bool {
	for _, c := range conditions {
		if c.condType == condType {
			return c.status == "True"
		}
	}
	return false
}

func extractConditions(obj map[string]interface{}) []condition {
	status, ok := obj["status"].(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := status["conditions"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]condition, 0, len(raw))
	for _, c := range raw {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		t, _ := cond["type"].(string)
		s, _ := cond["status"].(string)
		r, _ := cond["reason"].(string)
		msg, _ := cond["message"].(string)
		out = append(out, condition{condType: t, status: s, reason: r, message: msg})
	}
	return out
}
