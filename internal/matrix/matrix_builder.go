package matrix

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/config"
)

func generateCombinations(spec MatrixSpec) []MatrixCell {
	var cells []MatrixCell
	for _, ver := range spec.OCPVersions {
		for _, arch := range spec.Architectures {
			for _, op := range spec.OperatorVersions {
				cells = append(cells, MatrixCell{
					Name:            cellName(ver, arch, op),
					OCPVersion:      ver,
					Architecture:    arch,
					OperatorVersion: op,
				})
			}
		}
	}
	return cells
}

func cellName(ocpVersion, arch, operatorVersion string) string {
	v := strings.ReplaceAll(ocpVersion, ".", "")
	o := strings.ReplaceAll(operatorVersion, ".", "")
	return fmt.Sprintf("matrix-%s-%s-%s", v, arch, o)
}

func buildNamespace(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
}

func buildMatrixClusterDeployment(cell MatrixCell, cfg config.Config) *unstructured.Unstructured {
	platform := cfg.Platform
	if platform == "" {
		platform = "ibmcloud"
	}
	region := cfg.IBMCloudRegion
	if region == "" {
		region = "us-south"
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      cell.Name,
				"namespace": cell.Name,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/matrix":  "true",
					"ocp-version":               cell.OCPVersion,
					"arch":                      cell.Architecture,
					"ai-platform-version":       cell.OperatorVersion,
				},
			},
			"spec": map[string]interface{}{
				"clusterName": cell.Name,
				"platform": map[string]interface{}{
					platform: map[string]interface{}{
						"region": region,
					},
				},
				"provisioning": map[string]interface{}{
					"imageSetRef": map[string]interface{}{
						"name": fmt.Sprintf("img%s-multi", strings.ReplaceAll(cell.OCPVersion, ".", "")),
					},
					"installConfigSecretRef": map[string]interface{}{
						"name": cell.Name + "-install-config",
					},
				},
			},
		},
	}
}

func buildMatrixLabelPatch(cell MatrixCell, matrixID string) []byte {
	labels := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"ocp-version":         cell.OCPVersion,
				"arch":                cell.Architecture,
				"ai-platform-version": cell.OperatorVersion,
				"matrix":              "true",
				"matrix-id":           matrixID,
			},
		},
	}
	data, _ := json.Marshal(labels)
	return data
}

func parseMatrixCell(obj map[string]interface{}) MatrixCell {
	labels, _, _ := unstructured.NestedStringMap(obj, "metadata", "labels")
	name, _, _ := unstructured.NestedString(obj, "metadata", "name")
	return MatrixCell{
		Name:            name,
		OCPVersion:      labels["ocp-version"],
		Architecture:    labels["arch"],
		OperatorVersion: labels["ai-platform-version"],
	}
}
