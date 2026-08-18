package controller

import (
	"testing"

	"github.com/openziti/zrok/v2/agent/agentGrpc"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/zrokclient"
)

func TestAgentShareMatchesSpec_NameDrift(t *testing.T) {
	t.Parallel()
	share := testShare("nginx", "http://nginx.default.svc:80", "ko-default-nginx-new", "tok")
	d := &agentGrpc.ShareDetail{
		Token:            "tok",
		ShareMode:        "public",
		BackendMode:      "proxy",
		FrontendEndpoint: []string{"https://ko-default-nginx-old.shares.zrok.io"},
		BackendEndpoint:  "http://nginx.default.svc:80",
		Status:           "active",
	}
	if agentShareMatchesSpec(share, d, share.Spec.Upstream.URL) {
		t.Fatal("old frontend must not match new nameSelection")
	}
	d.FrontendEndpoint = []string{"https://ko-default-nginx-new.shares.zrok.io"}
	if !agentShareMatchesSpec(share, d, share.Spec.Upstream.URL) {
		t.Fatal("matching name should adopt")
	}
}

func TestAgentShareMatchesSpec_DropReservedName(t *testing.T) {
	t.Parallel()
	share := testShare("nginx", "http://nginx.default.svc:80", "demo", "tok")
	share.Spec.NameSelection = nil
	share.Status.Reservation = zrokv1alpha1.ReservationReserved
	d := &agentGrpc.ShareDetail{
		Token:            "tok",
		ShareMode:        "public",
		BackendMode:      "proxy",
		FrontendEndpoint: []string{"https://demo.shares.zrok.io"},
		BackendEndpoint:  "http://nginx.default.svc:80",
		Status:           "active",
	}
	if agentShareMatchesSpec(share, d, share.Spec.Upstream.URL) {
		t.Fatal("removing nameSelection must rebuild")
	}
}

func TestAgentShareMatchesSpec_ClosedAndBackend(t *testing.T) {
	t.Parallel()
	share := testShare("nginx", "http://nginx.default.svc:80", "demo", "tok")
	d := &agentGrpc.ShareDetail{
		Token:            "tok",
		ShareMode:        "public",
		BackendMode:      "proxy",
		FrontendEndpoint: []string{"https://demo.shares.zrok.io"},
		BackendEndpoint:  "http://nginx.default.svc:80",
		Status:           "active",
	}
	if !agentShareMatchesSpec(share, d, share.Spec.Upstream.URL) {
		t.Fatal("baseline should match")
	}
	d.Closed = true
	if agentShareMatchesSpec(share, d, share.Spec.Upstream.URL) {
		t.Fatal("closed drift")
	}
	d.Closed = false
	d.BackendMode = "web"
	if agentShareMatchesSpec(share, d, share.Spec.Upstream.URL) {
		t.Fatal("backendMode drift")
	}
}

func TestLiveFrontendName(t *testing.T) {
	t.Parallel()
	share := testShare("nginx", "http://nginx.default.svc:80", "new", "tok")
	share.Status.AssignedURL = "https://stale.shares.zrok.io"
	cls := classifiedShare{
		agent: &agentGrpc.ShareDetail{
			FrontendEndpoint: []string{"https://ko-default-nginx-old.shares.zrok.io"},
		},
	}
	got := liveFrontendName(share, cls)
	if got != "ko-default-nginx-old" {
		t.Fatalf("got %q", got)
	}
}

func TestShareDigestDrifted(t *testing.T) {
	t.Parallel()
	share := testShare("nginx", "http://nginx.default.svc:80", "demo", "tok")
	d1 := shareApplyDigest(share, nil)
	if shareDigestDrifted(share, d1) {
		t.Fatal("empty annotation is upgrade; do not rebuild")
	}
	share.Annotations = map[string]string{annotationAppliedDigest: d1}
	if shareDigestDrifted(share, d1) {
		t.Fatal("matching digest")
	}
	share.Spec.Insecure = true
	d2 := shareApplyDigest(share, nil)
	if d1 == d2 {
		t.Fatal("insecure must change digest")
	}
	if !shareDigestDrifted(share, d2) {
		t.Fatal("digest drift")
	}
	share.Spec.ReclaimPolicy = zrokv1alpha1.ReclaimRetain
	d3 := shareApplyDigest(share, nil)
	if d2 != d3 {
		t.Fatal("reclaimPolicy must not change digest")
	}
}

func TestShareApplyDigest_BasicAuthAndOAuth(t *testing.T) {
	t.Parallel()
	share := &zrokv1alpha1.ZrokShare{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
		Spec: zrokv1alpha1.ZrokShareSpec{
			EnvironmentRef:     corev1.LocalObjectReference{Name: "env"},
			Upstream:           zrokv1alpha1.UpstreamSpec{URL: "http://x"},
			NameSelection:      &zrokv1alpha1.NameSelectionSpec{Name: "demo"},
			BasicAuthSecretRef: &corev1.LocalObjectReference{Name: "ba"},
			OAuth:              &zrokv1alpha1.OAuthSpec{Provider: "github", EmailDomains: []string{"b.com", "a.com"}},
			AccessGrants:       []string{"z", "a"},
		},
	}
	a := shareApplyDigest(share, []string{"user:pass"})
	b := shareApplyDigest(share, []string{"user:pass"})
	if a != b {
		t.Fatal("digest must be stable")
	}
	c := shareApplyDigest(share, []string{"user:other"})
	if a == c {
		t.Fatal("credential rotation must rebuild")
	}
}

func TestFrontendNameFromEndpoint_SharesZrokIO(t *testing.T) {
	t.Parallel()
	got := zrokclient.FrontendNameFromEndpoint("https://ko-default-nginx-aaaaoaa.shares.zrok.io")
	if got != "ko-default-nginx-aaaaoaa" {
		t.Fatalf("got %q", got)
	}
	if zrokclient.FrontendEndpointMatchesName(
		"https://ko-default-nginx-aaaaoaa.shares.zrok.io",
		"ko-default-nginx-aaaaoaaaaa",
	) {
		t.Fatal("new longer name must not match old URL")
	}
}
