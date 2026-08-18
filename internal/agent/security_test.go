package agent

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

func TestImageAllowed(t *testing.T) {
	t.Parallel()
	if !ImageAllowed("", nil) {
		t.Fatal("empty (default) must be allowed")
	}
	if !ImageAllowed(DefaultImage, nil) {
		t.Fatal("DefaultImage must be allowed")
	}
	if ImageAllowed("evil/zrok:latest", nil) {
		t.Fatal("unknown image must be rejected")
	}
	if !ImageAllowed("evil/zrok:latest", []string{"evil/zrok:latest"}) {
		t.Fatal("allowlisted extra must pass")
	}
}

func TestDesiredServiceIsClusterIPGRPCOnly(t *testing.T) {
	t.Parallel()
	env := &zrokv1alpha1.ZrokEnvironment{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "demo"}}
	svc := DesiredService(env)
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("type %s", svc.Spec.Type)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Name != "grpc" {
		t.Fatalf("ports %+v", svc.Spec.Ports)
	}
}

func TestDesiredDeploymentHardening(t *testing.T) {
	t.Parallel()
	env := &zrokv1alpha1.ZrokEnvironment{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "demo"}}
	dep := DesiredDeployment(env)
	spec := dep.Spec.Template.Spec
	if spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
		t.Fatal("agent must not mount a service account token")
	}
	agent := spec.Containers[0]
	if !strings.Contains(agent.Command[2], "--console-address 127.0.0.1") {
		t.Fatalf("console must bind localhost: %v", agent.Command)
	}
	if agent.ReadinessProbe != nil {
		t.Fatal("agent container must not HTTP-probe the localhost console")
	}
	proxy := spec.Containers[1]
	if proxy.ReadinessProbe == nil || proxy.ReadinessProbe.TCPSocket == nil {
		t.Fatal("grpc-proxy must use TCP readiness")
	}
}

func TestDesiredNetworkPolicyFromManagerNS(t *testing.T) {
	t.Parallel()
	env := &zrokv1alpha1.ZrokEnvironment{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "demo"}}
	np := DesiredNetworkPolicy(env, "zrok-operator-system", "")
	if np.Namespace != "demo" || np.Name != "default-agent" {
		t.Fatalf("meta %s/%s", np.Namespace, np.Name)
	}
	from := np.Spec.Ingress[0].From[0]
	if from.NamespaceSelector.MatchLabels[NamespaceNameLabel] != "zrok-operator-system" {
		t.Fatalf("ns selector %+v", from.NamespaceSelector)
	}
	if from.PodSelector.MatchLabels["app.kubernetes.io/name"] != ManagerAppNameLabel {
		t.Fatalf("pod selector %+v", from.PodSelector)
	}
}
