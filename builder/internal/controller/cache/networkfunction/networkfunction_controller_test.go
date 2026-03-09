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

package networkfunction

import (
	"builder/pkg/util/ptr"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cachev1alpha1 "builder/api/cache/v1alpha1"
	corev1alpha1 "builder/api/core/v1alpha1"
	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"
)

const (
	TargetArchitectureBMv2 = "bmv2"
)

var _ = Describe("NetworkFunction Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		resourceLabels := map[string]string{
			"app": "test-nf",
		}

		BeforeEach(func() {})

		AfterEach(func() {
			resource := &cachev1alpha1.NetworkFunction{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance NetworkFunction")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("creating the custom resource for the Kind NetworkFunction")
			replicas := int32(1)
			resource := &cachev1alpha1.NetworkFunction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      typeNamespacedName.Name,
					Namespace: typeNamespacedName.Namespace,
				},
				Spec: cachev1alpha1.NetworkFunctionSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: resourceLabels,
					},
					Template: schedulingv1alpha1.NetworkFunctionBindingTemplate{
						ObjectMeta: metav1.ObjectMeta{
							Labels: resourceLabels,
						},
						Spec: schedulingv1alpha1.NetworkFunctionBindingSpec{
							ControlPlane: &schedulingv1alpha1.ControlPlaneSpec{
								Image: "example.com/control-plane:latest",
							},
							TargetSelector: map[string]string{
								corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureBMv2,
							},
							P4File: "https://example.com/p4file.p4",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &NetworkFunctionReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that the NetworkFunctionReplicaSet was created")
			replicaSetList := &schedulingv1alpha1.NetworkFunctionReplicaSetList{}
			Expect(k8sClient.List(ctx, replicaSetList, client.InNamespace(typeNamespacedName.Namespace))).To(Succeed())
			Expect(replicaSetList.Items).To(HaveLen(1))
			Expect(replicaSetList.Items[0].Spec.Template.Labels).To(Equal(resourceLabels))
		})

		It("Should create a new NetworkFunctionReplicaSet when the NetworkFunction is updated with a new P4 file",
			func() {
				originalP4File := "https://example.com/p4file.p4"
				newP4File := "https://example.com/newP4file.p4"

				By("creating the custom resource for the Kind NetworkFunction")
				original := &cachev1alpha1.NetworkFunction{
					ObjectMeta: metav1.ObjectMeta{
						Name:      typeNamespacedName.Name,
						Namespace: typeNamespacedName.Namespace,
					},
					Spec: cachev1alpha1.NetworkFunctionSpec{
						Replicas: ptr.To[int32](1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "test-nf",
							},
						},
						Template: schedulingv1alpha1.NetworkFunctionBindingTemplate{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app": "test-nf",
								},
							},
							Spec: schedulingv1alpha1.NetworkFunctionBindingSpec{
								ControlPlane: &schedulingv1alpha1.ControlPlaneSpec{
									Image: "example.com/control-plane:latest",
								},
								TargetSelector: map[string]string{
									corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureBMv2,
								},
								P4File: originalP4File,
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, original)).To(Succeed())

				By("Reconciling the created resource")
				controllerReconciler := &NetworkFunctionReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that the NetworkFunctionReplicaSet was created")
				replicaSetList := &schedulingv1alpha1.NetworkFunctionReplicaSetList{}
				Expect(k8sClient.List(ctx, replicaSetList, client.InNamespace(typeNamespacedName.Namespace))).To(Succeed())
				Expect(replicaSetList.Items).To(HaveLen(1))
				Expect(replicaSetList.Items[0].Spec.Template.Spec.P4File).To(Equal(originalP4File))

				By("Updating the NetworkFunction with a new P4 file")
				updated := &cachev1alpha1.NetworkFunction{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
				updated.Spec.Template.Spec.P4File = newP4File
				Expect(k8sClient.Update(ctx, updated)).To(Succeed())

				By("Reconciling the updated resource")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that a new NetworkFunctionReplicaSet was created with the new P4 file")
				Expect(k8sClient.List(ctx, replicaSetList, client.InNamespace(typeNamespacedName.Namespace))).To(Succeed())
				Expect(replicaSetList.Items).To(HaveLen(2))
				Expect(replicaSetList.Items[1].Spec.Template.Spec.P4File).To(Equal(newP4File))
			})
	})
})
