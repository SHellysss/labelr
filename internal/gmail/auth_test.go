package gmail

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"
)

func TestSaveAndLoadToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")

	token := &oauth2.Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
	}

	if err := saveToken(path, token); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}

	// Verify file permissions
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("got permissions %o, want 0600", info.Mode().Perm())
	}

	loaded, err := loadToken(path)
	if err != nil {
		t.Fatalf("loadToken failed: %v", err)
	}
	if loaded.AccessToken != "access-123" {
		t.Errorf("got access token %q, want access-123", loaded.AccessToken)
	}
	if loaded.RefreshToken != "refresh-456" {
		t.Errorf("got refresh token %q, want refresh-456", loaded.RefreshToken)
	}
}

func TestOAuthConfig(t *testing.T) {
	ClientID = "test-client-id"
	ClientSecret = "test-client-secret"
	defer func() { ClientID = ""; ClientSecret = "" }()

	cfg := oauthConfig("http://localhost:8080/callback")
	if cfg.ClientID != "test-client-id" {
		t.Errorf("got client ID %q, want test-client-id", cfg.ClientID)
	}
	if len(cfg.Scopes) != 2 {
		t.Errorf("got %d scopes, want 2", len(cfg.Scopes))
	}
}
