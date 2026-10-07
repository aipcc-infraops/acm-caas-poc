package submariner

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

const (
	DefaultServerImage = "registry.redhat.io/ubi9/ubi:latest"
	DefaultClientImage = "registry.redhat.io/ubi9/ubi:latest"

	testServerName = "submariner-test-server"
	testClientName = "submariner-test-client"
	testSvcName    = "submariner-test-svc"
)

type ConnectivityTestOpts struct {
	ClusterA    string
	ClusterB    string
	Namespace   string
	Timeout     time.Duration
	Cleanup     bool
	ServerImage string
	ClientImage string
	KubeconfigA string
	KubeconfigB string
}

type ConnectivityResult struct {
	ClusterA  string        `json:"clusterA"`
	ClusterB  string        `json:"clusterB"`
	Namespace string        `json:"namespace"`
	Phase     string        `json:"phase"`
	Success   bool          `json:"success"`
	Message   string        `json:"message"`
	Details   *DebugDetails `json:"details,omitempty"`
}

type DebugDetails struct {
	ServerPodPhase  string `json:"serverPodPhase,omitempty"`
	ServerPodReason string `json:"serverPodReason,omitempty"`
	ClientPodPhase  string `json:"clientPodPhase,omitempty"`
	ClientPodReason string `json:"clientPodReason,omitempty"`
}

func (m *Manager) TestConnectivity(ctx context.Context, opts ConnectivityTestOpts) (*ConnectivityResult, error) {
	m.logger.Info("submariner.TestConnectivity",
		"clusterA", opts.ClusterA,
		"clusterB", opts.ClusterB,
		"namespace", opts.Namespace)

	result := &ConnectivityResult{
		ClusterA:  opts.ClusterA,
		ClusterB:  opts.ClusterB,
		Namespace: opts.Namespace,
	}

	for _, cluster := range []string{opts.ClusterA, opts.ClusterB} {
		_, err := m.client.Get(ctx, client.GVRManagedCluster, "", cluster)
		if err != nil {
			result.Phase = "PreflightFailed"
			result.Message = fmt.Sprintf("Cluster %q not found in hub.", cluster)
			return result, nil
		}
	}

	for _, cluster := range []string{opts.ClusterA, opts.ClusterB} {
		addOn, err := m.client.Get(ctx, client.GVRManagedClusterAddOn, cluster, "submariner")
		if err != nil {
			result.Phase = "PreflightFailed"
			result.Message = fmt.Sprintf("ManagedClusterAddOn/submariner not found on %s. Enable Submariner first.", cluster)
			return result, nil
		}
		conditions := extractConditions(addOn.Object)
		if len(conditions) > 0 && !conditionIsTrue(conditions, "Available") {
			result.Phase = "PreflightFailed"
			result.Message = fmt.Sprintf("ManagedClusterAddOn/submariner on %s is not Available. Run 'acmlab submariner diagnose' for details.", cluster)
			return result, nil
		}
	}

	if opts.KubeconfigA == "" || opts.KubeconfigB == "" {
		result.Phase = "SpokeAccessRequired"
		result.Message = fmt.Sprintf(
			"Preflight passed: both clusters have Submariner add-on. "+
				"Provide spoke kubeconfigs: "+
				"--kubeconfig-a /path/to/%s.kubeconfig --kubeconfig-b /path/to/%s.kubeconfig",
			opts.ClusterA, opts.ClusterB)
		return result, nil
	}

	return m.runConnectivityTest(ctx, opts, result)
}

func (m *Manager) runConnectivityTest(ctx context.Context, opts ConnectivityTestOpts, result *ConnectivityResult) (*ConnectivityResult, error) {
	clientB, err := client.NewFromContext(opts.KubeconfigB, "")
	if err != nil {
		result.Phase = "HarnessFailed"
		result.Message = fmt.Sprintf("Cannot connect to cluster %s: %v", opts.ClusterB, err)
		return result, nil
	}

	clientA, err := client.NewFromContext(opts.KubeconfigA, "")
	if err != nil {
		result.Phase = "HarnessFailed"
		result.Message = fmt.Sprintf("Cannot connect to cluster %s: %v", opts.ClusterA, err)
		return result, nil
	}

	if err := ensureNamespace(ctx, clientB, buildTestNamespace(opts.Namespace)); err != nil {
		result.Phase = "HarnessFailed"
		result.Message = fmt.Sprintf("Cannot create namespace on %s: %v", opts.ClusterB, err)
		return result, nil
	}
	if err := ensureNamespace(ctx, clientA, buildTestNamespace(opts.Namespace)); err != nil {
		result.Phase = "HarnessFailed"
		result.Message = fmt.Sprintf("Cannot create namespace on %s: %v", opts.ClusterA, err)
		return result, nil
	}

	if opts.Cleanup {
		defer m.cleanupTestResources(clientA, clientB, opts.Namespace)
	}

	svc := buildTestService(opts.Namespace)
	if _, err := clientB.Create(ctx, client.GVRService, opts.Namespace, svc); err != nil && !apierrors.IsAlreadyExists(err) {
		result.Phase = "HarnessFailed"
		result.Message = fmt.Sprintf("Cannot create test service on %s: %v", opts.ClusterB, err)
		return result, nil
	}

	svcExport := buildServiceExport(opts.Namespace)
	if _, err := clientB.Create(ctx, client.GVRServiceExport, opts.Namespace, svcExport); err != nil && !apierrors.IsAlreadyExists(err) {
		if isResourceNotAvailable(err) {
			result.Phase = "PreflightFailed"
			result.Message = fmt.Sprintf("ServiceExport API not available on %s. Submariner may not be fully deployed or the cluster does not support multi-cluster services.", opts.ClusterB)
			return result, nil
		}
		result.Phase = "HarnessFailed"
		result.Message = fmt.Sprintf("Cannot create ServiceExport on %s: %v", opts.ClusterB, err)
		return result, nil
	}

	serverPod := buildTestServerPod(opts.Namespace, opts.ServerImage)
	if err := clientB.CreateIfNotExists(ctx, client.GVRPod, opts.Namespace, serverPod); err != nil {
		result.Phase = "HarnessFailed"
		result.Message = fmt.Sprintf("Cannot create server pod on %s: %v", opts.ClusterB, err)
		return result, nil
	}

	if err := m.waitForServerReady(ctx, clientB, opts, result); err != nil {
		return result, nil
	}

	clientPod := buildTestClientPod(opts.Namespace, opts.ClientImage)
	if err := clientA.CreateIfNotExists(ctx, client.GVRPod, opts.Namespace, clientPod); err != nil {
		result.Phase = "HarnessFailed"
		result.Message = fmt.Sprintf("Cannot create client pod on %s: %v", opts.ClusterA, err)
		return result, nil
	}

	return m.waitForClientResult(ctx, clientA, clientB, opts, result), nil
}

func (m *Manager) waitForServerReady(ctx context.Context, spokeB *client.Client, opts ConnectivityTestOpts, result *ConnectivityResult) error {
	deadline := time.Now().Add(opts.Timeout / 2)
	for time.Now().Before(deadline) {
		p, err := spokeB.Get(ctx, client.GVRPod, opts.Namespace, testServerName)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		phase := podPhase(p.Object)
		switch phase {
		case "Running":
			return nil
		case "Failed":
			result.Phase = "HarnessFailed"
			result.Message = fmt.Sprintf("Server pod failed before connectivity could be tested on %s.", opts.ClusterB)
			result.Details = &DebugDetails{
				ServerPodPhase:  phase,
				ServerPodReason: podStatusReason(p.Object),
			}
			return fmt.Errorf("server pod failed")
		}
		time.Sleep(3 * time.Second)
	}
	result.Phase = "HarnessFailed"
	result.Message = fmt.Sprintf("Server pod did not become ready within %s on %s.", opts.Timeout/2, opts.ClusterB)
	return fmt.Errorf("server pod timeout")
}

func (m *Manager) waitForClientResult(ctx context.Context, spokeA, spokeB *client.Client, opts ConnectivityTestOpts, result *ConnectivityResult) *ConnectivityResult {
	deadline := time.Now().Add(opts.Timeout)
	for time.Now().Before(deadline) {
		p, err := spokeA.Get(ctx, client.GVRPod, opts.Namespace, testClientName)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		phase := podPhase(p.Object)
		reason := podStatusReason(p.Object)
		switch phase {
		case "Succeeded":
			result.Phase = "Passed"
			result.Success = true
			result.Message = fmt.Sprintf("Cross-cluster connectivity verified: %s -> %s", opts.ClusterA, opts.ClusterB)
			return result
		case "Failed":
			if isImagePullFailure(reason) {
				result.Phase = "HarnessFailed"
				result.Message = fmt.Sprintf("Client pod image pull failed on %s: %s", opts.ClusterA, reason)
			} else {
				result.Phase = "ConnectivityFailed"
				result.Message = fmt.Sprintf("Cross-cluster connectivity test failed: %s -> %s", opts.ClusterA, opts.ClusterB)
			}
			result.Details = collectDebugDetails(ctx, spokeA, spokeB, opts.Namespace)
			return result
		}
		time.Sleep(5 * time.Second)
	}
	result.Phase = "Timeout"
	result.Message = fmt.Sprintf("Timed out after %s waiting for connectivity test result.", opts.Timeout)
	result.Details = collectDebugDetails(ctx, spokeA, spokeB, opts.Namespace)
	return result
}

func (m *Manager) cleanupTestResources(spokeA, spokeB *client.Client, namespace string) {
	ctx := context.Background()
	_ = spokeA.DeleteIfExists(ctx, client.GVRPod, namespace, testClientName)
	_ = spokeA.DeleteIfExists(ctx, client.GVRNamespace, "", namespace)
	_ = spokeB.DeleteIfExists(ctx, client.GVRPod, namespace, testServerName)
	_ = spokeB.DeleteIfExists(ctx, client.GVRServiceExport, namespace, testSvcName)
	_ = spokeB.DeleteIfExists(ctx, client.GVRService, namespace, testSvcName)
	_ = spokeB.DeleteIfExists(ctx, client.GVRNamespace, "", namespace)
}

func collectDebugDetails(ctx context.Context, spokeA, spokeB *client.Client, namespace string) *DebugDetails {
	d := &DebugDetails{}
	if p, err := spokeB.Get(ctx, client.GVRPod, namespace, testServerName); err == nil {
		d.ServerPodPhase = podPhase(p.Object)
		d.ServerPodReason = podStatusReason(p.Object)
	}
	if p, err := spokeA.Get(ctx, client.GVRPod, namespace, testClientName); err == nil {
		d.ClientPodPhase = podPhase(p.Object)
		d.ClientPodReason = podStatusReason(p.Object)
	}
	return d
}

func isImagePullFailure(reason string) bool {
	return reason == "ImagePullBackOff" || reason == "ErrImagePull" || reason == "SignatureValidationFailed"
}

func podPhase(obj map[string]interface{}) string {
	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		return ""
	}
	phase, _ := status["phase"].(string)
	return phase
}

func podStatusReason(obj map[string]interface{}) string {
	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		return ""
	}
	containers, _ := status["containerStatuses"].([]interface{})
	for _, c := range containers {
		cs, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		for _, stateKey := range []string{"waiting", "terminated"} {
			state, ok := cs[stateKey].(map[string]interface{})
			if !ok {
				continue
			}
			if reason, ok := state["reason"].(string); ok && reason != "" {
				return reason
			}
		}
	}
	if reason, ok := status["reason"].(string); ok {
		return reason
	}
	return ""
}

func isResourceNotAvailable(err error) bool {
	if apierrors.IsNotFound(err) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "the server could not find the requested resource") ||
		strings.Contains(msg, "no matches for kind")
}

func ensureNamespace(ctx context.Context, c *client.Client, ns *unstructured.Unstructured) error {
	return c.CreateIfNotExists(ctx, client.GVRNamespace, "", ns)
}

// --- Resource builders ---

var restrictedSecurityContext = map[string]interface{}{
	"runAsNonRoot":             true,
	"allowPrivilegeEscalation": false,
	"capabilities": map[string]interface{}{
		"drop": []interface{}{"ALL"},
	},
	"seccompProfile": map[string]interface{}{
		"type": "RuntimeDefault",
	},
}

func buildTestNamespace(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":         "true",
					"acmlab.redhat.com/submariner-test": "true",
				},
			},
		},
	}
}

func buildTestService(namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      testSvcName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app": "submariner-test-server",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"targetPort": int64(8080),
						"protocol":   "TCP",
					},
				},
			},
		},
	}
}

func buildServiceExport(namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "multicluster.x-k8s.io/v1alpha1",
			"kind":       "ServiceExport",
			"metadata": map[string]interface{}{
				"name":      testSvcName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
		},
	}
}

func buildTestServerPod(namespace, image string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      testServerName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app":                       "submariner-test-server",
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "server",
						"image": image,
						"command": []interface{}{
							"python3", "-c",
							"from http.server import HTTPServer, BaseHTTPRequestHandler\n" +
								"class H(BaseHTTPRequestHandler):\n" +
								"    def do_GET(self):\n" +
								"        self.send_response(200)\n" +
								"        self.end_headers()\n" +
								"        self.wfile.write(b'submariner-test-ok')\n" +
								"    def log_message(self, *a): pass\n" +
								"HTTPServer(('',8080),H).serve_forever()\n",
						},
						"ports": []interface{}{
							map[string]interface{}{
								"containerPort": int64(8080),
							},
						},
						"securityContext": restrictedSecurityContext,
					},
				},
				"restartPolicy": "Never",
			},
		},
	}
}

func buildTestClientPod(namespace, image string) *unstructured.Unstructured {
	svcHost := fmt.Sprintf("%s.%s.svc.clusterset.local", testSvcName, namespace)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      testClientName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "client",
						"image": image,
						"command": []interface{}{
							"sh", "-c",
							fmt.Sprintf(
								"for i in $(seq 1 30); do "+
									"if curl -sf --connect-timeout 5 http://%s:80 | grep -q submariner-test-ok; then exit 0; fi; "+
									"sleep 2; done; exit 1",
								svcHost),
						},
						"securityContext": restrictedSecurityContext,
					},
				},
				"restartPolicy": "Never",
			},
		},
	}
}
