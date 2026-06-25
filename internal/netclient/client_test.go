package netclient

import (
	"net/http"
	"testing"
)

func TestParseScutilProxyOutput(t *testing.T) {
	config := parseScutilProxyOutput(`
<dictionary> {
  HTTPEnable : 1
  HTTPPort : 7897
  HTTPProxy : 127.0.0.1
  HTTPSEnable : 1
  HTTPSPort : 7897
  HTTPSProxy : 127.0.0.1
  SOCKSEnable : 1
  SOCKSPort : 7897
  SOCKSProxy : 127.0.0.1
}`)
	if !config.HTTPSEnabled || config.HTTPSHost != "127.0.0.1" || config.HTTPSPort != 7897 {
		t.Fatalf("HTTPS proxy not parsed: %#v", config)
	}
	if !config.SOCKSEnabled || config.SOCKSHost != "127.0.0.1" || config.SOCKSPort != 7897 {
		t.Fatalf("SOCKS proxy not parsed: %#v", config)
	}
}

func TestProxyBypassesLoopback(t *testing.T) {
	t.Setenv(explicitProxyEnv, "http://127.0.0.1:7897")
	request, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8787/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, err := Proxy(request)
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL != nil {
		t.Fatalf("loopback request should bypass proxy, got %s", proxyURL)
	}
}

func TestExplicitProxy(t *testing.T) {
	t.Setenv(explicitProxyEnv, "127.0.0.1:7897")
	request, err := http.NewRequest(http.MethodGet, "https://www.okx.com/api/v5/public/time", nil)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, err := Proxy(request)
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:7897" {
		t.Fatalf("proxy = %v", proxyURL)
	}
}
