package e2e

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vulkoingim/zrok-operator/test/utils"
)

const (
	namespace         = "zrok-operator-system"
	liveTestNS        = "default"
	shareCRName       = "nginx-reserved"
	envCRName         = "default"
	credentialsSecret = "zrok-credentials"
)

var _ = Describe("Manager", Ordered, func() {
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	AfterAll(func() {
		By("cleaning up live zrok test resources while manager is still running")
		cleanupLiveZrokTestResources()

		By("undeploying the controller-manager")
		cmd := exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}

		if podName, err := controllerPodName(); err == nil {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", podName, "-n", namespace)
			if controllerLogs, err := utils.Run(cmd); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", podName, "-n", namespace)
			if podDescription, err := utils.Run(cmd); err == nil {
				fmt.Println("Pod description:\n", podDescription)
			}
		}

		By("Fetching Kubernetes events")
		cmd := exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
		if eventsOutput, err := utils.Run(cmd); err == nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events (%s):\n%s", namespace, eventsOutput)
		}
		cmd = exec.Command("kubectl", "get", "events", "-n", liveTestNS, "--sort-by=.lastTimestamp")
		if eventsOutput, err := utils.Run(cmd); err == nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events (%s):\n%s", liveTestNS, eventsOutput)
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	It("creates an Environment + Share when ZROK2_ENABLE_TOKEN is set", func() {
		token := os.Getenv("ZROK2_ENABLE_TOKEN")
		if token == "" {
			Skip("ZROK2_ENABLE_TOKEN not set; skipping live zrok e2e")
		}

		DeferCleanup(cleanupLiveZrokTestResources)

		By("waiting for the controller-manager pod")
		Eventually(func(g Gomega) {
			podName, err := controllerPodName()
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(podName).To(ContainSubstring("controller-manager"))

			cmd := exec.Command("kubectl", "get", "pods", podName,
				"-o", "jsonpath={.status.phase}", "-n", namespace)
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Running"))
		}).Should(Succeed())

		By("deploying nginx backend")
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(`
apiVersion: apps/v1
kind: Deployment
metadata: { name: nginx, namespace: default }
spec:
  replicas: 1
  selector: { matchLabels: { app: nginx } }
  template:
    metadata: { labels: { app: nginx } }
    spec:
      containers:
      - name: nginx
        image: nginx:stable
        ports: [{ containerPort: 80 }]
---
apiVersion: v1
kind: Service
metadata: { name: nginx, namespace: default }
spec:
  selector: { app: nginx }
  ports: [{ port: 80, targetPort: 80 }]
`)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		reservedName := randomReservedName()

		By("creating enable token secret")
		cmd = exec.Command("kubectl", "delete", "secret", credentialsSecret, "-n", liveTestNS, "--ignore-not-found")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "create", "secret", "generic", credentialsSecret,
			"--from-file=enable-token=/dev/stdin", "-n", liveTestNS)
		cmd.Stdin = strings.NewReader(token)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("applying Environment")
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(fmt.Sprintf(`
apiVersion: zrok.k8s.zrok.io/v1alpha1
kind: ZrokEnvironment
metadata:
  name: %s
  namespace: %s
spec:
  enableTokenSecretRef:
    name: %s
    key: enable-token
  reclaimPolicy: Delete
  agent:
    consolePort: 8888
`, envCRName, liveTestNS, credentialsSecret))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for Environment Ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "zrokenvironment", envCRName, "-n", liveTestNS,
				"-o", `jsonpath={.status.conditions[?(@.type=="Ready")].status}`)
			out, runErr := utils.Run(cmd)
			g.Expect(runErr).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("True"))
		}).WithTimeout(3 * time.Minute).Should(Succeed())

		By(fmt.Sprintf("creating share with reserved name %q", reservedName))
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(fmt.Sprintf(`
apiVersion: zrok.k8s.zrok.io/v1alpha1
kind: ZrokShare
metadata:
  name: %s
  namespace: %s
spec:
  environmentRef:
    name: %s
  shareMode: public
  backendMode: proxy
  upstream:
    url: http://nginx.%s.svc:80
  nameSelection:
    namespace: public
    name: %s
  reclaimPolicy: Delete
`, shareCRName, liveTestNS, envCRName, liveTestNS, reservedName))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for share Ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "zrokshare", shareCRName, "-n", liveTestNS,
				"-o", `jsonpath={.status.conditions[?(@.type=="Ready")].status}`)
			out, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("True"))
		}).WithTimeout(3 * time.Minute).Should(Succeed())

		By("fetching assigned URL")
		cmd = exec.Command("kubectl", "get", "zrokshare", shareCRName, "-n", liveTestNS,
			"-o", "jsonpath={.status.assignedURL}")
		url, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(url).NotTo(BeEmpty())
		Expect(url).To(SatisfyAny(
			HavePrefix("http://"),
			HavePrefix("https://"),
			ContainSubstring(reservedName+".shares.zrok.io"),
		))
		_, _ = fmt.Fprintf(GinkgoWriter, "assignedURL=%s\n", url)
	})
})

func controllerPodName() (string, error) {
	cmd := exec.Command("kubectl", "get",
		"pods", "-l", "control-plane=controller-manager",
		"-o", "go-template={{ range .items }}"+
			"{{ if not .metadata.deletionTimestamp }}"+
			"{{ .metadata.name }}"+
			"{{ \"\\n\" }}{{ end }}{{ end }}",
		"-n", namespace,
	)
	podOutput, err := utils.Run(cmd)
	if err != nil {
		return "", err
	}
	podNames := utils.GetNonEmptyLines(podOutput)
	if len(podNames) != 1 {
		return "", fmt.Errorf("expected 1 controller pod, got %d", len(podNames))
	}
	return podNames[0], nil
}

// randomReservedName returns a DNS label valid for spec.nameSelection.name.
func randomReservedName() string {
	var b [4]byte
	_, err := rand.Read(b[:])
	Expect(err).NotTo(HaveOccurred())
	return "e2e" + hex.EncodeToString(b[:])
}

// cleanupLiveZrokTestResources tears down default-namespace fixtures. Shares must go
// before the Environment so finalizers still have the enable token when the manager is up.
// If delete times out or the manager is already gone, finalizers are patched off.
func cleanupLiveZrokTestResources() {
	const wait = "3m"

	deleteZrokCRs("zrokshare", liveTestNS)
	deleteZrokCRs("zrokaccess", liveTestNS)
	deleteZrokCRs("zrokenvironment", liveTestNS)

	cmd := exec.Command("kubectl", "delete", "secret", credentialsSecret, envCRName+"-zrok-identity",
		"-n", liveTestNS, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	cmd = exec.Command("kubectl", "delete", "deploy,svc", "nginx", "-n", liveTestNS,
		"--ignore-not-found", "--wait=true", "--timeout="+wait)
	_, _ = utils.Run(cmd)

	cmd = exec.Command("kubectl", "delete", "deploy,svc", envCRName+"-agent", "-n", liveTestNS,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)
	cmd = exec.Command("kubectl", "delete", "pvc", envCRName+"-zrok-home", "-n", liveTestNS,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)
}

func deleteZrokCRs(resource, namespace string, names ...string) {
	const wait = "3m"
	var cmd *exec.Cmd
	if len(names) == 0 {
		cmd = exec.Command("kubectl", "delete", resource, "--all", "-n", namespace,
			"--ignore-not-found", "--wait=true", "--timeout="+wait)
	} else {
		args := append([]string{"delete", resource}, names...)
		args = append(args, "-n", namespace, "--ignore-not-found", "--wait=true", "--timeout="+wait)
		cmd = exec.Command("kubectl", args...)
	}
	_, _ = utils.Run(cmd)
	forceRemoveStuckZrokCRs(resource, namespace)
}

func forceRemoveStuckZrokCRs(resource, namespace string) {
	for _, name := range listKubeResourceNames(resource, namespace) {
		cmd := exec.Command("kubectl", "patch", resource, name, "-n", namespace,
			"--type=merge", "-p", `{"metadata":{"finalizers":null}}`)
		_, _ = utils.Run(cmd)
	}
	cmd := exec.Command("kubectl", "delete", resource, "--all", "-n", namespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)
}

func listKubeResourceNames(resource, namespace string) []string {
	cmd := exec.Command("kubectl", "get", resource, "-n", namespace,
		"-o", `jsonpath={range .items[*]}{.metadata.name}{"\n"}{end}`)
	out, err := utils.Run(cmd)
	if err != nil {
		return nil
	}
	return utils.GetNonEmptyLines(out)
}
