package agent

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

func TestDesiredResources(t *testing.T) {
	env := &zrokv1alpha1.ZrokEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "demo"},
		Spec:       zrokv1alpha1.ZrokEnvironmentSpec{},
	}
	if got := DeploymentName(env); got != "default-agent" {
		t.Fatalf("deployment name: %s", got)
	}
	if got := AgentDialAddr(env); got != "default-agent.demo.svc:7777" {
		t.Fatalf("dial addr: %s", got)
	}
	dep := DesiredDeployment(env)
	if len(dep.Spec.Template.Spec.InitContainers) != 1 {
		t.Fatalf("expected 1 init container (seed), got %d", len(dep.Spec.Template.Spec.InitContainers))
	}
	if dep.Spec.Template.Spec.InitContainers[0].Name != "zrok-seed" {
		t.Fatalf("unexpected init: %s", dep.Spec.Template.Spec.InitContainers[0].Name)
	}
	if dep.Spec.Template.Spec.Containers[0].Name != AppName {
		t.Fatalf("unexpected container")
	}
	cmd := dep.Spec.Template.Spec.Containers[0].Command
	if len(cmd) < 3 || !strings.Contains(cmd[2], "agent-registry.json") || !strings.Contains(cmd[2], "zrok2 agent start") {
		t.Fatalf("agent start must wipe registry: %v", cmd)
	}
}

func TestManagedFrontendName(t *testing.T) {
	share := &zrokv1alpha1.ZrokShare{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: "default"},
	}
	if got := ManagedFrontendName(share); got != "ko-default-nginx" {
		t.Fatalf("got %s", got)
	}
}

func TestEnvironmentHostAndDescription(t *testing.T) {
	env := &zrokv1alpha1.ZrokEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "demo"},
	}
	if got := EnvironmentDescription(env); got != "zrok-operator/demo/default" {
		t.Fatalf("description: %s", got)
	}
	if got := EnvironmentHost(env); got != "zrok-operator/demo/default" {
		t.Fatalf("host: %s", got)
	}
}

func TestShareLabels(t *testing.T) {
	share := &zrokv1alpha1.ZrokShare{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: "default"},
		Spec: zrokv1alpha1.ZrokShareSpec{
			EnvironmentRef: corev1.LocalObjectReference{Name: "env"},
			NameSelection:  &zrokv1alpha1.NameSelectionSpec{Name: "demo"},
		},
	}
	got := ShareLabels(share)
	if got["zrok.k8s.zrok.io/environment"] != "env" {
		t.Fatalf("env label: %v", got)
	}
	if got["zrok.k8s.zrok.io/frontend-name"] != "demo" {
		t.Fatalf("frontend label: %v", got)
	}
	if got["app.kubernetes.io/managed-by"] != "zrok-operator" {
		t.Fatalf("managed-by: %v", got)
	}
}
