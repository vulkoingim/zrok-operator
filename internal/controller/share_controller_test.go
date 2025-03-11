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

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/status"
	"github.com/vulkoingim/zrok-operator/internal/zrokclient"
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
			fakeREST := &zrokclient.FakeREST{}
			fakeAgent := &zrokclient.FakeAgent{}
			r := &ZrokShareReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
				Zrok:     &zrokclient.Clients{REST: fakeREST, Agent: fakeAgent},
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
			Expect(status.IsTrue(share.Status.Conditions, zrokv1alpha1.ConditionReady)).To(BeTrue())
			Expect(fakeREST.CreateNames).To(ContainElement("demo"))
			Expect(fakeAgent.ShareCalls).To(BeNumerically(">=", 1))
		})
	})
})
