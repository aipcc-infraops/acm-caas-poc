package mcp

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func seedAutomation(c *client.Client) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "policy.open-cluster-management.io/v1beta1",
		"kind":       "PolicyAutomation",
		"metadata": map[string]interface{}{
			"name":      "auto-1",
			"namespace": "open-cluster-management-policies",
			"labels":    map[string]interface{}{"acmlab.redhat.com/automation": "true"},
		},
		"spec": map[string]interface{}{
			"policyRef": "image-policy",
			"mode":      "scan",
			"automationDef": map[string]interface{}{
				"name":   "remediate-template",
				"secret": "tower-creds",
				"type":   "AnsibleJob",
			},
		},
	}}
	c.Dynamic.Resource(client.GVRPolicyAutomation).Namespace("open-cluster-management-policies").Create(context.Background(), obj, metav1.CreateOptions{})
}

func seedAppSet(c *client.Client) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "ApplicationSet",
		"metadata": map[string]interface{}{
			"name":      "app-1",
			"namespace": "openshift-gitops",
			"labels":    map[string]interface{}{"acmlab.redhat.com/gitops": "true"},
		},
		"spec": map[string]interface{}{
			"generators": []interface{}{},
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"source": map[string]interface{}{
						"repoURL":        "https://github.com/example/repo",
						"path":           "manifests",
						"targetRevision": "main",
					},
				},
			},
		},
	}}
	c.Dynamic.Resource(client.GVRApplicationSet).Namespace("openshift-gitops").Create(context.Background(), obj, metav1.CreateOptions{})
}

func seedBackupSchedule(c *client.Client) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cluster.open-cluster-management.io/v1beta1",
		"kind":       "BackupSchedule",
		"metadata": map[string]interface{}{
			"name":      "acm-backup-schedule",
			"namespace": "open-cluster-management-backup",
			"labels":    map[string]interface{}{"acmlab.redhat.com/backup": "true"},
		},
		"spec": map[string]interface{}{
			"veleroSchedule": "0 */6 * * *",
			"veleroTtl":      "720h",
		},
	}}
	c.Dynamic.Resource(client.GVRBackupSchedule).Namespace("open-cluster-management-backup").Create(context.Background(), obj, metav1.CreateOptions{})
}

func TestAutomationCreateViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_create_automation", map[string]interface{}{
		"name":         "auto-remediate",
		"policy":       "image-policy",
		"tower_secret": "tower-creds",
		"job_template": "remediate-template",
	}))
	if !strings.Contains(text, "auto-remediate") {
		t.Errorf("expected automation name in result, got: %s", text)
	}
}

func TestAutomationGetViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	seedAutomation(c)
	text := extractToolText(t, callTool(t, c, "acm_get_automation", map[string]interface{}{
		"name": "auto-1",
	}))
	if !strings.Contains(text, "auto-1") {
		t.Errorf("expected automation info, got: %s", text)
	}
}

func TestAutomationGetNotFoundViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text, isErr := extractToolResult(t, callTool(t, c, "acm_get_automation", map[string]interface{}{
		"name": "nonexistent",
	}))
	if !isErr {
		t.Errorf("expected error for nonexistent automation, got: %s", text)
	}
}

func TestAutomationListViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	seedAutomation(c)
	text := extractToolText(t, callTool(t, c, "acm_list_automations", nil))
	if !strings.Contains(text, "auto-1") {
		t.Errorf("expected auto-1 in list, got: %s", text)
	}
}

func TestAutomationDeleteViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	seedAutomation(c)
	text := extractToolText(t, callTool(t, c, "acm_delete_automation", map[string]interface{}{
		"name": "auto-1",
	}))
	if !strings.Contains(text, "deleted") {
		t.Errorf("expected deleted confirmation, got: %s", text)
	}
}

func TestAutomationDeleteNotFoundViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_delete_automation", map[string]interface{}{
		"name": "nonexistent",
	}))
	if !strings.Contains(text, "nothing to delete") {
		t.Errorf("expected not found message, got: %s", text)
	}
}

func TestAutomationSetModeViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	seedAutomation(c)
	text := extractToolText(t, callTool(t, c, "acm_set_automation_mode", map[string]interface{}{
		"name": "auto-1",
		"mode": "disabled",
	}))
	if !strings.Contains(text, "disabled") {
		t.Errorf("expected mode update confirmation, got: %s", text)
	}
}

func TestGitOpsCreateViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_create_appset", map[string]interface{}{
		"name":     "my-appset",
		"repo_url": "https://github.com/example/repo",
		"path":     "manifests",
	}))
	if !strings.Contains(text, "my-appset") {
		t.Errorf("expected appset name in result, got: %s", text)
	}
}

func TestGitOpsGetViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	seedAppSet(c)
	text := extractToolText(t, callTool(t, c, "acm_get_appset", map[string]interface{}{
		"name": "app-1",
	}))
	if !strings.Contains(text, "app-1") {
		t.Errorf("expected appset info, got: %s", text)
	}
}

func TestGitOpsGetNotFoundViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text, isErr := extractToolResult(t, callTool(t, c, "acm_get_appset", map[string]interface{}{
		"name": "nonexistent",
	}))
	if !isErr {
		t.Errorf("expected error for nonexistent appset, got: %s", text)
	}
}

func TestGitOpsListViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	seedAppSet(c)
	text := extractToolText(t, callTool(t, c, "acm_list_appsets", nil))
	if !strings.Contains(text, "app-1") {
		t.Errorf("expected app-1 in list, got: %s", text)
	}
}

func TestGitOpsDeleteViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	seedAppSet(c)
	text := extractToolText(t, callTool(t, c, "acm_delete_appset", map[string]interface{}{
		"name": "app-1",
	}))
	if !strings.Contains(text, "deleted") {
		t.Errorf("expected deleted confirmation, got: %s", text)
	}
}

func TestGitOpsDeleteNotFoundViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_delete_appset", map[string]interface{}{
		"name": "nonexistent",
	}))
	if !strings.Contains(text, "nothing to delete") {
		t.Errorf("expected not found message, got: %s", text)
	}
}

func TestGitOpsSyncViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	seedAppSet(c)
	text := extractToolText(t, callTool(t, c, "acm_sync_appset", map[string]interface{}{
		"name": "app-1",
	}))
	if !strings.Contains(strings.ToLower(text), "sync") {
		t.Errorf("expected sync confirmation, got: %s", text)
	}
}

func TestBackupEnableViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_enable_backup", nil))
	if !strings.Contains(strings.ToLower(text), "backup") {
		t.Errorf("expected backup confirmation, got: %s", text)
	}
}

func TestBackupDisableViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	seedBackupSchedule(c)
	text := extractToolText(t, callTool(t, c, "acm_disable_backup", nil))
	if !strings.Contains(strings.ToLower(text), "disabled") && !strings.Contains(strings.ToLower(text), "backup") {
		t.Errorf("expected disabled confirmation, got: %s", text)
	}
}

func TestBackupDisableNotEnabledViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_disable_backup", nil))
	if !strings.Contains(strings.ToLower(text), "nothing") && !strings.Contains(strings.ToLower(text), "not") {
		t.Errorf("expected not-found message, got: %s", text)
	}
}

func TestBackupStatusViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_backup_status", nil))
	if !strings.Contains(strings.ToLower(text), "enabled") && !strings.Contains(strings.ToLower(text), "false") {
		t.Errorf("expected status result, got: %s", text)
	}
}

func TestBackupListViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_list_backups", nil))
	if text == "" {
		t.Error("expected list result")
	}
}

func TestBackupRestoreViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_restore_backup", nil))
	if !strings.Contains(strings.ToLower(text), "restore") {
		t.Errorf("expected restore confirmation, got: %s", text)
	}
}
