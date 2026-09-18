package discovery

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type CloudCluster struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Type     string `json:"type"`
	Region   string `json:"region"`
	Version  string `json:"version"`
	Status   string `json:"status"`
	Managed  bool   `json:"managed"`
}

type ScanOpts struct {
	Provider string
	Region   string
}

type CmdRunner func(name string, args ...string) ([]byte, error)

type ImportOpts struct {
	DryRun        bool
	ClusterSet    string
	AllowInsecure bool
	FixTLS        bool
}

func (o ImportOpts) resolvedSet() string {
	if o.ClusterSet != "" {
		return o.ClusterSet
	}
	return "default"
}

type ImportPreview struct {
	ClusterName  string   `json:"clusterName"`
	ClusterSet   string   `json:"clusterSet"`
	Provider     string   `json:"provider,omitempty"`
	Type         string   `json:"type,omitempty"`
	Region       string   `json:"region,omitempty"`
	Resources    []string `json:"resources"`
	AutoImported bool     `json:"autoImported"`
}

type ImportedCluster struct {
	Name       string `json:"name"`
	CreatedVia string `json:"createdVia"`
	ClusterSet string `json:"clusterSet"`
	CreatedAt  string `json:"createdAt"`
	Status     string `json:"status"`
}

func DefaultCmdRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func (m *Manager) ScanClusters(ctx context.Context, opts ScanOpts) ([]CloudCluster, error) {
	m.logger.Info("discovery.ScanClusters", "provider", opts.Provider, "region", opts.Region)

	runner := m.cmdRunner
	if runner == nil {
		runner = DefaultCmdRunner
	}

	var all []CloudCluster

	if opts.Provider == "" || opts.Provider == "aws" {
		clusters, err := scanAWS(runner, opts.Region)
		if err != nil {
			m.logger.Warn("AWS scan skipped", "error", err)
		} else {
			all = append(all, clusters...)
		}
	}

	if opts.Provider == "" || opts.Provider == "ibmcloud" {
		clusters, err := scanIBMCloud(runner, opts.Region)
		if err != nil {
			m.logger.Warn("IBM Cloud scan skipped", "error", err)
		} else {
			all = append(all, clusters...)
		}
	}

	if opts.Provider != "" && opts.Provider != "aws" && opts.Provider != "ibmcloud" {
		return nil, fmt.Errorf("unsupported provider %q (valid: aws, ibmcloud)", opts.Provider)
	}

	managed, err := m.listManagedNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing managed clusters: %w", err)
	}
	for i := range all {
		if managed[all[i].Name] {
			all[i].Managed = true
		}
	}

	return all, nil
}

func (m *Manager) AutoImport(ctx context.Context, clusterName, provider string, opts ImportOpts) (*ImportPreview, error) {
	m.logger.Info("discovery.AutoImport", "cluster", clusterName, "provider", provider, "dryRun", opts.DryRun)

	runner := m.cmdRunner
	if runner == nil {
		runner = DefaultCmdRunner
	}

	var cluster *CloudCluster
	switch provider {
	case "aws":
		clusters, err := scanAWS(runner, "")
		if err != nil {
			return nil, fmt.Errorf("scanning AWS: %w", err)
		}
		for _, c := range clusters {
			if c.Name == clusterName {
				cc := c
				cluster = &cc
				break
			}
		}
	case "ibmcloud":
		clusters, err := scanIBMCloud(runner, "")
		if err != nil {
			return nil, fmt.Errorf("scanning IBM Cloud: %w", err)
		}
		for _, c := range clusters {
			if c.Name == clusterName {
				cc := c
				cluster = &cc
				break
			}
		}
	default:
		return nil, fmt.Errorf("unsupported provider %q (valid: aws, ibmcloud)", provider)
	}

	if cluster == nil {
		return nil, fmt.Errorf("cluster %s not found in %s", clusterName, provider)
	}

	resources := []string{
		fmt.Sprintf("Namespace/%s", clusterName),
		fmt.Sprintf("ManagedCluster/%s (set=%s)", clusterName, opts.resolvedSet()),
		fmt.Sprintf("KlusterletAddonConfig/%s", clusterName),
	}

	spokeKubeconfig, credErr := fetchSpokeKubeconfig(runner, clusterName, provider, cluster.Type, cluster.Region)
	if credErr == nil {
		if opts.FixTLS {
			if secured, n, secErr := SecureKubeconfig(spokeKubeconfig); secErr == nil && n > 0 {
				spokeKubeconfig = secured
				m.logger.Info("secured kubeconfig TLS", "cluster", clusterName, "fixed", n)
			}
		}
		resources = append(resources, fmt.Sprintf("Secret/%s/auto-import-secret", clusterName))
	}

	preview := &ImportPreview{
		ClusterName:  clusterName,
		ClusterSet:   opts.resolvedSet(),
		Provider:     provider,
		Type:         cluster.Type,
		Region:       cluster.Region,
		Resources:    resources,
		AutoImported: credErr == nil,
	}

	if opts.DryRun {
		return preview, nil
	}

	if err := m.ensureNamespace(ctx, clusterName); err != nil {
		return nil, fmt.Errorf("creating namespace: %w", err)
	}

	mc := buildCloudManagedCluster(clusterName, provider, cluster.Type, cluster.Region, opts.resolvedSet())
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedCluster, "", mc); err != nil {
		return nil, fmt.Errorf("creating ManagedCluster: %w", err)
	}

	kac := buildKlusterletAddonConfig(clusterName)
	if err := m.client.CreateIfNotExists(ctx, client.GVRKlusterletAddonConfig, clusterName, kac); err != nil {
		return nil, fmt.Errorf("creating KlusterletAddonConfig: %w", err)
	}

	if credErr != nil {
		m.logger.Warn("spoke credentials not available, manual import required", "cluster", clusterName, "reason", credErr.Error())
	} else {
		secret := buildAutoImportSecret(clusterName, spokeKubeconfig)
		if err := m.client.CreateIfNotExists(ctx, client.GVRSecret, clusterName, secret); err != nil {
			m.logger.Warn("failed to create auto-import secret", "cluster", clusterName, "error", err.Error())
			preview.AutoImported = false
		}
	}

	return preview, nil
}

func (m *Manager) listManagedNames(ctx context.Context) (map[string]bool, error) {
	list, err := m.client.List(ctx, client.GVRManagedCluster, "", "")
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool, len(list.Items))
	for _, item := range list.Items {
		names[item.GetName()] = true
	}
	return names, nil
}

type eksListOutput struct {
	Clusters []string `json:"clusters"`
}

type eksDescribeOutput struct {
	Cluster struct {
		Name     string `json:"name"`
		Version  string `json:"version"`
		Status   string `json:"status"`
		Arn      string `json:"arn"`
		Endpoint string `json:"endpoint"`
	} `json:"cluster"`
}

type rosaCluster struct {
	Name    string          `json:"name"`
	ID      string          `json:"id"`
	Region  json.RawMessage `json:"region,omitempty"`
	Status  json.RawMessage `json:"status,omitempty"`
	Version rosaVersion     `json:"version,omitempty"`
	AWS     rosaAWS         `json:"aws,omitempty"`
	State   string          `json:"state,omitempty"`
	OpenshiftVersion string `json:"openshift_version,omitempty"`
}

func (rc rosaCluster) regionID() string {
	var obj struct{ ID string `json:"id"` }
	if json.Unmarshal(rc.Region, &obj) == nil && obj.ID != "" {
		return obj.ID
	}
	var s string
	if json.Unmarshal(rc.Region, &s) == nil {
		return s
	}
	return rc.AWS.Region
}

func (rc rosaCluster) stateStr() string {
	if rc.State != "" {
		return rc.State
	}
	var obj struct{ State string `json:"state"` }
	if json.Unmarshal(rc.Status, &obj) == nil {
		return obj.State
	}
	return ""
}

type rosaVersion struct {
	ID string `json:"id"`
}

type rosaAWS struct {
	Region string `json:"region"`
}

type ibmCluster struct {
	Name         string `json:"name"`
	ID           string `json:"id"`
	Region       string `json:"region"`
	State        string `json:"state"`
	Type         string `json:"type"`
	MasterKubeVersion string `json:"masterKubeVersion"`
}

func scanAWS(runner CmdRunner, region string) ([]CloudCluster, error) {
	var all []CloudCluster

	eksArgs := []string{"eks", "list-clusters", "--output", "json"}
	if region != "" {
		eksArgs = append(eksArgs, "--region", region)
	}
	out, err := runner("aws", eksArgs...)
	if err == nil {
		var eksResult eksListOutput
		if json.Unmarshal(out, &eksResult) == nil {
			for _, name := range eksResult.Clusters {
				descArgs := []string{"eks", "describe-cluster", "--name", name, "--output", "json"}
				if region != "" {
					descArgs = append(descArgs, "--region", region)
				}
				descOut, descErr := runner("aws", descArgs...)
				cc := CloudCluster{
					Name:     name,
					Provider: "aws",
					Type:     "eks",
				}
				if descErr == nil {
					var desc eksDescribeOutput
					if json.Unmarshal(descOut, &desc) == nil {
						cc.Version = desc.Cluster.Version
						cc.Status = desc.Cluster.Status
					}
				}
				if region != "" {
					cc.Region = region
				}
				all = append(all, cc)
			}
		}
	}

	rosaOut, rosaErr := runner("rosa", "list", "clusters", "--output", "json")
	if rosaErr == nil {
		var rosaClusters []rosaCluster
		if json.Unmarshal(rosaOut, &rosaClusters) == nil {
			for _, rc := range rosaClusters {
				cc := CloudCluster{
					Name:     rc.Name,
					Provider: "aws",
					Type:     "rosa",
					Status:   rc.stateStr(),
					Region:   rc.regionID(),
				}
				if rc.Version.ID != "" {
					cc.Version = rc.Version.ID
				} else if rc.OpenshiftVersion != "" {
					cc.Version = rc.OpenshiftVersion
				}
				if region != "" && cc.Region != region {
					continue
				}
				all = append(all, cc)
			}
		}
	}

	if err != nil && rosaErr != nil {
		return nil, fmt.Errorf("neither aws nor rosa CLI available")
	}

	return all, nil
}

func scanIBMCloud(runner CmdRunner, region string) ([]CloudCluster, error) {
	out, err := runner("ibmcloud", "ks", "cluster", "ls", "--output", "json")
	if err != nil {
		return nil, fmt.Errorf("ibmcloud CLI not available: %w", err)
	}

	var clusters []ibmCluster
	if err := json.Unmarshal(out, &clusters); err != nil {
		return nil, fmt.Errorf("parsing ibmcloud output: %w", err)
	}

	var result []CloudCluster
	for _, c := range clusters {
		if region != "" && c.Region != region {
			continue
		}
		clusterType := "iks"
		if c.Type == "openshift" {
			clusterType = "roks"
		}
		result = append(result, CloudCluster{
			Name:     c.Name,
			Provider: "ibmcloud",
			Type:     clusterType,
			Region:   c.Region,
			Version:  c.MasterKubeVersion,
			Status:   c.State,
		})
	}
	return result, nil
}

type KubeconfigCluster struct {
	Name    string `json:"name"`
	Server  string `json:"server"`
	Context string `json:"context"`
	Managed bool   `json:"managed"`
	Source  string `json:"source"`
}

func (m *Manager) ScanKubeconfigs(ctx context.Context, dir string) ([]KubeconfigCluster, error) {
	m.logger.Info("discovery.ScanKubeconfigs", "dir", dir)

	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolving home directory: %w", err)
		}
		dir = filepath.Join(home, ".kube")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var all []KubeconfigCluster
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		cfg, err := clientcmd.LoadFromFile(path)
		if err != nil {
			continue
		}
		for ctxName, ctxVal := range cfg.Contexts {
			clusterInfo, ok := cfg.Clusters[ctxVal.Cluster]
			if !ok {
				continue
			}
			all = append(all, KubeconfigCluster{
				Name:    ctxVal.Cluster,
				Server:  clusterInfo.Server,
				Context: ctxName,
				Source:  path,
			})
		}
	}

	managed, err := m.listManagedWithURLs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing managed clusters: %w", err)
	}
	for i := range all {
		if managed[all[i].Name] || managed[all[i].Server] {
			all[i].Managed = true
		}
	}

	return all, nil
}

func (m *Manager) listManagedWithURLs(ctx context.Context) (map[string]bool, error) {
	list, err := m.client.List(ctx, client.GVRManagedCluster, "", "")
	if err != nil {
		return nil, err
	}
	lookup := make(map[string]bool, len(list.Items)*2)
	for _, item := range list.Items {
		lookup[item.GetName()] = true
		urls, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "managedClusterClientConfigs")
		if len(urls) == 0 {
			url, _, _ := unstructured.NestedString(item.Object, "spec", "managedClusterClientConfigs", "url")
			if url != "" {
				lookup[url] = true
			}
		}
		statusURL, _, _ := unstructured.NestedString(item.Object, "status", "apiServerURL")
		if statusURL != "" {
			lookup[statusURL] = true
		}
	}
	return lookup, nil
}

func (m *Manager) AutoImportKubeconfig(ctx context.Context, name, kubeconfigPath string, opts ImportOpts) (*ImportPreview, error) {
	m.logger.Info("discovery.AutoImportKubeconfig", "name", name, "kubeconfig", kubeconfigPath, "dryRun", opts.DryRun)

	data, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("reading kubeconfig %s: %w", kubeconfigPath, err)
	}

	cfg, err := clientcmd.Load(data)
	if err != nil {
		return nil, fmt.Errorf("invalid kubeconfig %s: %w", kubeconfigPath, err)
	}

	hasInsecure := false
	for _, cluster := range cfg.Clusters {
		if cluster.InsecureSkipTLSVerify {
			hasInsecure = true
			break
		}
	}

	if hasInsecure && !opts.AllowInsecure && !opts.FixTLS {
		return nil, fmt.Errorf("kubeconfig contains insecure-skip-tls-verify; use --allow-insecure to import as-is or --fix-tls to fetch CA certificates automatically")
	}

	if hasInsecure && opts.FixTLS {
		if secured, n, secErr := SecureKubeconfig(data); secErr == nil && n > 0 {
			data = secured
			m.logger.Info("secured kubeconfig TLS", "kubeconfig", kubeconfigPath, "fixed", n)
		} else if secErr != nil {
			m.logger.Warn("could not auto-secure kubeconfig", "error", secErr.Error())
			if !opts.AllowInsecure {
				return nil, fmt.Errorf("failed to secure kubeconfig and --allow-insecure not set: %w", secErr)
			}
		}
	}

	preview := &ImportPreview{
		ClusterName: name,
		ClusterSet:  opts.resolvedSet(),
		Resources: []string{
			fmt.Sprintf("Namespace/%s", name),
			fmt.Sprintf("ManagedCluster/%s (set=%s)", name, opts.resolvedSet()),
			fmt.Sprintf("KlusterletAddonConfig/%s", name),
			fmt.Sprintf("Secret/%s/auto-import-secret", name),
		},
	}

	if opts.DryRun {
		return preview, nil
	}

	if err := m.ensureNamespace(ctx, name); err != nil {
		return nil, fmt.Errorf("creating namespace: %w", err)
	}

	mc := buildKubeconfigManagedCluster(name, opts.resolvedSet())
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedCluster, "", mc); err != nil {
		return nil, fmt.Errorf("creating ManagedCluster: %w", err)
	}

	kac := buildKlusterletAddonConfig(name)
	if err := m.client.CreateIfNotExists(ctx, client.GVRKlusterletAddonConfig, name, kac); err != nil {
		return nil, fmt.Errorf("creating KlusterletAddonConfig: %w", err)
	}

	secret := buildAutoImportSecret(name, data)
	if err := m.client.CreateIfNotExists(ctx, client.GVRSecret, name, secret); err != nil {
		return nil, fmt.Errorf("creating auto-import secret: %w", err)
	}

	return preview, nil
}

func buildKubeconfigManagedCluster(name, clusterSet string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"name":                        name,
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/discovery": "true",
					"created-via":                 "kubeconfig-discovery",
					"cluster.open-cluster-management.io/clusterset": clusterSet,
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}
}

func buildAutoImportSecret(name string, kubeconfig []byte) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "auto-import-secret",
				"namespace": name,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"type": "Opaque",
			"data": map[string]interface{}{
				"kubeconfig": base64.StdEncoding.EncodeToString(kubeconfig),
			},
		},
	}
}

func buildCloudManagedCluster(name, provider, clusterType, region, clusterSet string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"name":                          name,
					"acmlab.redhat.com/managed":     "true",
					"acmlab.redhat.com/discovery":   "true",
					"acmlab.redhat.com/discovered-from": provider,
					"cloud":                         provider,
					"cluster-type":                  clusterType,
					"region":                        region,
					"created-via":                   "cloud-discovery",
					"cluster.open-cluster-management.io/clusterset": clusterSet,
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}
}

// PoC-grade credential handling: creates admin credentials on the spoke for import.
// Production use should prefer short-lived tokens, least-privilege service accounts,
// and post-import cleanup of the admin user.
func fetchSpokeKubeconfig(runner CmdRunner, clusterName, provider, clusterType, region string) ([]byte, error) {
	switch {
	case provider == "aws" && clusterType == "rosa":
		return fetchROSAKubeconfig(runner, clusterName)
	case provider == "aws" && clusterType == "eks":
		return fetchEKSKubeconfig(runner, clusterName, region)
	case provider == "ibmcloud":
		return fetchIBMCloudKubeconfig(runner, clusterName)
	default:
		return nil, fmt.Errorf("auto-credentials not supported for %s/%s", provider, clusterType)
	}
}

func fetchROSAKubeconfig(runner CmdRunner, clusterName string) ([]byte, error) {
	descOut, err := runner("rosa", "describe", "cluster", "-c", clusterName, "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("describing ROSA cluster: %w", err)
	}

	var desc struct {
		API struct {
			URL string `json:"url"`
		} `json:"api"`
	}
	if err := json.Unmarshal(descOut, &desc); err != nil || desc.API.URL == "" {
		return nil, fmt.Errorf("parsing ROSA cluster API URL")
	}

	adminOut, err := runner("rosa", "create", "admin", "-c", clusterName, "--yes")
	if err != nil {
		return nil, fmt.Errorf("creating ROSA admin: %w", err)
	}

	apiURL, username, password := parseROSAAdminOutput(string(adminOut))
	if username == "" || password == "" {
		return nil, fmt.Errorf("failed to parse ROSA admin credentials from output")
	}
	if apiURL == "" {
		apiURL = desc.API.URL
	}

	return buildBasicAuthKubeconfig(clusterName, apiURL, username, password, false)
}

var rosaLoginRe = regexp.MustCompile(`oc\s+login\s+(https?://\S+)\s+--username\s+(\S+)\s+--password\s+'?([^'\s]+)'?`)

func parseROSAAdminOutput(output string) (apiURL, username, password string) {
	for _, line := range strings.Split(output, "\n") {
		matches := rosaLoginRe.FindStringSubmatch(line)
		if len(matches) == 4 {
			return matches[1], matches[2], matches[3]
		}
	}
	return "", "", ""
}

func buildBasicAuthKubeconfig(name, server, username, password string, insecure bool) ([]byte, error) {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[name] = &clientcmdapi.Cluster{
		Server:                server,
		InsecureSkipTLSVerify: insecure,
	}
	cfg.AuthInfos[name] = &clientcmdapi.AuthInfo{
		Username: username,
		Password: password,
	}
	cfg.Contexts[name] = &clientcmdapi.Context{
		Cluster:  name,
		AuthInfo: name,
	}
	cfg.CurrentContext = name
	return clientcmd.Write(*cfg)
}

func fetchEKSKubeconfig(runner CmdRunner, clusterName, region string) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "eks-kubeconfig-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	args := []string{"eks", "update-kubeconfig", "--name", clusterName, "--kubeconfig", tmpPath}
	if region != "" {
		args = append(args, "--region", region)
	}
	if _, err := runner("aws", args...); err != nil {
		return nil, fmt.Errorf("fetching EKS kubeconfig: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("reading EKS kubeconfig: %w", err)
	}
	return data, nil
}

func fetchIBMCloudKubeconfig(runner CmdRunner, clusterName string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "ibmcloud-kubeconfig-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = runner("ibmcloud", "ks", "cluster", "config", "--cluster", clusterName, "--admin", "--output", "yaml", "--dir", tmpDir)
	if err != nil {
		return nil, fmt.Errorf("fetching IBM Cloud kubeconfig: %w", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("reading temp dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(tmpDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if _, err := clientcmd.Load(data); err == nil {
			return data, nil
		}
	}

	return nil, fmt.Errorf("no valid kubeconfig found in ibmcloud output")
}

func FetchServerCA(serverURL string) ([]byte, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("parsing server URL: %w", err)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "443"
	}

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp",
		net.JoinHostPort(host, port),
		&tls.Config{InsecureSkipVerify: true},
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", serverURL, err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates returned by %s", serverURL)
	}

	var pemData []byte
	for _, cert := range certs {
		if cert.IsCA || cert.BasicConstraintsValid && (cert.MaxPathLen > 0 || cert.MaxPathLenZero) {
			pemData = append(pemData, pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: cert.Raw,
			})...)
		}
	}

	if len(pemData) == 0 {
		pemData = pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certs[len(certs)-1].Raw,
		})
	}

	return pemData, nil
}

func SecureKubeconfig(kubeconfigData []byte) ([]byte, int, error) {
	cfg, err := clientcmd.Load(kubeconfigData)
	if err != nil {
		return nil, 0, fmt.Errorf("parsing kubeconfig: %w", err)
	}

	fixed := 0
	for name, cluster := range cfg.Clusters {
		if !cluster.InsecureSkipTLSVerify {
			continue
		}
		if cluster.Server == "" {
			continue
		}
		caData, err := FetchServerCA(cluster.Server)
		if err != nil {
			return nil, fixed, fmt.Errorf("fetching CA for cluster %s (%s): %w", name, cluster.Server, err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caData) {
			return nil, fixed, fmt.Errorf("fetched CA for %s is not valid PEM", name)
		}

		cluster.InsecureSkipTLSVerify = false
		cluster.CertificateAuthorityData = caData
		fixed++
	}

	data, err := clientcmd.Write(*cfg)
	if err != nil {
		return nil, fixed, fmt.Errorf("writing kubeconfig: %w", err)
	}
	return data, fixed, nil
}

type FixTLSResult struct {
	ClusterName string `json:"clusterName"`
	Server      string `json:"server"`
	Fixed       bool   `json:"fixed"`
	Message     string `json:"message"`
}

func (m *Manager) FixTLS(ctx context.Context, clusterName string) (*FixTLSResult, error) {
	m.logger.Info("discovery.FixTLS", "cluster", clusterName)

	secret, err := m.client.Get(ctx, client.GVRSecret, clusterName, "auto-import-secret")
	if err != nil {
		return nil, fmt.Errorf("getting auto-import-secret for %s: %w", clusterName, err)
	}

	dataField, _, _ := unstructured.NestedMap(secret.Object, "data")
	kubeconfigB64, ok := dataField["kubeconfig"].(string)
	if !ok {
		return nil, fmt.Errorf("auto-import-secret for %s has no kubeconfig field", clusterName)
	}

	kubeconfigData, err := base64.StdEncoding.DecodeString(kubeconfigB64)
	if err != nil {
		return nil, fmt.Errorf("decoding kubeconfig: %w", err)
	}

	cfg, err := clientcmd.Load(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("parsing kubeconfig: %w", err)
	}

	hasInsecure := false
	var server string
	for _, cluster := range cfg.Clusters {
		if cluster.InsecureSkipTLSVerify {
			hasInsecure = true
			server = cluster.Server
			break
		}
	}

	if !hasInsecure {
		return &FixTLSResult{
			ClusterName: clusterName,
			Server:      server,
			Fixed:       false,
			Message:     "already secure",
		}, nil
	}

	securedData, fixed, err := SecureKubeconfig(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("securing kubeconfig: %w", err)
	}
	if fixed == 0 {
		return &FixTLSResult{ClusterName: clusterName, Server: server, Fixed: false, Message: "no insecure clusters to fix"}, nil
	}

	updatedSecret := buildAutoImportSecret(clusterName, securedData)
	if _, err := m.client.Update(ctx, client.GVRSecret, clusterName, updatedSecret); err != nil {
		return nil, fmt.Errorf("updating auto-import-secret: %w", err)
	}

	return &FixTLSResult{
		ClusterName: clusterName,
		Server:      server,
		Fixed:       true,
		Message:     fmt.Sprintf("replaced insecure-skip-tls-verify with CA certificate for %d cluster(s)", fixed),
	}, nil
}

func (m *Manager) ListImports(ctx context.Context) ([]ImportedCluster, error) {
	m.logger.Info("discovery.ListImports")

	list, err := m.client.List(ctx, client.GVRManagedCluster, "", "acmlab.redhat.com/discovery=true")
	if err != nil {
		return nil, fmt.Errorf("listing imported clusters: %w", err)
	}

	result := make([]ImportedCluster, 0, len(list.Items))
	for _, item := range list.Items {
		labels := item.GetLabels()
		createdVia := labels["created-via"]
		clusterSet := labels["cluster.open-cluster-management.io/clusterset"]
		createdAt := item.GetCreationTimestamp().Format("2006-01-02T15:04:05Z")

		status := "Unknown"
		conditions, found, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
		if found {
			for _, cond := range conditions {
				c, ok := cond.(map[string]interface{})
				if !ok {
					continue
				}
				t, _, _ := unstructured.NestedString(c, "type")
				s, _, _ := unstructured.NestedString(c, "status")
				if t == "ManagedClusterConditionAvailable" {
					if s == "True" {
						status = "Available"
					} else {
						status = "Unavailable"
					}
					break
				}
			}
		}

		result = append(result, ImportedCluster{
			Name:       item.GetName(),
			CreatedVia: createdVia,
			ClusterSet: clusterSet,
			CreatedAt:  createdAt,
			Status:     status,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt > result[j].CreatedAt
	})

	return result, nil
}
