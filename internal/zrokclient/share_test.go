package zrokclient

import (
	"errors"
	"fmt"
	"testing"
)

func TestTargetsEqual(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
		want bool
	}{
		{"http://nginx.default.svc:80", "http://nginx.default.svc", true},
		{"http://nginx.default.svc:80/", "http://nginx.default.svc", true},
		{"https://x.example:443/foo/", "https://x.example/foo", true},
		{"http://nginx.default.svc:80", "http://other.default.svc:80", false},
		{"http://nginx.default.svc:80", "http://nginx.default.svc:8080", false},
		{"", "http://x", false},
	}
	for _, tc := range cases {
		if got := TargetsEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("TargetsEqual(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestFindByFrontendName(t *testing.T) {
	t.Parallel()
	shares := []RemoteShare{
		{Token: "shr-a", Target: "http://a", FrontendEndpoints: []string{"https://demo.share.zrok.io"}},
		{Token: "shr-b", Target: "http://b", FrontendEndpoints: []string{"https://other.share.zrok.io"}},
	}
	got := FindByFrontendName(shares, "demo")
	if got == nil || got.Token != "shr-a" {
		t.Fatalf("got %+v", got)
	}
	if FindByFrontendName(shares, "missing") != nil {
		t.Fatal("expected nil")
	}
}

func TestFrontendEndpointMatchesName(t *testing.T) {
	t.Parallel()
	if !FrontendEndpointMatchesName("https://demo.share.zrok.io", "demo") {
		t.Fatal("expected match")
	}
	if FrontendEndpointMatchesName("https://demo-extra.share.zrok.io", "demo") {
		t.Fatal("prefix must require a dot after the name")
	}
	if FrontendEndpointMatchesName("https://xdemo.share.zrok.io", "demo") {
		t.Fatal("must not match suffix")
	}
	if FrontendEndpointMatchesName("https://ko-default-nginx-aaaaoaa.shares.zrok.io", "ko-default-nginx-aaaaoaaaaa") {
		t.Fatal("old shorter hostname must not match the new name")
	}
}

func TestFrontendNameFromEndpoint(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"https://ko-default-nginx-aaaaoaa.shares.zrok.io", "ko-default-nginx-aaaaoaa"},
		{"http://demo.share.zrok.io/path", "demo"},
		{"demo.shares.zrok.io", "demo"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := FrontendNameFromEndpoint(tc.in); got != tc.want {
			t.Errorf("FrontendNameFromEndpoint(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsUnauthorized(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "other", err: errors.New("create share name: 409"), want: false},
		{
			name: "wrapped 401",
			err:  fmt.Errorf("update share name: %w", errors.New("[PATCH /share/name][401] updateShareNameUnauthorized")),
			want: true,
		},
		{name: "swagger type only", err: errors.New("updateShareNameUnauthorized"), want: true},
		{
			name: "delete 401",
			err:  fmt.Errorf("delete share name: %w", errors.New("[DELETE /share/name][401] deleteShareNameUnauthorized")),
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsUnauthorized(tc.err); got != tc.want {
				t.Fatalf("IsUnauthorized(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
