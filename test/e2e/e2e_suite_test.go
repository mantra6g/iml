//go:build e2e

package e2e

import (
	"fmt"
	//"strings"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting iml e2e test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("building the images")
	Run("docker", "bake", "image-all")

	By("creating kind cluster")
	Run("make", "kind-create")

	By("loading iml images to kind")
	Run("make", "kind-load-all")

	By("installing iml")
	Run("make", "build-installer")
	Run("kubectl", "apply", "--wait", "-f", "install.yaml")

	By("building example images")
	Run("make", "-C", "examples/demo/", "docker-build-all")

	By("loading example images to kind")
	Run("make", "-C", "examples/demo/", "kind-load-all")

	By("deploy test chain")
	Run("kubectl", "apply", "--wait", "-f", "test/e2e/simple-with-proxy")
})

var _ = AfterSuite(func() {
	By("deleting kind cluster")
	Run("make", "kind-delete")
})

func Run(cmds ...string) {
	cmd := exec.Command(cmds[0], cmds[1:]...)
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	cmd.Dir = filepath.Join(cwd, "..", "..")

	fmt.Fprintf(GinkgoWriter, "run: %v%v\n", cmd.Dir, cmds)
	output, err := cmd.CombinedOutput()

	Expect(err).NotTo(
		HaveOccurred(),
		"command failed:\n%v\n\n%s",
		cmds,
		output,
	)
}
