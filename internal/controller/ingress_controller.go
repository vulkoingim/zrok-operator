/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

const (
	// IngressClassName selects Ingresses managed by this operator.
	IngressClassName = "zrok"

	annotationEnvironment = "zrok.k8s.zrok.io/environment"
	annotationName        = "zrok.k8s.zrok.io/name"
	annotationNamespace   = "zrok.k8s.zrok.io/namespace-token"
)

// IngressReconciler translates Ingress resources with ingressClassName=zrok into ZrokShare CRs.
type IngressReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokshares,verbs=get;list;watch;create;update;patch;delete

func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ing := &networkingv1.Ingress{}
	if err := r.Get(ctx, req.NamespacedName, ing); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != IngressClassName {
		return ctrl.Result{}, nil
	}

	if !ing.DeletionTimestamp.IsZero() {
		// Owned ZrokShares are garbage-collected via ownerReference.
		return ctrl.Result{}, nil
	}

	envName := ing.Annotations[annotationEnvironment]
	if envName == "" {
		envName = "default"
	}

	if len(ing.Spec.Rules) == 0 || len(ing.Spec.Rules[0].HTTP.Paths) == 0 {
		r.Recorder.Event(ing, corev1.EventTypeWarning, "InvalidIngress", "no HTTP paths defined")
		return ctrl.Result{}, nil
	}

	path := ing.Spec.Rules[0].HTTP.Paths[0]
	if path.Backend.Service == nil {
		r.Recorder.Event(ing, corev1.EventTypeWarning, "InvalidIngress", "backend service required")
		return ctrl.Result{}, nil
	}

	svc := path.Backend.Service
	port := svc.Port.Number
	if port == 0 {
		r.Recorder.Event(ing, corev1.EventTypeWarning, "InvalidIngress", "numeric service port required")
		return ctrl.Result{}, nil
	}

	upstream := fmt.Sprintf("http://%s.%s.svc:%d", svc.Name, ing.Namespace, port)
	shareName := ing.Name

	desired := &zrokv1alpha1.ZrokShare{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shareName,
			Namespace: ing.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, desired, func() error {
		if err := controllerutil.SetControllerReference(ing, desired, r.Scheme); err != nil {
			return err
		}
		desired.Spec.EnvironmentRef = corev1.LocalObjectReference{Name: envName}
		desired.Spec.ShareMode = zrokv1alpha1.ShareModePublic
		desired.Spec.BackendMode = zrokv1alpha1.BackendModeProxy
		desired.Spec.Upstream = zrokv1alpha1.UpstreamSpec{URL: upstream}
		desired.Spec.ReclaimPolicy = zrokv1alpha1.ReclaimDelete

		name := ing.Annotations[annotationName]
		if name == "" && ing.Spec.Rules[0].Host != "" {
			name = ing.Spec.Rules[0].Host
		}
		if name != "" {
			nsToken := ing.Annotations[annotationNamespace]
			if nsToken == "" {
				nsToken = zrokv1alpha1.DefaultNamespaceToken
			}
			desired.Spec.NameSelection = &zrokv1alpha1.NameSelectionSpec{
				Namespace: nsToken,
				Name:      name,
			}
		}
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("reconciled ingress → share", "op", op)

	// Reflect share URL onto Ingress status when ready.
	share := &zrokv1alpha1.ZrokShare{}
	if err := r.Get(ctx, client.ObjectKey{Name: shareName, Namespace: ing.Namespace}, share); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if share.Status.AssignedURL != "" {
		host := strings.TrimPrefix(strings.TrimPrefix(share.Status.AssignedURL, "https://"), "http://")
		ing.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{
			Hostname: host,
		}}
		if err := r.Status().Update(ctx, ing); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Owns(&zrokv1alpha1.ZrokShare{}).
		Named("zrok-ingress").
		Complete(r)
}
