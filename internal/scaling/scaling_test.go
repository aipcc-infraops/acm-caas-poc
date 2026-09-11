package scaling

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGetMachinePoolInfo(t *testing.T) {
	mp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "MachinePool",
			"metadata": map[string]interface{}{
				"name":      "cluster1-worker",
				"namespace": "cluster1",
			},
			"spec": map[string]interface{}{
				"replicas": int64(3),
				"platform": map[string]interface{}{
					"aws": map[string]interface{}{
						"instanceType": "m5.xlarge",
					},
				},
			},
		},
	}

	info := machinePoolInfoFromUnstructured(mp)

	if info.Name != "cluster1-worker" {
		t.Errorf("Name = %q, want %q", info.Name, "cluster1-worker")
	}
	if info.Namespace != "cluster1" {
		t.Errorf("Namespace = %q, want %q", info.Namespace, "cluster1")
	}
	if info.Replicas == nil || *info.Replicas != 3 {
		t.Errorf("Replicas = %v, want 3", info.Replicas)
	}
	if info.Platform != "aws" {
		t.Errorf("Platform = %q, want %q", info.Platform, "aws")
	}
	if info.MinSize != nil || info.MaxSize != nil {
		t.Errorf("MinSize/MaxSize should be nil when no autoscaling set")
	}
}

func TestMachinePoolInfoReplicas(t *testing.T) {
	mp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "MachinePool",
			"metadata": map[string]interface{}{
				"name":      "cluster2-worker",
				"namespace": "cluster2",
			},
			"spec": map[string]interface{}{
				"autoscaling": map[string]interface{}{
					"minReplicas": int64(2),
					"maxReplicas": int64(5),
				},
			},
		},
	}

	info := machinePoolInfoFromUnstructured(mp)

	if info.Replicas != nil {
		t.Errorf("Replicas should be nil when autoscaling is active, got %v", *info.Replicas)
	}
}

func TestMachinePoolInfoAutoscaling(t *testing.T) {
	mp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "MachinePool",
			"metadata": map[string]interface{}{
				"name":      "cluster3-worker",
				"namespace": "cluster3",
			},
			"spec": map[string]interface{}{
				"autoscaling": map[string]interface{}{
					"minReplicas": int64(2),
					"maxReplicas": int64(10),
				},
				"platform": map[string]interface{}{
					"ibmvpc": map[string]interface{}{
						"type": "bx2-4x16",
					},
				},
			},
		},
	}

	info := machinePoolInfoFromUnstructured(mp)

	if info.MinSize == nil || *info.MinSize != 2 {
		t.Errorf("MinSize = %v, want 2", info.MinSize)
	}
	if info.MaxSize == nil || *info.MaxSize != 10 {
		t.Errorf("MaxSize = %v, want 10", info.MaxSize)
	}
	if info.Replicas != nil {
		t.Errorf("Replicas should be nil when autoscaling is set")
	}
	if info.Platform != "ibmvpc" {
		t.Errorf("Platform = %q, want %q", info.Platform, "ibmvpc")
	}
}
