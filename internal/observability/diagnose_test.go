package observability

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

func diagnoseFakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRNamespace:                 "NamespaceList",
			client.GVRPersistentVolumeClaim:     "PersistentVolumeClaimList",
			client.GVRDeployment:                "DeploymentList",
			client.GVRService:                   "ServiceList",
			client.GVRSecret:                    "SecretList",
			client.GVRMultiClusterObservability: "MultiClusterObservabilityList",
			client.GVRConfigMap:                 "ConfigMapList",
			client.GVRManagedClusterAddOn:       "ManagedClusterAddOnList",
			client.GVRObjectBucketClaim:         "ObjectBucketClaimList",
			client.GVRManagedCluster:            "ManagedClusterList",
			client.GVRRoute:                     "RouteList",
			client.GVRStatefulSet:               "StatefulSetList",
			client.GVRMultiClusterHub:           "MultiClusterHubList",
			client.GVRMultiClusterEngine:        "MultiClusterEngineList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func TestDiagnoseHealthyStack(t *testing.T) {
	mco := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "observability.open-cluster-management.io/v1beta2",
		"kind":       "MultiClusterObservability",
		"metadata":   map[string]interface{}{"name": MCOName},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
		},
	}}
	pullSecret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{"name": PullSecretName, "namespace": Namespace},
	}}
	mch := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "operator.open-cluster-management.io/v1",
		"kind":       "MultiClusterHub",
		"metadata":   map[string]interface{}{"name": "multiclusterhub", "namespace": "open-cluster-management"},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Complete", "status": "True"},
			},
		},
	}}
	mce := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "multicluster.openshift.io/v1",
		"kind":       "MultiClusterEngine",
		"metadata":   map[string]interface{}{"name": "multiclusterengine"},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Available", "status": "True"},
			},
		},
	}}
	cluster := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cluster.open-cluster-management.io/v1",
		"kind":       "ManagedCluster",
		"metadata":   map[string]interface{}{"name": "spoke1"},
	}}
	addon := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "addon.open-cluster-management.io/v1alpha1",
		"kind":       "ManagedClusterAddOn",
		"metadata":   map[string]interface{}{"name": "observability-controller", "namespace": "spoke1"},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Available", "status": "True"},
			},
		},
	}}

	c := diagnoseFakeClient(mco, pullSecret, mch, mce, cluster, addon)
	mgr := New(c, config.Config{}, discardLogger)
	result, err := mgr.Diagnose(context.Background())
	if err != nil {
		t.Fatalf("Diagnose failed: %v", err)
	}
	if !result.Healthy {
		for _, check := range result.Checks {
			if check.Status != "pass" {
				t.Errorf("check %s: %s — %s", check.Name, check.Status, check.Message)
			}
		}
	}
}

func TestDiagnoseMissingPullSecret(t *testing.T) {
	mco := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "observability.open-cluster-management.io/v1beta2",
		"kind":       "MultiClusterObservability",
		"metadata":   map[string]interface{}{"name": MCOName},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
		},
	}}
	mch := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "operator.open-cluster-management.io/v1",
		"kind":       "MultiClusterHub",
		"metadata":   map[string]interface{}{"name": "multiclusterhub", "namespace": "open-cluster-management"},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Complete", "status": "True"},
			},
		},
	}}
	mce := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "multicluster.openshift.io/v1",
		"kind":       "MultiClusterEngine",
		"metadata":   map[string]interface{}{"name": "multiclusterengine"},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Available", "status": "True"},
			},
		},
	}}

	c := diagnoseFakeClient(mco, mch, mce)
	mgr := New(c, config.Config{}, discardLogger)
	result, err := mgr.Diagnose(context.Background())
	if err != nil {
		t.Fatalf("Diagnose failed: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy result when pull secret is missing")
	}
	found := false
	for _, check := range result.Checks {
		if check.Name == "pull-secret" && check.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected pull-secret check to fail")
	}
}

func TestDiagnoseDisabledCluster(t *testing.T) {
	mco := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "observability.open-cluster-management.io/v1beta2",
		"kind":       "MultiClusterObservability",
		"metadata":   map[string]interface{}{"name": MCOName},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
		},
	}}
	pullSecret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{"name": PullSecretName, "namespace": Namespace},
	}}
	mch := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "operator.open-cluster-management.io/v1",
		"kind":       "MultiClusterHub",
		"metadata":   map[string]interface{}{"name": "multiclusterhub", "namespace": "open-cluster-management"},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Complete", "status": "True"},
			},
		},
	}}
	mce := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "multicluster.openshift.io/v1",
		"kind":       "MultiClusterEngine",
		"metadata":   map[string]interface{}{"name": "multiclusterengine"},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Available", "status": "True"},
			},
		},
	}}
	cluster := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cluster.open-cluster-management.io/v1",
		"kind":       "ManagedCluster",
		"metadata": map[string]interface{}{
			"name":   "spoke1",
			"labels": map[string]interface{}{"observability": "disabled"},
		},
	}}

	c := diagnoseFakeClient(mco, pullSecret, mch, mce, cluster)
	mgr := New(c, config.Config{}, discardLogger)
	result, err := mgr.Diagnose(context.Background())
	if err != nil {
		t.Fatalf("Diagnose failed: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy result when cluster is disabled")
	}
	found := false
	for _, check := range result.Checks {
		if check.Name == "disabled-clusters" && check.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected disabled-clusters check to fail")
	}
}

func TestDiagnoseMCHNotComplete(t *testing.T) {
	mco := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "observability.open-cluster-management.io/v1beta2",
		"kind":       "MultiClusterObservability",
		"metadata":   map[string]interface{}{"name": MCOName},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
		},
	}}
	pullSecret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{"name": PullSecretName, "namespace": Namespace},
	}}
	mch := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "operator.open-cluster-management.io/v1",
		"kind":       "MultiClusterHub",
		"metadata":   map[string]interface{}{"name": "multiclusterhub", "namespace": "open-cluster-management"},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Complete", "status": "False", "message": "Not all hub components ready."},
			},
		},
	}}
	mce := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "multicluster.openshift.io/v1",
		"kind":       "MultiClusterEngine",
		"metadata":   map[string]interface{}{"name": "multiclusterengine"},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Available", "status": "True"},
			},
		},
	}}

	c := diagnoseFakeClient(mco, pullSecret, mch, mce)
	mgr := New(c, config.Config{}, discardLogger)
	result, err := mgr.Diagnose(context.Background())
	if err != nil {
		t.Fatalf("Diagnose failed: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy when MCH is not complete")
	}
	found := false
	for _, check := range result.Checks {
		if check.Name == "mch-complete" && check.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected mch-complete check to fail")
	}
}

func TestRepairCreatesPullSecretAndEnablesCluster(t *testing.T) {
	srcSecret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{"name": "pull-secret", "namespace": PullSecretSourceNS},
		"type":     "kubernetes.io/dockerconfigjson",
		"data":     map[string]interface{}{".dockerconfigjson": "eyJ0ZXN0IjogdHJ1ZX0="},
	}}
	cluster := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cluster.open-cluster-management.io/v1",
		"kind":       "ManagedCluster",
		"metadata": map[string]interface{}{
			"name":   "spoke1",
			"labels": map[string]interface{}{"observability": "disabled", "vendor": "OpenShift"},
		},
	}}

	c := diagnoseFakeClient(srcSecret, cluster)
	mgr := New(c, config.Config{}, discardLogger)
	actions, err := mgr.Repair(context.Background())
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d: %v", len(actions), actions)
	}

	ctx := context.Background()
	if _, err := c.Get(ctx, client.GVRSecret, Namespace, PullSecretName); err != nil {
		t.Error("pull secret not created")
	}
	updated, err := c.Get(ctx, client.GVRManagedCluster, "", "spoke1")
	if err != nil {
		t.Fatalf("getting cluster: %v", err)
	}
	labels := updated.GetLabels()
	if labels["observability"] == "disabled" {
		t.Error("cluster still has observability=disabled")
	}
	if labels["vendor"] != "OpenShift" {
		t.Error("vendor label was lost")
	}
}

func TestRepairNoActionWhenHealthy(t *testing.T) {
	pullSecret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{"name": PullSecretName, "namespace": Namespace},
	}}
	cluster := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cluster.open-cluster-management.io/v1",
		"kind":       "ManagedCluster",
		"metadata":   map[string]interface{}{"name": "spoke1"},
	}}

	c := diagnoseFakeClient(pullSecret, cluster)
	mgr := New(c, config.Config{}, discardLogger)
	actions, err := mgr.Repair(context.Background())
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d: %v", len(actions), actions)
	}
}
