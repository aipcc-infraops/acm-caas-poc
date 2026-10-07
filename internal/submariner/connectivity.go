package submariner

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type ConnectivityTestOpts struct {
	ClusterA    string
	ClusterB    string
	Namespace   string
	Timeout     time.Duration
	Cleanup     bool
	Image       string
	KubeconfigA string
	KubeconfigB string
}

type ConnectivityResult struct {
	ClusterA  string `json:"clusterA"`
	ClusterB  string `json:"clusterB"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
	Success   bool   `json:"success"`
	Message   string `json:"message"`
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
		_, err := m.client.Get(ctx, client.GVRManagedClusterAddOn, cluster, "submariner")
		if err != nil {
			result.Phase = "PreflightFailed"
			result.Message = fmt.Sprintf("ManagedClusterAddOn/submariner not found on %s. Enable Submariner first.", cluster)
			return result, nil
		}
	}

	if opts.KubeconfigA == "" || opts.KubeconfigB == "" {
		result.Phase = "SpokeAccessRequired"
		result.Message = fmt.Sprintf(
			"Preflight passed: both clusters have Submariner add-on. "+
				"To run cross-cluster connectivity test, provide spoke kubeconfigs: "+
				"--kubeconfig-a /path/to/%s.kubeconfig --kubeconfig-b /path/to/%s.kubeconfig",
			opts.ClusterA, opts.ClusterB)
		return result, nil
	}

	return m.runConnectivityTest(ctx, opts, result)
}

func (m *Manager) runConnectivityTest(ctx context.Context, opts ConnectivityTestOpts, result *ConnectivityResult) (*ConnectivityResult, error) {
	clientB, err := client.NewFromContext(opts.KubeconfigB, "")
	if err != nil {
		result.Phase = "Failed"
		result.Message = fmt.Sprintf("Cannot connect to cluster %s: %v", opts.ClusterB, err)
		return result, nil
	}

	result.Phase = "CreatingResources"

	ns := buildTestNamespace(opts.Namespace)
	if err := ensureNamespace(ctx, clientB, ns); err != nil {
		result.Phase = "Failed"
		result.Message = fmt.Sprintf("Cannot create namespace on %s: %v", opts.ClusterB, err)
		return result, nil
	}

	svc := buildTestService(opts.Namespace)
	pod := buildTestServerPod(opts.Namespace, opts.Image)

	if _, err := clientB.Create(ctx, client.GVRService, opts.Namespace, svc); err != nil && !apierrors.IsAlreadyExists(err) {
		result.Phase = "Failed"
		result.Message = fmt.Sprintf("Cannot create test service on %s: %v", opts.ClusterB, err)
		return result, nil
	}

	if err := clientB.CreateIfNotExists(ctx, client.GVRPod, opts.Namespace, pod); err != nil {
		result.Phase = "Failed"
		result.Message = fmt.Sprintf("Cannot create test pod on %s: %v", opts.ClusterB, err)
		return result, nil
	}

	clientA, err := client.NewFromContext(opts.KubeconfigA, "")
	if err != nil {
		result.Phase = "Failed"
		result.Message = fmt.Sprintf("Cannot connect to cluster %s: %v", opts.ClusterA, err)
		return result, nil
	}

	nsA := buildTestNamespace(opts.Namespace)
	if err := ensureNamespace(ctx, clientA, nsA); err != nil {
		result.Phase = "Failed"
		result.Message = fmt.Sprintf("Cannot create namespace on %s: %v", opts.ClusterA, err)
		return result, nil
	}

	clientPod := buildTestClientPod(opts.Namespace, opts.Image, opts.ClusterB)
	if err := clientA.CreateIfNotExists(ctx, client.GVRPod, opts.Namespace, clientPod); err != nil {
		result.Phase = "Failed"
		result.Message = fmt.Sprintf("Cannot create client pod on %s: %v", opts.ClusterA, err)
		return result, nil
	}

	result.Phase = "WaitingForResult"
	result.Message = "Test resources created. Poll client pod status for result."

	if opts.Cleanup {
		defer func() {
			cleanupCtx := context.Background()
			_ = clientA.DeleteIfExists(cleanupCtx, client.GVRPod, opts.Namespace, "submariner-test-client")
			_ = clientA.DeleteIfExists(cleanupCtx, client.GVRNamespace, "", opts.Namespace)
			_ = clientB.DeleteIfExists(cleanupCtx, client.GVRPod, opts.Namespace, "submariner-test-server")
			_ = clientB.DeleteIfExists(cleanupCtx, client.GVRService, opts.Namespace, "submariner-test-svc")
			_ = clientB.DeleteIfExists(cleanupCtx, client.GVRNamespace, "", opts.Namespace)
		}()
	}

	deadline := time.Now().Add(opts.Timeout)
	for time.Now().Before(deadline) {
		p, err := clientA.Get(ctx, client.GVRPod, opts.Namespace, "submariner-test-client")
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		phase := podPhase(p.Object)
		if phase == "Succeeded" {
			result.Phase = "Passed"
			result.Success = true
			result.Message = fmt.Sprintf("Cross-cluster connectivity verified: %s -> %s", opts.ClusterA, opts.ClusterB)
			return result, nil
		}
		if phase == "Failed" {
			result.Phase = "Failed"
			result.Message = fmt.Sprintf("Client pod failed. Cross-cluster connectivity not working: %s -> %s", opts.ClusterA, opts.ClusterB)
			return result, nil
		}
		time.Sleep(5 * time.Second)
	}

	result.Phase = "Timeout"
	result.Message = fmt.Sprintf("Timed out waiting for connectivity test result after %s", opts.Timeout)
	return result, nil
}

func ensureNamespace(ctx context.Context, c *client.Client, ns *unstructured.Unstructured) error {
	return c.CreateIfNotExists(ctx, client.GVRNamespace, "", ns)
}

func podPhase(obj map[string]interface{}) string {
	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		return ""
	}
	phase, _ := status["phase"].(string)
	return phase
}

func buildTestNamespace(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":        "true",
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
				"name":      "submariner-test-svc",
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

func buildTestServerPod(namespace, image string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "submariner-test-server",
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
							"sh", "-c", "echo 'submariner-test-ok' | nc -l -p 8080",
						},
						"ports": []interface{}{
							map[string]interface{}{
								"containerPort": int64(8080),
							},
						},
					},
				},
				"restartPolicy": "Never",
			},
		},
	}
}

func buildTestClientPod(namespace, image, targetCluster string) *unstructured.Unstructured {
	svcHost := fmt.Sprintf("submariner-test-svc.%s.svc.clusterset.local", namespace)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "submariner-test-client",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
				"annotations": map[string]interface{}{
					"acmlab.redhat.com/target-cluster": targetCluster,
				},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "client",
						"image": image,
						"command": []interface{}{
							"sh", "-c", fmt.Sprintf("for i in $(seq 1 30); do if curl -s --connect-timeout 5 http://%s:80 | grep -q submariner-test-ok; then exit 0; fi; sleep 2; done; exit 1", svcHost),
						},
					},
				},
				"restartPolicy": "Never",
			},
		},
	}
}
