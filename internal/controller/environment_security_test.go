package controller

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/agent"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := zrokv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func TestAdoptIdentitySecret_DeletesUnowned(t *testing.T) {
	t.Parallel()
	scheme := testScheme(t)
	env := &zrokv1alpha1.ZrokEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "ns", UID: "env-uid"},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: agent.IdentitySecretName(env), Namespace: "ns"},
		Data:       map[string][]byte{"envZID": []byte("planted")},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(env, secret).Build()
	r := &ZrokEnvironmentReconciler{Client: cl, Scheme: scheme}

	err := r.adoptIdentitySecret(context.Background(), env, secret)
	if err == nil || !strings.Contains(err.Error(), "unowned") {
		t.Fatalf("got %v", err)
	}
	got := &corev1.Secret{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: secret.Name, Namespace: "ns"}, got); !apierrors.IsNotFound(err) {
		t.Fatalf("planted secret still present: %v", err)
	}
}

func TestAdoptIdentitySecret_OwnedCrashRecovery(t *testing.T) {
	t.Parallel()
	scheme := testScheme(t)
	env := &zrokv1alpha1.ZrokEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "ns", UID: "env-uid"},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: agent.IdentitySecretName(env), Namespace: "ns"},
		Data:       map[string][]byte{"envZID": []byte("zid-1")},
	}
	if err := controllerutil.SetControllerReference(env, secret, scheme); err != nil {
		t.Fatal(err)
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(env).
		WithObjects(env, secret).
		Build()
	r := &ZrokEnvironmentReconciler{Client: cl, Scheme: scheme}

	if err := r.adoptIdentitySecret(context.Background(), env, secret); err != nil {
		t.Fatal(err)
	}
	updated := &zrokv1alpha1.ZrokEnvironment{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(env), updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.EnvZID != "zid-1" {
		t.Fatalf("status envZID %q", updated.Status.EnvZID)
	}
}

func TestEnsureServiceDeletesExternalName(t *testing.T) {
	t.Parallel()
	scheme := testScheme(t)
	env := &zrokv1alpha1.ZrokEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "ns", UID: "env-uid"},
	}
	hijack := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: agent.ServiceName(env), Namespace: "ns"},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: "evil.example",
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(env, hijack).Build()
	r := &ZrokEnvironmentReconciler{Client: cl, Scheme: scheme}

	err := r.ensureService(context.Background(), env)
	if err == nil || !strings.Contains(err.Error(), "non-ClusterIP") {
		t.Fatalf("got %v", err)
	}
	got := &corev1.Service{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(hijack), got); !apierrors.IsNotFound(err) {
		t.Fatalf("hijack Service still present: %v", err)
	}
}

func TestHijackedAgentService(t *testing.T) {
	t.Parallel()
	if hijackedAgentService(&corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}}) {
		t.Fatal("plain ClusterIP")
	}
	if !hijackedAgentService(&corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort}}) {
		t.Fatal("NodePort")
	}
	if !hijackedAgentService(&corev1.Service{Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeClusterIP, ExternalName: "x.example",
	}}) {
		t.Fatal("ExternalName leftover")
	}
}
