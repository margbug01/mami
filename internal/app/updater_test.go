//go:build windows

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"2.5.0", "2.5.1", true},
		{"2.5.0", "v2.10.0", true},
		{"2.5.0", "3.0", true},
		{"2.5.0", "2.5.0", false},
		{"2.5.1", "2.5.0", false},
		{"2.10.0", "2.9.9", false},
		{"2.5.0-beta", "2.5.0", false},
	} {
		if got := newerVersion(tc.a, tc.b); got != tc.want {
			t.Errorf("newerVersion(%q, %q) = %v", tc.a, tc.b, got)
		}
	}
}

// 假的 GitHub：latest 指向 v2.6.0，资产就是 body
func fakeRelease(t *testing.T, body []byte, digest string) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fmt.Fprintf(w, `{"tag_name":"v2.6.0","assets":[
				{"name":"Mami.exe","browser_download_url":"%[1]s/portable","size":%[2]d,"digest":"%[3]s"},
				{"name":"Mami.Setup.2.6.0.amd64.exe","browser_download_url":"%[1]s/setup","size":%[2]d,"digest":"%[3]s"}]}`,
				srv.URL, len(body), digest)
		default:
			w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("MAMI_UPDATE_API", srv.URL+"/latest")
	return srv
}

func TestFetchLatestPicksAsset(t *testing.T) {
	srv := fakeRelease(t, []byte("x"), "")
	a := &App{}
	a.setConfig(defaultConfig())
	for portable, want := range map[bool]string{false: "/setup", true: "/portable"} {
		r, err := a.fetchLatest(context.Background(), portable)
		if err != nil {
			t.Fatal(err)
		}
		if r.version != "2.6.0" || r.url != srv.URL+want {
			t.Errorf("portable=%v: %+v", portable, r)
		}
	}
}

func TestDownloadVerifies(t *testing.T) {
	body := []byte("new mami bits")
	sum := sha256.Sum256(body)
	a := &App{}
	a.setConfig(defaultConfig())
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		digest string
		size   int64
		ok     bool
	}{
		"good":      {"sha256:" + hex.EncodeToString(sum[:]), int64(len(body)), true},
		"no digest": {"", int64(len(body)), true},
		"tampered":  {"sha256:" + hex.EncodeToString(make([]byte, 32)), int64(len(body)), false},
		"truncated": {"", int64(len(body)) + 5, false},
	} {
		srv := fakeRelease(t, body, tc.digest)
		file := filepath.Join(dir, name)
		err := a.download(&release{url: srv.URL + "/setup", size: tc.size, sha256: tc.digest[min(7, len(tc.digest)):]}, file)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v", name, err)
		}
		if tc.ok {
			if got, _ := os.ReadFile(file); string(got) != string(body) {
				t.Errorf("%s: wrote %q", name, got)
			}
		}
	}
}

func TestReplaceExe(t *testing.T) {
	dir := t.TempDir()
	target, src := filepath.Join(dir, "Mami.exe"), filepath.Join(dir, "new.exe")
	os.WriteFile(target, []byte("old"), 0o644)
	os.WriteFile(src, []byte("new"), 0o644)
	if err := replaceExe(src, target); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != "new" {
		t.Errorf("target = %q", got)
	}
	if got, _ := os.ReadFile(target + ".old"); string(got) != "old" {
		t.Errorf(".old = %q", got)
	}
}
