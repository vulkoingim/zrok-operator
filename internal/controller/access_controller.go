package controller

import (
	"context"
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

// ZrokAccessReconciler reconciles private share accesses.
type ZrokAccessReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	Zrok     *zrokclient.Clients
}

// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokaccesses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokaccesses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokaccesses/finalizers,verbs=update
// +kubebuilder:rbac:groups=zrok.k8s.zrok.io,resources=zrokenvironments,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch;update

func (r *ZrokAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

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
			_ = r.setNotReady(ctx, access, "EnvironmentMissing", err.Error())
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	if !status.IsTrue(env.Status.Conditions, zrokv1alpha1.ConditionReady) {
		_ = r.setNotReady(ctx, access, "WaitingForEnvironment", "environment not ready")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	addr := agent.AgentDialAddr(env)

	// Idempotent: if access already running in agent, refresh status.
	if access.Status.AccessToken != "" {
		st, err := r.Zrok.Agent.Status(ctx, addr)
		if err == nil {
			for _, a := range st.GetAccesses() {
				if a.GetFrontendToken() != access.Status.AccessToken || !isAgentAccessActive(a) {
					continue
				}
				if accessSpecDrifted(access, a) {
					logger.Info("access spec drifted; rebuilding",
						"token", access.Status.AccessToken,
						"liveShare", a.GetToken(), "wantShare", access.Spec.ShareToken,
						"liveBind", a.GetBindAddress(), "wantBind", access.Spec.BindAddress)
					break
				}
				return r.markAccessReady(ctx, access, a.GetFrontendToken(), a.GetBindAddress())
			}
			logger.Info("access token missing/inactive/drifted in agent; healing", "token", access.Status.AccessToken)
			_ = r.Zrok.Agent.ReleaseAccess(ctx, addr, access.Status.AccessToken)
			if err := status.PatchStatus(ctx, r.Client, access, func() error {
				access.Status.AccessToken = ""
				access.Status.FrontendEndpoint = ""
				status.SetCondition(&access.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, "Healing", "recreating inactive access", access.Generation)
				return nil
			}); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if access.Status.AccessToken == "" {
		bind := access.Spec.BindAddress
		if bind == "" {
			bind = "0.0.0.0:0"
		}
		resp, err := r.Zrok.Agent.AccessPrivate(ctx, addr, &agentGrpc.AccessPrivateRequest{
			Token:       access.Spec.ShareToken,
			BindAddress: bind,
		})
		if err != nil {
			_ = r.setNotReady(ctx, access, "AccessError", err.Error())
			opmetrics.AccessReconcileErrors.Inc()
			r.Recorder.Eventf(access, nil, corev1.EventTypeWarning, "AccessError", "Error", "%s", err.Error())
			return ctrl.Result{}, err
		}
		frontend := resp.GetFrontendToken()
		bindAddr := ""
		if st, serr := r.Zrok.Agent.Status(ctx, addr); serr == nil {
			for _, a := range st.GetAccesses() {
				if a.GetFrontendToken() == frontend {
					bindAddr = a.GetBindAddress()
					break
				}
			}
		}
		return r.markAccessReady(ctx, access, frontend, bindAddr)
	}

	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

func (r *ZrokAccessReconciler) markAccessReady(ctx context.Context, access *zrokv1alpha1.ZrokAccess, token, bindAddress string) (ctrl.Result, error) {
	wasReady := status.IsTrue(access.Status.Conditions, zrokv1alpha1.ConditionReady)
	endpoint := bindAddress
	if endpoint == "" {
		endpoint = token
	}
	if err := status.PatchStatus(ctx, r.Client, access, func() error {
		access.Status.AccessToken = token
		access.Status.FrontendEndpoint = endpoint
		access.Status.ObservedGeneration = access.Generation
		status.SetCondition(&access.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "access active", access.Generation)
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}
	if !wasReady {
		r.Recorder.Eventf(access, nil, corev1.EventTypeNormal, "Ready", "Ready", "private access ready")
	}
	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

func (r *ZrokAccessReconciler) setNotReady(ctx context.Context, access *zrokv1alpha1.ZrokAccess, reason, message string) error {
	return status.PatchStatus(ctx, r.Client, access, func() error {
		access.Status.ObservedGeneration = access.Generation
		status.SetCondition(&access.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionFalse, reason, message, access.Generation)
		return nil
	})
}

func isAgentAccessActive(a *agentGrpc.AccessDetail) bool {
	if a == nil {
		return false
	}
	switch a.GetStatus() {
	case "", "active":
		return true
	default:
		return false
	}
}

func accessSpecDrifted(access *zrokv1alpha1.ZrokAccess, live *agentGrpc.AccessDetail) bool {
	if live == nil {
		return true
	}
	if tok := live.GetToken(); tok != "" && tok != access.Spec.ShareToken {
		return true
	}
	want := access.Spec.BindAddress
	if want == "" {
		want = "0.0.0.0:0"
	}
	if strings.HasSuffix(want, ":0") {
		return false
	}
	got := live.GetBindAddress()
	return got != "" && got != want
}

func (r *ZrokAccessReconciler) reconcileDelete(ctx context.Context, access *zrokv1alpha1.ZrokAccess) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(access, zrokv1alpha1.AccessFinalizer) {
		return ctrl.Result{}, nil
	}

	env := &zrokv1alpha1.ZrokEnvironment{}
	err := r.Get(ctx, types.NamespacedName{Name: access.Spec.EnvironmentRef.Name, Namespace: access.Namespace}, env)
	if err == nil && access.Status.AccessToken != "" {
		if err := r.Zrok.Agent.ReleaseAccess(ctx, agent.AgentDialAddr(env), access.Status.AccessToken); err != nil {
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
	if err := setupEnvironmentRefIndex(mgr, &zrokv1alpha1.ZrokAccess{}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&zrokv1alpha1.ZrokAccess{}).
		Watches(&zrokv1alpha1.ZrokEnvironment{}, handler.EnqueueRequestsFromMapFunc(r.mapEnvToAccesses)).
		Named("zrokaccess").
		Complete(r)
}

func (r *ZrokAccessReconciler) mapEnvToAccesses(ctx context.Context, obj client.Object) []reconcile.Request {
	list := &zrokv1alpha1.ZrokAccessList{}
	if err := r.List(ctx, list,
		client.InNamespace(obj.GetNamespace()),
		client.MatchingFields{environmentRefField: obj.GetName()},
	); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		a := &list.Items[i]
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: a.Name, Namespace: a.Namespace}})
	}
	return reqs
}
