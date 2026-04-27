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

package loomnode

import (
	"context"
	"net/netip"

	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"
	"github.com/mantra6g/iml/operator/test/mocks"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("LoomNode Controller", func() {
	Context("When reconciling a resource", func() {
		const nodeName = "test-resource"
		const nodeNamespace = "default"
		ctx := context.Background()
		node := &infrav1alpha1.LoomNode{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nodeName,
				Namespace: nodeNamespace,
			},
		}
		nodeKey := ctrlclient.ObjectKeyFromObject(node)
		ipv4Allocator := mocks.NewFakePrefixAllocator(netip.MustParsePrefix("10.123.0.0/24"))
		ipv6Allocator := mocks.NewFakePrefixAllocator(netip.MustParsePrefix("fd00::/64"))

		BeforeEach(func() {
			_ = k8sClient.Delete(ctx, node)
		})

		AfterEach(func() {})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &LoomNodeReconciler{
				Client:              k8sClient,
				Scheme:              k8sClient.Scheme(),
				NodeCIDRv4Allocator: ipv4Allocator,
				NodeCIDRv6Allocator: ipv6Allocator,
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: nodeKey,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
