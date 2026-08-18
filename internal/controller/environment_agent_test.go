package controller

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

func TestIsAgentReady_MissingDeployment(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	r := &ZrokEnvironmentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}
	ready, err := r.isAgentReady(context.Background(), &zrokv1alpha1.ZrokEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("missing Deployment must not be a reconcile error: %v", err)
	}
	if ready {
		t.Fatal("expected not ready")
	}
}
