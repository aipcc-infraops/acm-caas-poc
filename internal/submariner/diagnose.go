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
	m.checkBrokerCR(ctx, clusterSet, result)

	for _, name := range clusters {
		m.checkClusterAddOn(ctx, name, result)
		m.checkClusterConfig(ctx, name, result)
	}

	m.checkGlobalnetRecommendation(ctx, clusters, result)
	m.checkIBMCloudUDP(ctx, clusters, result)
	m.checkEndpoints(ctx, clusterSet, result)
	m.checkFirewallPorts(ctx, clusters, result)

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

func (m *Manager) checkBrokerCR(ctx context.Context, clusterSet string, result *DiagnoseResult) {
	brokerNS := clusterSet + "-broker"
	_, err := m.client.Get(ctx, client.GVRSubmarinerBroker, brokerNS, "submariner-broker")
	if err != nil {
		result.addCheck("broker-cr", "fail",
			fmt.Sprintf("Broker CR (submariner-broker) not found in namespace %s. The ACM add-on controller requires this object. Run 'acmlab submariner repair %s' or re-enable Submariner.", brokerNS, clusterSet))
		return
	}
	result.addCheck("broker-cr", "pass",
		fmt.Sprintf("Broker CR exists in %s.", brokerNS))
}

func (m *Manager) checkIBMCloudUDP(ctx context.Context, clusters []string, result *DiagnoseResult) {
	for _, cluster := range clusters {
		cfg, err := m.client.Get(ctx, client.GVRSubmarinerConfig, cluster, "submariner")
		if err != nil {
			continue
		}
		lbEnabled, _, _ := unstructured.NestedBool(cfg.Object, "spec", "loadBalancerEnable")
		if !lbEnabled {
			continue
		}
		cd, err := m.client.Get(ctx, client.GVRClusterDeployment, cluster, cluster)
		if err != nil {
			continue
		}
		_, hasIBM, _ := unstructured.NestedMap(cd.Object, "spec", "platform", "ibmcloud")
		if hasIBM {
			result.addCheck(fmt.Sprintf("config/%s/ibm-udp", cluster), "warn",
				fmt.Sprintf("LoadBalancer is enabled on %s (IBM Cloud). IBM Cloud VPC load balancers do not support UDP — Submariner gateway connections may fail. Consider using --force-udp-encaps without --load-balancer.", cluster))
		}
	}
}

type endpointInfo struct {
	ClusterID         string
	PublicIP          string
	PrivateIP         string
	NATEnabled        bool
	Backend           string
	UDPPort           string
	NATTDiscoveryPort string
}

func parseEndpoint(obj map[string]interface{}) endpointInfo {
	spec, _ := obj["spec"].(map[string]interface{})
	backendCfg, _ := spec["backend_config"].(map[string]interface{})
	clusterID, _ := spec["cluster_id"].(string)
	publicIP, _ := spec["public_ip"].(string)
	privateIP, _ := spec["private_ip"].(string)
	natEnabled, _ := spec["nat_enabled"].(bool)
	backend, _ := spec["backend"].(string)
	udpPort, _ := backendCfg["udp-port"].(string)
	nattPort, _ := backendCfg["natt-discovery-port"].(string)
	return endpointInfo{
		ClusterID:         clusterID,
		PublicIP:          publicIP,
		PrivateIP:         privateIP,
		NATEnabled:        natEnabled,
		Backend:           backend,
		UDPPort:           udpPort,
		NATTDiscoveryPort: nattPort,
	}
}

func (m *Manager) checkEndpoints(ctx context.Context, clusterSet string, result *DiagnoseResult) {
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

	brokerNS := clusterSet + "-broker"
	list, err := m.client.List(ctx, client.GVRSubmarinerEndpoint, brokerNS, "")
	if err != nil {
		result.addCheck("endpoints", "warn",
			fmt.Sprintf("Cannot read Submariner endpoints from %s — broker namespace may not be accessible.", brokerNS))
		return
	}

	if len(list.Items) == 0 {
		result.addCheck("endpoints", "fail",
			fmt.Sprintf("No gateway endpoints found in broker namespace %s. Gateways have not registered — check addon status.", brokerNS))
		return
	}

	for _, ep := range list.Items {
		info := parseEndpoint(ep.Object)
		result.addCheck(fmt.Sprintf("endpoint/%s", info.ClusterID), "warn",
			fmt.Sprintf("Gateway %s: publicIP=%s privateIP=%s NAT=%v backend=%s ports=%s/%s — "+
				"verify inbound UDP %s and %s are open on the gateway node's cloud firewall.",
				info.ClusterID, info.PublicIP, info.PrivateIP, info.NATEnabled, info.Backend,
				info.UDPPort, info.NATTDiscoveryPort, info.UDPPort, info.NATTDiscoveryPort))
	}

	if len(list.Items) < 2 {
		result.addCheck("endpoints/count", "warn",
			fmt.Sprintf("Only %d gateway endpoint(s) registered — need at least 2 for cross-cluster connectivity.", len(list.Items)))
	}
}

func (m *Manager) checkFirewallPorts(ctx context.Context, clusters []string, result *DiagnoseResult) {
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

	for _, cluster := range clusters {
		cd, err := m.client.Get(ctx, client.GVRClusterDeployment, cluster, cluster)
		if err != nil {
			continue
		}
		_, hasIBM, _ := unstructured.NestedMap(cd.Object, "spec", "platform", "ibmcloud")
		if hasIBM {
			result.addCheck(fmt.Sprintf("firewall/%s", cluster), "fail",
				fmt.Sprintf("IBM Cloud VPC security groups on %s must allow inbound: "+
					"UDP 4500 (IPSec NAT-T), UDP 4490 (NAT discovery), UDP 500 (IKE). "+
					"Open ports: ibmcloud is security-group-rule-add <sg-id> inbound --protocol udp --port-min 4500 --port-max 4500 --remote 0.0.0.0/0 "+
					"(repeat for 4490 and 500). Re-enable with --force-udp-encaps if not already set.", cluster))
		}
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
