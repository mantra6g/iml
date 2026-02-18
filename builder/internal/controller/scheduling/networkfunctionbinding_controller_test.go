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

package scheduling

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "builder/api/core/v1alpha1"
	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"
)

var _ = Describe("NetworkFunctionBinding Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {})

		AfterEach(func() {
			resource := &schedulingv1alpha1.NetworkFunctionBinding{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if errors.IsNotFound(err) {
				// Resource already deleted
				return
			}
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance NetworkFunctionBinding")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			controllerReconciler := &NetworkFunctionBindingReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
		})

		It("should successfully reconcile the resource", func() {
			By("Creating a new NetworkFunctionBinding resource")
			resource := &schedulingv1alpha1.NetworkFunctionBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: schedulingv1alpha1.NetworkFunctionBindingSpec{
					Selector: corev1alpha1.P4TargetSelector{
						SupportedTargets: []corev1alpha1.TargetClass{corev1alpha1.TARGET_CLASS_BMV2},
					},
					P4File: "https://example.com/p4file.p4",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &NetworkFunctionBindingReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add finalizer on creation", func() {
			By("Creating a new NetworkFunctionBinding resource")
			resource := &schedulingv1alpha1.NetworkFunctionBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: schedulingv1alpha1.NetworkFunctionBindingSpec{
					Selector: corev1alpha1.P4TargetSelector{
						SupportedTargets: []corev1alpha1.TargetClass{corev1alpha1.TARGET_CLASS_BMV2},
					},
					P4File: "https://example.com/p4file.p4",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the created resource to add finalizer")
			controllerReconciler := &NetworkFunctionBindingReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying finalizer is added")
			updatedResource := &schedulingv1alpha1.NetworkFunctionBinding{}
			err = k8sClient.Get(ctx, typeNamespacedName, updatedResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(containsString(updatedResource.GetFinalizers(), schedulingv1alpha1.BINDING_FINALIZER_LABEL)).To(BeTrue())
		})

		It("should not return an error when reconciling a deleted resource", func() {
			By("Creating a new NetworkFunctionBinding resource")
			resource := &schedulingv1alpha1.NetworkFunctionBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: schedulingv1alpha1.NetworkFunctionBindingSpec{
					Selector: corev1alpha1.P4TargetSelector{
						SupportedTargets: []corev1alpha1.TargetClass{corev1alpha1.TARGET_CLASS_BMV2},
					},
					P4File: "https://example.com/p4file.p4",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the created resource to ensure it exists")
			controllerReconciler := &NetworkFunctionBindingReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the created resource")
			retrievedResource := &schedulingv1alpha1.NetworkFunctionBinding{}
			err = k8sClient.Get(ctx, typeNamespacedName, retrievedResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, retrievedResource)).To(Succeed())

			By("Reconciling the deleted resource")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
