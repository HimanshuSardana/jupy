package kaggle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// authBaseURL is a variable so tests can point token refresh at a fake server.
var authBaseURL = DefaultBaseURL

// Creds hold whichever credential the caller has: an API key (HTTP Basic) or
// an OAuth access token (Bearer).
type Creds struct {
	Username string
	Key      string
	Token    string
}

func (c Creds) String() string {
	switch {
	case c.Token != "":
		return "bearer token"
	case c.Key != "":
		return "api key for " + c.Username
	}
	return "none"
}

// LoadCreds looks at, in order: KAGGLE_USERNAME/KAGGLE_KEY, KAGGLE_API_TOKEN,
// $KAGGLE_CONFIG_DIR/kaggle.json, $KAGGLE_CONFIG_DIR/credentials.json (the OAuth
// file written by `kaggle auth login`, refreshed when expired).
func LoadCreds() (Creds, error) {
	var tried []string

	if u, k := os.Getenv("KAGGLE_USERNAME"), os.Getenv("KAGGLE_KEY"); u != "" && k != "" {
		return Creds{Username: u, Key: k}, nil
	}
	tried = append(tried, "KAGGLE_USERNAME/KAGGLE_KEY")

	if token := os.Getenv("KAGGLE_API_TOKEN"); token != "" {
		return Creds{Token: token}, nil
	}
	tried = append(tried, "KAGGLE_API_TOKEN")

	dir := configDir()

	apiKeyPath := filepath.Join(dir, "kaggle.json")
	tried = append(tried, apiKeyPath)
	if creds, err := readAPIKeyFile(apiKeyPath); err == nil {
		return creds, nil
	}

	oauthPath := filepath.Join(dir, "credentials.json")
	tried = append(tried, oauthPath)
	creds, err := readOAuthFile(oauthPath)
	if err == nil {
		return creds, nil
	}

	return Creds{}, fmt.Errorf("no usable Kaggle credentials (tried %s): %w\n  run `kaggle auth login`, or write %s",
		strings.Join(tried, ", "), err, apiKeyPath)
}

func configDir() string {
	if dir := os.Getenv("KAGGLE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".kaggle"
	}
	return filepath.Join(home, ".kaggle")
}

func readAPIKeyFile(path string) (Creds, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Creds{}, err
	}
	var file struct {
		Username string `json:"username"`
		Key      string `json:"key"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return Creds{}, fmt.Errorf("%s: %w", path, err)
	}
	if file.Username == "" || file.Key == "" {
		return Creds{}, fmt.Errorf("%s: username or key is empty", path)
	}
	return Creds{Username: file.Username, Key: file.Key}, nil
}

type oauthFile struct {
	RefreshToken          string    `json:"refresh_token"`
	AccessToken           string    `json:"access_token"`
	AccessTokenExpiration time.Time `json:"access_token_expiration"`
	Username              string    `json:"username"`
	Scopes                []string  `json:"scopes"`
}

func readOAuthFile(path string) (Creds, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Creds{}, err
	}
	var file oauthFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return Creds{}, fmt.Errorf("%s: %w", path, err)
	}

	if file.AccessToken != "" && time.Now().Before(file.AccessTokenExpiration.Add(-time.Minute)) {
		return Creds{Username: file.Username, Token: file.AccessToken}, nil
	}
	if file.RefreshToken == "" {
		return Creds{}, fmt.Errorf("%s: access token expired and no refresh token", path)
	}

	creds, err := refreshToken(file.RefreshToken)
	if err != nil {
		return Creds{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := persistToken(path, creds); err != nil {
		return creds, nil // the fresh token works; failing to cache it is not fatal
	}
	return creds, nil
}

// refreshToken trades a refresh token for a new 12h access token. The request
// is unauthenticated by design.
func refreshToken(refresh string) (Creds, error) {
	body, err := json.Marshal(map[string]any{
		"refreshToken":       refresh,
		"apiVersion":         "API_VERSION_V1",
		"expirationDuration": "43200s",
	})
	if err != nil {
		return Creds{}, err
	}

	url := authBaseURL + "/v1/users.AccountService/GenerateAccessToken"
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return Creds{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Creds{}, err
	}
	if resp.StatusCode/100 != 2 {
		return Creds{}, decodeError(resp.StatusCode, data)
	}

	var out struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expiresIn"`
		UserName  string `json:"userName"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return Creds{}, err
	}
	if out.Token == "" {
		return Creds{}, fmt.Errorf("refresh returned no token")
	}
	return Creds{Username: out.UserName, Token: out.Token}, nil
}

func persistToken(path string, creds Creds) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	doc["access_token"] = creds.Token
	doc["username"] = creds.Username
	doc["access_token_expiration"] = time.Now().Add(12 * time.Hour).UTC().Format(time.RFC3339)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}
