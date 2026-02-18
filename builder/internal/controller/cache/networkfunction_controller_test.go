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

package cache

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachev1alpha1 "builder/api/cache/v1alpha1"
	corev1alpha1 "builder/api/core/v1alpha1"
	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"
	"builder/test/mocks"
)

var _ = Describe("NetworkFunction Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		networkfunction := &cachev1alpha1.NetworkFunction{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind NetworkFunction")
			err := k8sClient.Get(ctx, typeNamespacedName, networkfunction)
			if err != nil && errors.IsNotFound(err) {
				replicas := int32(1)
				resource := &cachev1alpha1.NetworkFunction{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: cachev1alpha1.NetworkFunctionSpec{
						Replicas: &replicas,
						Template: schedulingv1alpha1.NetworkFunctionBindingTemplate{
							Selector: corev1alpha1.P4TargetSelector{
								SupportedTargets: []corev1alpha1.TargetClass{corev1alpha1.TARGET_CLASS_BMV2},
							},
							P4File: "https://example.com/p4file.p4",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &cachev1alpha1.NetworkFunction{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance NetworkFunction")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			fakeEventBus := &mocks.FakeEventBus{}

			controllerReconciler := &NetworkFunctionReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Bus:    fakeEventBus,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
