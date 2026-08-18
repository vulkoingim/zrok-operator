package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"time"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/test/utils"
)

var _ = Describe("Manager", Ordered, func() {
	BeforeAll(func() {
		Expect(initK8sClients(context.Background())).To(Succeed())

		By("creating manager namespace")
		Expect(ensureManagerNamespace(testCtx)).To(Succeed())

		By("installing CRDs")
		cmd := exec.Command("make", "install")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	AfterAll(func() {
		By("cleaning up live zrok test resources while manager is still running")
		cleanupLiveZrokTestResources(testCtx)

		By("undeploying the controller-manager")
		cmd := exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		_ = deleteManagerNamespace(testCtx)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}

		if pod, err := controllerManagerPod(testCtx); err == nil {
			By("Fetching controller manager pod logs")
			if controllerLogs, err := podLogs(testCtx, managerNS, pod.Name); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			}

			By("Fetching controller manager pod description")
			_, _ = fmt.Fprintf(GinkgoWriter, "Pod phase: %s\n", pod.Status.Phase)
		}

		for _, ns := range []string{managerNS, liveTestNS} {
			By(fmt.Sprintf("Fetching Kubernetes events in %s", ns))
			if events, err := listEvents(testCtx, ns); err == nil {
				for _, ev := range events {
					_, _ = fmt.Fprintf(GinkgoWriter, "%s %s %s %s\n",
						ev.LastTimestamp.Format(time.RFC3339), ev.Type, ev.Reason, ev.Message)
				}
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	It("creates an Environment + Share when ZROK2_ENABLE_TOKEN is set", func() {
		token := os.Getenv("ZROK2_ENABLE_TOKEN")
		if token == "" {
			Skip("ZROK2_ENABLE_TOKEN not set; skipping live zrok e2e")
		}

		DeferCleanup(func() { cleanupLiveZrokTestResources(testCtx) })

		By("waiting for the controller-manager pod")
		Eventually(func(g Gomega) {
			pod, err := controllerManagerPod(testCtx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))
		}).Should(Succeed())

		reservedName := randomReservedName()
		fixture, err := loadEnvironmentShareFixture(token, reservedName)
		Expect(err).NotTo(HaveOccurred())

		By("deploying nginx backend")
		Expect(createOrUpdate(testCtx, desiredNginxDeployment(liveTestNS))).To(Succeed())
		Expect(createOrUpdate(testCtx, desiredNginxService(liveTestNS))).To(Succeed())

		By("creating credentials secret from sample")
		Expect(createOrUpdate(testCtx, fixture.Secret)).To(Succeed())

		By("creating Environment from sample")
		Expect(createOrUpdate(testCtx, fixture.Env)).To(Succeed())

		By("waiting for Environment Ready")
		Eventually(func(g Gomega) {
			env := &zrokv1alpha1.ZrokEnvironment{}
			err := k8sClient.Get(testCtx, types.NamespacedName{
				Name: fixture.Env.Name, Namespace: fixture.Env.Namespace,
			}, env)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(conditionReady(env)).To(BeTrue())
		}).WithTimeout(3 * time.Minute).Should(Succeed())

		By(fmt.Sprintf("creating Share from sample with reserved name %q", reservedName))
		Expect(createOrUpdate(testCtx, fixture.Share)).To(Succeed())

		By("waiting for share Ready")
		Eventually(func(g Gomega) {
			share := &zrokv1alpha1.ZrokShare{}
			err := k8sClient.Get(testCtx, types.NamespacedName{
				Name: fixture.Share.Name, Namespace: fixture.Share.Namespace,
			}, share)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(conditionReady(share)).To(BeTrue())
		}).WithTimeout(3 * time.Minute).Should(Succeed())

		share := &zrokv1alpha1.ZrokShare{}
		Expect(k8sClient.Get(testCtx, types.NamespacedName{
			Name: fixture.Share.Name, Namespace: fixture.Share.Namespace,
		}, share)).To(Succeed())

		url := share.Status.AssignedURL
		Expect(url).NotTo(BeEmpty())
		Expect(url).To(SatisfyAny(
			HavePrefix("http://"),
			HavePrefix("https://"),
			ContainSubstring(reservedName+".shares.zrok.io"),
		))
		_, _ = fmt.Fprintf(GinkgoWriter, "assignedURL=%s\n", url)
	})
})

func randomReservedName() string {
	var b [4]byte
	_, err := rand.Read(b[:])
	Expect(err).NotTo(HaveOccurred())
	return "e2e" + hex.EncodeToString(b[:])
}
