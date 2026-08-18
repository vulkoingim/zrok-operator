/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

    10|Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/agent"
	opmetrics "github.com/vulkoingim/zrok-operator/internal/metrics"
	"github.com/vulkoingim/zrok-operator/internal/status"
	"github.com/vulkoingim/zrok-operator/internal/zrokclient"
)

// ZrokEnvironmentReconciler reconciles a ZrokEnvironment object.
type ZrokEnvironmentReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	Zrok     *zrokclient.Clients
	// APIReader bypasses the informer cache. Namespaces are not watched.
	APIReader client.Reader

	// ManagerNamespace is the namespace the manager pod runs in (NetworkPolicy from:).
	ManagerNamespace string
	// ManagerAppName is app.kubernetes.io/name on the manager pod.
	ManagerAppName string
	// AgentNetworkPolicy creates per-environment NetworkPolicies (Helm networkPolicy.enabled).
	AgentNetworkPolicy bool
	// AllowedAgentImages are extra images beyond agent.DefaultImage.
	AllowedAgentImages []string
}

// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokenvironments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokenvironments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokenvironments/finalizers,verbs=update
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokshares,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch;update
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=namespaces,resourceNames=kube-system,verbs=get
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

func (r *ZrokEnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	env := &zrokv1alpha1.ZrokEnvironment{}
	if err := r.Get(ctx, req.NamespacedName, env); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !env.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, env)
	}

	if !controllerutil.ContainsFinalizer(env, zrokv1alpha1.EnvironmentFinalizer) {
		controllerutil.AddFinalizer(env, zrokv1alpha1.EnvironmentFinalizer)
		if err := r.Update(ctx, env); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	token, err := r.readEnableToken(ctx, env)
	if err != nil {
		_ = r.setNotReady(ctx, env, "SecretError", err.Error())
		opmetrics.EnvironmentReconcileErrors.Inc()
		r.Recorder.Eventf(env, nil, corev1.EventTypeWarning, "SecretError", "Error", "%s", err.Error())
		return ctrl.Result{}, err
	}

	if err = agent.ValidateImage(env.Spec.Agent.Image, r.AllowedAgentImages); err != nil {
		_ = r.setNotReady(ctx, env, "ImageNotAllowed", err.Error())
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
	}

	if err = r.ensureEnabled(ctx, env, token); err != nil {
		if zrokclient.IsEndpointNotAllowed(err) {
			_ = r.setNotReady(ctx, env, "EndpointNotAllowed", err.Error())
			return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
		}
		_ = r.setNotReady(ctx, env, "EnableError", err.Error())
		opmetrics.EnvironmentReconcileErrors.Inc()
		r.Recorder.Eventf(env, nil, corev1.EventTypeWarning, "EnableError", "Error", "%s", err.Error())
		return ctrl.Result{}, err
	}

	if err = r.ensurePVC(ctx, env); err != nil {
		_ = r.setNotReady(ctx, env, "PVCError", err.Error())
		opmetrics.EnvironmentReconcileErrors.Inc()
		return ctrl.Result{}, err
	}

	if err = r.ensureService(ctx, env); err != nil {
		_ = r.setNotReady(ctx, env, "ServiceError", err.Error())
		opmetrics.EnvironmentReconcileErrors.Inc()
		return ctrl.Result{}, err
	}

	if err = r.ensureDeployment(ctx, env); err != nil {
		_ = r.setNotReady(ctx, env, "DeploymentError", err.Error())
		opmetrics.EnvironmentReconcileErrors.Inc()
		return ctrl.Result{}, err
	}

	if err = r.ensureNetworkPolicy(ctx, env); err != nil {
		_ = r.setNotReady(ctx, env, "NetworkPolicyError", err.Error())
		opmetrics.EnvironmentReconcileErrors.Inc()
		return ctrl.Result{}, err
	}

	agentReady, err := r.isAgentReady(ctx, env)
	if err != nil {
		_ = r.setNotReady(ctx, env, "AgentCheckError", err.Error())
		opmetrics.EnvironmentReconcileErrors.Inc()
		return ctrl.Result{}, err
	}

	agentService := fmt.Sprintf("%s.%s.svc", agent.ServiceName(env), env.Namespace)

	if !agentReady {
		if err := status.PatchStatus(ctx, r.Client, env, func() error {
			env.Status.ObservedGeneration = env.Generation
			env.Status.AgentService = agentService
			env.Status.AgentReady = false
			status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionAgentReady, metav1.ConditionFalse, "Waiting", "agent Deployment not ready", env.Generation)
			status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionEnabled, metav1.ConditionTrue, "Enabled", "remote environment enabled", env.Generation)
			status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "WaitingForAgent", "agent not ready", env.Generation)
			return nil
		}); err != nil {
			return ctrl.Result{}, err
		}
		opmetrics.SetEnvironmentReady(env.Namespace, env.Name, false)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if _, statusErr := r.Zrok.Agent.Status(ctx, agent.AgentDialAddr(env)); statusErr != nil {
		logger.Info("agent status not ready yet", "error", statusErr)
		if err := status.PatchStatus(ctx, r.Client, env, func() error {
			env.Status.ObservedGeneration = env.Generation
			env.Status.AgentService = agentService
			env.Status.AgentReady = false
			status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionAgentReady, metav1.ConditionFalse, "ConsoleUnreachable", statusErr.Error(), env.Generation)
			status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "ConsoleUnreachable", statusErr.Error(), env.Generation)
			return nil
		}); err != nil {
			return ctrl.Result{}, err
		}
		opmetrics.SetEnvironmentReady(env.Namespace, env.Name, false)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	wasReady := status.IsTrue(env.Status.Conditions, zrokv1alpha1.ConditionReady)
	if err := status.PatchStatus(ctx, r.Client, env, func() error {
		env.Status.ObservedGeneration = env.Generation
		env.Status.AgentService = agentService
		env.Status.AgentReady = true
		status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionAgentReady, metav1.ConditionTrue, "Ready", "agent gRPC reachable", env.Generation)
		status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionEnabled, metav1.ConditionTrue, "Enabled", "environment enabled", env.Generation)
		status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "environment ready", env.Generation)
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}
	opmetrics.SetEnvironmentReady(env.Namespace, env.Name, true)
	if !wasReady {
		r.Recorder.Eventf(env, nil, corev1.EventTypeNormal, "Ready", "Ready", "ZrokEnvironment is ready")
	}
	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

func (r *ZrokEnvironmentReconciler) reconcileDelete(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(env, zrokv1alpha1.EnvironmentFinalizer) {
		return ctrl.Result{}, nil
	}

	// Block delete while Shares still reference this environment.
	shares := &zrokv1alpha1.ZrokShareList{}
	if err := r.List(ctx, shares, client.InNamespace(env.Namespace)); err != nil {
		return ctrl.Result{}, err
	}
	for i := range shares.Items {
		s := &shares.Items[i]
		if s.Spec.EnvironmentRef.Name != env.Name || !s.DeletionTimestamp.IsZero() {
			continue
		}
		msg := fmt.Sprintf("cannot delete environment while share %s exists", s.Name)
		_ = status.PatchStatus(ctx, r.Client, env, func() error {
			status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "SharesExist", msg, env.Generation)
			return nil
		})
		opmetrics.SetEnvironmentReady(env.Namespace, env.Name, false)
		r.Recorder.Eventf(env, nil, corev1.EventTypeWarning, "SharesExist", "Error", "%s", msg)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// Disable remote environment before tearing down local resources.
	if env.Spec.ReclaimPolicy != zrokv1alpha1.ReclaimRetain && env.Status.EnvZID != "" {
		token, err := r.readEnableToken(ctx, env)
		if err != nil {
			return ctrl.Result{}, err
		}
		api := env.Spec.ApiEndpoint
		if api == "" {
			api = zrokclient.DefaultAPIEndpoint
		}
		if err := r.Zrok.REST.Disable(ctx, api, token, env.Status.EnvZID); err != nil {
			log.FromContext(ctx).Error(err, "disable environment failed; will retry")
			r.Recorder.Eventf(env, nil, corev1.EventTypeWarning, "DisableError", "Error", "%s", err.Error())
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}
		r.Recorder.Eventf(env, nil, corev1.EventTypeNormal, "Disabled", "Disable", "disabled remote environment %s", env.Status.EnvZID)
	}

	_ = r.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: agent.DeploymentName(env), Namespace: env.Namespace}})
	_ = r.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: agent.ServiceName(env), Namespace: env.Namespace}})
	_ = r.Delete(ctx, &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: agent.NetworkPolicyName(env), Namespace: env.Namespace}})
	_ = r.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: agent.IdentitySecretName(env), Namespace: env.Namespace}})
	if env.Spec.ReclaimPolicy != zrokv1alpha1.ReclaimRetain {
		_ = r.Delete(ctx, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: agent.PVCName(env), Namespace: env.Namespace}})
	}

	controllerutil.RemoveFinalizer(env, zrokv1alpha1.EnvironmentFinalizer)
	if err := r.Update(ctx, env); err != nil {
		return ctrl.Result{}, err
	}
	opmetrics.DeleteEnvironmentReady(env.Namespace, env.Name)
	return ctrl.Result{}, nil
}

func (r *ZrokEnvironmentReconciler) readEnableToken(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment) (string, error) {
	ref := env.Spec.EnableTokenSecretRef
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: env.Namespace, Name: ref.Name}
	if err := r.Get(ctx, key, secret); err != nil {
		return "", fmt.Errorf("get enable token secret %s: %w", ref.Name, err)
	}
	secretKey := ref.Key
	if secretKey == "" {
		secretKey = zrokv1alpha1.DefaultEnableTokenKey
	}
	raw, ok := secret.Data[secretKey]
	if !ok || len(raw) == 0 {
		return "", fmt.Errorf("secret %s missing key %q", ref.Name, secretKey)
	}
	return string(raw), nil
}

// ensureEnabled enables the remote zrok environment (once) and stores EnvZID + identity Secret.
func (r *ZrokEnvironmentReconciler) ensureEnabled(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment, token string) error {
	api := env.Spec.ApiEndpoint
	if api == "" {
		api = zrokclient.DefaultAPIEndpoint
	}

	secretName := agent.IdentitySecretName(env)
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: env.Namespace}, existing)
	if err == nil {
		return r.adoptIdentitySecret(ctx, env, existing)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	if env.Status.EnvZID != "" {
		// Status has EnvZID but Secret missing (manual delete) — cannot rebuild identity cfg; force re-enable.
		_ = status.PatchStatus(ctx, r.Client, env, func() error {
			env.Status.EnvZID = ""
			return nil
		})
	}

	uniqueID, err := r.resolveUniqueID(ctx, env)
	if err != nil {
		return err
	}

	host := agent.EnvironmentHost(env, uniqueID)
	desc := agent.EnvironmentDescription(env, uniqueID)
	zid, cfg, err := r.Zrok.REST.Enable(ctx, api, token, host, desc)
	if err != nil {
		return err
	}

	envJSON, err := json.Marshal(map[string]string{
		"zrok_token":    token,
		"ziti_identity": zid,
		"api_endpoint":  api,
	})
	if err != nil {
		return err
	}
	metaJSON := []byte(`{"v":"v0.4"}`)
	cfgJSON, err := json.Marshal(map[string]any{
		"api_endpoint": api,
		"headless":     true,
	})
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: env.Namespace,
			Labels:    agent.Labels(env),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"envZID":           []byte(zid),
			"environment.json": envJSON,
			"identity":         []byte(cfg),
			"metadata.json":    metaJSON,
			"config.json":      cfgJSON,
		},
	}
	if err := controllerutil.SetControllerReference(env, secret, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, secret); err != nil {
		// Another reconcile won the race — disable the orphan we just created.
		_ = r.Zrok.REST.Disable(ctx, api, token, zid)
		return err
	}

	if err := status.PatchStatus(ctx, r.Client, env, func() error {
		env.Status.EnvZID = zid
		env.Status.UniqueID = uniqueID
		status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionEnabled, metav1.ConditionTrue, "Enabled", "remote environment enabled", env.Generation)
		return nil
	}); err != nil {
		return err
	}
	r.Recorder.Eventf(env, nil, corev1.EventTypeNormal, "Enabled", "Enable", "enabled remote environment %s", zid)
	return nil
}

// adoptIdentitySecret trusts a Secret only if this Env is its controller owner.
// Unowned Secrets are deleted so Enable can proceed (planted-identity defense).
func (r *ZrokEnvironmentReconciler) adoptIdentitySecret(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment, existing *corev1.Secret) error {
	if !metav1.IsControlledBy(existing, env) {
		if err := r.Delete(ctx, existing); err != nil {
			return fmt.Errorf("deleting unowned identity secret %s: %w", existing.Name, err)
		}
		return fmt.Errorf("deleted unowned identity secret %s; will retry Enable", existing.Name)
	}
	zid := string(existing.Data["envZID"])
	if zid == "" {
		if err := r.Delete(ctx, existing); err != nil {
			return fmt.Errorf("deleting incomplete identity secret %s: %w", existing.Name, err)
		}
		return fmt.Errorf("deleted incomplete identity secret %s; will retry Enable", existing.Name)
	}
	if env.Status.EnvZID == "" {
		return status.PatchStatus(ctx, r.Client, env, func() error {
			env.Status.EnvZID = zid
			return nil
		})
	}
	if env.Status.EnvZID != zid {
		return fmt.Errorf("identity secret envZID does not match status")
	}
	return nil
}

func (r *ZrokEnvironmentReconciler) resolveUniqueID(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment) (string, error) {
	if env.Spec.UniqueID != "" {
		return env.Spec.UniqueID, nil
	}
	reader := client.Reader(r)
	if r.APIReader != nil {
		reader = r.APIReader
	}
	ns := &corev1.Namespace{}
	if err := reader.Get(ctx, types.NamespacedName{Name: zrokv1alpha1.DefaultUniqueIDNamespace}, ns); err != nil {
		return "", fmt.Errorf("getting %s UUID for default uniqueID: %w",
			zrokv1alpha1.DefaultUniqueIDNamespace, err)
	}
	uid := string(ns.UID)
	if uid == "" {
		return "", fmt.Errorf("%s namespace has empty UID", zrokv1alpha1.DefaultUniqueIDNamespace)
	}
	return uid, nil
}

func (r *ZrokEnvironmentReconciler) ensurePVC(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment) error {
	desired := agent.DesiredPVC(env)
	if err := controllerutil.SetControllerReference(env, desired, r.Scheme); err != nil {
		return err
	}
	existing := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(err) {
		return ignoreAlreadyExists(r.Create(ctx, desired))
	}
	return err
}

func (r *ZrokEnvironmentReconciler) ensureService(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment) error {
	desired := agent.DesiredService(env)
	if err := controllerutil.SetControllerReference(env, desired, r.Scheme); err != nil {
		return err
	}
	existing := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(err) {
		return ignoreAlreadyExists(r.Create(ctx, desired))
	}
	if err != nil {
		return err
	}
	if hijackedAgentService(existing) {
		if err := r.Delete(ctx, existing); err != nil {
			return err
		}
		return fmt.Errorf("deleted non-ClusterIP agent Service %s; will recreate", existing.Name)
	}
	if apiequality.Semantic.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) &&
		apiequality.Semantic.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) &&
		existing.Spec.Type == corev1.ServiceTypeClusterIP {
		return nil
	}
	existing.Spec.Type = corev1.ServiceTypeClusterIP
	existing.Spec.ExternalName = ""
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Ports = desired.Spec.Ports
	return r.Update(ctx, existing)
}

func hijackedAgentService(svc *corev1.Service) bool {
	return svc.Spec.Type != corev1.ServiceTypeClusterIP || svc.Spec.ExternalName != ""
}

func (r *ZrokEnvironmentReconciler) ensureNetworkPolicy(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment) error {
	existing := &networkingv1.NetworkPolicy{}
	key := types.NamespacedName{Name: agent.NetworkPolicyName(env), Namespace: env.Namespace}
	err := r.Get(ctx, key, existing)
	if !r.AgentNetworkPolicy {
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return client.IgnoreNotFound(r.Delete(ctx, existing))
	}
	desired := agent.DesiredNetworkPolicy(env, r.ManagerNamespace, r.ManagerAppName)
	if err := controllerutil.SetControllerReference(env, desired, r.Scheme); err != nil {
		return err
	}
	if apierrors.IsNotFound(err) {
		return ignoreAlreadyExists(r.Create(ctx, desired))
	}
	if err != nil {
		return err
	}
	if apiequality.Semantic.DeepEqual(existing.Spec, desired.Spec) &&
		apiequality.Semantic.DeepEqual(existing.Labels, desired.Labels) {
		return nil
	}
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

func (r *ZrokEnvironmentReconciler) ensureDeployment(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment) error {
	desired := agent.DesiredDeployment(env)
	if err := controllerutil.SetControllerReference(env, desired, r.Scheme); err != nil {
		return err
	}
	existing := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(err) {
		return ignoreAlreadyExists(r.Create(ctx, desired))
	}
	if err != nil {
		return err
	}
	if apiequality.Semantic.DeepEqual(existing.Spec, desired.Spec) &&
		apiequality.Semantic.DeepEqual(existing.Labels, desired.Labels) {
		return nil
	}
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

func (r *ZrokEnvironmentReconciler) isAgentReady(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment) (bool, error) {
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: agent.DeploymentName(env), Namespace: env.Namespace}, dep); err != nil {
		// Cache can lag the Create in ensureDeployment; missing deploy is "not ready", not a reconcile error.
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return dep.Status.ReadyReplicas >= 1 && dep.Status.UpdatedReplicas >= 1, nil
}

func ignoreAlreadyExists(err error) error {
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (r *ZrokEnvironmentReconciler) setNotReady(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment, reason, message string) error {
	err := status.PatchStatus(ctx, r.Client, env, func() error {
		env.Status.ObservedGeneration = env.Generation
		env.Status.AgentReady = false
		status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, reason, message, env.Generation)
		return nil
	})
	opmetrics.SetEnvironmentReady(env.Namespace, env.Name, false)
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZrokEnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zrokv1alpha1.ZrokEnvironment{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Secret{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Named("zrokenvironment").
		Complete(r)
}
