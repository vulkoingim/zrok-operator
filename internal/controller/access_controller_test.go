package controller

import (
	"context"

	"github.com/openziti/zrok/v2/agent/agentGrpc"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/status"
	"github.com/vulkoingim/zrok-operator/internal/zrokclient"
	zrokclientmock "github.com/vulkoingim/zrok-operator/internal/zrokclient/mock"
)

var _ = Describe("ZrokAccess Controller", func() {
	Context("when environment becomes ready", func() {
		const (
			accessName = "test-access"
			envName    = "test-access-env"
			ns         = "default"
		)

		ctx := context.Background()

		BeforeEach(func() {
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

			access := &zrokv1alpha1.ZrokAccess{
				ObjectMeta: metav1.ObjectMeta{Name: accessName, Namespace: ns},
				Spec: zrokv1alpha1.ZrokAccessSpec{
					EnvironmentRef: corev1.LocalObjectReference{Name: envName},
					ShareToken:     "share-tok",
					BindAddress:    "127.0.0.1:0",
				},
			}
			_ = k8sClient.Delete(ctx, access)
			Expect(k8sClient.Create(ctx, access)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, &zrokv1alpha1.ZrokAccess{ObjectMeta: metav1.ObjectMeta{Name: accessName, Namespace: ns}})
			_ = k8sClient.Delete(ctx, &zrokv1alpha1.ZrokEnvironment{ObjectMeta: metav1.ObjectMeta{Name: envName, Namespace: ns}})
		})

		It("creates access once environment is Ready and heals when agent loses it", func() {
			agentMock := zrokclientmock.NewAgentClient(GinkgoT())
			restMock := zrokclientmock.NewRESTClient(GinkgoT())
			r := &ZrokAccessReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: events.NewFakeRecorder(16),
				Zrok:     &zrokclient.Clients{REST: restMock, Agent: agentMock},
			}

			By("waiting while environment is not ready")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: accessName, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: accessName, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())
			access := &zrokv1alpha1.ZrokAccess{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: accessName, Namespace: ns}, access)).To(Succeed())
			Expect(status.IsTrue(access.Status.Conditions, zrokv1alpha1.ConditionReady)).To(BeFalse())

			By("marking environment ready")
			env := &zrokv1alpha1.ZrokEnvironment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: envName, Namespace: ns}, env)).To(Succeed())
			status.SetCondition(&env.Status.Conditions, zrokv1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "ready", 1)
			env.Status.AgentReady = true
			Expect(k8sClient.Status().Update(ctx, env)).To(Succeed())

			agentMock.EXPECT().AccessPrivate(mock.Anything, mock.Anything, mock.Anything).
				Return(&agentGrpc.AccessPrivateResponse{FrontendToken: "access-tok"}, nil).Once()
			agentMock.EXPECT().Status(mock.Anything, mock.Anything).
				Return(&agentGrpc.StatusResponse{
					Accesses: []*agentGrpc.AccessDetail{{
						FrontendToken: "access-tok",
						BindAddress:   "127.0.0.1:4040",
						Status:        "active",
					}},
				}, nil).Once()

			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: accessName, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: accessName, Namespace: ns}, access)).To(Succeed())
			Expect(access.Status.AccessToken).To(Equal("access-tok"))
			Expect(access.Status.FrontendEndpoint).To(Equal("127.0.0.1:4040"))
			Expect(status.IsTrue(access.Status.Conditions, zrokv1alpha1.ConditionReady)).To(BeTrue())

			By("healing when access missing from agent")
			agentMock.EXPECT().Status(mock.Anything, mock.Anything).
				Return(&agentGrpc.StatusResponse{Accesses: nil}, nil).Once()
			agentMock.EXPECT().ReleaseAccess(mock.Anything, mock.Anything, "access-tok").Return(nil).Once()
			agentMock.EXPECT().AccessPrivate(mock.Anything, mock.Anything, mock.Anything).
				Return(&agentGrpc.AccessPrivateResponse{FrontendToken: "access-tok-2"}, nil).Once()
			agentMock.EXPECT().Status(mock.Anything, mock.Anything).
				Return(&agentGrpc.StatusResponse{
					Accesses: []*agentGrpc.AccessDetail{{
						FrontendToken: "access-tok-2",
						BindAddress:   "127.0.0.1:4041",
						Status:        "active",
					}},
				}, nil).Once()

			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: accessName, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: accessName, Namespace: ns}, access)).To(Succeed())
			Expect(access.Status.AccessToken).To(Equal("access-tok-2"))
			Expect(access.Status.FrontendEndpoint).To(Equal("127.0.0.1:4041"))
		})
	})
})
