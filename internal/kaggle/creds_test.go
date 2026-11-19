package kaggle

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeOAuthFile(t *testing.T, dir string, doc map[string]any) string {
	t.Helper()
	path := filepath.Join(dir, "credentials.json")
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadCredsFromOAuthFile(t *testing.T) {
	t.Setenv("KAGGLE_USERNAME", "")
	t.Setenv("KAGGLE_KEY", "")
	t.Setenv("KAGGLE_API_TOKEN", "")
	dir := t.TempDir()
	t.Setenv("KAGGLE_CONFIG_DIR", dir)

	writeOAuthFile(t, dir, map[string]any{
		"refresh_token":           "refresh",
		"access_token":            "live-token",
		"access_token_expiration": time.Now().Add(6 * time.Hour).Format(time.RFC3339),
		"username":                "me",
	})

	creds, err := LoadCreds()
	if err != nil {
		t.Fatal(err)
	}
	if creds.Token != "live-token" || creds.Username != "me" {
		t.Fatalf("creds = %+v", creds)
	}
}

func TestLoadCredsRefreshesExpiredOAuthToken(t *testing.T) {
	t.Setenv("KAGGLE_USERNAME", "")
	t.Setenv("KAGGLE_KEY", "")
	t.Setenv("KAGGLE_API_TOKEN", "")
	dir := t.TempDir()
	t.Setenv("KAGGLE_CONFIG_DIR", dir)
	path := writeOAuthFile(t, dir, map[string]any{
		"refresh_token":           "refresh",
		"access_token":            "stale-token",
		"access_token_expiration": time.Now().Add(-time.Hour).Format(time.RFC3339),
		"username":                "me",
	})

	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"token":"fresh-token","expiresIn":43200,"userName":"me"}`))
	}))
	defer srv.Close()

	saved := authBaseURL
	authBaseURL = srv.URL
	defer func() { authBaseURL = saved }()

	creds, err := LoadCreds()
	if err != nil {
		t.Fatal(err)
	}
	if creds.Token != "fresh-token" {
		t.Fatalf("token = %q", creds.Token)
	}
	if gotPath != "/v1/users.AccountService/GenerateAccessToken" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["refreshToken"] != "refresh" || gotBody["apiVersion"] != "API_VERSION_V1" {
		t.Errorf("body = %v", gotBody)
	}

	// the refreshed token must be cached for the next run
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["access_token"] != "fresh-token" {
		t.Errorf("cached access_token = %v", doc["access_token"])
	}
}

func TestClientUsesBearerWhenTokenPresent(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{"kernels":[]}`))
	}))
	defer srv.Close()

	c := New(Creds{Token: "tok"})
	c.BaseURL = srv.URL
	if _, _, err := c.ListKernels("", "", 1); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("authorization = %q", gotAuth)
	}
}
