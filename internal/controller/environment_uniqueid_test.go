package controller

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

func TestResolveUniqueID(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: zrokv1alpha1.DefaultUniqueIDNamespace,
			UID:  "kube-system-uid",
		},
	}
	r := &ZrokEnvironmentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build(),
	}

	got, err := r.resolveUniqueID(context.Background(), &zrokv1alpha1.ZrokEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "kube-system-uid" {
		t.Fatalf("got %q", got)
	}

	env := &zrokv1alpha1.ZrokEnvironment{
		Spec: zrokv1alpha1.ZrokEnvironmentSpec{UniqueID: "override"},
	}
	got, err = r.resolveUniqueID(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if got != "override" {
		t.Fatalf("override: %q", got)
	}
}

func TestResolveUniqueIDOverrideWithoutKubeSystem(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	r := &ZrokEnvironmentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}
	env := &zrokv1alpha1.ZrokEnvironment{
		Spec: zrokv1alpha1.ZrokEnvironmentSpec{UniqueID: "override"},
	}
	got, err := r.resolveUniqueID(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if got != "override" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveUniqueIDMissingKubeSystem(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	r := &ZrokEnvironmentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}
	_, err := r.resolveUniqueID(context.Background(), &zrokv1alpha1.ZrokEnvironment{})
	if err == nil {
		t.Fatal("expected error when kube-system is missing")
	}
}
