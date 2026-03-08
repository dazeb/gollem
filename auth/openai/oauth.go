// Package openai implements OAuth 2.0 PKCE authentication for ChatGPT
// subscription-based API access. This allows users with ChatGPT Plus/Pro/Team
// subscriptions to use OpenAI models via their subscription quota instead of
// pay-per-token API keys.
package openai

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// ClientID is the public OAuth client ID used by Codex CLI.
	ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

	// OAuth endpoints.
	authEndpoint  = "https://auth.openai.com/oauth/authorize"
	tokenEndpoint = "https://auth.openai.com/oauth/token"

	// Device code endpoints.
	deviceUserCodeEndpoint = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	deviceTokenEndpoint    = "https://auth.openai.com/api/accounts/deviceauth/token"

	// Default redirect port.
	defaultPort = 1455

	// OAuth scopes.
	oauthScopes = "openid profile email offline_access api.connectors.read api.connectors.invoke"
)

// Credentials stores OAuth tokens for ChatGPT subscription access.
type Credentials struct {
	AccessToken  string    `json:"access_token"`  //nolint:gosec // Not a hardcoded credential.
	RefreshToken string    `json:"refresh_token"` //nolint:gosec // Not a hardcoded credential.
	IDToken      string    `json:"id_token"`
	AccountID    string    `json:"account_id"`
	AuthMode     string    `json:"auth_mode"` // "chatgpt" or "api"
	ExpiresAt    time.Time `json:"expires_at"`
}

// LoginConfig configures the OAuth login flow.
type LoginConfig struct {
	Port       int  // Local server port (default: 1455)
	DeviceAuth bool // Use device code flow instead of browser
}

// Login performs the OAuth PKCE flow and returns credentials.
// For browser-based auth, it starts a local server and opens the browser.
// For device auth, it displays a code for the user to enter.
func Login(ctx context.Context, config LoginConfig) (*Credentials, error) {
	if config.DeviceAuth {
		return loginDeviceCode(ctx)
	}
	return loginBrowser(ctx, config)
}

// loginBrowser performs the browser-based OAuth PKCE flow.
func loginBrowser(ctx context.Context, config LoginConfig) (*Credentials, error) {
	port := config.Port
	if port == 0 {
		port = defaultPort
	}

	// Generate PKCE code verifier and challenge.
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, fmt.Errorf("generating PKCE: %w", err)
	}

	// Generate state parameter.
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("generating state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	// Channel to receive the auth code.
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	redirectURI := "http://localhost:" + strconv.Itoa(port) + "/auth/callback"

	// Start local server to receive the callback.
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		receivedState := r.URL.Query().Get("state")
		if receivedState != state {
			errCh <- fmt.Errorf("state mismatch: expected %q, got %q", state, receivedState)
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- errors.New("no authorization code received")
			http.Error(w, "No code", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, "<html><body><h1>Login successful!</h1><p>You can close this window.</p></body></html>")
		codeCh <- code
	})

	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return nil, fmt.Errorf("starting local server on port %d: %w", port, err)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	// Build authorization URL.
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {oauthScopes},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		// Extra params matching Codex CLI.
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"originator":                 {"codex_cli_rs"},
	}
	authURL := authEndpoint + "?" + params.Encode()

	fmt.Printf("Open this URL to log in:\n\n  %s\n\nWaiting for authentication...\n", authURL)

	// Wait for the auth code or context cancellation.
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Exchange authorization code for tokens.
	return exchangeCode(ctx, code, redirectURI, verifier)
}

// loginDeviceCode performs the device code OAuth flow.
func loginDeviceCode(ctx context.Context) (*Credentials, error) {
	// Request a user code.
	data := url.Values{
		"client_id": {ClientID},
		"scope":     {oauthScopes},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceUserCodeEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}
	defer resp.Body.Close()

	var deviceResp struct {
		UserCode        string `json:"user_code"`
		DeviceCode      string `json:"device_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return nil, fmt.Errorf("decoding device code response: %w", err)
	}

	verifyURL := deviceResp.VerificationURI
	if verifyURL == "" {
		verifyURL = "https://auth.openai.com/codex/device"
	}

	fmt.Printf("Visit %s and enter code: %s\n\nWaiting for authorization...\n", verifyURL, deviceResp.UserCode)

	// Poll for authorization.
	interval := time.Duration(deviceResp.Interval) * time.Second
	if interval == 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(deviceResp.ExpiresIn) * time.Second)
	if deviceResp.ExpiresIn == 0 {
		deadline = time.Now().Add(5 * time.Minute)
	}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		pollData := url.Values{
			"client_id":   {ClientID},
			"device_code": {deviceResp.DeviceCode},
		}
		pollReq, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceTokenEndpoint, strings.NewReader(pollData.Encode()))
		if err != nil {
			continue
		}
		pollReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		tokenResp, err := http.DefaultClient.Do(pollReq)
		if err != nil {
			continue
		}

		body, _ := io.ReadAll(tokenResp.Body)
		tokenResp.Body.Close()

		var result struct {
			AuthorizationCode string `json:"authorization_code"`
			CodeVerifier      string `json:"code_verifier"`
			CodeChallenge     string `json:"code_challenge"`
			Error             string `json:"error"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		if result.Error == "authorization_pending" || result.Error == "slow_down" {
			continue
		}
		if result.Error != "" {
			return nil, fmt.Errorf("device authorization failed: %s", result.Error)
		}

		if result.AuthorizationCode != "" {
			return exchangeCode(ctx, result.AuthorizationCode, "http://localhost:"+strconv.Itoa(defaultPort)+"/auth/callback", result.CodeVerifier)
		}
	}

	return nil, errors.New("device authorization timed out")
}

// exchangeCode exchanges an authorization code for tokens.
func exchangeCode(ctx context.Context, code, redirectURI, codeVerifier string) (*Credentials, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {ClientID},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchanging code for tokens: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`  //nolint:gosec // JSON field name, not a credential.
		RefreshToken string `json:"refresh_token"` //nolint:gosec // JSON field name, not a credential.
		IDToken      string `json:"id_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}

	accountID := extractAccountID(tokenResp.IDToken)

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	if tokenResp.ExpiresIn == 0 {
		expiresAt = time.Now().Add(1 * time.Hour)
	}

	return &Credentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		AccountID:    accountID,
		AuthMode:     "chatgpt",
		ExpiresAt:    expiresAt,
	}, nil
}

// RefreshIfNeeded refreshes the access token if expired or near-expiry.
// Returns the same credentials if still valid, or new credentials if refreshed.
func RefreshIfNeeded(creds *Credentials) (*Credentials, error) {
	// Refresh if within 5 minutes of expiry.
	if time.Until(creds.ExpiresAt) > 5*time.Minute {
		return creds, nil
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {creds.RefreshToken},
		"client_id":     {ClientID},
	}

	// Use background context since RefreshIfNeeded has no ctx parameter.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refreshing token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed (HTTP %d): %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`  //nolint:gosec // JSON field name, not a credential.
		RefreshToken string `json:"refresh_token"` //nolint:gosec // JSON field name, not a credential.
		IDToken      string `json:"id_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decoding refresh response: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	if tokenResp.ExpiresIn == 0 {
		expiresAt = time.Now().Add(1 * time.Hour)
	}

	return &Credentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		AccountID:    creds.AccountID,
		AuthMode:     "chatgpt",
		ExpiresAt:    expiresAt,
	}, nil
}

// credentialsPath returns the path to the credentials file.
func credentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".golem", "auth.json"), nil
}

// LoadCredentials reads stored credentials from ~/.golem/auth.json.
func LoadCredentials() (*Credentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	return LoadCredentialsFrom(path)
}

// LoadCredentialsFrom reads credentials from a specific file path.
func LoadCredentialsFrom(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}
	return &creds, nil
}

// SaveCredentials writes credentials to ~/.golem/auth.json.
func SaveCredentials(creds *Credentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	return SaveCredentialsTo(creds, path)
}

// SaveCredentialsTo writes credentials to a specific file path.
func SaveCredentialsTo(creds *Credentials, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating credentials directory: %w", err)
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// generatePKCE generates a PKCE code verifier and S256 challenge.
func generatePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// extractAccountID parses the id_token JWT to extract the account_id.
// This is a minimal parser that doesn't verify the signature (the token
// was just received from the token endpoint over TLS).
func extractAccountID(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	// Try common claim paths.
	if id, ok := claims["account_id"].(string); ok {
		return id
	}
	// Codex extracts from organizations claim.
	if orgs, ok := claims["organizations"].([]any); ok && len(orgs) > 0 {
		if org, ok := orgs[0].(map[string]any); ok {
			if id, ok := org["id"].(string); ok {
				return id
			}
		}
	}
	return ""
}
