package zrokclient

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestValidateAPIEndpoint(t *testing.T) {
	t.Parallel()
	allowed := []string{"zrok.example.com", "zrok.internal:8443"}
	cases := []struct {
		name    string
		ep      string
		wantErr bool
	}{
		{name: "default empty", ep: "", wantErr: false},
		{name: "default explicit", ep: DefaultAPIEndpoint, wantErr: false},
		{name: "allowlisted extra", ep: "https://zrok.example.com", wantErr: false},
		{name: "allowlisted extra with path", ep: "https://zrok.example.com/v2", wantErr: false},
		{name: "allowlisted port", ep: "https://zrok.internal:8443", wantErr: false},
		{name: "http rejected", ep: "http://api-v2.zrok.io", wantErr: true},
		{name: "unknown host", ep: "https://evil.example", wantErr: true},
		{name: "userinfo", ep: "https://u:p@api-v2.zrok.io", wantErr: true},
		{name: "loopback", ep: "https://127.0.0.1", wantErr: true},
		{name: "localhost", ep: "https://localhost", wantErr: true},
		{name: "link-local metadata", ep: "https://169.254.169.254", wantErr: true},
		{name: "missing scheme treated as path", ep: "api-v2.zrok.io", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateAPIEndpoint(tc.ep, allowed)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if tc.wantErr && err != nil && !IsEndpointNotAllowed(err) {
				t.Fatalf("want EndpointNotAllowed, got %v", err)
			}
		})
	}
}

func TestNormalizeAPIHostsAlwaysIncludesDefault(t *testing.T) {
	t.Parallel()
	got := NormalizeAPIHosts(nil)
	if len(got) != 1 || got[0] != DefaultAPIHost() {
		t.Fatalf("got %v", got)
	}
	got = NormalizeAPIHosts([]string{"Zrok.Example.COM", DefaultAPIHost()})
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestCheckRedirectDropsTokenAndCrossHost(t *testing.T) {
	t.Parallel()

	var sawToken bool
	dest := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TOKEN") != "" {
			sawToken = true
		}
	}))
	t.Cleanup(dest.Close)

	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", dest.URL+"/next")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	client := NewSecureHTTPClient()
	client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // test-only twin httptest certs

	req, err := http.NewRequest(http.MethodGet, origin.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-TOKEN", "secret-enable-token")
	_, err = client.Do(req)
	if err == nil {
		t.Fatal("expected cross-host redirect to fail")
	}
	if !strings.Contains(err.Error(), "cross-host") && !strings.Contains(err.Error(), "non-https") {
		t.Fatalf("unexpected error: %v", err)
	}
	if sawToken {
		t.Fatal("X-TOKEN must not follow a cross-host redirect")
	}
}

func TestCheckRedirectStripsTokenSameHost(t *testing.T) {
	t.Parallel()

	var hops []string
	var tokenOnSecond bool
	mux := http.NewServeMux()
	mux.HandleFunc("/from", func(w http.ResponseWriter, r *http.Request) {
		hops = append(hops, r.URL.Path)
		w.Header().Set("Location", "/to")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/to", func(w http.ResponseWriter, r *http.Request) {
		hops = append(hops, r.URL.Path)
		tokenOnSecond = r.Header.Get("X-TOKEN") != ""
		_, _ = io.WriteString(w, "ok")
	})

	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	client := NewSecureHTTPClient()
	client.Transport = srv.Client().Transport

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/from", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-TOKEN", "secret-enable-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if tokenOnSecond {
		t.Fatal("X-TOKEN must be stripped on redirect even for same host")
	}
	if len(hops) != 2 {
		t.Fatalf("hops %v", hops)
	}
}

func TestHostsEqualDefaultHTTPSPort(t *testing.T) {
	t.Parallel()
	a, _ := url.Parse("https://api-v2.zrok.io")
	b, _ := url.Parse("https://api-v2.zrok.io:443/api/v2")
	if !hostsEqual(a, b) {
		t.Fatal("443 is the default https port")
	}
}
