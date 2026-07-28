//go:build e2e

package e2e

import (
	"fmt"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	nodeip        = ""
	nodeport      = "30007"
	proxiedport   = "3000"
	chainStart    = "local-web-proxy"
	chainEnd      = "dummy-web-server"
	realApp       = "web-server"
	lbNF          = "web-lb"
	upstreamChain = "web-uplink"
	kindCluster   = "iml"
	deployments   = []string{
		realApp,
		chainEnd,
		"web-lb-controller",
		chainStart,
	}
	p4targets = []string{
		"bmv2-target-1",
	}
)

var _ = Describe("IML", Ordered, func() {

	BeforeAll(func() {
		cmd := exec.Command(
			"docker", "inspect",
			"iml-control-plane", "-f",
			"'{{.NetworkSettings.Networks.kind.IPAddress}}'",
		)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		nodeip = strings.TrimSpace(string(output))
	})

	AfterAll(func() {})

	AfterEach(func() {
		cmd := exec.Command(
			"kubectl", "exec",
			"deployment/"+realApp,
			"--",
			"killall", "nc",
		)
		cmd.Run()
		//out, err := cmd.CombinedOutput()
		//fmt.Fprintf(GinkgoWriter, "err: %s\n", err)
		//fmt.Fprintf(GinkgoWriter, "out: %s\n", out)
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Test chain", func() {
		It("Should deploy successfully", func() {
			for _, d := range deployments {
				Eventually(func(g Gomega) {
					cmd := exec.Command(
						"kubectl", "get",
						"deployment/"+d,
						"-o",
						"jsonpath={.status.conditions[?(@.type==\"Available\")].status}",
					)

					output, err := cmd.CombinedOutput()
					//fmt.Fprintf(GinkgoWriter, "out: %s\n", output)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(string(output)).To(Equal("True"))
				}).Should(Succeed())
			}
		})

		It("P4 targets should be ready", func() {
			for _, d := range p4targets {
				Eventually(func(g Gomega) {
					cmd := exec.Command(
						"kubectl", "get",
						"p4target/" + d,
						"-o",
						"jsonpath={.status.conditions[?(@.type==\"Ready\")].status}",
					)

					output, err := cmd.CombinedOutput()
					//fmt.Fprintf(GinkgoWriter, "out: %s\n", output)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(string(output)).To(Equal("True"))
				}).Should(Succeed())
			}
			time.Sleep(time.Second * 5)
		})

		It("Connectivity should work upstream", func() {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
			defer cancel()

			serverCmd := exec.CommandContext(
				ctx,
				"kubectl", "exec",
				"deployment/" + realApp,
				"--",
				"nc", "-6l", proxiedport,
				"-W", "1",
			)
			clientCmd := exec.Command("sh", "-c", "echo 'testupstream' | nc "+nodeip+" "+nodeport)

			var buf bytes.Buffer
			serverCmd.Stdout = &buf
			serverCmd.Stderr = &buf

			err := serverCmd.Start()
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(time.Second * 2)

			_, err = clientCmd.CombinedOutput()

			err = serverCmd.Wait()
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal("testupstream\n"))
		})

		It("Connectivity should work downstream", func() {

			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
			defer cancel()

			serverCmd := exec.CommandContext(
				ctx,
				"kubectl", "exec",
				"deployment/" + realApp,
				"--",
				"sh", "-c",
				"echo 'testdownstream' | nc -6l " + proxiedport,
			)
			clientCmd := exec.Command(
				"sh", "-c",
				"nc " + nodeip + " " + nodeport + " -W 1",
			)

			err := serverCmd.Start()
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(time.Second * 2)

			out, err := clientCmd.CombinedOutput()

			err = serverCmd.Wait()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("testdownstream\n"))
		})
	})
})
