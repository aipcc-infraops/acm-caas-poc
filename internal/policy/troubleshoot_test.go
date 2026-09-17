package policy

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

func fakeTroubleshootClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRPolicy: "PolicyList",
			client.GVREvent:  "EventList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func nonCompliantPolicy(name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.Object["status"] = map[string]interface{}{
		"compliant": "NonCompliant",
		"status": []interface{}{
			map[string]interface{}{
				"clustername": "spoke1",
				"compliant":   "NonCompliant",
				"clusterconditions": []interface{}{
					map[string]interface{}{
						"type":    "policy.open-cluster-management.io/policy-status",
						"status":  "True",
						"message": "Violation detected: container using latest tag",
					},
				},
			},
			map[string]interface{}{
				"clustername": "spoke2",
				"compliant":   "Compliant",
			},
		},
	}
	return obj
}

func policyEvent(namespace, reason, message string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "Event",
	})
	obj.SetName("event-1")
	obj.SetNamespace(namespace)
	obj.Object["type"] = "Warning"
	obj.Object["reason"] = reason
	obj.Object["message"] = message
	obj.Object["lastTimestamp"] = "2026-09-17T10:00:00Z"
	return obj
}

func TestGetViolationsReturnsNonCompliant(t *testing.T) {
	p := nonCompliantPolicy("test-policy", DefaultNamespace)
	c := fakeTroubleshootClient(p)
	mgr := New(c, config.Config{}, testLogger)

	violations, err := mgr.GetViolations(context.Background(), "test-policy", "")
	if err != nil {
		t.Fatalf("GetViolations failed: %v", err)
	}
	if violations.Compliant != "NonCompliant" {
		t.Errorf("Compliant = %q, want %q", violations.Compliant, "NonCompliant")
	}
	if len(violations.Violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations.Violations))
	}
	if violations.Violations[0].Cluster != "spoke1" {
		t.Errorf("Cluster = %q, want %q", violations.Violations[0].Cluster, "spoke1")
	}
	if violations.Violations[0].Message == "" {
		t.Error("expected violation message")
	}
}

func TestGetViolationsReturnsErrorForMissing(t *testing.T) {
	c := fakeTroubleshootClient()
	mgr := New(c, config.Config{}, testLogger)

	_, err := mgr.GetViolations(context.Background(), "nonexistent", "")
	if err == nil {
		t.Error("expected error for missing policy")
	}
}

func TestTroubleshootCombinesViolationsAndEvents(t *testing.T) {
	p := nonCompliantPolicy("test-policy", DefaultNamespace)
	e := policyEvent(DefaultNamespace, "PolicyPropagation", "policy propagated to spoke1")
	c := fakeTroubleshootClient(p, e)
	mgr := New(c, config.Config{}, testLogger)

	report, err := mgr.Troubleshoot(context.Background(), "test-policy", "")
	if err != nil {
		t.Fatalf("Troubleshoot failed: %v", err)
	}
	if len(report.Violations) != 1 {
		t.Errorf("got %d violations, want 1", len(report.Violations))
	}
	if len(report.Events) != 1 {
		t.Errorf("got %d events, want 1", len(report.Events))
	}
	if report.Events[0].Reason != "PolicyPropagation" {
		t.Errorf("Reason = %q, want %q", report.Events[0].Reason, "PolicyPropagation")
	}
}
