package controller

import "testing"

func TestValidateShareUpstream(t *testing.T) {
	t.Parallel()
	const ns = "default"
	cases := []struct {
		url     string
		wantErr bool
	}{
		{url: "http://nginx.default.svc:80", wantErr: false},
		{url: "http://nginx.default.svc.cluster.local:80", wantErr: false},
		{url: "http://nginx.default:80", wantErr: false},
		{url: "http://nginx", wantErr: false},
		{url: "tcp://db.default.svc:5432", wantErr: false},
		{url: "http://nginx.other.svc:80", wantErr: true},
		{url: "http://evil.example", wantErr: true},
		{url: "http://127.0.0.1:80", wantErr: true},
		{url: "http://10.0.0.5:80", wantErr: true},
		{url: "http://kubernetes.default.svc", wantErr: false}, // same-ns; kube-system would be other ns
		{url: "https://nginx.default.svc", wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			err := validateShareUpstream(tc.url, ns)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected: %v", err)
			}
		})
	}
	if err := validateShareUpstream("http://nginx.kube-system.svc", "kube-system"); err != nil {
		t.Fatalf("kube-system svc: %v", err)
	}
}
