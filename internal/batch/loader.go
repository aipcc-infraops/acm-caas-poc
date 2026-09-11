// internal/batch/loader.go
package batch

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ClusterItem struct {
	Name              string            `yaml:"name"`
	KubeconfigPath    string            `yaml:"kubeconfigPath"`
	KubeconfigContext string            `yaml:"kubeconfigContext"`
	Labels            map[string]string `yaml:"labels"`
	ClusterSet        string            `yaml:"clusterSet"`
	Platform          string            `yaml:"platform"`
	Region            string            `yaml:"region"`
	Workers           *int              `yaml:"workers"`
	WorkerType        string            `yaml:"workerType"`
	MirrorRegistry    string            `yaml:"mirror"`
	PullSecretPath    string            `yaml:"pullSecretPath"`
}

func (c *ClusterItem) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		c.Name = value.Value
		return nil
	}
	type plain ClusterItem
	return value.Decode((*plain)(c))
}

type clusterFile struct {
	Clusters []ClusterItem `yaml:"clusters"`
}

func LoadFile(path string) ([]ClusterItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", path, err)
	}
	var f clusterFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(f.Clusters) == 0 {
		return nil, fmt.Errorf("file %s has empty clusters list", path)
	}
	return f.Clusters, nil
}

func NamesFromArgs(args []string, fileItems []ClusterItem) ([]ClusterItem, error) {
	if len(args) == 0 && len(fileItems) == 0 {
		return nil, fmt.Errorf("provide at least one cluster name or --from-file")
	}
	seen := make(map[string]bool, len(args))
	for _, name := range args {
		seen[name] = true
	}
	for _, item := range fileItems {
		if seen[item.Name] {
			return nil, fmt.Errorf("cluster %q appears in both args and --from-file", item.Name)
		}
	}
	result := make([]ClusterItem, 0, len(args)+len(fileItems))
	for _, name := range args {
		result = append(result, ClusterItem{Name: name})
	}
	result = append(result, fileItems...)
	return result, nil
}
