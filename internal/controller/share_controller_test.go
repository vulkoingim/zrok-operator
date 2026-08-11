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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openziti/zrok/v2/agent/agentGrpc"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
			_ = k8sClient.Delete(ctx, &zrokv1alpha1.ZrokShare{ObjectMeta: metav1.ObjectMeta{Name: shareName, Namespace: ns}})
			_ = k8sClient.Delete(ctx, &zrokv1alpha1.ZrokEnvironment{ObjectMeta: metav1.ObjectMeta{Name: envName, Namespace: ns}})
			_ = k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "zrok-credentials", Namespace: ns}})
		})

		It("should create a share via the agent client", func() {
			restMock := zrokclientmock.NewRESTClient(GinkgoT())
			agentMock := zrokclientmock.NewAgentClient(GinkgoT())

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
		})
	})
})
