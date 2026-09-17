package provisioning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

func TestCreateIBMCloudWithoutManifestsDir(t *testing.T) {
	// This path triggers generateIBMCloudCredentials instead of reading from disk
	srv := iamMockServer(t)
	defer srv.Close()

	origNew := newIAMClient
	newIAMClient = func(apiKey string) *iamClient {
		c := origNew(apiKey)
		c.http = &http.Client{
			Transport: &redirectTransport{testServerURL: srv.URL},
		}
		return c
	}
	defer func() { newIAMClient = origNew }()

	c := fakeClient()
	m := New(c, testConfig(), discardLogger)

	err := m.Create(context.Background(), ClusterOpts{
		Name:       "ibm-cluster",
		PullSecret: `{"auths":{}}`,
		// No ManifestsDir - triggers generateIBMCloudCredentials path
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify manifests secret was created from generated creds
	_, err = c.Get(context.Background(), client.GVRSecret, "ibm-cluster", "ibm-cluster-manifests")
	if err != nil {
		t.Fatalf("manifests secret not created: %v", err)
	}
}

func TestCreateIBMCloudGenerateCredsFails(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("bad"))
	}))
	defer failSrv.Close()

	origNew := newIAMClient
	newIAMClient = func(apiKey string) *iamClient {
		c := origNew(apiKey)
		c.http = &http.Client{
			Transport: &redirectTransport{testServerURL: failSrv.URL},
		}
		return c
	}
	defer func() { newIAMClient = origNew }()

	c := fakeClient()
	m := New(c, testConfig(), discardLogger)

	err := m.Create(context.Background(), ClusterOpts{
		Name:       "ibm-fail",
		PullSecret: `{"auths":{}}`,
	})
	if err == nil {
		t.Fatal("expected error when generateIBMCloudCredentials fails")
	}
	if !strings.Contains(err.Error(), "IBM Cloud IAM") {
		t.Errorf("expected 'IBM Cloud IAM' in error, got: %v", err)
	}
}

func TestCreateIBMCloudMissingAPIKey(t *testing.T) {
	c := fakeClient()
	m := New(c, config.Config{Platform: "ibmcloud"}, discardLogger)

	err := m.Create(context.Background(), ClusterOpts{
		Name:       "spoke1",
		Platform:   "ibmcloud",
		PullSecret: `{"auths":{}}`,
	})
	if err == nil {
		t.Fatal("expected error for missing API key on ibmcloud")
	}
	if !strings.Contains(err.Error(), "IBM Cloud API key") {
		t.Errorf("expected 'IBM Cloud API key' message, got: %v", err)
	}
}

func TestCreateNamespaceError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("injected ns error")
	})
	m := New(c, testConfig(), discardLogger)

	err := m.Create(context.Background(), ClusterOpts{
		Name:       "spoke1",
		PullSecret: `{"auths":{}}`,
	})
	if err == nil || !strings.Contains(err.Error(), "creating namespace") {
		t.Fatalf("expected namespace error, got: %v", err)
	}
}

func TestCreateCredentialsSecretError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("injected secret error")
	})
	m := New(c, testConfig(), discardLogger)

	err := m.Create(context.Background(), ClusterOpts{
		Name:       "spoke1",
		PullSecret: `{"auths":{}}`,
	})
	if err == nil || !strings.Contains(err.Error(), "credentials secret") {
		t.Fatalf("expected credentials secret error, got: %v", err)
	}
}

func TestCreateWithBadManifestsDir(t *testing.T) {
	c := fakeClient()
	m := New(c, testConfig(), discardLogger)

	err := m.Create(context.Background(), ClusterOpts{
		Name:         "spoke1",
		PullSecret:   `{"auths":{}}`,
		ManifestsDir: "/nonexistent/path/that/does/not/exist",
	})
	if err == nil {
		t.Fatal("expected error for bad manifests dir")
	}
	if !strings.Contains(err.Error(), "manifests") {
		t.Errorf("expected 'manifests' in error, got: %v", err)
	}
}

func TestDestroyWithIBMCloudCleanup(t *testing.T) {
	srv := iamMockServer(t)
	defer srv.Close()

	origNew := newIAMClient
	newIAMClient = func(apiKey string) *iamClient {
		c := origNew(apiKey)
		c.http = &http.Client{
			Transport: &redirectTransport{testServerURL: srv.URL},
		}
		return c
	}
	defer func() { newIAMClient = origNew }()

	cd := &unstructured.Unstructured{}
	cd.SetGroupVersionKind(client.GVRClusterDeployment.GroupVersion().WithKind("ClusterDeployment"))
	cd.SetNamespace("spoke1")
	cd.SetName("spoke1")
	c := fakeClient(cd)
	cfg := testConfig()
	m := New(c, cfg, discardLogger)

	if err := m.Destroy(context.Background(), "spoke1"); err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}
}

func TestCleanupIBMCloudCredentials_ListError(t *testing.T) {
	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/identity/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/v1/apikeys/details", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"account_id": "acct"})
	})
	mux.HandleFunc("/v1/serviceids", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	origNew := newIAMClient
	newIAMClient = func(apiKey string) *iamClient {
		c := origNew(apiKey)
		c.http = &http.Client{
			Transport: &redirectTransport{testServerURL: srv.URL},
		}
		return c
	}
	defer func() { newIAMClient = origNew }()

	err := cleanupIBMCloudCredentials("key", "cluster")
	if err != nil {
		t.Fatalf("expected no error (list errors are swallowed), got: %v", err)
	}
	if callCount != len(ibmCloudCredentialRequests) {
		t.Errorf("expected %d list calls, got %d", len(ibmCloudCredentialRequests), callCount)
	}
}
