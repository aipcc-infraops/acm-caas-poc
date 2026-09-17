package gpu

import (
	"io"
	"log/slog"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

var gpuGVRKinds = map[schema.GroupVersionResource]string{
	client.GVRManagedCluster:      "ManagedClusterList",
	client.GVRManifestWork:        "ManifestWorkList",
	client.GVRPlacement:           "PlacementList",
	client.GVRPlacementDecision:   "PlacementDecisionList",
	client.GVRPolicy:              "PolicyList",
	client.GVRPlacementBinding:    "PlacementBindingList",
	client.GVRConfigurationPolicy: "ConfigurationPolicyList",
	client.GVRNamespace:           "NamespaceList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gpuGVRKinds, objs...)
	return &client.Client{Dynamic: fc}
}

func newTestManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func managedCluster(name string, labels map[string]string) *unstructured.Unstructured {
	labelMap := make(map[string]interface{})
	for k, v := range labels {
		labelMap[k] = v
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name":   name,
				"labels": labelMap,
			},
		},
	}
}
