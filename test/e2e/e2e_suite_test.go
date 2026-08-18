package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vulkoingim/zrok-operator/test/utils"
)

// projectImage is the image built and loaded into Kind. Honor IMG so mise
// kind:load and this suite share a tag (default matches mise.toml IMG).
var projectImage = getenvDefault("IMG", "zrok-operator:dev")

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting zrok-operator integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	if os.Getenv("SKIP_IMAGE_BUILD") != "true" {
		By("building the manager(Operator) image")
		cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
		_, err := utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")
	}

	By("verifying the manager image exists locally")
	inspect := exec.Command("docker", "image", "inspect", projectImage)
	_, err := utils.Run(inspect)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(),
		"image %q is not present in the local docker daemon (docker-build must --load)", projectImage)

	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")
})
