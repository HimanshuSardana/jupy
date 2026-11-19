package kaggle

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestListKernels(t *testing.T) {
	var gotPath, gotMethod, gotUser, gotPass string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotUser, gotPass, _ = r.BasicAuth()
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"kernels":[
			{"ref":"me/titanic-eda","title":"Titanic EDA","author":"me","slug":"titanic-eda",
			 "language":"python","kernelType":"notebook","isPrivate":true,"enableGpu":true,
			 "totalVotes":12,"currentVersionNumber":3,"lastRunTime":"2026-02-03T04:05:06.789Z"},
			{"ref":"me/xgb","title":"XGB baseline","author":"me","slug":"xgb","language":"r"}
		],"nextPageToken":"tok2"}`))
	}))
	defer srv.Close()

	c := New(Creds{Username: "me", Key: "secret"})
	c.BaseURL = srv.URL

	kernels, next, err := c.ListKernels("", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if want := "/v1/kernels.KernelsApiService/ListKernels"; gotPath != want {
		t.Errorf("path = %s, want %s", gotPath, want)
	}
	if gotUser != "me" || gotPass != "secret" {
		t.Errorf("basic auth = %q/%q", gotUser, gotPass)
	}
	if gotBody["group"] != "PROFILE" || gotBody["sortBy"] != "DATE_RUN" || gotBody["pageSize"] != float64(50) {
		t.Errorf("request body = %v", gotBody)
	}
	if _, ok := gotBody["user"]; ok {
		t.Error("user should be omitted for own profile")
	}
	if next != "tok2" || len(kernels) != 2 {
		t.Fatalf("next = %q, kernels = %d", next, len(kernels))
	}
	first := kernels[0]
	if first.Title != "Titanic EDA" || !first.Private || !first.GPU || first.Version != 3 || first.Votes != 12 {
		t.Errorf("first kernel = %+v", first)
	}
	if got := first.LastRunTime().UTC().Format("2006-01-02T15:04:05"); got != "2026-02-03T04:05:06" {
		t.Errorf("last run = %q", got)
	}
	if got := first.Badges(); got != "python GPU private" {
		t.Errorf("badges = %q", got)
	}
	if kernels[1].Badges() != "r" {
		t.Errorf("badges = %q", kernels[1].Badges())
	}
}

func TestListKernelsForOtherUserSendsUser(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"kernels":[]}`))
	}))
	defer srv.Close()

	c := New(Creds{Username: "me", Key: "k"})
	c.BaseURL = srv.URL
	if _, _, err := c.ListKernels("someone", "page2", 10); err != nil {
		t.Fatal(err)
	}
	if gotBody["user"] != "someone" || gotBody["pageToken"] != "page2" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestGetKernelReturnsNotebookSource(t *testing.T) {
	nb, err := os.ReadFile(filepath.Join("..", "notebook", "testdata", "sample.ipynb"))
	if err != nil {
		t.Fatal(err)
	}
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/kernels.KernelsApiService/GetKernel" {
			t.Errorf("path = %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &gotBody)
		resp := map[string]any{
			"metadata": map[string]any{"ref": "me/titanic-eda", "title": "Titanic EDA", "slug": "titanic-eda"},
			"blob":     map[string]any{"source": string(nb), "language": "python", "kernelType": "notebook"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(Creds{Username: "me", Key: "k"})
	c.BaseURL = srv.URL

	meta, source, err := c.GetKernel("me", "titanic-eda")
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["userName"] != "me" || gotBody["kernelSlug"] != "titanic-eda" {
		t.Errorf("body = %v", gotBody)
	}
	if meta.Title != "Titanic EDA" {
		t.Errorf("meta = %+v", meta)
	}
	if string(source) != string(nb) {
		t.Errorf("source = %q", source)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"unauthenticated", 401, `{"error":{"code":401,"message":"Unauthenticated","status":"UNAUTHENTICATED"}}`, "kaggle: 401 Unauthenticated"},
		{"plain", 500, `boom`, "kaggle: 500 boom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c := New(Creds{Username: "me", Key: "k"})
			c.BaseURL = srv.URL
			_, _, err := c.ListKernels("", "", 1)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestGetKernelEmptySource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"metadata":{"ref":"me/x"},"blob":{}}`))
	}))
	defer srv.Close()

	c := New(Creds{Username: "me", Key: "k"})
	c.BaseURL = srv.URL
	if _, _, err := c.GetKernel("me", "x"); err == nil {
		t.Fatal("want error for empty source")
	}
}

func TestLoadCredsFromEnvAndFile(t *testing.T) {
	t.Setenv("KAGGLE_USERNAME", "envuser")
	t.Setenv("KAGGLE_KEY", "envkey")
	creds, err := LoadCreds()
	if err != nil || creds.Username != "envuser" || creds.Key != "envkey" {
		t.Fatalf("creds = %+v, err = %v", creds, err)
	}

	dir := t.TempDir()
	t.Setenv("KAGGLE_USERNAME", "")
	t.Setenv("KAGGLE_KEY", "")
	t.Setenv("KAGGLE_CONFIG_DIR", dir)

	if _, err := LoadCreds(); err == nil {
		t.Fatal("want error when no credentials exist")
	}

	if err := os.WriteFile(filepath.Join(dir, "kaggle.json"), []byte(`{"username":"fileuser","key":"filekey"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	creds, err = LoadCreds()
	if err != nil || creds.Username != "fileuser" || creds.Key != "filekey" {
		t.Fatalf("creds = %+v, err = %v", creds, err)
	}
}
