package zrokclient

import "testing"

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
