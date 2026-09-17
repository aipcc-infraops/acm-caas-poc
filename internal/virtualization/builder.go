package virtualization

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildVMManifestWork(cluster, name string, opts VMOpts) *unstructured.Unstructured {
	vm := map[string]interface{}{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachine",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"running": true,
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"kubevirt.io/vm": name,
					},
				},
				"spec": map[string]interface{}{
					"domain": map[string]interface{}{
						"cpu": map[string]interface{}{
							"cores": mustParseInt(opts.CPU),
						},
						"memory": map[string]interface{}{
							"guest": opts.Memory,
						},
						"devices": map[string]interface{}{
							"disks": []interface{}{
								map[string]interface{}{
									"name": "rootdisk",
									"disk": map[string]interface{}{
										"bus": "virtio",
									},
								},
								map[string]interface{}{
									"name": "cloudinitdisk",
									"disk": map[string]interface{}{
										"bus": "virtio",
									},
								},
							},
							"interfaces": []interface{}{
								map[string]interface{}{
									"name":       "default",
									"masquerade": map[string]interface{}{},
								},
							},
						},
					},
					"networks": []interface{}{
						map[string]interface{}{
							"name": "default",
							"pod":  map[string]interface{}{},
						},
					},
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "rootdisk",
							"containerDisk": map[string]interface{}{
								"image": opts.Image,
							},
						},
						map[string]interface{}{
							"name": "cloudinitdisk",
							"cloudInitNoCloud": map[string]interface{}{
								"userData": "#cloud-config\npassword: acmlab\nchpasswd: { expire: False }\n",
							},
						},
					},
				},
			},
		},
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      mwName(name, cluster),
				"namespace": cluster,
				"labels": map[string]interface{}{
					LabelVM:                    "true",
					LabelVMName:               name,
					"acmlab.redhat.com/vm-image": opts.Image,
					"acmlab.redhat.com/vm-disk":  opts.DiskSize,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{vm},
				},
			},
		},
	}
}

func mustParseInt(s string) int64 {
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	if n == 0 {
		return 2
	}
	return n
}
