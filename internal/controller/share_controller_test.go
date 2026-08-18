package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/openziti/zrok/v2/agent/agentGrpc"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/status"
	"github.com/vulkoingim/zrok-operator/internal/zrokclient"
	zrokclientmock "github.com/vulkoingim/zrok-operator/internal/zrokclient/mock"
)

var _ = Describe("ZrokShare Controller", func() {
	Context("when reconciling a public share", func() {
		const (
			shareName = "test-share"
			envName   = "test-env"
			ns        = "default"
		)

		ctx := context.Background()

		BeforeEach(func() {
			By("creating enable token secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "zrok-credentials", Namespace: ns},
				Data:       map[string][]byte{"enable-token": []byte("test-token")},
			}
			_ = k8sClient.Delete(ctx, secret)
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("creating ready environment")
			env := &zrokv1alpha1.ZrokEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: envName, Namespace: ns},
				Spec: zrokv1alpha1.ZrokEnvironmentSpec{
					EnableTokenSecretRef: corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "zrok-credentials"},
						Key:                  "enable-token",
					},
				},
			}
			_ = k8sClient.Delete(ctx, env)
			Expect(k8sClient.Create(ctx, env)).To(Succeed())
			env.Status.Conditions = nil
			status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "ready", 1)
			env.Status.AgentReady = true
			env.Status.EnvZID = "envzid"
			Expect(k8sClient.Status().Update(ctx, env)).To(Succeed())

			By("creating share")
			share := &zrokv1alpha1.ZrokShare{
				ObjectMeta: metav1.ObjectMeta{Name: shareName, Namespace: ns},
				Spec: zrokv1alpha1.ZrokShareSpec{
					EnvironmentRef: corev1.LocalObjectReference{Name: envName},
					ShareMode:      zrokv1alpha1.ShareModePublic,
					BackendMode:    zrokv1alpha1.BackendModeProxy,
					Upstream:       zrokv1alpha1.UpstreamSpec{URL: "http://nginx.default.svc:80"},
					NameSelection: &zrokv1alpha1.NameSelectionSpec{
						Namespace: "public",
						Name:      "demo",
					},
				},
			}
			_ = k8sClient.Delete(ctx, share)
			Expect(k8sClient.Create(ctx, share)).To(Succeed())
		})

		AfterEach(func() {
			share := &zrokv1alpha1.ZrokShare{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: shareName, Namespace: ns}, share); err == nil {
				controllerutil.RemoveFinalizer(share, zrokv1alpha1.ShareFinalizer)
				Expect(k8sClient.Update(ctx, share)).To(Succeed())
				Expect(k8sClient.Delete(ctx, share)).To(Succeed())
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: shareName, Namespace: ns}, &zrokv1alpha1.ZrokShare{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
			_ = k8sClient.Delete(ctx, &zrokv1alpha1.ZrokEnvironment{ObjectMeta: metav1.ObjectMeta{Name: envName, Namespace: ns}})
			_ = k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "zrok-credentials", Namespace: ns}})
		})

		It("should create a share via the agent client", func() {
			restMock := zrokclientmock.NewRESTClient(GinkgoT())
			agentMock := zrokclientmock.NewAgentClient(GinkgoT())

			restMock.EXPECT().
				ListShares(mock.Anything, mock.Anything, mock.Anything, "envzid").
				Return(nil, nil)
			restMock.EXPECT().
				CreateShareName(mock.Anything, mock.Anything, mock.Anything, mock.Anything, "demo").
				Return(nil)
			restMock.EXPECT().
				UpdateShareName(mock.Anything, mock.Anything, mock.Anything, mock.Anything, "demo", true).
				Return(nil)
			agentMock.EXPECT().
				Status(mock.Anything, mock.Anything).
				Return(&agentGrpc.StatusResponse{}, nil).
				Maybe()
			agentMock.EXPECT().
				SharePublic(mock.Anything, mock.Anything, mock.Anything).
				Return(&agentGrpc.SharePublicResponse{
					Token:             "shr-token",
					FrontendEndpoints: []string{"https://demo.share.zrok.io"},
				}, nil)

			r := &ZrokShareReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: events.NewFakeRecorder(10),
				Zrok:     &zrokclient.Clients{REST: restMock, Agent: agentMock},
			}

			_, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: shareName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())

			// Finalizer add requeues once.
			_, err = r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: shareName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())

			share := &zrokv1alpha1.ZrokShare{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: shareName, Namespace: ns}, share)).To(Succeed())
			Expect(share.Status.AssignedURL).To(Equal("https://demo.share.zrok.io"))
			Expect(share.Status.ShareToken).To(Equal("shr-token"))
			Expect(share.Status.Reservation).To(Equal(zrokv1alpha1.ReservationReserved))
			Expect(status.IsTrue(share.Status.Conditions, zrokv1alpha1.ConditionReady)).To(BeTrue())
			Expect(share.Labels["app.kubernetes.io/managed-by"]).To(Equal("zrok-operator"))
			Expect(share.Labels["zrok.k8s.zrok.io/frontend-name"]).To(Equal("demo"))
		})

		It("does not unshare a reserved name held by a different target", func() {
			restMock := zrokclientmock.NewRESTClient(GinkgoT())
			agentMock := zrokclientmock.NewAgentClient(GinkgoT())

			restMock.EXPECT().
				ListShares(mock.Anything, mock.Anything, mock.Anything, "envzid").
				Return([]zrokclient.RemoteShare{{
					Token:             "foreign",
					Target:            "http://not-ours:80",
					FrontendEndpoints: []string{"https://demo.share.zrok.io"},
				}}, nil)
			agentMock.EXPECT().
				Status(mock.Anything, mock.Anything).
				Return(&agentGrpc.StatusResponse{}, nil)

			r := &ZrokShareReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: events.NewFakeRecorder(10),
				Zrok:     &zrokclient.Clients{REST: restMock, Agent: agentMock},
			}

			_, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: shareName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: shareName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())

			share := &zrokv1alpha1.ZrokShare{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: shareName, Namespace: ns}, share)).To(Succeed())
			Expect(status.IsTrue(share.Status.Conditions, zrokv1alpha1.ConditionReady)).To(BeFalse())
			cond := metav1.Condition{}
			for _, c := range share.Status.Conditions {
				if c.Type == zrokv1alpha1.ConditionReady {
					cond = c
				}
			}
			Expect(cond.Reason).To(Equal("NameConflict"))
		})

		It("maps UpdateShareName 401 to NameConflict without a reconcile error", func() {
			restMock := zrokclientmock.NewRESTClient(GinkgoT())
			agentMock := zrokclientmock.NewAgentClient(GinkgoT())

			restMock.EXPECT().
				ListShares(mock.Anything, mock.Anything, mock.Anything, "envzid").
				Return(nil, nil)
			restMock.EXPECT().
				CreateShareName(mock.Anything, mock.Anything, mock.Anything, mock.Anything, "demo").
				Return(nil)
			restMock.EXPECT().
				UpdateShareName(mock.Anything, mock.Anything, mock.Anything, mock.Anything, "demo", true).
				Return(fmt.Errorf("update share name: [PATCH /share/name][401] updateShareNameUnauthorized"))
			agentMock.EXPECT().
				Status(mock.Anything, mock.Anything).
				Return(&agentGrpc.StatusResponse{}, nil).
				Maybe()

			r := &ZrokShareReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: events.NewFakeRecorder(10),
				Zrok:     &zrokclient.Clients{REST: restMock, Agent: agentMock},
			}

			_, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: shareName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())
			res, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: shareName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(res.RequeueAfter).To(Equal(2 * time.Minute))

			share := &zrokv1alpha1.ZrokShare{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: shareName, Namespace: ns}, share)).To(Succeed())
			Expect(status.IsTrue(share.Status.Conditions, zrokv1alpha1.ConditionReady)).To(BeFalse())
			cond := metav1.Condition{}
			for _, c := range share.Status.Conditions {
				if c.Type == zrokv1alpha1.ConditionReady {
					cond = c
				}
			}
			Expect(cond.Reason).To(Equal("NameConflict"))
			Expect(cond.Message).To(ContainSubstring("owned by another zrok account"))
		})

		It("unshares our remote share when the agent is empty then recreates", func() {
			restMock := zrokclientmock.NewRESTClient(GinkgoT())
			agentMock := zrokclientmock.NewAgentClient(GinkgoT())

			restMock.EXPECT().
				ListShares(mock.Anything, mock.Anything, mock.Anything, "envzid").
				Return([]zrokclient.RemoteShare{{
					Token:             "old-tok",
					Target:            "http://nginx.default.svc:80",
					FrontendEndpoints: []string{"https://demo.share.zrok.io"},
				}}, nil)
			restMock.EXPECT().
				Unshare(mock.Anything, mock.Anything, mock.Anything, "envzid", "old-tok").
				Return(nil)
			restMock.EXPECT().
				CreateShareName(mock.Anything, mock.Anything, mock.Anything, mock.Anything, "demo").
				Return(nil)
			restMock.EXPECT().
				UpdateShareName(mock.Anything, mock.Anything, mock.Anything, mock.Anything, "demo", true).
				Return(nil)
			agentMock.EXPECT().
				Status(mock.Anything, mock.Anything).
				Return(&agentGrpc.StatusResponse{}, nil)
			agentMock.EXPECT().
				ReleaseShare(mock.Anything, mock.Anything, "old-tok").
				Return(nil).
				Maybe()
			agentMock.EXPECT().
				SharePublic(mock.Anything, mock.Anything, mock.Anything).
				Return(&agentGrpc.SharePublicResponse{
					Token:             "shr-token",
					FrontendEndpoints: []string{"https://demo.share.zrok.io"},
				}, nil)

			r := &ZrokShareReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: events.NewFakeRecorder(10),
				Zrok:     &zrokclient.Clients{REST: restMock, Agent: agentMock},
			}

			_, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: shareName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: shareName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())

			share := &zrokv1alpha1.ZrokShare{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: shareName, Namespace: ns}, share)).To(Succeed())
			Expect(share.Status.ShareToken).To(Equal("shr-token"))
			Expect(status.IsTrue(share.Status.Conditions, zrokv1alpha1.ConditionReady)).To(BeTrue())
		})

		It("rebuilds when nameSelection.name changes and deletes the old reserved name", func() {
			restMock := zrokclientmock.NewRESTClient(GinkgoT())
			agentMock := zrokclientmock.NewAgentClient(GinkgoT())

			restMock.EXPECT().
				ListShares(mock.Anything, mock.Anything, mock.Anything, "envzid").
				Return(nil, nil).Once()
			restMock.EXPECT().
				CreateShareName(mock.Anything, mock.Anything, mock.Anything, mock.Anything, "demo").
				Return(nil).Once()
			restMock.EXPECT().
				UpdateShareName(mock.Anything, mock.Anything, mock.Anything, mock.Anything, "demo", true).
				Return(nil).Once()
			agentMock.EXPECT().
				Status(mock.Anything, mock.Anything).
				Return(&agentGrpc.StatusResponse{}, nil).Once()
			agentMock.EXPECT().
				SharePublic(mock.Anything, mock.Anything, mock.Anything).
				Return(&agentGrpc.SharePublicResponse{
					Token:             "shr-token",
					FrontendEndpoints: []string{"https://demo.shares.zrok.io"},
				}, nil).Once()

			r := &ZrokShareReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: events.NewFakeRecorder(16),
				Zrok:     &zrokclient.Clients{REST: restMock, Agent: agentMock},
			}

			_, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: shareName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: shareName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())

			share := &zrokv1alpha1.ZrokShare{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: shareName, Namespace: ns}, share)).To(Succeed())
			Expect(share.Status.AssignedURL).To(Equal("https://demo.shares.zrok.io"))

			share.Spec.NameSelection.Name = "demo-new"
			Expect(k8sClient.Update(ctx, share)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: shareName, Namespace: ns}, share)).To(Succeed())

			restMock.EXPECT().
				ListShares(mock.Anything, mock.Anything, mock.Anything, "envzid").
				Return([]zrokclient.RemoteShare{{
					Token:             "shr-token",
					Target:            "http://nginx.default.svc:80",
					ShareMode:         "public",
					BackendMode:       "proxy",
					FrontendEndpoints: []string{"https://demo.shares.zrok.io"},
				}}, nil)
			agentMock.EXPECT().
				Status(mock.Anything, mock.Anything).
				Return(&agentGrpc.StatusResponse{
					Shares: []*agentGrpc.ShareDetail{{
						Token:            "shr-token",
						ShareMode:        "public",
						BackendMode:      "proxy",
						FrontendEndpoint: []string{"https://demo.shares.zrok.io"},
						BackendEndpoint:  "http://nginx.default.svc:80",
						Status:           "active",
					}},
				}, nil).Once()
			restMock.EXPECT().
				CreateShareName(mock.Anything, mock.Anything, mock.Anything, mock.Anything, "demo-new").
				Return(nil)
			restMock.EXPECT().
				UpdateShareName(mock.Anything, mock.Anything, mock.Anything, mock.Anything, "demo-new", true).
				Return(nil)
			agentMock.EXPECT().
				ReleaseShare(mock.Anything, mock.Anything, "shr-token").
				Return(nil)
			restMock.EXPECT().
				Unshare(mock.Anything, mock.Anything, mock.Anything, "envzid", "shr-token").
				Return(nil)
			restMock.EXPECT().
				DeleteShareName(mock.Anything, mock.Anything, mock.Anything, mock.Anything, "demo").
				Return(nil)
			agentMock.EXPECT().
				SharePublic(mock.Anything, mock.Anything, mock.Anything).
				Return(&agentGrpc.SharePublicResponse{
					Token:             "shr-token-2",
					FrontendEndpoints: []string{"https://demo-new.shares.zrok.io"},
				}, nil)

			_, err = r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: shareName, Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: shareName, Namespace: ns}, share)).To(Succeed())
			Expect(share.Status.AssignedURL).To(Equal("https://demo-new.shares.zrok.io"))
			Expect(share.Status.ShareToken).To(Equal("shr-token-2"))
			Expect(share.Labels["zrok.k8s.zrok.io/frontend-name"]).To(Equal("demo-new"))
			Expect(status.IsTrue(share.Status.Conditions, zrokv1alpha1.ConditionReady)).To(BeTrue())
		})
	})
})
