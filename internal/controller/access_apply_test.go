package controller

import (
	"testing"

	"github.com/openziti/zrok/v2/agent/agentGrpc"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

func TestAccessSpecDrifted(t *testing.T) {
	t.Parallel()
	access := &zrokv1alpha1.ZrokAccess{
		Spec: zrokv1alpha1.ZrokAccessSpec{
			ShareToken:  "shr-a",
			BindAddress: "127.0.0.1:0",
		},
	}
	live := &agentGrpc.AccessDetail{
		Token:         "shr-a",
		BindAddress:   "127.0.0.1:4040",
		FrontendToken: "acc",
		Status:        "active",
	}
	if accessSpecDrifted(access, live) {
		t.Fatal(":0 bind must not compare against assigned port")
	}
	live.Token = "shr-b"
	if !accessSpecDrifted(access, live) {
		t.Fatal("shareToken change")
	}
	live.Token = "shr-a"
	access.Spec.BindAddress = "127.0.0.1:8080"
	if !accessSpecDrifted(access, live) {
		t.Fatal("explicit bind drift")
	}
	live.BindAddress = "127.0.0.1:8080"
	if accessSpecDrifted(access, live) {
		t.Fatal("matching bind")
	}
}
