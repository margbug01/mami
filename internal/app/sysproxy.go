package app

import (
	"bufio"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// systemProxy 系统设置里的代理（macOS 网络设置 / Windows 的 Internet 选项），给检查更新、下载更新用：
// 国内直连 GitHub 下载常常只有几 KB/s（实测 47 KB/s，走代理 818 KB/s）。设置里自己填了代理的照旧用那个，
// 识别、润色、翻译也照旧（它们连的多是国内接口，直连更快）。环境变量里有代理时听环境变量的
func systemProxy(req *http.Request) (*url.URL, error) {
	if os.Getenv("HTTPS_PROXY") != "" || os.Getenv("HTTP_PROXY") != "" || os.Getenv("https_proxy") != "" || os.Getenv("http_proxy") != "" {
		return http.ProxyFromEnvironment(req)
	}
	if h := req.URL.Hostname(); h == "localhost" || net.ParseIP(h).IsLoopback() { // 本机的不绕代理（和环境变量那套一样）
		return nil, nil
	}
	sysProxyMu.Lock()
	defer sysProxyMu.Unlock()
	if time.Since(sysProxyAt) > 3*time.Second { // 下载要发好几个请求，不用每个都读一遍
		sysProxyAt, sysProxyURL = time.Now(), readSystemProxy()
	}
	return sysProxyURL, nil
}

var (
	sysProxyMu  sync.Mutex
	sysProxyAt  time.Time
	sysProxyURL *url.URL
	// readSystemProxy 读一次系统代理，没开就是 nil；测试里换成假的
	readSystemProxy = platformSystemProxy
)

// parseScutilProxy macOS `scutil --proxy` 的输出：开着的 HTTPS 代理，没有就 HTTP，再没有就 SOCKS
func parseScutilProxy(text string) *url.URL {
	fields := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), " : "); ok {
			fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	for _, p := range []struct{ key, scheme string }{{"HTTPS", "http"}, {"HTTP", "http"}, {"SOCKS", "socks5"}} {
		host, port := fields[p.key+"Proxy"], fields[p.key+"Port"]
		if fields[p.key+"Enable"] != "1" || host == "" || port == "" {
			continue
		}
		return &url.URL{Scheme: p.scheme, Host: net.JoinHostPort(host, port)}
	}
	return nil
}

// parseProxyServer Windows 注册表 ProxyServer：「host:port」全用这一个，或者「http=…;https=…;socks=…」分开写
func parseProxyServer(server string) *url.URL {
	server = strings.TrimSpace(server)
	if server == "" {
		return nil
	}
	if !strings.Contains(server, "=") {
		return proxyAt("http", server)
	}
	parts := map[string]string{}
	for _, p := range strings.Split(server, ";") {
		if k, v, ok := strings.Cut(p, "="); ok {
			parts[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	for _, p := range []struct{ key, scheme string }{{"https", "http"}, {"http", "http"}, {"socks", "socks5"}} {
		if u := proxyAt(p.scheme, parts[p.key]); u != nil {
			return u
		}
	}
	return nil
}

func proxyAt(scheme, addr string) *url.URL {
	addr = strings.TrimSpace(addr)
	if i := strings.Index(addr, "://"); i >= 0 { // 有人写成 http://127.0.0.1:7890
		addr = addr[i+3:]
	}
	if host, port, err := net.SplitHostPort(addr); err != nil || host == "" || port == "" {
		return nil
	}
	return &url.URL{Scheme: scheme, Host: addr}
}
