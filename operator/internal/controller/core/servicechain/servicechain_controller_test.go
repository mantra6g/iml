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

package servicechain

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "loom/api/core/v1alpha1"
	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
)

var _ = Describe("ServiceChain Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		servicechain := &corev1alpha1.ServiceChain{}

		BeforeEach(func() {})

		AfterEach(func() {
			resource := &corev1alpha1.ServiceChain{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ServiceChain")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("creating the referenced Application resources")
			app1 := &corev1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-1",
					Namespace: "default",
				},
				Spec: corev1alpha1.ApplicationSpec{},
			}
			Expect(k8sClient.Create(ctx, app1)).To(Succeed())

			app2 := &corev1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-2",
					Namespace: "default",
				},
				Spec: corev1alpha1.ApplicationSpec{},
			}
			Expect(k8sClient.Create(ctx, app2)).To(Succeed())

			By("creating the referenced NetworkFunctionDeployment resource")
			replicas := int32(1)
			nf1 := &schedulingv1alpha1.NetworkFunctionDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nf-1",
					Namespace: "default",
				},
				Spec: schedulingv1alpha1.NetworkFunctionDeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-nf",
						},
					},
					Template: corev1alpha1.NetworkFunctionTemplate{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "test-nf",
							},
						},
						Spec: corev1alpha1.NetworkFunctionSpec{
							TargetSelector: map[string]string{},
							P4File:         "https://example.com/p4file.p4",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, nf1)).To(Succeed())

			By("creating the custom resource for the Kind ServiceChain")
			err := k8sClient.Get(ctx, typeNamespacedName, servicechain)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.ServiceChain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: corev1alpha1.ServiceChainSpec{
						From: &corev1alpha1.ApplicationReference{
							Name:      "app-1",
							Namespace: "default",
						},
						To: &corev1alpha1.ApplicationReference{
							Name:      "app-2",
							Namespace: "default",
						},
						Functions: []metav1.LabelSelector{
							{
								MatchLabels: map[string]string{
									"app": "test-nf",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("Reconciling the created resource")
			controllerReconciler := &ServiceChainReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
