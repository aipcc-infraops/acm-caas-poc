package provisioning

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// redirectTransport rewrites every outgoing request to hit the test server
// instead of the hardcoded IBM Cloud URLs.
type redirectTransport struct {
	testServerURL string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	parsed := strings.TrimPrefix(t.testServerURL, "http://")
	req.URL.Scheme = "http"
	req.URL.Host = parsed
	return http.DefaultTransport.RoundTrip(req)
}

func newTestIAMClient(apiKey, serverURL string) *iamClient {
	c := newIAMClient(apiKey)
	c.http = &http.Client{
		Transport: &redirectTransport{testServerURL: serverURL},
	}
	return c
}

func TestNewIAMClient(t *testing.T) {
	c := newIAMClient("test-key")
	if c.apiKey != "test-key" {
		t.Errorf("apiKey = %s, want test-key", c.apiKey)
	}
	if c.http == nil {
		t.Error("http client should not be nil")
	}
}

func TestAuthenticate_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/identity/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "apikey=test-key") {
			t.Errorf("expected apikey in body, got %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-123"})
	})
	mux.HandleFunc("/v1/apikeys/details", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-123" {
			t.Errorf("expected Bearer tok-123, got %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"account_id": "acc-456"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestIAMClient("test-key", srv.URL)
	if err := c.authenticate(); err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}
	if c.token != "tok-123" {
		t.Errorf("token = %s, want tok-123", c.token)
	}
	if c.accountID != "acc-456" {
		t.Errorf("accountID = %s, want acc-456", c.accountID)
	}
}

func TestAuthenticate_TokenRequestFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("invalid key"))
	}))
	defer srv.Close()

	c := newTestIAMClient("bad-key", srv.URL)
	err := c.authenticate()
	if err == nil {
		t.Fatal("expected error for failed token request")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

func TestAuthenticate_FetchAccountIDFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/identity/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-123"})
	})
	mux.HandleFunc("/v1/apikeys/details", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestIAMClient("test-key", srv.URL)
	err := c.authenticate()
	if err == nil {
		t.Fatal("expected error when fetchAccountID fails")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected 403 in error, got: %v", err)
	}
}

func TestAuthenticate_HTTPError(t *testing.T) {
	// Point at a server that isn't listening
	c := newTestIAMClient("key", "http://127.0.0.1:1")
	err := c.authenticate()
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestFetchAccountID_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apikeys/details", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("wrong auth header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]string{"account_id": "acct-789"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "test-token"
	if err := c.fetchAccountID(); err != nil {
		t.Fatalf("fetchAccountID failed: %v", err)
	}
	if c.accountID != "acct-789" {
		t.Errorf("accountID = %s, want acct-789", c.accountID)
	}
}

func TestFetchAccountID_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	err := c.fetchAccountID()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 in error, got: %v", err)
	}
}

func TestCreateServiceID_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/serviceids", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "test-sid" {
			t.Errorf("name = %v, want test-sid", body["name"])
		}
		json.NewEncoder(w).Encode(serviceIDResponse{ID: "sid-1", IAMID: "iam-1"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	c.accountID = "acct"
	sid, err := c.createServiceID("test-sid", "test description")
	if err != nil {
		t.Fatalf("createServiceID failed: %v", err)
	}
	if sid.ID != "sid-1" {
		t.Errorf("ID = %s, want sid-1", sid.ID)
	}
	if sid.IAMID != "iam-1" {
		t.Errorf("IAMID = %s, want iam-1", sid.IAMID)
	}
}

func TestCreateServiceID_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	_, err := c.createServiceID("test", "desc")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreatePolicy_Success(t *testing.T) {
	tests := []struct {
		name string
		spec iamPolicySpec
	}{
		{
			name: "with service name",
			spec: iamPolicySpec{
				ServiceName: "is",
				Roles:       []string{"crn:v1:bluemix:public:iam::::role:Viewer"},
			},
		},
		{
			name: "with resource type",
			spec: iamPolicySpec{
				ResourceType: "resource-group",
				Roles:        []string{"crn:v1:bluemix:public:iam::::role:Viewer"},
			},
		},
		{
			name: "with both service and resource type",
			spec: iamPolicySpec{
				ServiceName:  "is",
				ResourceType: "resource-group",
				Roles:        []string{"crn:v1:bluemix:public:iam::::role:Editor"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("expected POST, got %s", r.Method)
				}
				var body map[string]interface{}
				json.NewDecoder(r.Body).Decode(&body)
				if body["type"] != "access" {
					t.Errorf("type = %v, want access", body["type"])
				}
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			c := newTestIAMClient("key", srv.URL)
			c.token = "tok"
			c.accountID = "acct"
			err := c.createPolicy("iam-id-1", tt.spec)
			if err != nil {
				t.Fatalf("createPolicy failed: %v", err)
			}
		})
	}
}

func TestCreatePolicy_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	c.accountID = "acct"
	err := c.createPolicy("iam-id", iamPolicySpec{
		ServiceName: "is",
		Roles:       []string{"crn:v1:bluemix:public:iam::::role:Viewer"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating policy for is") {
		t.Errorf("expected 'creating policy for is' in error, got: %v", err)
	}
}

func TestCreatePolicy_ErrorWithResourceType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	c.accountID = "acct"
	err := c.createPolicy("iam-id", iamPolicySpec{
		ResourceType: "resource-group",
		Roles:        []string{"crn:v1:bluemix:public:iam::::role:Viewer"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating policy for resource-group") {
		t.Errorf("expected 'creating policy for resource-group' in error, got: %v", err)
	}
}

func TestCreateAPIKey_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(apiKeyResponse{APIKey: "generated-key-123"})
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	c.accountID = "acct"
	key, err := c.createAPIKey("test-key", "iam-id-1")
	if err != nil {
		t.Fatalf("createAPIKey failed: %v", err)
	}
	if key != "generated-key-123" {
		t.Errorf("key = %s, want generated-key-123", key)
	}
}

func TestCreateAPIKey_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("error"))
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	c.accountID = "acct"
	_, err := c.createAPIKey("test-key", "iam-id")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteServiceID_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/v1/serviceids/sid-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	err := c.deleteServiceID("sid-1")
	if err != nil {
		t.Fatalf("deleteServiceID failed: %v", err)
	}
}

func TestDeleteServiceID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	err := c.deleteServiceID("missing")
	if err != nil {
		t.Fatalf("deleteServiceID should succeed for 404: %v", err)
	}
}

func TestDeleteServiceID_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	err := c.deleteServiceID("sid-1")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestListServiceIDs_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.RawQuery, "name=test-prefix") {
			t.Errorf("expected name=test-prefix in query, got %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"serviceids": []serviceIDResponse{
				{ID: "sid-1", IAMID: "iam-1"},
				{ID: "sid-2", IAMID: "iam-2"},
			},
		})
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	c.accountID = "acct"
	sids, err := c.listServiceIDs("test-prefix")
	if err != nil {
		t.Fatalf("listServiceIDs failed: %v", err)
	}
	if len(sids) != 2 {
		t.Fatalf("got %d service IDs, want 2", len(sids))
	}
	if sids[0].ID != "sid-1" {
		t.Errorf("first ID = %s, want sid-1", sids[0].ID)
	}
}

func TestListServiceIDs_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	c.accountID = "acct"
	_, err := c.listServiceIDs("prefix")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDoJSON_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-tok" {
			t.Errorf("Authorization = %s, want Bearer test-tok", r.Header.Get("Authorization"))
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "test-tok"
	resp, err := c.doJSON("POST", srv.URL+"/v1/test", map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatalf("doJSON failed: %v", err)
	}
	var result map[string]string
	json.Unmarshal(resp, &result)
	if result["result"] != "ok" {
		t.Errorf("result = %s, want ok", result["result"])
	}
}

func TestDoJSON_NonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("access denied"))
	}))
	defer srv.Close()

	c := newTestIAMClient("key", srv.URL)
	c.token = "tok"
	_, err := c.doJSON("POST", srv.URL+"/test", map[string]string{})
	if err == nil {
		t.Fatal("expected error for non-success status")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected 403 in error, got: %v", err)
	}
}

func TestDoJSON_HTTPError(t *testing.T) {
	c := newTestIAMClient("key", "http://127.0.0.1:1")
	c.token = "tok"
	_, err := c.doJSON("GET", "http://127.0.0.1:1/test", map[string]string{})
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}
