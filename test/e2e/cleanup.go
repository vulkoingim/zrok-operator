package e2e

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

const crDeleteTimeout = 3 * time.Minute

func cleanupLiveZrokTestResources(ctx context.Context) {
	deleteAllZrokShares(ctx)
	deleteAllZrokAccesses(ctx)
	deleteAllZrokEnvironments(ctx)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      credentialsSecret,
			Namespace: liveTestNS,
		},
	}
	deleteObject(ctx, secret, false)

	identity := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      envCRName + "-zrok-identity",
			Namespace: liveTestNS,
		},
	}
	deleteObject(ctx, identity, false)

	deleteObject(ctx, desiredNginxDeployment(liveTestNS), true)
	deleteObject(ctx, desiredNginxService(liveTestNS), true)

	agentDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      envCRName + "-agent",
			Namespace: liveTestNS,
		},
	}
	deleteObject(ctx, agentDeploy, true)

	agentSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      envCRName + "-agent",
			Namespace: liveTestNS,
		},
	}
	deleteObject(ctx, agentSvc, true)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      envCRName + "-zrok-home",
			Namespace: liveTestNS,
		},
	}
	deleteObject(ctx, pvc, true)
}

func deleteAllZrokShares(ctx context.Context) {
	list := &zrokv1alpha1.ZrokShareList{}
	if err := k8sClient.List(ctx, list, client.InNamespace(liveTestNS)); err != nil {
		return
	}
	for i := range list.Items {
		item := list.Items[i]
		deleteZrokCR(ctx, &item)
	}
}

func deleteAllZrokAccesses(ctx context.Context) {
	list := &zrokv1alpha1.ZrokAccessList{}
	if err := k8sClient.List(ctx, list, client.InNamespace(liveTestNS)); err != nil {
		return
	}
	for i := range list.Items {
		item := list.Items[i]
		deleteZrokCR(ctx, &item)
	}
}

func deleteAllZrokEnvironments(ctx context.Context) {
	list := &zrokv1alpha1.ZrokEnvironmentList{}
	if err := k8sClient.List(ctx, list, client.InNamespace(liveTestNS)); err != nil {
		return
	}
	for i := range list.Items {
		item := list.Items[i]
		deleteZrokCR(ctx, &item)
	}
}

func deleteZrokCR(ctx context.Context, obj client.Object) {
	key := client.ObjectKeyFromObject(obj)
	if err := k8sClient.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return
	}

	deadline, cancel := context.WithTimeout(ctx, crDeleteTimeout)
	defer cancel()

	for {
		err := k8sClient.Get(deadline, key, obj)
		if apierrors.IsNotFound(err) {
			return
		}
		select {
		case <-deadline.Done():
			clearFinalizersAndDelete(ctx, key, obj)
			return
		case <-time.After(time.Second):
		}
	}
}
