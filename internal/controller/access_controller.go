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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/agent"
	"github.com/vulkoingim/zrok-operator/internal/status"
	"github.com/vulkoingim/zrok-operator/internal/zrokclient"
)

// ZrokAccessReconciler reconciles private share accesses.
type ZrokAccessReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Zrok     *zrokclient.Clients
}

// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokaccesses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokaccesses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokaccesses/finalizers,verbs=update
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokenvironments,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *ZrokAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	access := &zrokv1alpha1.ZrokAccess{}
	if err := r.Get(ctx, req.NamespacedName, access); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !access.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, access)
	}

	if !controllerutil.ContainsFinalizer(access, zrokv1alpha1.AccessFinalizer) {
		controllerutil.AddFinalizer(access, zrokv1alpha1.AccessFinalizer)
		if err := r.Update(ctx, access); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	env := &zrokv1alpha1.ZrokEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: access.Spec.EnvironmentRef.Name, Namespace: access.Namespace}, env); err != nil {
		if apierrors.IsNotFound(err) {
			status.SetCondition(&access.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "EnvironmentMissing", err.Error(), access.Generation)
			_ = r.Status().Update(ctx, access)
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	if !status.IsTrue(env.Status.Conditions, zrokv1alpha1.ConditionReady) {
		status.SetCondition(&access.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "WaitingForEnvironment", "environment not ready", access.Generation)
		_ = r.Status().Update(ctx, access)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if access.Status.AccessToken != "" {
		status.SetCondition(&access.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "access active", access.Generation)
		access.Status.ObservedGeneration = access.Generation
		_ = r.Status().Update(ctx, access)
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
	}

	bind := access.Spec.BindAddress
	if bind == "" {
		bind = "0.0.0.0:0"
	}
	resp, err := r.Zrok.Agent.AccessPrivate(ctx, agent.AgentBaseURL(env), zrokclient.AccessPrivateRequest{
		Token:       access.Spec.ShareToken,
		BindAddress: bind,
	})
	if err != nil {
		status.SetCondition(&access.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "AccessError", err.Error(), access.Generation)
		_ = r.Status().Update(ctx, access)
		r.Recorder.Event(access, corev1.EventTypeWarning, "AccessError", err.Error())
		return ctrl.Result{}, err
	}

	access.Status.AccessToken = resp.FrontendToken
	access.Status.FrontendEndpoint = resp.FrontendToken
	access.Status.ObservedGeneration = access.Generation
	status.SetCondition(&access.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "access active", access.Generation)
	if err := r.Status().Update(ctx, access); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Event(access, corev1.EventTypeNormal, "Ready", "private access ready")
	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

func (r *ZrokAccessReconciler) reconcileDelete(ctx context.Context, access *zrokv1alpha1.ZrokAccess) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(access, zrokv1alpha1.AccessFinalizer) {
		return ctrl.Result{}, nil
	}

	env := &zrokv1alpha1.ZrokEnvironment{}
	err := r.Get(ctx, types.NamespacedName{Name: access.Spec.EnvironmentRef.Name, Namespace: access.Namespace}, env)
	if err == nil && access.Status.AccessToken != "" {
		if err := r.Zrok.Agent.ReleaseAccess(ctx, agent.AgentBaseURL(env), access.Status.AccessToken); err != nil {
			log.FromContext(ctx).Error(err, "release access failed; continuing")
		}
	}

	controllerutil.RemoveFinalizer(access, zrokv1alpha1.AccessFinalizer)
	if err := r.Update(ctx, access); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZrokAccessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zrokv1alpha1.ZrokAccess{}).
		Named("zrokaccess").
		Complete(r)
}
