package app

import (
	"net/http"
	"net/url"
	"testing"
)

func TestParseScutilProxy(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"<dictionary> {\n  HTTPEnable : 1\n  HTTPPort : 10808\n  HTTPProxy : 127.0.0.1\n  HTTPSEnable : 1\n  HTTPSPort : 10809\n  HTTPSProxy : 127.0.0.1\n}", "http://127.0.0.1:10809"},
		{"  HTTPEnable : 1\n  HTTPPort : 7890\n  HTTPProxy : 127.0.0.1\n  HTTPSEnable : 0\n", "http://127.0.0.1:7890"},
		{"  SOCKSEnable : 1\n  SOCKSPort : 1080\n  SOCKSProxy : 10.0.0.2\n", "socks5://10.0.0.2:1080"},
		{"  HTTPSEnable : 0\n  HTTPSPort : 10808\n  HTTPSProxy : 127.0.0.1\n", ""},
		{"<dictionary> {\n  ExceptionsList : <array> {\n  }\n}", ""},
	} {
		got := ""
		if u := parseScutilProxy(c.text); u != nil {
			got = u.String()
		}
		if got != c.want {
			t.Errorf("%q: got %q want %q", c.text, got, c.want)
		}
	}
}

func TestParseProxyServer(t *testing.T) {
	for _, c := range []struct{ server, want string }{
		{"127.0.0.1:7890", "http://127.0.0.1:7890"},
		{"http://127.0.0.1:10810", "http://127.0.0.1:10810"},
		{"http=127.0.0.1:8080;https=127.0.0.1:8443", "http://127.0.0.1:8443"},
		{"ftp=1.2.3.4:21;socks=127.0.0.1:1080", "socks5://127.0.0.1:1080"},
		{"", ""}, {"127.0.0.1", ""}, {"http=;https=", ""},
	} {
		got := ""
		if u := parseProxyServer(c.server); u != nil {
			got = u.String()
		}
		if got != c.want {
			t.Errorf("%q: got %q want %q", c.server, got, c.want)
		}
	}
}

// 检查更新、下载更新：设置里没填代理就走系统代理；填了就用填的
func TestUpdateClientUsesTheSystemProxy(t *testing.T) {
	for _, k := range []string{"HTTPS_PROXY", "HTTP_PROXY", "https_proxy", "http_proxy"} {
		t.Setenv(k, "")
	}
	defer func(f func() *url.URL) { readSystemProxy = f; sysProxyAt = sysProxyAt.AddDate(-1, 0, 0) }(readSystemProxy)
	readSystemProxy = func() *url.URL { return &url.URL{Scheme: "http", Host: "127.0.0.1:10808"} }
	sysProxyAt = sysProxyAt.AddDate(-1, 0, 0)
	req, _ := http.NewRequest("GET", "https://github.com/margbug01/mami/releases/latest/download/mami-mac.zip", nil)

	a := &App{}
	cfg := defaultConfig()
	if err := a.setConfig(cfg); err != nil {
		t.Fatal(err)
	}
	u, err := a.updateClient(0).Transport.(*http.Transport).Proxy(req)
	if err != nil || u == nil || u.Host != "127.0.0.1:10808" {
		t.Fatalf("no proxy of its own: got %v, %v; want the system's 127.0.0.1:10808", u, err)
	}
	local, _ := http.NewRequest("GET", "http://127.0.0.1:8123/releases/latest", nil) // 测试用的假接口
	if u, _ := a.updateClient(0).Transport.(*http.Transport).Proxy(local); u != nil {
		t.Fatalf("a local address went through the proxy %v", u)
	}
	cfg.Proxy = "http://127.0.0.1:7890"
	if err := a.setConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if u, _ := a.updateClient(0).Transport.(*http.Transport).Proxy(req); u == nil || u.Host != "127.0.0.1:7890" {
		t.Fatalf("own proxy set: got %v, want 127.0.0.1:7890", u)
	}
}
