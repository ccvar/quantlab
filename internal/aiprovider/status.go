package aiprovider

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const commandTimeout = 2500 * time.Millisecond

// ProviderStatus describes locally available AI routes without exposing secrets.
type ProviderStatus struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	State    string `json:"state"`
	Source   string `json:"source,omitempty"`
	Command  string `json:"command,omitempty"`
	Model    string `json:"model,omitempty"`
	Detail   string `json:"detail"`
	Guidance string `json:"guidance,omitempty"`
}

// Report is the response returned to the UI AI configuration panel.
type Report struct {
	GeneratedAt string           `json:"generatedAt"`
	Providers   []ProviderStatus `json:"providers"`
}

// Detect checks local AI provider affordances. It never returns token values.
func Detect(ctx context.Context) Report {
	return Report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Providers: []ProviderStatus{
			localPolicyStatus(),
			codexCLIStatus(ctx),
			claudeCLIStatus(ctx),
			apiKeyStatus("openai_api", "OpenAI API Key", "OPENAI_API_KEY", "gpt-4.1-mini"),
			apiKeyStatus("anthropic_api", "Anthropic API Key", "ANTHROPIC_API_KEY", "claude-3-5-sonnet-latest"),
			compatibleEndpointStatus(),
		},
	}
}

func localPolicyStatus() ProviderStatus {
	return ProviderStatus{
		ID:       "local_policy",
		Label:    "Local AI Policy",
		Kind:     "local",
		State:    "ok",
		Source:   "built-in",
		Model:    "v0.2.0",
		Detail:   "Deterministic local policy is available for shadow, paper, and guarded live validation.",
		Guidance: "No external model credentials are required for the first release.",
	}
}

func codexCLIStatus(ctx context.Context) ProviderStatus {
	path, ok := findExecutable("codex")
	status := ProviderStatus{
		ID:       "codex_cli",
		Label:    "Codex CLI / ChatGPT subscription",
		Kind:     "subscription_cli",
		State:    "missing",
		Command:  "codex login",
		Model:    envOrDefault("CODEX_MODEL", "gpt-5"),
		Detail:   "Codex CLI was not found on PATH or common local install paths.",
		Guidance: "Install the Codex CLI and run codex login to use a local ChatGPT/Codex subscription in assisted mode.",
	}
	if hasEnv("OPENAI_API_KEY") || envFileHasKey(".openai.env", "OPENAI_API_KEY") {
		status.State = "ok"
		status.Source = "OPENAI_API_KEY"
		status.Detail = "OpenAI API key is configured in the local environment."
		status.Guidance = "API-key routing can be enabled after encrypted AI Vault storage is wired into model execution."
		if ok {
			status.Source = "OPENAI_API_KEY + " + path
		}
		return status
	}
	if !ok {
		return status
	}
	status.Source = path
	output, err := runCommand(ctx, path, "login", "status")
	if codexLoggedIn(output) {
		status.State = "ok"
		status.Detail = "Codex CLI is installed and reports a logged-in local account."
		status.Guidance = "This subscription route is suitable for assisted analysis until a guarded CLI execution bridge is explicitly enabled."
		return status
	}
	status.State = "noauth"
	status.Detail = "Codex CLI is installed, but no active local login was detected."
	if err != nil && strings.TrimSpace(output) != "" {
		status.Detail = firstLine(output)
	}
	status.Guidance = "Run codex login in Terminal, then refresh this panel."
	return status
}

func claudeCLIStatus(ctx context.Context) ProviderStatus {
	path, ok := findExecutable("claude")
	status := ProviderStatus{
		ID:       "claude_cli",
		Label:    "Claude CLI / Claude subscription",
		Kind:     "subscription_cli",
		State:    "missing",
		Command:  "claude setup-token",
		Model:    envOrDefault("CLAUDE_MODEL", "claude-sonnet-4"),
		Detail:   "Claude CLI was not found on PATH or common local install paths.",
		Guidance: "Install Claude Code or the Claude CLI, then run claude setup-token to connect a subscription account locally.",
	}
	if source := detectClaudeAuthSource(); source != "" {
		status.State = "ok"
		status.Source = source
		status.Detail = "Claude local auth material was detected. Token values are not read into the UI."
		status.Guidance = "Use this route for assisted analysis until the guarded CLI execution bridge is enabled."
		if ok {
			status.Source = source + " + " + path
		}
		return status
	}
	if !ok {
		return status
	}
	status.Source = path
	output, err := runCommand(ctx, path, "--version")
	if err == nil || strings.TrimSpace(output) != "" {
		status.State = "noauth"
		status.Detail = "Claude CLI is installed, but no subscription token or API key was detected."
		status.Guidance = "Run claude setup-token, then refresh this panel."
		return status
	}
	status.State = "noauth"
	status.Detail = "Claude CLI is installed, but local auth status could not be confirmed."
	status.Guidance = "Run claude setup-token, then refresh this panel."
	return status
}

func apiKeyStatus(id, label, envName, model string) ProviderStatus {
	status := ProviderStatus{
		ID:       id,
		Label:    label,
		Kind:     "api_key",
		State:    "noauth",
		Command:  "configure in AI Vault",
		Model:    model,
		Detail:   envName + " is not present in the local environment.",
		Guidance: "Use encrypted AI Vault configuration before enabling unattended external model calls.",
	}
	if hasEnv(envName) {
		status.State = "ok"
		status.Source = envName
		status.Detail = envName + " is present in the local environment."
		status.Guidance = "Keep production trading guarded by Live Guard and risk limits."
	}
	return status
}

func compatibleEndpointStatus() ProviderStatus {
	envs := []string{"OPENAI_BASE_URL", "OPENAI_API_BASE", "OLLAMA_HOST", "LM_STUDIO_BASE_URL"}
	status := ProviderStatus{
		ID:       "compatible_endpoint",
		Label:    "OpenAI-compatible / local endpoint",
		Kind:     "endpoint",
		State:    "noauth",
		Command:  "set OPENAI_BASE_URL",
		Model:    envOrDefault("OPENAI_COMPAT_MODEL", "local-model"),
		Detail:   "No local or OpenAI-compatible endpoint was detected in the environment.",
		Guidance: "Set OPENAI_BASE_URL, OLLAMA_HOST, or LM_STUDIO_BASE_URL to route through a local/private model endpoint.",
	}
	for _, envName := range envs {
		if hasEnv(envName) {
			status.State = "ok"
			status.Source = envName
			status.Detail = envName + " is present in the local environment."
			status.Guidance = "Use a private endpoint only after model-routing guardrails are enabled."
			return status
		}
	}
	return status
}

func findExecutable(name string) (string, bool) {
	if path, err := exec.LookPath(name); err == nil && path != "" {
		return path, true
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/opt/homebrew/bin/" + name,
		"/usr/local/bin/" + name,
		"/usr/bin/" + name,
		"/bin/" + name,
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".local", "bin", name),
			filepath.Join(home, ".npm-global", "bin", name),
			filepath.Join(home, ".bun", "bin", name),
			filepath.Join(home, ".cargo", "bin", name),
		)
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, name+".exe")
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate, true
		}
	}
	return "", false
}

func runCommand(ctx context.Context, path string, args ...string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, path, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return strings.TrimSpace(output.String()), err
}

func codexLoggedIn(output string) bool {
	normalized := strings.ToLower(output)
	if strings.Contains(normalized, "not logged in") ||
		strings.Contains(normalized, "not authenticated") ||
		strings.Contains(normalized, "login required") {
		return false
	}
	return strings.Contains(normalized, "logged in") ||
		strings.Contains(normalized, "authenticated") ||
		strings.Contains(normalized, "login: ok")
}

func detectClaudeAuthSource() string {
	envs := []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "CLAUDE_API_KEY"}
	for _, envName := range envs {
		if hasEnv(envName) {
			return envName
		}
	}
	if envFileHasKey(".claude-auth.env", "CLAUDE_CODE_OAUTH_TOKEN") {
		return ".claude-auth.env"
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	for _, path := range []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(home, ".claude", "setting.json"),
		filepath.Join(home, ".claude.json"),
	} {
		if fileContainsAny(path, "oauthAccount", "primaryApiKey", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_AUTH_TOKEN") {
			return path
		}
	}
	return ""
}

func envFileHasKey(path string, keys ...string) bool {
	return fileContainsAny(path, keys...)
}

func fileContainsAny(path string, keys ...string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(data)
	for _, key := range keys {
		if strings.Contains(text, key) {
			return true
		}
	}
	return false
}

func hasEnv(name string) bool {
	return strings.TrimSpace(os.Getenv(name)) != ""
}

func envOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func firstLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return strings.TrimSpace(value)
}
