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

package networkfunctionreplicaset

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
)

var _ = Describe("NetworkFunctionReplicaSet Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"
		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: metav1.NamespaceDefault,
		}
		testLabels := map[string]string{
			"app": "test-nf",
		}
		nfReplicaSet := &schedulingv1alpha1.NetworkFunctionReplicaSet{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind NetworkFunctionReplicaSet")
			err := k8sClient.Get(ctx, typeNamespacedName, nfReplicaSet)
			if err != nil && errors.IsNotFound(err) {
				replicas := int32(1)
				resource := &schedulingv1alpha1.NetworkFunctionReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Labels:    testLabels,
					},
					Spec: schedulingv1alpha1.NetworkFunctionReplicaSetSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: testLabels,
						},
						Template: schedulingv1alpha1.NetworkFunctionTemplate{
							ObjectMeta: metav1.ObjectMeta{
								Labels: testLabels,
							},
							Spec: schedulingv1alpha1.NetworkFunctionSpec{
								TargetSelector: map[string]string{},
								P4File:         "https://example.com/p4file.p4",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &schedulingv1alpha1.NetworkFunctionReplicaSet{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance NetworkFunctionReplicaSet")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &NetworkFunctionReplicaSetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
