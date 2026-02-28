package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmailapi "google.golang.org/api/gmail/v1"
)

// Set via -ldflags at build time. See Makefile.
var (
	ClientID     string
	ClientSecret string
)

func oauthConfig(redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			gmailapi.GmailModifyScope,
			gmailapi.GmailLabelsScope,
		},
		Endpoint: google.Endpoint,
	}
}

// AuthSession holds the state for an in-progress OAuth flow.
type AuthSession struct {
	AuthURL string // URL the user should visit
	wait    func() (*oauth2.Token, error)
}

// Wait blocks until the user completes (or abandons) the OAuth flow.
func (s *AuthSession) Wait() (*oauth2.Token, error) {
	return s.wait()
}

// StartAuth begins the OAuth flow: starts a local callback server and opens
// the browser. Returns an AuthSession immediately so the caller can display
// the auth URL. Call session.Wait() to block until completion.
func StartAuth(credentialsPath string) (*AuthSession, error) {
	// Find available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("finding available port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	redirectURL := fmt.Sprintf("http://localhost:%d/callback", port)
	cfg := oauthConfig(redirectURL)

	// Channel to receive the auth code
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			fmt.Fprintln(w, "Error: no authorization code received.")
			return
		}
		codeCh <- code
		fmt.Fprintln(w, "Success! You can close this tab and return to the terminal.")
	})

	server := &http.Server{Addr: fmt.Sprintf("localhost:%d", port), Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Open browser
	authURL := cfg.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	openBrowser(authURL)

	session := &AuthSession{
		AuthURL: authURL,
		wait: func() (*oauth2.Token, error) {
			// Wait for code, error, or timeout
			var code string
			select {
			case code = <-codeCh:
			case err := <-errCh:
				server.Close()
				return nil, err
			case <-time.After(2 * time.Minute):
				server.Close()
				return nil, fmt.Errorf("authentication timed out — no response from browser")
			}

			server.Close()

			// Exchange code for token
			token, err := cfg.Exchange(context.Background(), code)
			if err != nil {
				return nil, fmt.Errorf("exchanging code: %w", err)
			}

			if err := saveToken(credentialsPath, token); err != nil {
				return nil, fmt.Errorf("saving token: %w", err)
			}

			return token, nil
		},
	}

	return session, nil
}

// TokenSource returns an oauth2.TokenSource from saved credentials.
// It auto-refreshes the token and saves the refreshed token back.
func TokenSource(credentialsPath string) (oauth2.TokenSource, error) {
	token, err := loadToken(credentialsPath)
	if err != nil {
		return nil, err
	}
	cfg := oauthConfig("")
	ts := cfg.TokenSource(context.Background(), token)

	return &savingTokenSource{
		src:  ts,
		path: credentialsPath,
		prev: token,
	}, nil
}

type savingTokenSource struct {
	src  oauth2.TokenSource
	path string
	prev *oauth2.Token
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	token, err := s.src.Token()
	if err != nil {
		// Token refresh failed — try reloading from disk in case
		// credentials were updated by a fresh 'labelr init'
		reloaded, loadErr := loadToken(s.path)
		if loadErr != nil {
			return nil, err // return original error
		}
		if reloaded.AccessToken != s.prev.AccessToken || reloaded.RefreshToken != s.prev.RefreshToken {
			// Credentials file was updated, rebuild token source
			cfg := oauthConfig("")
			s.src = cfg.TokenSource(context.Background(), reloaded)
			s.prev = reloaded
			return s.src.Token()
		}
		return nil, err // same token on disk, nothing to retry
	}
	// If token was refreshed, save it
	if token.AccessToken != s.prev.AccessToken {
		saveToken(s.path, token)
		s.prev = token
	}
	return token, nil
}

func saveToken(path string, token *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func loadToken(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd == nil {
		return fmt.Errorf("unsupported platform")
	}
	return cmd.Start()
}
