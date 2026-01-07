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

package infra

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1alpha1 "builder/api/infra/v1alpha1"
)

var _ = Describe("P4TargetDeployment Controller", func() {
	Context("When creating a P4TargetDeployment", func() {
		const resourceName = "test-resource"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		AfterEach(func() {
			// Cleanup the specific resource instance P4TargetDeployment
			resource := &infrav1alpha1.P4TargetDeployment{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if errors.IsNotFound(err) {
				return
			}

			By("Cleaning up the specific resource instance P4TargetDeployment")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully create a resource with all required fields", func() {
			By("Creating the custom resource for the Kind P4TargetDeployment")
			resource := &infrav1alpha1.P4TargetDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: infrav1alpha1.P4TargetDeploymentSpec{
					Replicas: nil,
					Class:    "bmv2",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		It("should fail to create a resource when class is unknown", func() {
			By("Creating the custom resource for the Kind P4TargetDeployment with unknown class")
			resource := &infrav1alpha1.P4TargetDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: infrav1alpha1.P4TargetDeploymentSpec{
					Replicas: nil,
					Class:    "unknown-class",
				},
			}
			err := k8sClient.Create(ctx, resource)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})

		It("should succeed to create a resource when replicas are non-nil", func() {
			By("Creating the custom resource for the Kind P4TargetDeployment with unknown class")
			replicas := int32(2)
			resource := &infrav1alpha1.P4TargetDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: infrav1alpha1.P4TargetDeploymentSpec{
					Replicas: &replicas,
					Class:    "bmv2",
				},
			}
			err := k8sClient.Create(ctx, resource)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: infrav1alpha1.BMV2_POD_NAMESPACE,
		}
		p4targetdeployment := &infrav1alpha1.P4TargetDeployment{}

		BeforeEach(func() {
			By("creating the namespace for the infrastructure resources")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: infrav1alpha1.BMV2_POD_NAMESPACE,
				},
			}
			err := k8sClient.Create(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())

			By("creating the custom resource for the Kind P4TargetDeployment")
			err = k8sClient.Get(ctx, typeNamespacedName, p4targetdeployment)
			if err != nil && errors.IsNotFound(err) {
				replicas := int32(1)
				resource := &infrav1alpha1.P4TargetDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: infrav1alpha1.P4TargetDeploymentSpec{
						Replicas: &replicas,
						Class:    "bmv2",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &infrav1alpha1.P4TargetDeployment{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance P4TargetDeployment")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Cleaning up the namespace for the infrastructure resources")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: infrav1alpha1.BMV2_POD_NAMESPACE,
				},
			}
			Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &P4TargetDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
