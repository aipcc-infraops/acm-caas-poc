package provisioning

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// iamMockServer creates an httptest server that handles all IAM endpoints
// needed for generateIBMCloudCredentials and cleanupIBMCloudCredentials.
func iamMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	serviceIDCounter := 0
	mux := http.NewServeMux()

	mux.HandleFunc("/identity/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "mock-token"})
	})
	mux.HandleFunc("/v1/apikeys/details", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"account_id": "mock-account"})
	})
	mux.HandleFunc("/v1/serviceids/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/v1/serviceids", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			serviceIDCounter++
			json.NewEncoder(w).Encode(serviceIDResponse{
				ID:    "sid-" + string(rune('0'+serviceIDCounter)),
				IAMID: "iam-" + string(rune('0'+serviceIDCounter)),
			})
		case "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"serviceids": []serviceIDResponse{
					{ID: "sid-cleanup-1", IAMID: "iam-cleanup-1"},
				},
			})
		}
	})
	mux.HandleFunc("/v1/policies", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/v1/apikeys", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(apiKeyResponse{APIKey: "generated-api-key"})
	})

	return httptest.NewServer(mux)
}

func TestGenerateIBMCloudCredentials_Success(t *testing.T) {
	srv := iamMockServer(t)
	defer srv.Close()

	// Temporarily override the iamClient factory to use our test server
	origNew := newIAMClient
	newIAMClient = func(apiKey string) *iamClient {
		c := origNew(apiKey)
		c.http = &http.Client{
			Transport: &redirectTransport{testServerURL: srv.URL},
		}
		return c
	}
	defer func() { newIAMClient = origNew }()

	creds, err := generateIBMCloudCredentials("test-api-key", "test-cluster")
	if err != nil {
		t.Fatalf("generateIBMCloudCredentials failed: %v", err)
	}
	if len(creds) != len(ibmCloudCredentialRequests) {
		t.Fatalf("got %d credentials, want %d", len(creds), len(ibmCloudCredentialRequests))
	}
	for i, cred := range creds {
		if cred.SecretName != ibmCloudCredentialRequests[i].SecretName {
			t.Errorf("cred[%d].SecretName = %s, want %s", i, cred.SecretName, ibmCloudCredentialRequests[i].SecretName)
		}
		if cred.Namespace != ibmCloudCredentialRequests[i].Namespace {
			t.Errorf("cred[%d].Namespace = %s, want %s", i, cred.Namespace, ibmCloudCredentialRequests[i].Namespace)
		}
		if cred.APIKey != "generated-api-key" {
			t.Errorf("cred[%d].APIKey = %s, want generated-api-key", i, cred.APIKey)
		}
	}
}

func TestGenerateIBMCloudCredentials_AuthFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("bad key"))
	}))
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

	_, err := generateIBMCloudCredentials("bad-key", "cluster")
	if err == nil {
		t.Fatal("expected error for auth failure")
	}
	if !strings.Contains(err.Error(), "authenticating") {
		t.Errorf("expected 'authenticating' in error, got: %v", err)
	}
}

func TestGenerateIBMCloudCredentials_CreateServiceIDFail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/identity/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/v1/apikeys/details", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"account_id": "acct"})
	})
	mux.HandleFunc("/v1/serviceids", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("fail"))
			return
		}
	})
	// Handle cleanup delete calls
	mux.HandleFunc("/v1/serviceids/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
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

	_, err := generateIBMCloudCredentials("key", "cluster")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCleanupIBMCloudCredentials_Success(t *testing.T) {
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

	err := cleanupIBMCloudCredentials("test-key", "test-cluster")
	if err != nil {
		t.Fatalf("cleanupIBMCloudCredentials failed: %v", err)
	}
}

func TestCleanupIBMCloudCredentials_AuthFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("bad"))
	}))
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

	err := cleanupIBMCloudCredentials("bad-key", "cluster")
	if err == nil {
		t.Fatal("expected error for auth failure")
	}
}

func TestCleanupServiceIDs(t *testing.T) {
	deletedIDs := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			parts := strings.Split(r.URL.Path, "/")
			deletedIDs = append(deletedIDs, parts[len(parts)-1])
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	cleanupServiceIDs(c, []string{"sid-1", "sid-2", "sid-3"})

	if len(deletedIDs) != 3 {
		t.Fatalf("deleted %d IDs, want 3", len(deletedIDs))
	}
}

func TestCleanupServiceIDs_Empty(t *testing.T) {
	c := newIAMClient("key")
	// Should not panic with empty slice
	cleanupServiceIDs(c, []string{})
}

func TestBuildManifestYAMLs(t *testing.T) {
	creds := []componentCredential{
		{SecretName: "ibm-cloud-credentials", Namespace: "openshift-cloud-controller-manager", APIKey: "key-1"},
		{SecretName: "ibmcloud-credentials", Namespace: "openshift-machine-api", APIKey: "key-2"},
	}
	yamls := buildManifestYAMLs(creds)

	if len(yamls) != 2 {
		t.Fatalf("got %d manifests, want 2", len(yamls))
	}

	for _, cred := range creds {
		filename := cred.Namespace + "-" + cred.SecretName + "-credentials.yaml"
		yaml, ok := yamls[filename]
		if !ok {
			t.Errorf("missing manifest for %s", filename)
			continue
		}
		if !strings.Contains(yaml, "name: "+cred.SecretName) {
			t.Errorf("manifest %s missing secret name", filename)
		}
		if !strings.Contains(yaml, "namespace: "+cred.Namespace) {
			t.Errorf("manifest %s missing namespace", filename)
		}
		if !strings.Contains(yaml, cred.APIKey) {
			t.Errorf("manifest %s missing API key", filename)
		}
	}
}

func TestBuildManifestYAMLs_Empty(t *testing.T) {
	yamls := buildManifestYAMLs(nil)
	if len(yamls) != 0 {
		t.Errorf("expected empty map, got %d entries", len(yamls))
	}
}
