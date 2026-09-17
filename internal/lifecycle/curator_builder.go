package lifecycle

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func buildClusterCurator(opts CuratorOpts) *unstructured.Unstructured {
	ns := opts.Namespace
	if ns == "" {
		ns = opts.Cluster
	}

	spec := map[string]interface{}{}

	if opts.PreHook != nil {
		spec["prehook"] = []interface{}{buildHookSpec(opts.PreHook)}
	}
	if opts.PostHook != nil {
		spec["posthook"] = []interface{}{buildHookSpec(opts.PostHook)}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "ClusterCurator",
			"metadata": map[string]interface{}{
				"name":      opts.Cluster,
				"namespace": ns,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/curator": "true",
				},
			},
			"spec": spec,
		},
	}
}

func buildHookSpec(hook *CuratorHook) map[string]interface{} {
	h := map[string]interface{}{
		"name": hook.Name,
	}

	if hook.Type != "" {
		h["type"] = hook.Type
	}

	if hook.Image != "" {
		h["extra_vars"] = map[string]interface{}{}
		if hook.ExtraVars != nil {
			evars := map[string]interface{}{}
			for k, v := range hook.ExtraVars {
				evars[k] = v
			}
			h["extra_vars"] = evars
		}
	}

	if len(hook.Commands) > 0 {
		cmds := make([]interface{}, len(hook.Commands))
		for i, cmd := range hook.Commands {
			cmds[i] = cmd
		}
		h["commands"] = cmds
	}

	if hook.JobTTL > 0 {
		h["job_ttl"] = int64(hook.JobTTL)
	}

	return h
}
