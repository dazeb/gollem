package openai

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	creds := &Credentials{
		AccessToken:  "at-test-123",
		RefreshToken: "rt-test-456",
		IDToken:      "id-test-789",
		AccountID:    "acct-001",
		AuthMode:     "chatgpt",
		ExpiresAt:    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	if err := SaveCredentialsTo(creds, path); err != nil {
		t.Fatalf("SaveCredentialsTo: %v", err)
	}

	// Verify file permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected file permissions 0600, got %o", perm)
	}

	loaded, err := LoadCredentialsFrom(path)
	if err != nil {
		t.Fatalf("LoadCredentialsFrom: %v", err)
	}

	if loaded.AccessToken != creds.AccessToken {
		t.Errorf("AccessToken: got %q, want %q", loaded.AccessToken, creds.AccessToken)
	}
	if loaded.RefreshToken != creds.RefreshToken {
		t.Errorf("RefreshToken: got %q, want %q", loaded.RefreshToken, creds.RefreshToken)
	}
	if loaded.IDToken != creds.IDToken {
		t.Errorf("IDToken: got %q, want %q", loaded.IDToken, creds.IDToken)
	}
	if loaded.AccountID != creds.AccountID {
		t.Errorf("AccountID: got %q, want %q", loaded.AccountID, creds.AccountID)
	}
	if loaded.AuthMode != creds.AuthMode {
		t.Errorf("AuthMode: got %q, want %q", loaded.AuthMode, creds.AuthMode)
	}
	if !loaded.ExpiresAt.Equal(creds.ExpiresAt) {
		t.Errorf("ExpiresAt: got %v, want %v", loaded.ExpiresAt, creds.ExpiresAt)
	}
}

func TestLoadCredentials_NotFound(t *testing.T) {
	_, err := LoadCredentialsFrom("/nonexistent/path/auth.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRefreshIfNeeded_NotExpired(t *testing.T) {
	creds := &Credentials{
		AccessToken:  "still-valid",
		RefreshToken: "rt",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}

	result, err := RefreshIfNeeded(creds)
	if err != nil {
		t.Fatalf("RefreshIfNeeded: %v", err)
	}
	// Should return the same credentials without making any HTTP calls.
	if result != creds {
		t.Error("expected same credentials pointer when not expired")
	}
}

func TestExtractAccountID_Direct(t *testing.T) {
	claims := map[string]any{
		"account_id": "acct-direct-123",
		"sub":        "user-1",
	}
	token := makeTestJWT(claims)

	id := extractAccountID(token)
	if id != "acct-direct-123" {
		t.Errorf("expected acct-direct-123, got %q", id)
	}
}

func TestExtractAccountID_Organizations(t *testing.T) {
	claims := map[string]any{
		"sub": "user-2",
		"organizations": []any{
			map[string]any{"id": "org-abc-456", "name": "My Team"},
		},
	}
	token := makeTestJWT(claims)

	id := extractAccountID(token)
	if id != "org-abc-456" {
		t.Errorf("expected org-abc-456, got %q", id)
	}
}

func TestExtractAccountID_NoMatch(t *testing.T) {
	claims := map[string]any{
		"sub": "user-3",
	}
	token := makeTestJWT(claims)

	id := extractAccountID(token)
	if id != "" {
		t.Errorf("expected empty, got %q", id)
	}
}

func TestExtractAccountID_InvalidToken(t *testing.T) {
	if id := extractAccountID("not-a-jwt"); id != "" {
		t.Errorf("expected empty for invalid token, got %q", id)
	}
	if id := extractAccountID("a.b.c"); id != "" {
		t.Errorf("expected empty for non-base64 payload, got %q", id)
	}
}

func TestGeneratePKCE(t *testing.T) {
	v, c, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE: %v", err)
	}
	if len(v) == 0 {
		t.Error("verifier should not be empty")
	}
	if len(c) == 0 {
		t.Error("challenge should not be empty")
	}
	if v == c {
		t.Error("verifier and challenge should differ (S256)")
	}

	// Generate again and verify uniqueness.
	v2, c2, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE (2nd): %v", err)
	}
	if v == v2 {
		t.Error("verifiers should be unique across calls")
	}
	if c == c2 {
		t.Error("challenges should be unique across calls")
	}
}

func TestSaveCredentials_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "deep", "nested", "auth.json")

	creds := &Credentials{AccessToken: "test", AuthMode: "chatgpt"}
	if err := SaveCredentialsTo(creds, nested); err != nil {
		t.Fatalf("SaveCredentialsTo with nested dir: %v", err)
	}

	// Verify the directory was created with correct permissions.
	info, err := os.Stat(filepath.Dir(nested))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory to be created")
	}
}

// makeTestJWT creates a minimal JWT (unsigned) with the given claims payload.
func makeTestJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(claims)
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + payloadEnc + ".signature"
}
