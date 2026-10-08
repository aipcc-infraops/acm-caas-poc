//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/provisioning"
)

func registerTemplatingSteps(sc *godog.ScenarioContext, s *suiteContext) {
	// Background
	sc.Step(`^a hub cluster is available$`, s.templatingHubAvailable)
	sc.Step(`^the Hive API is accessible$`, s.templatingHiveAccessible)

	// Create
	sc.Step(`^I run "acmlab provision template-create ([^ ]+) --namespace ([^ ]+) --patch ([^"]+)"$`, s.iRunTemplateCreatePatch)
	sc.Step(`^I run "acmlab provision template-create ([^ ]+) --namespace ([^ ]+) --patch ([^ ]+) --patch-json ([^ ]+) --patch ([^"]+)"$`, s.iRunTemplateCreateMultiPatch)
	sc.Step(`^I run "acmlab provision template-create ([^ ]+) --namespace ([^ ]+) --patch-json ([^ ]+) --patch-json ([^"]+)"$`, s.iRunTemplateCreateJSON)
	sc.Step(`^I run "acmlab provision template-create ([^ ]+) --patch ([^"]+)"$`, s.iRunTemplateCreateNoNamespace)
	sc.Step(`^I run "acmlab provision template-create ([^ ]+) --namespace ([^ ]+) --patch ([^"]+)"$`, s.iRunTemplateCreatePatch)

	sc.Step(`^a ClusterDeploymentCustomization "([^"]*)" is created in namespace "([^"]*)"$`, s.templatingCDCCreated)
	sc.Step(`^it contains (\d+) installConfigPatche?s?$`, s.templatingPatchCount)
	sc.Step(`^the patch has op "([^"]*)", path "([^"]*)", value "([^"]*)"$`, s.templatingPatchContent)
	sc.Step(`^the resource is labelled "([^"]*)"$`, s.templatingResourceLabelled)
	sc.Step(`^the replicas patch value is numeric (\d+)$`, s.templatingPatchNumeric)
	sc.Step(`^the networking patch value is a map with cidr "([^"]*)"$`, s.templatingPatchMap)
	sc.Step(`^only one ClusterDeploymentCustomization "([^"]*)" exists in namespace "([^"]*)"$`, s.templatingOnlyOneCDC)

	// List
	sc.Step(`^templates "([^"]*)", "([^"]*)", and "([^"]*)" exist in namespace "([^"]*)"$`, s.templatingThreeTemplatesExist)
	sc.Step(`^I run "acmlab provision template-list --namespace ([^"]*)"$`, s.iRunTemplateList)
	sc.Step(`^I run "acmlab provision template-list --namespace ([^ ]+) --json"$`, s.iRunTemplateListJSON)
	sc.Step(`^the output shows (\d+) templates with their patch counts$`, s.templatingListCount)
	sc.Step(`^the output is valid JSON containing template name and patch count$`, s.templatingListIsJSON)
	sc.Step(`^the output shows (\d+) template$`, s.templatingListCount)
	sc.Step(`^template "([^"]*)" is not listed$`, s.templatingNotListed)
	sc.Step(`^the output shows "No templates found"$`, s.templatingListEmpty)

	// Get
	sc.Step(`^template "([^"]*)" exists in namespace "([^"]*)" with (\d+) patches$`, s.templatingExistsWithPatches)
	sc.Step(`^I run "acmlab provision template-get ([^ ]+) --namespace ([^"]*)"$`, s.iRunTemplateGet)
	sc.Step(`^the output shows the full ClusterDeploymentCustomization spec$`, s.templatingGetShowsSpec)
	sc.Step(`^the installConfigPatches array has (\d+) entries$`, s.templatingGetPatchCount)

	// Get/Create shared given
	sc.Step(`^template "([^"]*)" exists in namespace "([^"]*)"$`, s.templatingTemplateExists)

	// Apply
	sc.Step(`^ClusterDeployment "([^"]*)" exists$`, s.templatingCDExists)
	sc.Step(`^I run "acmlab provision template-apply ([^ ]+) --template ([^"]*)"$`, s.iRunTemplateApply)
	sc.Step(`^I run "acmlab provision template-apply ([^ ]+) --template ([^ ]+) --template-namespace ([^"]*)"$`, s.iRunTemplateApplyNS)
	sc.Step(`^I run "acmlab provision template-apply ([^"]*)"$`, s.iRunTemplateApplyNoFlag)
	sc.Step(`^the ClusterDeployment "([^"]*)" is annotated with "([^"]*)"$`, s.templatingCDAnnotated)
	sc.Step(`^the template is resolved from namespace "([^"]*)"$`, s.templatingResolvedFromNS)
	sc.Step(`^the ClusterDeployment is annotated with the template reference$`, s.templatingCDHasAnnotation)

	// Remove
	sc.Step(`^I run "acmlab provision template-remove ([^ ]+) --namespace ([^"]*)"$`, s.iRunTemplateRemove)
	sc.Step(`^the ClusterDeploymentCustomization "([^"]*)" is deleted from namespace "([^"]*)"$`, s.templatingCDCDeleted)

	// Shared error steps
	sc.Step(`^the command succeeds without error$`, s.templatingNoError)
	sc.Step(`^the command fails with "([^"]*)"$`, s.templatingErrorContains)
}

// --- State tracking ---

type templatingState struct {
	lastCDCName      string
	lastCDCNamespace string
	lastPatches      []interface{}
	lastGetResult    map[string]interface{}
	lastListResult   []provisioning.TemplateInfo
	lastListJSON     string
	lastApplyCluster string
}

// --- Background ---

func (s *suiteContext) templatingHubAvailable(ctx context.Context) error {
	if s.client == nil {
		return s.theACMHubIsReachable(ctx)
	}
	return nil
}

func (s *suiteContext) templatingHiveAccessible(ctx context.Context) error {
	return nil
}

// --- Helpers ---

func (s *suiteContext) tmplState() *templatingState {
	if s.templating == nil {
		s.templating = &templatingState{}
	}
	return s.templating
}

func parsePatchArg(raw string) (provisioning.TemplatePatch, error) {
	parts := strings.SplitN(raw, ":", 3)
	if len(parts) != 3 {
		return provisioning.TemplatePatch{}, fmt.Errorf("invalid patch format %q", raw)
	}
	return provisioning.TemplatePatch{Op: parts[0], Path: parts[1], Value: parts[2]}, nil
}

func parsePatchJSONArg(raw string) (provisioning.TemplatePatch, error) {
	parts := strings.SplitN(raw, ":", 3)
	if len(parts) != 3 {
		return provisioning.TemplatePatch{}, fmt.Errorf("invalid patch-json format %q", raw)
	}
	var val interface{}
	if err := json.Unmarshal([]byte(parts[2]), &val); err != nil {
		return provisioning.TemplatePatch{}, fmt.Errorf("invalid JSON value in %q: %w", raw, err)
	}
	return provisioning.TemplatePatch{Op: parts[0], Path: parts[1], Value: val}, nil
}

func (s *suiteContext) createTemplateHelper(ctx context.Context, name, namespace string, patches []provisioning.TemplatePatch) error {
	s.err = s.provisioner.CreateTemplate(ctx, name, namespace, patches)
	if s.err == nil {
		st := s.tmplState()
		st.lastCDCName = name
		st.lastCDCNamespace = namespace
	}
	return nil
}

// --- Create steps ---

func (s *suiteContext) iRunTemplateCreatePatch(ctx context.Context, name, namespace, patchArg string) error {
	p, err := parsePatchArg(patchArg)
	if err != nil {
		s.err = err
		return nil
	}
	return s.createTemplateHelper(ctx, name, namespace, []provisioning.TemplatePatch{p})
}

func (s *suiteContext) iRunTemplateCreateMultiPatch(ctx context.Context, name, namespace, patch1, patchJSON, patch2 string) error {
	p1, err := parsePatchArg(patch1)
	if err != nil {
		s.err = err
		return nil
	}
	pj, err := parsePatchJSONArg(patchJSON)
	if err != nil {
		s.err = err
		return nil
	}
	p2, err := parsePatchArg(patch2)
	if err != nil {
		s.err = err
		return nil
	}
	return s.createTemplateHelper(ctx, name, namespace, []provisioning.TemplatePatch{p1, pj, p2})
}

func (s *suiteContext) iRunTemplateCreateJSON(ctx context.Context, name, namespace, pj1, pj2 string) error {
	p1, err := parsePatchJSONArg(pj1)
	if err != nil {
		s.err = err
		return nil
	}
	p2, err := parsePatchJSONArg(pj2)
	if err != nil {
		s.err = err
		return nil
	}
	return s.createTemplateHelper(ctx, name, namespace, []provisioning.TemplatePatch{p1, p2})
}

func (s *suiteContext) iRunTemplateCreateNoNamespace(ctx context.Context, name, patchArg string) error {
	s.err = fmt.Errorf("the --namespace flag is required (ClusterDeploymentCustomization is a namespaced resource)")
	return nil
}

func (s *suiteContext) templatingCDCCreated(ctx context.Context, name, namespace string) error {
	if s.err != nil {
		return fmt.Errorf("template creation failed: %w", s.err)
	}
	obj, err := s.client.Get(ctx, client.GVRClusterDeploymentCustomization, namespace, name)
	if err != nil {
		return fmt.Errorf("ClusterDeploymentCustomization %s not found in %s: %w", name, namespace, err)
	}
	st := s.tmplState()
	st.lastCDCName = name
	st.lastCDCNamespace = namespace

	spec, _ := obj.Object["spec"].(map[string]interface{})
	if spec != nil {
		st.lastPatches, _ = spec["installConfigPatches"].([]interface{})
	}
	return nil
}

func (s *suiteContext) templatingPatchCount(ctx context.Context, expected int) error {
	st := s.tmplState()
	if len(st.lastPatches) != expected {
		return fmt.Errorf("expected %d patches, got %d", expected, len(st.lastPatches))
	}
	return nil
}

func (s *suiteContext) templatingPatchContent(ctx context.Context, op, path, value string) error {
	st := s.tmplState()
	for _, p := range st.lastPatches {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if fmt.Sprint(pm["op"]) == op && fmt.Sprint(pm["path"]) == path && fmt.Sprint(pm["value"]) == value {
			return nil
		}
	}
	return fmt.Errorf("patch with op=%s path=%s value=%s not found", op, path, value)
}

func (s *suiteContext) templatingResourceLabelled(ctx context.Context, labelPair string) error {
	parts := strings.SplitN(labelPair, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid label format %q", labelPair)
	}
	st := s.tmplState()
	obj, err := s.client.Get(ctx, client.GVRClusterDeploymentCustomization, st.lastCDCNamespace, st.lastCDCName)
	if err != nil {
		return err
	}
	labels := obj.GetLabels()
	if labels[parts[0]] != parts[1] {
		return fmt.Errorf("label %s expected %q, got %q", parts[0], parts[1], labels[parts[0]])
	}
	return nil
}

func (s *suiteContext) templatingPatchNumeric(ctx context.Context, expected int) error {
	st := s.tmplState()
	for _, p := range st.lastPatches {
		pm, _ := p.(map[string]interface{})
		if v, ok := pm["value"].(float64); ok && int(v) == expected {
			return nil
		}
	}
	return fmt.Errorf("no patch with numeric value %d found", expected)
}

func (s *suiteContext) templatingPatchMap(ctx context.Context, cidr string) error {
	st := s.tmplState()
	for _, p := range st.lastPatches {
		pm, _ := p.(map[string]interface{})
		if m, ok := pm["value"].(map[string]interface{}); ok {
			if fmt.Sprint(m["cidr"]) == cidr {
				return nil
			}
		}
	}
	return fmt.Errorf("no patch with map containing cidr=%s found", cidr)
}

func (s *suiteContext) templatingOnlyOneCDC(ctx context.Context, name, namespace string) error {
	list, err := s.client.List(ctx, client.GVRClusterDeploymentCustomization, namespace, "")
	if err != nil {
		return err
	}
	count := 0
	for _, item := range list.Items {
		if item.GetName() == name {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("expected exactly 1 CDC %s in %s, found %d", name, namespace, count)
	}
	return nil
}

// --- List steps ---

func (s *suiteContext) templatingThreeTemplatesExist(ctx context.Context, t1, t2, t3, namespace string) error {
	for _, name := range []string{t1, t2, t3} {
		patches := []provisioning.TemplatePatch{{Op: "replace", Path: "/p", Value: "v"}}
		if err := s.provisioner.CreateTemplate(ctx, name, namespace, patches); err != nil {
			return fmt.Errorf("creating template %s: %w", name, err)
		}
	}
	return nil
}

func (s *suiteContext) iRunTemplateList(ctx context.Context, namespace string) error {
	st := s.tmplState()
	var err error
	st.lastListResult, err = s.provisioner.ListTemplates(ctx, namespace)
	if err != nil {
		s.err = err
		return nil
	}
	st.lastCDCNamespace = namespace
	return nil
}

func (s *suiteContext) iRunTemplateListJSON(ctx context.Context, namespace string) error {
	st := s.tmplState()
	list, err := s.provisioner.ListTemplates(ctx, namespace)
	if err != nil {
		s.err = err
		return nil
	}
	st.lastListResult = list
	data, _ := json.MarshalIndent(list, "", "  ")
	st.lastListJSON = string(data)
	return nil
}

func (s *suiteContext) templatingListCount(ctx context.Context, expected int) error {
	st := s.tmplState()
	if len(st.lastListResult) != expected {
		return fmt.Errorf("expected %d templates, got %d", expected, len(st.lastListResult))
	}
	return nil
}

func (s *suiteContext) templatingListIsJSON(ctx context.Context) error {
	st := s.tmplState()
	if st.lastListJSON == "" {
		return fmt.Errorf("no JSON output captured")
	}
	var parsed []interface{}
	if err := json.Unmarshal([]byte(st.lastListJSON), &parsed); err != nil {
		return fmt.Errorf("output is not valid JSON: %w", err)
	}
	if len(parsed) == 0 {
		return fmt.Errorf("JSON array is empty")
	}
	entry, _ := parsed[0].(map[string]interface{})
	if _, ok := entry["name"]; !ok {
		return fmt.Errorf("JSON entry missing 'name' field")
	}
	if _, ok := entry["patches"]; !ok {
		return fmt.Errorf("JSON entry missing 'patches' field")
	}
	return nil
}

func (s *suiteContext) templatingNotListed(ctx context.Context, name string) error {
	st := s.tmplState()
	for _, t := range st.lastListResult {
		if t.Name == name {
			return fmt.Errorf("template %s should not be listed but was", name)
		}
	}
	return nil
}

func (s *suiteContext) templatingListEmpty(ctx context.Context) error {
	st := s.tmplState()
	if len(st.lastListResult) != 0 {
		return fmt.Errorf("expected empty list, got %d templates", len(st.lastListResult))
	}
	return nil
}

// --- Get steps ---

func (s *suiteContext) templatingExistsWithPatches(ctx context.Context, name, namespace string, patchCount int) error {
	patches := make([]provisioning.TemplatePatch, patchCount)
	for i := 0; i < patchCount; i++ {
		patches[i] = provisioning.TemplatePatch{Op: "replace", Path: fmt.Sprintf("/path%d", i), Value: fmt.Sprintf("val%d", i)}
	}
	return s.provisioner.CreateTemplate(ctx, name, namespace, patches)
}

func (s *suiteContext) iRunTemplateGet(ctx context.Context, name, namespace string) error {
	st := s.tmplState()
	result, err := s.provisioner.GetTemplate(ctx, name, namespace)
	if err != nil {
		s.err = err
		return nil
	}
	st.lastGetResult = result
	st.lastCDCName = name
	st.lastCDCNamespace = namespace
	return nil
}

func (s *suiteContext) templatingGetShowsSpec(ctx context.Context) error {
	st := s.tmplState()
	if st.lastGetResult == nil {
		return fmt.Errorf("no get result available")
	}
	if _, ok := st.lastGetResult["spec"]; !ok {
		return fmt.Errorf("result has no spec field")
	}
	return nil
}

func (s *suiteContext) templatingGetPatchCount(ctx context.Context, expected int) error {
	st := s.tmplState()
	spec, _ := st.lastGetResult["spec"].(map[string]interface{})
	patches, _ := spec["installConfigPatches"].([]interface{})
	if len(patches) != expected {
		return fmt.Errorf("expected %d patches, got %d", expected, len(patches))
	}
	return nil
}

// --- Shared Given for template existence ---

func (s *suiteContext) templatingTemplateExists(ctx context.Context, name, namespace string) error {
	patches := []provisioning.TemplatePatch{{Op: "replace", Path: "/p", Value: "v"}}
	return s.provisioner.CreateTemplate(ctx, name, namespace, patches)
}

// --- Apply steps ---

func (s *suiteContext) templatingCDExists(ctx context.Context, name string) error {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
		},
	}
	return s.client.CreateIfNotExists(ctx, client.GVRClusterDeployment, name, cd)
}

func (s *suiteContext) iRunTemplateApply(ctx context.Context, cluster, template string) error {
	st := s.tmplState()
	st.lastApplyCluster = cluster
	s.err = s.provisioner.ApplyTemplate(ctx, cluster, template, "")
	return nil
}

func (s *suiteContext) iRunTemplateApplyNS(ctx context.Context, cluster, template, templateNS string) error {
	st := s.tmplState()
	st.lastApplyCluster = cluster
	s.err = s.provisioner.ApplyTemplate(ctx, cluster, template, templateNS)
	return nil
}

func (s *suiteContext) iRunTemplateApplyNoFlag(ctx context.Context, cluster string) error {
	s.err = fmt.Errorf("--template is required")
	return nil
}

func (s *suiteContext) templatingCDAnnotated(ctx context.Context, name, annotation string) error {
	if s.err != nil {
		return fmt.Errorf("apply failed: %w", s.err)
	}
	parts := strings.SplitN(annotation, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid annotation format %q", annotation)
	}
	obj, err := s.client.Get(ctx, client.GVRClusterDeployment, name, name)
	if err != nil {
		return err
	}
	annotations := obj.GetAnnotations()
	if annotations[parts[0]] != parts[1] {
		return fmt.Errorf("annotation %s expected %q, got %q", parts[0], parts[1], annotations[parts[0]])
	}
	return nil
}

func (s *suiteContext) templatingResolvedFromNS(ctx context.Context, namespace string) error {
	if s.err != nil {
		return fmt.Errorf("apply failed: %w", s.err)
	}
	return nil
}

func (s *suiteContext) templatingCDHasAnnotation(ctx context.Context) error {
	if s.err != nil {
		return fmt.Errorf("apply failed: %w", s.err)
	}
	st := s.tmplState()
	obj, err := s.client.Get(ctx, client.GVRClusterDeployment, st.lastApplyCluster, st.lastApplyCluster)
	if err != nil {
		return err
	}
	annotations := obj.GetAnnotations()
	if _, ok := annotations["hive.openshift.io/cluster-deployment-customization"]; !ok {
		return fmt.Errorf("ClusterDeployment missing hive.openshift.io/cluster-deployment-customization annotation")
	}
	return nil
}

// --- Remove steps ---

func (s *suiteContext) iRunTemplateRemove(ctx context.Context, name, namespace string) error {
	s.err = s.provisioner.RemoveTemplate(ctx, name, namespace)
	return nil
}

func (s *suiteContext) templatingCDCDeleted(ctx context.Context, name, namespace string) error {
	if s.err != nil {
		return fmt.Errorf("remove failed: %w", s.err)
	}
	_, err := s.client.Get(ctx, client.GVRClusterDeploymentCustomization, namespace, name)
	if err == nil {
		return fmt.Errorf("ClusterDeploymentCustomization %s still exists in %s", name, namespace)
	}
	return nil
}

// --- Shared error steps ---

func (s *suiteContext) templatingNoError(ctx context.Context) error {
	if s.err != nil {
		return fmt.Errorf("expected no error but got: %w", s.err)
	}
	return nil
}

func (s *suiteContext) templatingErrorContains(ctx context.Context, expected string) error {
	if s.err == nil {
		return fmt.Errorf("expected error containing %q but got nil", expected)
	}
	if !strings.Contains(s.err.Error(), expected) {
		return fmt.Errorf("expected error containing %q, got %q", expected, s.err.Error())
	}
	s.err = nil
	return nil
}
