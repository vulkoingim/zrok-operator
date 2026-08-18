package e2e

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/status"
)

const (
	managerNS         = "zrok-operator-system"
	liveTestNS        = "default"
	nginxName         = "nginx"
	envCRName         = "default"
	credentialsSecret = "zrok-credentials"
)

var (
	testCtx      context.Context
	restCfg      *rest.Config
	k8sClient    client.Client
	clientset    *kubernetes.Clientset
	e2eScheme    = runtime.NewScheme()
	codecFactory serializer.CodecFactory
)

//nolint:gochecknoinits // shut it
func init() {
	_ = clientgoscheme.AddToScheme(e2eScheme)
	_ = zrokv1alpha1.AddToScheme(e2eScheme)
	codecFactory = serializer.NewCodecFactory(e2eScheme)
}

func initK8sClients(ctx context.Context) error {
	var err error
	restCfg, err = config.GetConfig()
	if err != nil {
		return fmt.Errorf("load kubeconfig: %w", err)
	}

	k8sClient, err = client.New(restCfg, client.Options{Scheme: e2eScheme})
	if err != nil {
		return fmt.Errorf("create controller-runtime client: %w", err)
	}

	clientset, err = kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("create kubernetes clientset: %w", err)
	}

	testCtx = ctx
	return nil
}

func ensureManagerNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managerNS}}
	if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: managerNS}, ns); err != nil {
		return err
	}
	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}
	ns.Labels["pod-security.kubernetes.io/enforce"] = "restricted"
	return k8sClient.Update(ctx, ns)
}

func deleteManagerNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managerNS}}
	if err := k8sClient.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func controllerManagerPod(ctx context.Context) (*corev1.Pod, error) {
	list := &corev1.PodList{}
	if err := k8sClient.List(ctx, list,
		client.InNamespace(managerNS),
		client.MatchingLabels{"control-plane": "controller-manager"},
	); err != nil {
		return nil, err
	}
	var active []corev1.Pod
	for _, pod := range list.Items {
		if pod.DeletionTimestamp == nil {
			active = append(active, pod)
		}
	}
	if len(active) != 1 {
		return nil, fmt.Errorf("expected 1 controller pod, got %d", len(active))
	}
	return &active[0], nil
}

func podLogs(ctx context.Context, namespace, name string) (string, error) {
	req := clientset.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{})
	raw, err := req.DoRaw(ctx)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func listEvents(ctx context.Context, namespace string) ([]corev1.Event, error) {
	list, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func createOrUpdate(ctx context.Context, obj client.Object) error {
	current := obj.DeepCopyObject().(client.Object)
	key := client.ObjectKeyFromObject(obj)
	err := k8sClient.Get(ctx, key, current)
	if apierrors.IsNotFound(err) {
		return k8sClient.Create(ctx, obj)
	}
	if err != nil {
		return err
	}
	obj.SetResourceVersion(current.GetResourceVersion())
	return k8sClient.Update(ctx, obj)
}

func deleteObject(ctx context.Context, obj client.Object, wait bool) {
	key := client.ObjectKeyFromObject(obj)
	if err := k8sClient.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return
	}
	if !wait {
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

func clearFinalizersAndDelete(ctx context.Context, key types.NamespacedName, prototype client.Object) {
	fresh := prototype.DeepCopyObject().(client.Object)
	if err := k8sClient.Get(ctx, key, fresh); apierrors.IsNotFound(err) {
		return
	}
	original := fresh.DeepCopyObject().(client.Object)
	fresh.SetFinalizers(nil)
	if err := k8sClient.Patch(ctx, fresh, client.MergeFrom(original)); err != nil && !apierrors.IsNotFound(err) {
		return
	}
	_ = k8sClient.Delete(ctx, fresh, client.PropagationPolicy(metav1.DeletePropagationBackground))
}

func conditionReady(obj client.Object) bool {
	switch o := obj.(type) {
	case *zrokv1alpha1.ZrokEnvironment:
		return status.IsTrue(o.Status.Conditions, zrokv1alpha1.ConditionReady)
	case *zrokv1alpha1.ZrokShare:
		return status.IsTrue(o.Status.Conditions, zrokv1alpha1.ConditionReady)
	default:
		return false
	}
}

func desiredNginxDeployment(namespace string) *appsv1.Deployment {
	replicas := int32(1)
	labels := map[string]string{"app": nginxName}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: nginxName, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  nginxName,
						Image: "nginx:stable",
						Ports: []corev1.ContainerPort{{ContainerPort: 80}},
					}},
				},
			},
		},
	}
}

func desiredNginxService(namespace string) *corev1.Service {
	labels := map[string]string{"app": nginxName}
	port := int32(80)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: nginxName, Namespace: namespace},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Port:       port,
				TargetPort: intstr.FromInt32(port),
			}},
		},
	}
}
