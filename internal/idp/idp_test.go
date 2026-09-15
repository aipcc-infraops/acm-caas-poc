package idp

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRManifestWork: "ManifestWorkList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) (*Manager, *client.Client) {
	c := fakeClient(objs...)
	return New(c, config.Config{}, discardLogger), c
}

func getManifests(t *testing.T, c *client.Client, cluster, mwName string) []interface{} {
	t.Helper()
	obj, err := c.Get(context.Background(), client.GVRManifestWork, cluster, mwName)
	if err != nil {
		t.Fatalf("ManifestWork %s not found: %v", mwName, err)
	}
	spec, _ := obj.Object["spec"].(map[string]interface{})
	workload, _ := spec["workload"].(map[string]interface{})
	manifests, _ := workload["manifests"].([]interface{})
	return manifests
}

func TestConfigureGitHub(t *testing.T) {
	mgr, c := newManager()
	opts := IdPOpts{
		Name:          "corp-github",
		Cluster:       "spoke1",
		Type:          IdPGitHub,
		ClientID:      "my-client-id",
		ClientSecret:  "my-secret",
		Organizations: []string{"myorg"},
	}
	if err := mgr.Configure(context.Background(), opts); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	obj, err := c.Get(context.Background(), client.GVRManifestWork, "spoke1", "idp-corp-github")
	if err != nil {
		t.Fatalf("ManifestWork not created: %v", err)
	}
	labels := obj.GetLabels()
	if labels[idpLabel] != "corp-github" {
		t.Errorf("idp label = %q, want corp-github", labels[idpLabel])
	}
	if labels["acmlab.redhat.com/type"] != "github" {
		t.Errorf("type label = %q, want github", labels["acmlab.redhat.com/type"])
	}

	manifests := getManifests(t, c, "spoke1", "idp-corp-github")
	if len(manifests) != 2 {
		t.Fatalf("manifests count = %d, want 2 (Secret + OAuth)", len(manifests))
	}

	secret, _ := manifests[0].(map[string]interface{})
	if secret["kind"] != "Secret" {
		t.Errorf("first manifest kind = %q, want Secret", secret["kind"])
	}
	data, _ := secret["data"].(map[string]interface{})
	decoded, _ := base64.StdEncoding.DecodeString(data["clientSecret"].(string))
	if string(decoded) != "my-secret" {
		t.Errorf("clientSecret = %q, want my-secret", string(decoded))
	}

	oauth, _ := manifests[1].(map[string]interface{})
	if oauth["kind"] != "OAuth" {
		t.Errorf("second manifest kind = %q, want OAuth", oauth["kind"])
	}
	spec, _ := oauth["spec"].(map[string]interface{})
	providers, _ := spec["identityProviders"].([]interface{})
	if len(providers) != 1 {
		t.Fatalf("providers count = %d, want 1", len(providers))
	}
	provider, _ := providers[0].(map[string]interface{})
	if provider["type"] != "GitHub" {
		t.Errorf("provider type = %q, want GitHub", provider["type"])
	}
	github, _ := provider["github"].(map[string]interface{})
	orgs, _ := github["organizations"].([]interface{})
	if len(orgs) != 1 || orgs[0] != "myorg" {
		t.Errorf("organizations = %v, want [myorg]", orgs)
	}
}

func TestConfigureGoogle(t *testing.T) {
	mgr, c := newManager()
	opts := IdPOpts{
		Name:         "corp-google",
		Cluster:      "spoke1",
		Type:         IdPGoogle,
		ClientID:     "google-client-id",
		ClientSecret: "google-secret",
	}
	if err := mgr.Configure(context.Background(), opts); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	manifests := getManifests(t, c, "spoke1", "idp-corp-google")
	oauth, _ := manifests[1].(map[string]interface{})
	spec, _ := oauth["spec"].(map[string]interface{})
	providers, _ := spec["identityProviders"].([]interface{})
	provider, _ := providers[0].(map[string]interface{})
	if provider["type"] != "Google" {
		t.Errorf("provider type = %q, want Google", provider["type"])
	}
	google, _ := provider["google"].(map[string]interface{})
	if google["clientID"] != "google-client-id" {
		t.Errorf("clientID = %q, want google-client-id", google["clientID"])
	}
}

func TestConfigureHTPasswd(t *testing.T) {
	mgr, c := newManager()
	opts := IdPOpts{
		Name:    "local-users",
		Cluster: "spoke1",
		Type:    IdPHTPasswd,
		Users:   map[string]string{"admin": "pass123", "dev": "devpass"},
	}
	if err := mgr.Configure(context.Background(), opts); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	manifests := getManifests(t, c, "spoke1", "idp-local-users")
	secret, _ := manifests[0].(map[string]interface{})
	data, _ := secret["data"].(map[string]interface{})
	decoded, _ := base64.StdEncoding.DecodeString(data["htpasswd"].(string))
	content := string(decoded)
	lines := strings.Split(content, "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "admin:$2a$") || !strings.HasPrefix(lines[1], "dev:$2a$") {
		t.Errorf("htpasswd should have bcrypt hashes for admin and dev, got %q", content)
	}
	parts := strings.SplitN(lines[0], ":", 2)
	if err := bcrypt.CompareHashAndPassword([]byte(parts[1]), []byte("pass123")); err != nil {
		t.Errorf("admin password hash verification failed: %v", err)
	}

	oauth, _ := manifests[1].(map[string]interface{})
	spec, _ := oauth["spec"].(map[string]interface{})
	providers, _ := spec["identityProviders"].([]interface{})
	provider, _ := providers[0].(map[string]interface{})
	if provider["type"] != "HTPasswd" {
		t.Errorf("provider type = %q, want HTPasswd", provider["type"])
	}
}

func TestConfigureLDAP(t *testing.T) {
	mgr, c := newManager()
	opts := IdPOpts{
		Name:         "corp-ldap",
		Cluster:      "spoke1",
		Type:         IdPLDAP,
		LDAPURL:      "ldap://ldap.example.com:389/ou=users,dc=example,dc=com?uid",
		BindDN:       "cn=admin,dc=example,dc=com",
		BindPassword: "ldap-secret",
		Insecure:     true,
	}
	if err := mgr.Configure(context.Background(), opts); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	manifests := getManifests(t, c, "spoke1", "idp-corp-ldap")
	secret, _ := manifests[0].(map[string]interface{})
	data, _ := secret["data"].(map[string]interface{})
	decoded, _ := base64.StdEncoding.DecodeString(data["bindPassword"].(string))
	if string(decoded) != "ldap-secret" {
		t.Errorf("bindPassword = %q, want ldap-secret", string(decoded))
	}

	oauth, _ := manifests[1].(map[string]interface{})
	spec, _ := oauth["spec"].(map[string]interface{})
	providers, _ := spec["identityProviders"].([]interface{})
	provider, _ := providers[0].(map[string]interface{})
	if provider["type"] != "LDAP" {
		t.Errorf("provider type = %q, want LDAP", provider["type"])
	}
	ldap, _ := provider["ldap"].(map[string]interface{})
	if ldap["url"] != "ldap://ldap.example.com:389/ou=users,dc=example,dc=com?uid" {
		t.Errorf("ldap url = %q", ldap["url"])
	}
	if ldap["insecure"] != true {
		t.Errorf("insecure = %v, want true", ldap["insecure"])
	}
}

func TestConfigureOIDC(t *testing.T) {
	mgr, c := newManager()
	opts := IdPOpts{
		Name:         "keycloak",
		Cluster:      "spoke1",
		Type:         IdPOIDC,
		ClientID:     "kc-client",
		ClientSecret: "kc-secret",
		IssuerURL:    "https://keycloak.example.com/realms/myrealm",
	}
	if err := mgr.Configure(context.Background(), opts); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	manifests := getManifests(t, c, "spoke1", "idp-keycloak")
	oauth, _ := manifests[1].(map[string]interface{})
	spec, _ := oauth["spec"].(map[string]interface{})
	providers, _ := spec["identityProviders"].([]interface{})
	provider, _ := providers[0].(map[string]interface{})
	if provider["type"] != "OpenID" {
		t.Errorf("provider type = %q, want OpenID", provider["type"])
	}
	oidc, _ := provider["openID"].(map[string]interface{})
	if oidc["issuer"] != "https://keycloak.example.com/realms/myrealm" {
		t.Errorf("issuer = %q", oidc["issuer"])
	}
	if oidc["clientID"] != "kc-client" {
		t.Errorf("clientID = %q, want kc-client", oidc["clientID"])
	}
	claims, _ := oidc["claims"].(map[string]interface{})
	if claims["preferredUsername"] == nil {
		t.Error("missing preferredUsername claim")
	}
}

func TestConfigureIdempotent(t *testing.T) {
	mgr, _ := newManager()
	opts := IdPOpts{
		Name:         "dup",
		Cluster:      "spoke1",
		Type:         IdPGitHub,
		ClientID:     "id",
		ClientSecret: "secret",
	}
	if err := mgr.Configure(context.Background(), opts); err != nil {
		t.Fatalf("first Configure failed: %v", err)
	}
	if err := mgr.Configure(context.Background(), opts); err != nil {
		t.Fatalf("second Configure should be idempotent: %v", err)
	}
}

func TestRemove(t *testing.T) {
	mgr, c := newManager()
	opts := IdPOpts{Name: "to-remove", Cluster: "spoke1", Type: IdPGitHub, ClientID: "x", ClientSecret: "y"}
	if err := mgr.Configure(context.Background(), opts); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}
	if err := mgr.Remove(context.Background(), "to-remove", "spoke1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	_, err := c.Get(context.Background(), client.GVRManifestWork, "spoke1", "idp-to-remove")
	if err == nil {
		t.Error("ManifestWork should be deleted")
	}
}

func TestRemoveNotFound(t *testing.T) {
	mgr, _ := newManager()
	if err := mgr.Remove(context.Background(), "nonexistent", "spoke1"); err != nil {
		t.Fatalf("Remove of non-existent should succeed (idempotent): %v", err)
	}
}

func TestList(t *testing.T) {
	mw := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "idp-corp-github",
				"namespace": "spoke1",
				"labels": map[string]interface{}{
					idpLabel:                "corp-github",
					"acmlab.redhat.com/type": "github",
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Applied", "status": "True"},
				},
			},
		},
	}
	mgr, _ := newManager(mw)
	infos, err := mgr.List(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("count = %d, want 1", len(infos))
	}
	if infos[0].Name != "corp-github" {
		t.Errorf("name = %q, want corp-github", infos[0].Name)
	}
	if infos[0].Type != "github" {
		t.Errorf("type = %q, want github", infos[0].Type)
	}
	if infos[0].Status != "Applied" {
		t.Errorf("status = %q, want Applied", infos[0].Status)
	}
	if infos[0].Cluster != "spoke1" {
		t.Errorf("cluster = %q, want spoke1", infos[0].Cluster)
	}
}

func TestListEmpty(t *testing.T) {
	mgr, _ := newManager()
	infos, err := mgr.List(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("count = %d, want 0", len(infos))
	}
}

func TestListPending(t *testing.T) {
	mw := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "idp-pending",
				"namespace": "spoke1",
				"labels": map[string]interface{}{
					idpLabel:                "pending",
					"acmlab.redhat.com/type": "google",
				},
			},
		},
	}
	mgr, _ := newManager(mw)
	infos, err := mgr.List(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if infos[0].Status != "Pending" {
		t.Errorf("status = %q, want Pending", infos[0].Status)
	}
}

func TestRotate(t *testing.T) {
	mgr, c := newManager()
	opts := IdPOpts{
		Name:         "rotate-me",
		Cluster:      "spoke1",
		Type:         IdPGitHub,
		ClientID:     "old-id",
		ClientSecret: "old-secret",
	}
	if err := mgr.Configure(context.Background(), opts); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	newOpts := IdPOpts{
		Type:         IdPGitHub,
		ClientID:     "new-id",
		ClientSecret: "new-secret",
	}
	if err := mgr.Rotate(context.Background(), "rotate-me", "spoke1", newOpts); err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}

	manifests := getManifests(t, c, "spoke1", "idp-rotate-me")
	secret, _ := manifests[0].(map[string]interface{})
	data, _ := secret["data"].(map[string]interface{})
	decoded, _ := base64.StdEncoding.DecodeString(data["clientSecret"].(string))
	if string(decoded) != "new-secret" {
		t.Errorf("rotated clientSecret = %q, want new-secret", string(decoded))
	}
}

func TestRotateNotFound(t *testing.T) {
	mgr, _ := newManager()
	opts := IdPOpts{Type: IdPGitHub, ClientSecret: "new"}
	err := mgr.Rotate(context.Background(), "missing", "spoke1", opts)
	if err == nil {
		t.Error("Rotate of non-existent should fail")
	}
}

func TestRotateInheritsType(t *testing.T) {
	mgr, _ := newManager()
	opts := IdPOpts{
		Name:         "typed",
		Cluster:      "spoke1",
		Type:         IdPGoogle,
		ClientID:     "gid",
		ClientSecret: "gsecret",
	}
	if err := mgr.Configure(context.Background(), opts); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}
	newOpts := IdPOpts{ClientID: "new-gid", ClientSecret: "new-gsecret"}
	if err := mgr.Rotate(context.Background(), "typed", "spoke1", newOpts); err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}
}

func TestBuildOAuthGitHub(t *testing.T) {
	opts := IdPOpts{Name: "gh", Type: IdPGitHub, ClientID: "cid", Organizations: []string{"org1", "org2"}}
	oauth := buildOAuthManifest(opts)
	if oauth["kind"] != "OAuth" {
		t.Errorf("kind = %q", oauth["kind"])
	}
	spec, _ := oauth["spec"].(map[string]interface{})
	providers, _ := spec["identityProviders"].([]interface{})
	p, _ := providers[0].(map[string]interface{})
	gh, _ := p["github"].(map[string]interface{})
	orgs, _ := gh["organizations"].([]interface{})
	if len(orgs) != 2 {
		t.Errorf("orgs count = %d, want 2", len(orgs))
	}
}

func TestBuildOAuthGoogle(t *testing.T) {
	opts := IdPOpts{Name: "g", Type: IdPGoogle, ClientID: "gcid"}
	oauth := buildOAuthManifest(opts)
	spec, _ := oauth["spec"].(map[string]interface{})
	providers, _ := spec["identityProviders"].([]interface{})
	p, _ := providers[0].(map[string]interface{})
	if p["type"] != "Google" {
		t.Errorf("type = %q", p["type"])
	}
}

func TestBuildOAuthHTPasswd(t *testing.T) {
	opts := IdPOpts{Name: "ht", Type: IdPHTPasswd}
	oauth := buildOAuthManifest(opts)
	spec, _ := oauth["spec"].(map[string]interface{})
	providers, _ := spec["identityProviders"].([]interface{})
	p, _ := providers[0].(map[string]interface{})
	if p["type"] != "HTPasswd" {
		t.Errorf("type = %q", p["type"])
	}
}

func TestBuildOAuthLDAP(t *testing.T) {
	opts := IdPOpts{Name: "ld", Type: IdPLDAP, LDAPURL: "ldap://example.com", BindDN: "cn=admin"}
	oauth := buildOAuthManifest(opts)
	spec, _ := oauth["spec"].(map[string]interface{})
	providers, _ := spec["identityProviders"].([]interface{})
	p, _ := providers[0].(map[string]interface{})
	if p["type"] != "LDAP" {
		t.Errorf("type = %q", p["type"])
	}
	ldap, _ := p["ldap"].(map[string]interface{})
	attrs, _ := ldap["attributes"].(map[string]interface{})
	if attrs["preferredUsername"] == nil {
		t.Error("missing preferredUsername attribute")
	}
}

func TestBuildOAuthOIDC(t *testing.T) {
	opts := IdPOpts{Name: "kc", Type: IdPOIDC, ClientID: "kcid", IssuerURL: "https://kc.example.com/realms/r"}
	oauth := buildOAuthManifest(opts)
	spec, _ := oauth["spec"].(map[string]interface{})
	providers, _ := spec["identityProviders"].([]interface{})
	p, _ := providers[0].(map[string]interface{})
	if p["type"] != "OpenID" {
		t.Errorf("type = %q", p["type"])
	}
	oidc, _ := p["openID"].(map[string]interface{})
	if oidc["issuer"] != "https://kc.example.com/realms/r" {
		t.Errorf("issuer = %q", oidc["issuer"])
	}
}

func TestBuildSecretGitHub(t *testing.T) {
	opts := IdPOpts{Name: "gh", Type: IdPGitHub, ClientSecret: "mysecret"}
	s := buildSecretManifest(opts)
	if s["kind"] != "Secret" {
		t.Errorf("kind = %q", s["kind"])
	}
	meta, _ := s["metadata"].(map[string]interface{})
	if meta["namespace"] != "openshift-config" {
		t.Errorf("namespace = %q, want openshift-config", meta["namespace"])
	}
	if meta["name"] != "gh-secret" {
		t.Errorf("name = %q, want gh-secret", meta["name"])
	}
	data, _ := s["data"].(map[string]interface{})
	decoded, _ := base64.StdEncoding.DecodeString(data["clientSecret"].(string))
	if string(decoded) != "mysecret" {
		t.Errorf("clientSecret = %q", string(decoded))
	}
}

func TestBuildSecretHTPasswd(t *testing.T) {
	opts := IdPOpts{Name: "ht", Type: IdPHTPasswd, Users: map[string]string{"bob": "pw1", "alice": "pw2"}}
	s := buildSecretManifest(opts)
	data, _ := s["data"].(map[string]interface{})
	decoded, _ := base64.StdEncoding.DecodeString(data["htpasswd"].(string))
	content := string(decoded)
	lines := strings.Split(content, "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "alice:$2a$") || !strings.HasPrefix(lines[1], "bob:$2a$") {
		t.Errorf("htpasswd should have sorted bcrypt entries for alice and bob, got %q", content)
	}
}

func TestBuildSecretHTPasswdEmpty(t *testing.T) {
	opts := IdPOpts{Name: "ht", Type: IdPHTPasswd, Users: map[string]string{}}
	s := buildSecretManifest(opts)
	data, _ := s["data"].(map[string]interface{})
	decoded, _ := base64.StdEncoding.DecodeString(data["htpasswd"].(string))
	if string(decoded) != "" {
		t.Errorf("htpasswd should be empty for no users, got %q", string(decoded))
	}
}

func TestBuildSecretLDAP(t *testing.T) {
	opts := IdPOpts{Name: "ld", Type: IdPLDAP, BindPassword: "ldappw"}
	s := buildSecretManifest(opts)
	data, _ := s["data"].(map[string]interface{})
	decoded, _ := base64.StdEncoding.DecodeString(data["bindPassword"].(string))
	if string(decoded) != "ldappw" {
		t.Errorf("bindPassword = %q", string(decoded))
	}
}

func TestBuildHTPasswdData(t *testing.T) {
	users := map[string]string{"charlie": "pw3", "alice": "pw1", "bob": "pw2"}
	result := buildHTPasswdData(users)
	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	expected := []string{"alice", "bob", "charlie"}
	passwords := []string{"pw1", "pw2", "pw3"}
	for i, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if parts[0] != expected[i] {
			t.Errorf("line %d user = %q, want %q", i, parts[0], expected[i])
		}
		if err := bcrypt.CompareHashAndPassword([]byte(parts[1]), []byte(passwords[i])); err != nil {
			t.Errorf("line %d password verification failed: %v", i, err)
		}
	}
}

func TestBuildHTPasswdDataEmpty(t *testing.T) {
	result := buildHTPasswdData(nil)
	if result != "" {
		t.Errorf("htpasswd data = %q, want empty", result)
	}
}

func TestManifestWorkName(t *testing.T) {
	if manifestWorkName("github") != "idp-github" {
		t.Errorf("manifestWorkName = %q", manifestWorkName("github"))
	}
}

func TestSecretName(t *testing.T) {
	if secretName("gh") != "gh-secret" {
		t.Errorf("secretName = %q", secretName("gh"))
	}
}

func TestParseIdPInfoNoStatus(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "spoke1",
			"labels": map[string]interface{}{
				idpLabel:                "test",
				"acmlab.redhat.com/type": "oidc",
			},
		},
	}
	info := parseIdPInfo(obj)
	if info.Status != "Pending" {
		t.Errorf("status = %q, want Pending", info.Status)
	}
	if info.Type != "oidc" {
		t.Errorf("type = %q, want oidc", info.Type)
	}
}

func TestBuildOAuthDefaultType(t *testing.T) {
	opts := IdPOpts{Name: "unknown", Type: "unsupported", ClientID: "x"}
	oauth := buildOAuthManifest(opts)
	spec, _ := oauth["spec"].(map[string]interface{})
	providers, _ := spec["identityProviders"].([]interface{})
	p, _ := providers[0].(map[string]interface{})
	if p["type"] != "GitHub" {
		t.Errorf("default type = %q, want GitHub", p["type"])
	}
}

func TestBuildSecretOIDC(t *testing.T) {
	opts := IdPOpts{Name: "kc", Type: IdPOIDC, ClientSecret: "oidcsecret"}
	s := buildSecretManifest(opts)
	data, _ := s["data"].(map[string]interface{})
	decoded, _ := base64.StdEncoding.DecodeString(data["clientSecret"].(string))
	if string(decoded) != "oidcsecret" {
		t.Errorf("clientSecret = %q, want oidcsecret", string(decoded))
	}
}

func TestGitHubNoOrganizations(t *testing.T) {
	opts := IdPOpts{Name: "gh", Type: IdPGitHub, ClientID: "cid"}
	entry := githubIdPEntry(opts)
	gh, _ := entry["github"].(map[string]interface{})
	if _, ok := gh["organizations"]; ok {
		t.Error("organizations should be absent when empty")
	}
}
