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
	"regexp"
	"strings"
	"time"

	"github.com/openziti/zrok/v2/agent/agentGrpc"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/agent"
	opmetrics "github.com/vulkoingim/zrok-operator/internal/metrics"
	"github.com/vulkoingim/zrok-operator/internal/status"
	"github.com/vulkoingim/zrok-operator/internal/zrokclient"
)

var attachedShareTokenRE = regexp.MustCompile(`share '([^']+)'`)

// ZrokShareReconciler reconciles a ZrokShare object.
type ZrokShareReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	Zrok     *zrokclient.Clients
}

// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokshares,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokshares/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokshares/finalizers,verbs=update
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokenvironments,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch;update

func (r *ZrokShareReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	share := &zrokv1alpha1.ZrokShare{}
	if err := r.Get(ctx, req.NamespacedName, share); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !share.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, share)
	}

	if !controllerutil.ContainsFinalizer(share, zrokv1alpha1.ShareFinalizer) {
		controllerutil.AddFinalizer(share, zrokv1alpha1.ShareFinalizer)
		if err := r.Update(ctx, share); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	mode := share.Spec.ShareMode
	if mode == "" {
		mode = zrokv1alpha1.ShareModePublic
	}

	if mode == zrokv1alpha1.ShareModePrivate && share.Spec.NameSelection != nil {
		r.setNotReady(ctx, share, "InvalidSpec", "nameSelection is only valid with shareMode=public")
		return ctrl.Result{}, nil
	}
	if mode == zrokv1alpha1.ShareModePublic && share.Spec.PrivateShareToken != "" {
		r.setNotReady(ctx, share, "InvalidSpec", "privateShareToken is only valid with shareMode=private")
		return ctrl.Result{}, nil
	}

	env, err := r.getEnvironment(ctx, share)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.setNotReady(ctx, share, "EnvironmentMissing", err.Error())
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		opmetrics.ShareReconcileErrors.Inc()
		return ctrl.Result{}, err
	}

	if !status.IsTrue(env.Status.Conditions, zrokv1alpha1.ConditionReady) {
		status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionEnvironmentReady, metav1.ConditionFalse, "Waiting", "environment not ready", share.Generation)
		status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "WaitingForEnvironment", "environment not ready", share.Generation)
		share.Status.ObservedGeneration = share.Generation
		_ = r.Status().Update(ctx, share)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionEnvironmentReady, metav1.ConditionTrue, "Ready", "environment ready", share.Generation)

	token, err := r.readEnableToken(ctx, env)
	if err != nil {
		r.setNotReady(ctx, share, "SecretError", err.Error())
		opmetrics.ShareReconcileErrors.Inc()
		return ctrl.Result{}, err
	}

	apiEndpoint := env.Spec.ApiEndpoint
	if apiEndpoint == "" {
		apiEndpoint = zrokclient.DefaultAPIEndpoint
	}
	baseURL := agent.AgentBaseURL(env)

	backendMode := string(share.Spec.BackendMode)
	if backendMode == "" {
		backendMode = string(zrokv1alpha1.BackendModeProxy)
	}

	basicAuth, err := r.readBasicAuth(ctx, share)
	if err != nil {
		r.setNotReady(ctx, share, "BasicAuthError", err.Error())
		return ctrl.Result{}, err
	}

	// Ensure reserved name when requested (create + promote reserved=true).
	if share.Spec.NameSelection != nil && share.Spec.NameSelection.Name != "" {
		ns := share.Spec.NameSelection.Namespace
		if ns == "" {
			ns = zrokv1alpha1.DefaultNamespaceToken
		}
		if err := r.Zrok.REST.CreateShareName(ctx, apiEndpoint, token, ns, share.Spec.NameSelection.Name); err != nil {
			status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionNameReady, metav1.ConditionFalse, "NameError", err.Error(), share.Generation)
			r.setNotReady(ctx, share, "NameError", err.Error())
			opmetrics.ShareReconcileErrors.Inc()
			return ctrl.Result{}, err
		}
		// Promote ephemeral→reserved if the name already existed (CreateShareName 409 path).
		if err := r.Zrok.REST.UpdateShareName(ctx, apiEndpoint, token, ns, share.Spec.NameSelection.Name, true); err != nil {
			status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionNameReady, metav1.ConditionFalse, "ReserveError", err.Error(), share.Generation)
			r.setNotReady(ctx, share, "ReserveError", err.Error())
			opmetrics.ShareReconcileErrors.Inc()
			return ctrl.Result{}, err
		}
		status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionNameReady, metav1.ConditionTrue, "Ready", "name reserved", share.Generation)
	}

	// Idempotent: if share already running in agent, refresh status.
	if share.Status.ShareToken != "" {
		st, err := r.Zrok.Agent.Status(ctx, baseURL)
		if err == nil {
			for _, s := range st.GetShares() {
				if s.GetToken() == share.Status.ShareToken {
					return r.markShareReady(ctx, share, s.GetToken(), s.GetFrontendEndpoint())
				}
			}
			logger.Info("share token missing from agent; recreating", "token", share.Status.ShareToken)
			share.Status.ShareToken = ""
		}
	}

	// Adopt live agent share by reserved name before creating another.
	if share.Status.ShareToken == "" && share.Spec.NameSelection != nil && share.Spec.NameSelection.Name != "" {
		if tok, eps, ok := r.findAgentShareByName(ctx, baseURL, share.Spec.NameSelection.Name); ok {
			return r.markShareReady(ctx, share, tok, eps)
		}
	}

	switch mode {
	case zrokv1alpha1.ShareModePrivate:
		resp, err := r.Zrok.Agent.SharePrivate(ctx, baseURL, &agentGrpc.SharePrivateRequest{
			Target:            share.Spec.Upstream.URL,
			BackendMode:       backendMode,
			PrivateShareToken: share.Spec.PrivateShareToken,
			Closed:            share.Spec.Closed,
			AccessGrants:      share.Spec.AccessGrants,
		})
		if err != nil {
			r.setNotReady(ctx, share, "ShareError", err.Error())
			opmetrics.ShareReconcileErrors.Inc()
			return ctrl.Result{}, err
		}
		return r.markShareReady(ctx, share, resp.GetToken(), nil)

	default: // public
		req := &agentGrpc.SharePublicRequest{
			Target:       share.Spec.Upstream.URL,
			BackendMode:  backendMode,
			BasicAuth:    basicAuth,
			Insecure:     share.Spec.Insecure,
			Closed:       share.Spec.Closed,
			AccessGrants: share.Spec.AccessGrants,
		}
		if share.Spec.NameSelection != nil && share.Spec.NameSelection.Name != "" {
			ns := share.Spec.NameSelection.Namespace
			if ns == "" {
				ns = zrokv1alpha1.DefaultNamespaceToken
			}
			req.NameSelections = []*agentGrpc.NameSelection{{
				NamespaceToken: ns,
				Name:           share.Spec.NameSelection.Name,
			}}
		}
		if share.Spec.OAuth != nil {
			req.OauthProvider = share.Spec.OAuth.Provider
			req.OauthEmailDomains = share.Spec.OAuth.EmailDomains
			req.OauthRefreshInterval = share.Spec.OAuth.RefreshInterval
		}
		resp, err := r.Zrok.Agent.SharePublic(ctx, baseURL, req)
		if err != nil {
			if isShareConflict(err) && share.Spec.NameSelection != nil && share.Spec.NameSelection.Name != "" {
				name := share.Spec.NameSelection.Name
				if tok, eps, ok := r.findAgentShareByName(ctx, baseURL, name); ok {
					return r.markShareReady(ctx, share, tok, eps)
				}
				// Remote orphan (share exists in controller, not in this agent): tear down and recreate.
				if env.Status.EnvZID != "" {
					if tok, _, findErr := r.Zrok.REST.FindShareByFrontendName(ctx, apiEndpoint, token, env.Status.EnvZID, name); findErr == nil && tok != "" {
						logger.Info("releasing orphan remote share before recreate", "token", tok, "name", name)
						_ = r.Zrok.Agent.ReleaseShare(ctx, baseURL, tok)
						if uerr := r.Zrok.REST.Unshare(ctx, apiEndpoint, token, env.Status.EnvZID, tok); uerr != nil {
							logger.Error(uerr, "unshare orphan failed")
						}
						return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
					}
				}
			}
			r.setNotReady(ctx, share, "ShareError", err.Error())
			opmetrics.ShareReconcileErrors.Inc()
			r.Recorder.Eventf(share, nil, corev1.EventTypeWarning, "ShareError", "Error", "%s", err.Error())
			return ctrl.Result{}, err
		}
		return r.markShareReady(ctx, share, resp.GetToken(), resp.GetFrontendEndpoints())
	}
}

func (r *ZrokShareReconciler) reconcileDelete(ctx context.Context, share *zrokv1alpha1.ZrokShare) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(share, zrokv1alpha1.ShareFinalizer) {
		return ctrl.Result{}, nil
	}

	env, err := r.getEnvironment(ctx, share)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	logger := log.FromContext(ctx)

	var apiEndpoint, enableToken string
	if env != nil {
		apiEndpoint = env.Spec.ApiEndpoint
		if apiEndpoint == "" {
			apiEndpoint = zrokclient.DefaultAPIEndpoint
		}
		enableToken, err = r.readEnableToken(ctx, env)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Resolve share token even when status was never written (failed create / crash).
	shareToken := share.Status.ShareToken
	if env != nil && shareToken == "" {
		addr := agent.AgentDialAddr(env)
		if share.Spec.NameSelection != nil && share.Spec.NameSelection.Name != "" {
			if tok, _, ok := r.findAgentShareByName(ctx, addr, share.Spec.NameSelection.Name); ok {
				shareToken = tok
			}
		}
		if shareToken == "" && env.Status.EnvZID != "" && share.Spec.NameSelection != nil && share.Spec.NameSelection.Name != "" {
			if tok, _, findErr := r.Zrok.REST.FindShareByFrontendName(ctx, apiEndpoint, enableToken, env.Status.EnvZID, share.Spec.NameSelection.Name); findErr == nil {
				shareToken = tok
			}
		}
	}

	// Release live share first — reserved names stay attached until unshared.
	if env != nil && shareToken != "" {
		addr := agent.AgentDialAddr(env)
		if err := r.Zrok.Agent.ReleaseShare(ctx, addr, shareToken); err != nil {
			logger.Error(err, "agent release share failed; trying REST unshare")
		}
		if env.Status.EnvZID != "" {
			if err := r.Zrok.REST.Unshare(ctx, apiEndpoint, enableToken, env.Status.EnvZID, shareToken); err != nil {
				logger.Error(err, "REST unshare failed; will retry")
				r.Recorder.Eventf(share, nil, corev1.EventTypeWarning, "ReleaseError", "Error", "%s", err.Error())
				return ctrl.Result{RequeueAfter: 5 * time.Second}, err
			}
		}
		share.Status.ShareToken = ""
		share.Status.AssignedURL = ""
		share.Status.FrontendEndpoints = nil
		_ = r.Status().Update(ctx, share)
	}

	if env != nil && share.Spec.ReclaimPolicy != zrokv1alpha1.ReclaimRetain &&
		share.Spec.NameSelection != nil && share.Spec.NameSelection.Name != "" {
		ns := share.Spec.NameSelection.Namespace
		if ns == "" {
			ns = zrokv1alpha1.DefaultNamespaceToken
		}
		if err := r.Zrok.REST.DeleteShareName(ctx, apiEndpoint, enableToken, ns, share.Spec.NameSelection.Name); err != nil {
			// 409 = still attached; parse token and release, then retry.
			if isNameStillAttached(err) {
				if tok := extractAttachedShareToken(err); tok != "" && env.Status.EnvZID != "" {
					logger.Info("name still attached; releasing discovered share", "token", tok)
					_ = r.Zrok.Agent.ReleaseShare(ctx, agent.AgentDialAddr(env), tok)
					if uerr := r.Zrok.REST.Unshare(ctx, apiEndpoint, enableToken, env.Status.EnvZID, tok); uerr != nil {
						logger.Error(uerr, "unshare attached share failed")
					}
				}
				logger.Info("share name still attached; retrying", "error", err)
				return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
			}
			logger.Error(err, "delete share name failed; will retry")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}
	}

	controllerutil.RemoveFinalizer(share, zrokv1alpha1.ShareFinalizer)
	if err := r.Update(ctx, share); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ZrokShareReconciler) findAgentShareByName(ctx context.Context, addr, name string) (token string, endpoints []string, ok bool) {
	st, err := r.Zrok.Agent.Status(ctx, addr)
	if err != nil || st == nil {
		return "", nil, false
	}
	for _, s := range st.GetShares() {
		eps := s.GetFrontendEndpoint()
		for _, ep := range eps {
			if zrokclient.FrontendEndpointMatchesName(ep, name) {
				return s.GetToken(), eps, true
			}
		}
	}
	return "", nil, false
}

func isShareConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "409") ||
		strings.Contains(msg, "shareconflict") ||
		strings.Contains(msg, "already in use")
}

func isNameStillAttached(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "409") || strings.Contains(msg, "still attached")
}

func extractAttachedShareToken(err error) string {
	if err == nil {
		return ""
	}
	m := attachedShareTokenRE.FindStringSubmatch(err.Error())
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func (r *ZrokShareReconciler) markShareReady(ctx context.Context, share *zrokv1alpha1.ZrokShare, token string, endpoints []string) (ctrl.Result, error) {
	share.Status.ObservedGeneration = share.Generation
	share.Status.ShareToken = token
	share.Status.FrontendEndpoints = endpoints
	if len(endpoints) > 0 {
		share.Status.AssignedURL = endpoints[0]
	} else if token != "" && share.Spec.ShareMode == zrokv1alpha1.ShareModePrivate {
		share.Status.AssignedURL = token
	}

	mode := share.Spec.ShareMode
	if mode == "" {
		mode = zrokv1alpha1.ShareModePublic
	}
	switch {
	case mode == zrokv1alpha1.ShareModePrivate:
		share.Status.Reservation = zrokv1alpha1.ReservationPrivate
		status.SetCondition(
			&share.Status.Conditions,
			zrokv1alpha1.ConditionNameReady,
			metav1.ConditionTrue,
			"Private",
			"private share; use ZrokAccess or zrok2 access private",
			share.Generation,
		)
	case share.Spec.NameSelection != nil && share.Spec.NameSelection.Name != "":
		share.Status.Reservation = zrokv1alpha1.ReservationReserved
	default:
		share.Status.Reservation = zrokv1alpha1.ReservationEphemeral
		status.SetCondition(
			&share.Status.Conditions,
			zrokv1alpha1.ConditionNameReady,
			metav1.ConditionTrue,
			"Ephemeral",
			"no reserved name; share will not survive agent restart",
			share.Generation,
		)
	}
	status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionShareCreated, metav1.ConditionTrue, "Created", "share active", share.Generation)
	status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "share ready", share.Generation)
	if err := r.Status().Update(ctx, share); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Eventf(share, nil, corev1.EventTypeNormal, "Ready", "Ready", "share ready: %s", share.Status.AssignedURL)
	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

func (r *ZrokShareReconciler) setNotReady(ctx context.Context, share *zrokv1alpha1.ZrokShare, reason, message string) {
	share.Status.ObservedGeneration = share.Generation
	status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, reason, message, share.Generation)
	_ = r.Status().Update(ctx, share)
}

func (r *ZrokShareReconciler) getEnvironment(ctx context.Context, share *zrokv1alpha1.ZrokShare) (*zrokv1alpha1.ZrokEnvironment, error) {
	env := &zrokv1alpha1.ZrokEnvironment{}
	err := r.Get(ctx, types.NamespacedName{Name: share.Spec.EnvironmentRef.Name, Namespace: share.Namespace}, env)
	return env, err
}

func (r *ZrokShareReconciler) readEnableToken(ctx context.Context, env *zrokv1alpha1.ZrokEnvironment) (string, error) {
	ref := env.Spec.EnableTokenSecretRef
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: env.Namespace, Name: ref.Name}, secret); err != nil {
		return "", err
	}
	key := ref.Key
	if key == "" {
		key = zrokv1alpha1.DefaultEnableTokenKey
	}
	raw, ok := secret.Data[key]
	if !ok || len(raw) == 0 {
		return "", fmt.Errorf("secret %s missing key %q", ref.Name, key)
	}
	return string(raw), nil
}

func (r *ZrokShareReconciler) readBasicAuth(ctx context.Context, share *zrokv1alpha1.ZrokShare) ([]string, error) {
	if share.Spec.BasicAuthSecretRef == nil || share.Spec.BasicAuthSecretRef.Name == "" {
		return nil, nil
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: share.Namespace, Name: share.Spec.BasicAuthSecretRef.Name}, secret); err != nil {
		return nil, err
	}
	user := string(secret.Data["username"])
	pass := string(secret.Data["password"])
	if user == "" || pass == "" {
		return nil, fmt.Errorf("basic auth secret must contain username and password keys")
	}
	return []string{user + ":" + pass}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZrokShareReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zrokv1alpha1.ZrokShare{}).
		Watches(&zrokv1alpha1.ZrokEnvironment{}, handler.EnqueueRequestsFromMapFunc(r.mapEnvToShares)).
		Named("zrokshare").
		Complete(r)
}

func (r *ZrokShareReconciler) mapEnvToShares(ctx context.Context, obj client.Object) []reconcile.Request {
	list := &zrokv1alpha1.ZrokShareList{}
	if err := r.List(ctx, list, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0)
	for i := range list.Items {
		s := &list.Items[i]
		if s.Spec.EnvironmentRef.Name == obj.GetName() {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: s.Name, Namespace: s.Namespace}})
		}
	}
	return reqs
}
