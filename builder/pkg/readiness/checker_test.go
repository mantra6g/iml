package readiness

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Pod-Based Target Readiness Checker", func() {
	Context("When checking for readiness of a pod's target", func() {
		ctx = context.Background()

		BeforeEach(func() {

		})

		AfterEach(func() {
			Expect("foo").To(Equal("foo"))
		})

		It("should return PodNotFound when no matching pods exist for the target", func() {

		})
	})
})
