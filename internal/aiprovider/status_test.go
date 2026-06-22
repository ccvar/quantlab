package aiprovider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodexLoggedInRecognizesCommonOutput(t *testing.T) {
	cases := []string{
		"Logged in as user@example.com",
		"authenticated",
		"login: ok",
	}
	for _, tc := range cases {
		if !codexLoggedIn(tc) {
			t.Fatalf("expected %q to be treated as logged in", tc)
		}
	}
	if codexLoggedIn("not logged out, please login") {
		t.Fatal("unexpected positive login detection")
	}
	if codexLoggedIn("not logged in") {
		t.Fatal("not logged in must not be treated as authenticated")
	}
}

func TestDetectClaudeAuthSourcePrefersEnvironment(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "secret")
	if got := detectClaudeAuthSource(); got != "CLAUDE_CODE_OAUTH_TOKEN" {
		t.Fatalf("expected env source, got %q", got)
	}
}

func TestDetectClaudeAuthSourceChecksHomeFilesWithoutLeakingValue(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_API_KEY", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"oauthAccount":{"token":"secret"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := detectClaudeAuthSource(); got != settingsPath {
		t.Fatalf("expected settings path, got %q", got)
	}
}
