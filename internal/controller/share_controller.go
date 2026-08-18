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

	// RestrictUpstream requires spec.upstream to be a Service in the Share namespace.
	RestrictUpstream bool
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

	if requeue, err := r.ensureFinalizerAndLabels(ctx, share); err != nil {
		return ctrl.Result{}, err
	} else if requeue {
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

	if r.RestrictUpstream {
		if err := validateShareUpstream(share.Spec.Upstream.URL, share.Namespace); err != nil {
			r.setNotReady(ctx, share, "UpstreamNotAllowed", err.Error())
			return ctrl.Result{}, nil
		}
		if share.Spec.BackendMode == zrokv1alpha1.BackendModeSocks {
			r.setNotReady(ctx, share, "BackendModeNotAllowed", "socks is disabled when --restrict-upstream is set")
			return ctrl.Result{}, nil
		}
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
		_ = status.PatchStatus(ctx, r.Client, share, func() error {
			share.Status.ObservedGeneration = share.Generation
			status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionEnvironmentReady, metav1.ConditionFalse, "Waiting", "environment not ready", share.Generation)
			status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "WaitingForEnvironment", "environment not ready", share.Generation)
			return nil
		})
		opmetrics.SetShareReady(share.Namespace, share.Name, false)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

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

	desired := share.Spec.Upstream.URL
	if other, err := r.otherShareClaimingName(ctx, share); err != nil {
		return ctrl.Result{}, err
	} else if other != nil {
		msg := fmt.Sprintf(
			"reserved name %q is claimed by ZrokShare %s/%s; not unsharing",
			reservedFrontendName(share), other.Namespace, other.Name,
		)
		return r.setNameConflict(ctx, share, msg)
	}

	digest := shareApplyDigest(share, basicAuth)

	inv, listErr := r.loadInventory(ctx, env, apiEndpoint, token, baseURL)
	cls := classifyShare(share, desired, inv)
	if listErr != nil {
		logger.Error(listErr, "list shares failed")
		if d := adoptableAgentShare(share, cls, desired, digest); d != nil {
			return r.markShareReady(ctx, share, d.GetToken(), d.GetFrontendEndpoint(), digest)
		}
		r.setNotReady(ctx, share, "InventoryError", listErr.Error())
		opmetrics.ShareReconcileErrors.Inc()
		return ctrl.Result{}, listErr
	}

	if cls.foreignName {
		holder := cls.remote
		msg := fmt.Sprintf(
			"reserved name %q is attached to share %s targeting %q, not this CR (%q); not unsharing",
			reservedFrontendName(share), holder.Token, holder.Target, desired,
		)
		logger.Info("name conflict; leaving remote share alone", "token", holder.Token, "remoteTarget", holder.Target)
		return r.setNameConflict(ctx, share, msg)
	}

	if err := r.ensureReservedName(ctx, share, apiEndpoint, token); err != nil {
		if zrokclient.IsUnauthorized(err) {
			msg := fmt.Sprintf(
				"reserved name %q is owned by another zrok account; pick a different nameSelection.name",
				reservedFrontendName(share),
			)
			return r.setNameConflict(ctx, share, msg)
		}
		return ctrl.Result{}, err
	}

	if res, err := r.flushReclaimName(ctx, share, apiEndpoint, token); !res.IsZero() || err != nil {
		return res, err
	}

	if d := adoptableAgentShare(share, cls, desired, digest); d != nil {
		return r.markShareReady(ctx, share, d.GetToken(), d.GetFrontendEndpoint(), digest)
	}

	oldName := liveFrontendName(share, cls)
	prevReserved := share.Status.Reservation == zrokv1alpha1.ReservationReserved
	released := false

	if d := cls.agent; d != nil {
		logger.Info("agent share drifted or inactive; rebuilding",
			"token", d.GetToken(), "backend", d.GetBackendEndpoint(),
			"agentStatus", d.GetStatus(), "liveName", oldName, "wantName", reservedFrontendName(share))
		if err := r.releaseAndClear(ctx, share, env, apiEndpoint, token, baseURL, d.GetToken()); err != nil {
			return ctrl.Result{}, err
		}
		released = true
	} else if rem := cls.remote; rem != nil && zrokclient.TargetsEqual(rem.Target, desired) {
		// Ours remotely, missing from agent (registry wipe) — Unshare our token so SharePublic can rebind.
		// Empty status + matching name/target is only safe because otherShareClaimingName already ran.
		logger.Info("rebind reserved name after agent miss", "token", rem.Token)
		if err := r.releaseAndClear(ctx, share, env, apiEndpoint, token, baseURL, rem.Token); err != nil {
			return ctrl.Result{}, err
		}
		released = true
	} else if rem := cls.remote; rem != nil && share.Status.ShareToken == rem.Token {
		logger.Info("share spec target changed; recreating", "token", rem.Token)
		if err := r.releaseAndClear(ctx, share, env, apiEndpoint, token, baseURL, rem.Token); err != nil {
			return ctrl.Result{}, err
		}
		released = true
	}

	if released {
		if res, err := r.reclaimPreviousName(ctx, share, apiEndpoint, token, oldName, prevReserved); !res.IsZero() || err != nil {
			return res, err
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
		return r.markShareReady(ctx, share, resp.GetToken(), nil, digest)

	default: // public
		req := &agentGrpc.SharePublicRequest{
			Target:       share.Spec.Upstream.URL,
			BackendMode:  backendMode,
			BasicAuth:    basicAuth,
			Insecure:     share.Spec.Insecure,
			Closed:       share.Spec.Closed,
			AccessGrants: share.Spec.AccessGrants,
		}
		if name := reservedFrontendName(share); name != "" {
			req.NameSelections = []*agentGrpc.NameSelection{{
				NamespaceToken: nameNamespaceToken(share),
				Name:           name,
			}}
		}
		if share.Spec.OAuth != nil {
			req.OauthProvider = share.Spec.OAuth.Provider
			req.OauthEmailDomains = share.Spec.OAuth.EmailDomains
			req.OauthRefreshInterval = share.Spec.OAuth.RefreshInterval
		}
		resp, err := r.Zrok.Agent.SharePublic(ctx, baseURL, req)
		if err != nil {
			if isShareConflict(err) && reservedFrontendName(share) != "" {
				return r.handleShareConflict(ctx, share, env, apiEndpoint, token, baseURL, desired, digest)
			}
			r.setNotReady(ctx, share, "ShareError", err.Error())
			opmetrics.ShareReconcileErrors.Inc()
			r.Recorder.Eventf(share, nil, corev1.EventTypeWarning, "ShareError", "Error", "%s", err.Error())
			return ctrl.Result{}, err
		}
		return r.markShareReady(ctx, share, resp.GetToken(), resp.GetFrontendEndpoints(), digest)
	}
}

func (r *ZrokShareReconciler) handleShareConflict(
	ctx context.Context,
	share *zrokv1alpha1.ZrokShare,
	env *zrokv1alpha1.ZrokEnvironment,
	apiEndpoint, enableToken, baseURL, desired, digest string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	inv, err := r.loadInventory(ctx, env, apiEndpoint, enableToken, baseURL)
	if err != nil {
		r.setNotReady(ctx, share, "InventoryError", err.Error())
		return ctrl.Result{}, err
	}
	cls := classifyShare(share, desired, inv)
	if cls.foreignName {
		holder := cls.remote
		msg := fmt.Sprintf(
			"reserved name %q is attached to share %s targeting %q, not this CR (%q); not unsharing",
			reservedFrontendName(share), holder.Token, holder.Target, desired,
		)
		return r.setNameConflict(ctx, share, msg)
	}
	if d := adoptableAgentShare(share, cls, desired, digest); d != nil {
		return r.markShareReady(ctx, share, d.GetToken(), d.GetFrontendEndpoint(), digest)
	}
	if rem := cls.remote; rem != nil && zrokclient.TargetsEqual(rem.Target, desired) {
		logger.Info("releasing our remote share after SharePublic 409", "token", rem.Token)
		if err := r.releaseOurs(ctx, env, apiEndpoint, enableToken, baseURL, rem.Token); err != nil {
			r.setNotReady(ctx, share, "HealError", err.Error())
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}
	r.setNotReady(ctx, share, "ShareError", "share name conflict and inventory did not identify an owned share")
	opmetrics.ShareReconcileErrors.Inc()
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *ZrokShareReconciler) loadInventory(
	ctx context.Context,
	env *zrokv1alpha1.ZrokEnvironment,
	apiEndpoint, enableToken, baseURL string,
) (shareInventory, error) {
	inv := shareInventory{}
	var listErr error
	if env.Status.EnvZID != "" {
		shares, err := r.Zrok.REST.ListShares(ctx, apiEndpoint, enableToken, env.Status.EnvZID)
		if err != nil {
			listErr = err
		} else {
			inv.remote = shares
		}
	}
	st, err := r.Zrok.Agent.Status(ctx, baseURL)
	if err != nil {
		log.FromContext(ctx).Info("agent status unavailable; continuing with remote inventory only", "error", err.Error())
	} else if st != nil {
		inv.agent = st.GetShares()
	}
	return inv, listErr
}

func (r *ZrokShareReconciler) ensureReservedName(
	ctx context.Context,
	share *zrokv1alpha1.ZrokShare,
	apiEndpoint, enableToken string,
) error {
	name := reservedFrontendName(share)
	if name == "" {
		return nil
	}
	ns := nameNamespaceToken(share)
	if err := r.Zrok.REST.CreateShareName(ctx, apiEndpoint, enableToken, ns, name); err != nil {
		_ = status.PatchStatus(ctx, r.Client, share, func() error {
			share.Status.ObservedGeneration = share.Generation
			status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionNameReady, metav1.ConditionFalse, "NameError", err.Error(), share.Generation)
			status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "NameError", err.Error(), share.Generation)
			return nil
		})
		opmetrics.SetShareReady(share.Namespace, share.Name, false)
		opmetrics.ShareReconcileErrors.Inc()
		return err
	}
	if err := r.Zrok.REST.UpdateShareName(ctx, apiEndpoint, enableToken, ns, name, true); err != nil {
		if zrokclient.IsUnauthorized(err) {
			return err
		}
		_ = status.PatchStatus(ctx, r.Client, share, func() error {
			share.Status.ObservedGeneration = share.Generation
			status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionNameReady, metav1.ConditionFalse, "ReserveError", err.Error(), share.Generation)
			status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "ReserveError", err.Error(), share.Generation)
			return nil
		})
		opmetrics.SetShareReady(share.Namespace, share.Name, false)
		opmetrics.ShareReconcileErrors.Inc()
		return err
	}
	return nil
}

func (r *ZrokShareReconciler) releaseOurs(
	ctx context.Context,
	env *zrokv1alpha1.ZrokEnvironment,
	apiEndpoint, enableToken, baseURL, shareToken string,
) error {
	if shareToken == "" {
		return nil
	}
	_ = r.Zrok.Agent.ReleaseShare(ctx, baseURL, shareToken)
	if env.Status.EnvZID == "" {
		return nil
	}
	return r.Zrok.REST.Unshare(ctx, apiEndpoint, enableToken, env.Status.EnvZID, shareToken)
}

func (r *ZrokShareReconciler) clearShareBinding(ctx context.Context, share *zrokv1alpha1.ZrokShare) error {
	if share.Status.ShareToken == "" && share.Status.AssignedURL == "" {
		return nil
	}
	return status.PatchStatus(ctx, r.Client, share, func() error {
		share.Status.ShareToken = ""
		share.Status.AssignedURL = ""
		share.Status.FrontendEndpoints = nil
		status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "Healing", "recreating share", share.Generation)
		return nil
	})
}

func (r *ZrokShareReconciler) releaseAndClear(
	ctx context.Context,
	share *zrokv1alpha1.ZrokShare,
	env *zrokv1alpha1.ZrokEnvironment,
	apiEndpoint, enableToken, baseURL, shareToken string,
) error {
	if err := r.releaseOurs(ctx, env, apiEndpoint, enableToken, baseURL, shareToken); err != nil {
		r.setNotReady(ctx, share, "HealError", err.Error())
		return err
	}
	return r.clearShareBinding(ctx, share)
}

func (r *ZrokShareReconciler) reclaimPreviousName(
	ctx context.Context,
	share *zrokv1alpha1.ZrokShare,
	apiEndpoint, enableToken, oldName string,
	prevReserved bool,
) (ctrl.Result, error) {
	newName := reservedFrontendName(share)
	if !prevReserved || oldName == "" || oldName == newName {
		return ctrl.Result{}, nil
	}
	if share.Spec.ReclaimPolicy == zrokv1alpha1.ReclaimRetain {
		r.Recorder.Eventf(share, nil, corev1.EventTypeNormal, "NameRetained", "Ready",
			"kept reserved name %q after rename to %q", oldName, newName)
		return ctrl.Result{}, nil
	}
	if err := r.patchShareAnnotation(ctx, share, annotationReclaimName, oldName); err != nil {
		return ctrl.Result{}, err
	}
	r.Recorder.Eventf(share, nil, corev1.EventTypeNormal, "ShareRenamed", "Ready",
		"replacing reserved name %q → %q", oldName, newName)
	return r.flushReclaimName(ctx, share, apiEndpoint, enableToken)
}

func (r *ZrokShareReconciler) flushReclaimName(
	ctx context.Context,
	share *zrokv1alpha1.ZrokShare,
	apiEndpoint, enableToken string,
) (ctrl.Result, error) {
	oldName := ""
	if share.Annotations != nil {
		oldName = share.Annotations[annotationReclaimName]
	}
	if oldName == "" || oldName == reservedFrontendName(share) {
		if oldName != "" {
			return ctrl.Result{}, r.patchShareAnnotation(ctx, share, annotationReclaimName, "")
		}
		return ctrl.Result{}, nil
	}
	if err := r.Zrok.REST.DeleteShareName(ctx, apiEndpoint, enableToken, nameNamespaceToken(share), oldName); err != nil {
		if isNameStillAttached(err) {
			log.FromContext(ctx).Info("old reserved name still attached; retrying delete", "name", oldName)
			return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
		}
		if zrokclient.IsUnauthorized(err) {
			log.FromContext(ctx).Info("old reserved name not deletable; leaving it", "name", oldName, "error", err.Error())
			return ctrl.Result{}, r.patchShareAnnotation(ctx, share, annotationReclaimName, "")
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, r.patchShareAnnotation(ctx, share, annotationReclaimName, "")
}

func (r *ZrokShareReconciler) patchShareAnnotation(ctx context.Context, share *zrokv1alpha1.ZrokShare, key, value string) error {
	orig := share.DeepCopy()
	if share.Annotations == nil {
		share.Annotations = map[string]string{}
	}
	if value == "" {
		if _, ok := share.Annotations[key]; !ok {
			return nil
		}
		delete(share.Annotations, key)
	} else if share.Annotations[key] == value {
		return nil
	} else {
		share.Annotations[key] = value
	}
	return r.Patch(ctx, share, client.MergeFrom(orig))
}

func (r *ZrokShareReconciler) ensureFinalizerAndLabels(ctx context.Context, share *zrokv1alpha1.ZrokShare) (requeue bool, err error) {
	changed := false
	if !controllerutil.ContainsFinalizer(share, zrokv1alpha1.ShareFinalizer) {
		controllerutil.AddFinalizer(share, zrokv1alpha1.ShareFinalizer)
		changed = true
		requeue = true
	}
	if share.Labels == nil {
		share.Labels = map[string]string{}
	}
	want := agent.ShareLabels(share)
	for k, v := range want {
		if share.Labels[k] != v {
			share.Labels[k] = v
			changed = true
		}
	}
	if _, keep := want[agent.LabelFrontendName]; !keep {
		if _, had := share.Labels[agent.LabelFrontendName]; had {
			delete(share.Labels, agent.LabelFrontendName)
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	return requeue, r.Update(ctx, share)
}

func (r *ZrokShareReconciler) otherShareClaimingName(ctx context.Context, share *zrokv1alpha1.ZrokShare) (*zrokv1alpha1.ZrokShare, error) {
	if reservedFrontendName(share) == "" {
		return nil, nil
	}
	list := &zrokv1alpha1.ZrokShareList{}
	if err := r.List(ctx, list); err != nil {
		return nil, err
	}
	return otherShareWithFrontendName(share, list.Items), nil
}

func (r *ZrokShareReconciler) setNameConflict(ctx context.Context, share *zrokv1alpha1.ZrokShare, msg string) (ctrl.Result, error) {
	was := status.Reason(share.Status.Conditions, zrokv1alpha1.ConditionReady)
	_ = status.PatchStatus(ctx, r.Client, share, func() error {
		share.Status.ObservedGeneration = share.Generation
		status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionNameReady, metav1.ConditionFalse, "NameConflict", msg, share.Generation)
		status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "NameConflict", msg, share.Generation)
		return nil
	})
	opmetrics.SetShareReady(share.Namespace, share.Name, false)
	if was != "NameConflict" {
		r.Recorder.Eventf(share, nil, corev1.EventTypeWarning, "NameConflict", "Error", "%s", msg)
	}
	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
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
		if env.Spec.EnableTokenSecretRef.Name == "" {
			logger.Info("enableTokenSecretRef.name empty; skipping REST during delete")
		} else {
			enableToken, err = r.readEnableToken(ctx, env)
			if apierrors.IsNotFound(err) {
				logger.Info("enable token secret missing; skipping REST during delete", "error", err.Error())
				err = nil
			} else if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	baseURL := ""
	inv := shareInventory{}
	if env != nil {
		baseURL = agent.AgentBaseURL(env)
		loaded, lerr := r.loadInventory(ctx, env, apiEndpoint, enableToken, baseURL)
		inv = loaded
		if lerr != nil {
			logger.Error(lerr, "list shares during delete; using agent inventory only")
		}
	}

	shareToken := share.Status.ShareToken
	desired := share.Spec.Upstream.URL
	name := reservedFrontendName(share)
	other, oerr := r.otherShareClaimingName(ctx, share)
	if oerr != nil {
		logger.Error(oerr, "listing shares during delete")
	}
	if env != nil && shareToken == "" && other == nil {
		if d := inv.agentByName(name); d != nil {
			shareToken = d.GetToken()
		}
		if shareToken == "" {
			if rem := inv.remoteByName(name); rem != nil && zrokclient.TargetsEqual(rem.Target, desired) {
				shareToken = rem.Token
			}
		}
	}

	if env != nil && shareToken != "" {
		if isOurShareToken(share, shareToken, inv) {
			if err := r.Zrok.Agent.ReleaseShare(ctx, baseURL, shareToken); err != nil {
				logger.Error(err, "agent release share failed; trying REST unshare")
			}
			if env.Status.EnvZID != "" && enableToken != "" {
				if err := r.Zrok.REST.Unshare(ctx, apiEndpoint, enableToken, env.Status.EnvZID, shareToken); err != nil {
					if zrokclient.IsUnauthorized(err) {
						logger.Info("REST unshare unauthorized; leaving remote share", "token", shareToken)
					} else {
						logger.Error(err, "REST unshare failed; will retry")
						r.Recorder.Eventf(share, nil, corev1.EventTypeWarning, "ReleaseError", "Error", "%s", err.Error())
						return ctrl.Result{}, err
					}
				}
			}
			if err := r.clearShareBinding(ctx, share); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			logger.Info("skipping unshare; token is not owned by this CR", "token", shareToken)
		}
	}

	if env != nil && enableToken != "" && share.Spec.ReclaimPolicy != zrokv1alpha1.ReclaimRetain && name != "" {
		if other != nil {
			logger.Info("reserved name claimed by another share; skipping DeleteShareName",
				"other", other.Namespace+"/"+other.Name)
			r.Recorder.Eventf(share, nil, corev1.EventTypeWarning, "NameRetained", "Warning",
				"reserved name claimed by ZrokShare %s/%s; not deleting name", other.Namespace, other.Name)
		} else if err := r.Zrok.REST.DeleteShareName(ctx, apiEndpoint, enableToken, nameNamespaceToken(share), name); err != nil {
			if isNameStillAttached(err) {
				tok := extractAttachedShareToken(err)
				if tok != "" && isOurShareToken(share, tok, inv) {
					logger.Info("name still attached to our share; releasing", "token", tok)
					_ = r.Zrok.Agent.ReleaseShare(ctx, baseURL, tok)
					if env.Status.EnvZID != "" {
						if uerr := r.Zrok.REST.Unshare(ctx, apiEndpoint, enableToken, env.Status.EnvZID, tok); uerr != nil {
							logger.Error(uerr, "unshare attached share failed")
						}
					}
					return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
				}
				if env.Status.EnvZID != "" && inv.remote == nil {
					logger.Info("DeleteShareName 409 and inventory unavailable; retrying", "token", tok)
					return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
				}
				logger.Info("DeleteShareName 409 but attached share is not ours; leaving name", "token", tok)
				r.Recorder.Eventf(share, nil, corev1.EventTypeWarning, "NameRetained", "Warning",
					"reserved name still attached to another share; not deleting name")
			} else if zrokclient.IsUnauthorized(err) {
				logger.Info("DeleteShareName unauthorized; leaving reserved name", "name", name)
				r.Recorder.Eventf(share, nil, corev1.EventTypeWarning, "NameRetained", "Warning",
					"cannot delete reserved name %q (unauthorized); leaving it", name)
			} else {
				logger.Error(err, "delete share name failed; will retry")
				return ctrl.Result{}, err
			}
		}
	}

	controllerutil.RemoveFinalizer(share, zrokv1alpha1.ShareFinalizer)
	if err := r.Update(ctx, share); err != nil {
		return ctrl.Result{}, err
	}
	opmetrics.DeleteShareReady(share.Namespace, share.Name)
	return ctrl.Result{}, nil
}

func isAgentShareActive(s *agentGrpc.ShareDetail) bool {
	if s == nil {
		return false
	}
	switch s.GetStatus() {
	case "", "active":
		return true
	default:
		return false
	}
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

func (r *ZrokShareReconciler) markShareReady(
	ctx context.Context,
	share *zrokv1alpha1.ZrokShare,
	token string,
	endpoints []string,
	digest string,
) (ctrl.Result, error) {
	wasReady := status.IsTrue(share.Status.Conditions, zrokv1alpha1.ConditionReady)
	prevURL := share.Status.AssignedURL
	assignedURL := ""
	if len(endpoints) > 0 {
		assignedURL = endpoints[0]
	} else if token != "" && share.Spec.ShareMode == zrokv1alpha1.ShareModePrivate {
		assignedURL = token
	}

	mode := share.Spec.ShareMode
	if mode == "" {
		mode = zrokv1alpha1.ShareModePublic
	}

	if err := status.PatchStatus(ctx, r.Client, share, func() error {
		share.Status.ObservedGeneration = share.Generation
		share.Status.ShareToken = token
		share.Status.FrontendEndpoints = endpoints
		share.Status.AssignedURL = assignedURL

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
			status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionNameReady, metav1.ConditionTrue, "Ready", "name reserved", share.Generation)
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
		status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionEnvironmentReady, metav1.ConditionTrue, "Ready", "environment ready", share.Generation)
		status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionShareCreated, metav1.ConditionTrue, "Created", "share active", share.Generation)
		status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "share ready", share.Generation)
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}
	opmetrics.SetShareReady(share.Namespace, share.Name, true)
	if err := r.patchShareAnnotation(ctx, share, annotationAppliedDigest, digest); err != nil {
		return ctrl.Result{}, err
	}
	if !wasReady {
		r.Recorder.Eventf(share, nil, corev1.EventTypeNormal, "Ready", "Ready", "share ready: %s", assignedURL)
	} else if prevURL != "" && assignedURL != "" && prevURL != assignedURL {
		r.Recorder.Eventf(share, nil, corev1.EventTypeNormal, "ShareRenamed", "Ready",
			"frontend URL %s → %s", prevURL, assignedURL)
	}
	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

func (r *ZrokShareReconciler) setNotReady(ctx context.Context, share *zrokv1alpha1.ZrokShare, reason, message string) {
	_ = status.PatchStatus(ctx, r.Client, share, func() error {
		share.Status.ObservedGeneration = share.Generation
		status.SetCondition(&share.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, reason, message, share.Generation)
		return nil
	})
	opmetrics.SetShareReady(share.Namespace, share.Name, false)
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
	if err := setupEnvironmentRefIndex(mgr, &zrokv1alpha1.ZrokShare{}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&zrokv1alpha1.ZrokShare{}).
		Watches(&zrokv1alpha1.ZrokEnvironment{}, handler.EnqueueRequestsFromMapFunc(r.mapEnvToShares)).
		Named("zrokshare").
		Complete(r)
}

func (r *ZrokShareReconciler) mapEnvToShares(ctx context.Context, obj client.Object) []reconcile.Request {
	list := &zrokv1alpha1.ZrokShareList{}
	if err := r.List(ctx, list,
		client.InNamespace(obj.GetNamespace()),
		client.MatchingFields{environmentRefField: obj.GetName()},
	); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		s := &list.Items[i]
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: s.Name, Namespace: s.Namespace}})
	}
	return reqs
}
