package controller

import (
	"testing"

	"github.com/openziti/zrok/v2/agent/agentGrpc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/zrokclient"
)

func testShare(name, upstream, frontend, token string) *zrokv1alpha1.ZrokShare {
	s := &zrokv1alpha1.ZrokShare{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: zrokv1alpha1.ZrokShareSpec{
			EnvironmentRef: corev1.LocalObjectReference{Name: "env"},
			ShareMode:      zrokv1alpha1.ShareModePublic,
			Upstream:       zrokv1alpha1.UpstreamSpec{URL: upstream},
			NameSelection:  &zrokv1alpha1.NameSelectionSpec{Name: frontend},
		},
	}
	s.Status.ShareToken = token
	return s
}

func TestClassifyShare_ForeignName(t *testing.T) {
	t.Parallel()
	share := testShare("nginx", "http://nginx.default.svc:80", "demo", "")
	inv := shareInventory{remote: []zrokclient.RemoteShare{{
		Token:             "foreign",
		Target:            "http://someone-else:80",
		FrontendEndpoints: []string{"https://demo.share.zrok.io"},
	}}}
	cls := classifyShare(share, share.Spec.Upstream.URL, inv)
	if !cls.foreignName {
		t.Fatal("expected foreignName")
	}
	if cls.remote == nil || cls.remote.Token != "foreign" {
		t.Fatalf("remote: %+v", cls.remote)
	}
}

func TestClassifyShare_OursByTarget(t *testing.T) {
	t.Parallel()
	share := testShare("nginx", "http://nginx.default.svc:80", "demo", "")
	inv := shareInventory{remote: []zrokclient.RemoteShare{{
		Token:             "ours",
		Target:            "http://nginx.default.svc",
		FrontendEndpoints: []string{"https://demo.share.zrok.io"},
	}}}
	cls := classifyShare(share, share.Spec.Upstream.URL, inv)
	if cls.foreignName {
		t.Fatal("should not be foreign")
	}
	if cls.remote == nil || cls.remote.Token != "ours" {
		t.Fatalf("remote: %+v", cls.remote)
	}
}

func TestClassifyShare_SpecChangeIsOurs(t *testing.T) {
	t.Parallel()
	share := testShare("nginx", "http://nginx.default.svc:80", "demo", "shr-old")
	inv := shareInventory{remote: []zrokclient.RemoteShare{{
		Token:             "shr-old",
		Target:            "http://old-upstream:80",
		FrontendEndpoints: []string{"https://demo.share.zrok.io"},
	}}}
	cls := classifyShare(share, share.Spec.Upstream.URL, inv)
	if cls.foreignName {
		t.Fatal("status token + name holder is our share after spec change")
	}
}

func TestClassifyShare_AdoptAgent(t *testing.T) {
	t.Parallel()
	share := testShare("nginx", "http://nginx.default.svc:80", "demo", "")
	inv := shareInventory{
		agent: []*agentGrpc.ShareDetail{{
			Token:            "live",
			FrontendEndpoint: []string{"https://demo.share.zrok.io"},
			BackendEndpoint:  "http://nginx.default.svc:80",
			Status:           "active",
		}},
	}
	cls := classifyShare(share, share.Spec.Upstream.URL, inv)
	if cls.agent == nil || cls.agent.GetToken() != "live" {
		t.Fatalf("agent: %+v", cls.agent)
	}
	if !agentTargetOK(cls.agent, share.Spec.Upstream.URL) {
		t.Fatal("agent target should match")
	}
}
