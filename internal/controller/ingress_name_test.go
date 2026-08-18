package controller

import "testing"

func TestIngressReservedName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"ko-default-nginx-aaaaoaaaaa", "ko-default-nginx-aaaaoaaaaa"},
		{"ko-default-nginx-aaaaoaa.shares.zrok.io", "ko-default-nginx-aaaaoaa"},
		{"https://demo.shares.zrok.io", "demo"},
		{"Demo.share.zrok.io", "demo"},
		{"", ""},
		{"nginx.example.com", "nginx.example.com"},
	}
	for _, tc := range cases {
		if got := ingressReservedName(tc.in); got != tc.want {
			t.Errorf("ingressReservedName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
