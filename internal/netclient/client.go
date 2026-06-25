package netclient

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const explicitProxyEnv = "CCVAR_EXCHANGE_PROXY"

var darwinProxy struct {
	once   sync.Once
	config systemProxyConfig
}

type systemProxyConfig struct {
	HTTPEnabled  bool
	HTTPHost     string
	HTTPPort     int
	HTTPSEnabled bool
	HTTPSHost    string
	HTTPSPort    int
	SOCKSEnabled bool
	SOCKSHost    string
	SOCKSPort    int
}

// New returns an exchange HTTP client that follows environment proxies first and
// macOS system proxy settings second. Finder-launched apps often miss shell env.
func New(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = Proxy
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func Proxy(req *http.Request) (*url.URL, error) {
	if req == nil || req.URL == nil {
		return nil, nil
	}
	if shouldBypassProxy(req.URL.Hostname()) {
		return nil, nil
	}
	if explicit := strings.TrimSpace(os.Getenv(explicitProxyEnv)); explicit != "" {
		if isDirectProxyValue(explicit) {
			return nil, nil
		}
		proxyURL, err := parseProxyURL(explicit, explicitProxyEnv)
		if err != nil {
			return nil, err
		}
		return proxyURL, nil
	}
	if proxyURL, err := http.ProxyFromEnvironment(req); proxyURL != nil || err != nil {
		return proxyURL, err
	}
	if runtime.GOOS == "darwin" {
		return darwinSystemProxy(req.URL.Scheme), nil
	}
	return nil, nil
}

func ProxySummary(targetURL string) string {
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return "invalid"
	}
	proxyURL, err := Proxy(req)
	if err != nil {
		return "invalid"
	}
	if proxyURL == nil {
		return "direct"
	}
	return sanitizeProxyURL(proxyURL)
}

func shouldBypassProxy(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

func isDirectProxyValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "off", "none", "direct":
		return true
	default:
		return false
	}
}

func parseProxyURL(value, source string) (*url.URL, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%s must be a valid proxy URL", source)
	}
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
		return parsed, nil
	default:
		return nil, fmt.Errorf("%s must use http, https, socks5, or socks5h", source)
	}
}

func darwinSystemProxy(scheme string) *url.URL {
	darwinProxy.once.Do(func() {
		darwinProxy.config = loadDarwinSystemProxy()
	})
	switch strings.ToLower(scheme) {
	case "https":
		if darwinProxy.config.HTTPSEnabled {
			return proxyURL("http", darwinProxy.config.HTTPSHost, darwinProxy.config.HTTPSPort)
		}
	case "http":
		if darwinProxy.config.HTTPEnabled {
			return proxyURL("http", darwinProxy.config.HTTPHost, darwinProxy.config.HTTPPort)
		}
	}
	if darwinProxy.config.SOCKSEnabled {
		return proxyURL("socks5", darwinProxy.config.SOCKSHost, darwinProxy.config.SOCKSPort)
	}
	return nil
}

func loadDarwinSystemProxy() systemProxyConfig {
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer cancel()
	output, err := exec.CommandContext(ctx, "scutil", "--proxy").Output()
	if err != nil {
		return systemProxyConfig{}
	}
	return parseScutilProxyOutput(string(output))
}

func parseScutilProxyOutput(output string) systemProxyConfig {
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		before, after, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(before)
		value := strings.TrimSpace(after)
		if key != "" {
			values[key] = value
		}
	}
	return systemProxyConfig{
		HTTPEnabled:  values["HTTPEnable"] == "1",
		HTTPHost:     values["HTTPProxy"],
		HTTPPort:     parsePort(values["HTTPPort"]),
		HTTPSEnabled: values["HTTPSEnable"] == "1",
		HTTPSHost:    values["HTTPSProxy"],
		HTTPSPort:    parsePort(values["HTTPSPort"]),
		SOCKSEnabled: values["SOCKSEnable"] == "1",
		SOCKSHost:    values["SOCKSProxy"],
		SOCKSPort:    parsePort(values["SOCKSPort"]),
	}
}

func parsePort(value string) int {
	port, _ := strconv.Atoi(strings.TrimSpace(value))
	return port
}

func proxyURL(scheme, host string, port int) *url.URL {
	host = strings.TrimSpace(host)
	if host == "" || port <= 0 {
		return nil
	}
	return &url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}
}

func sanitizeProxyURL(proxyURL *url.URL) string {
	if proxyURL == nil {
		return "direct"
	}
	clone := *proxyURL
	if clone.User != nil {
		clone.User = url.User("redacted")
	}
	return clone.String()
}
