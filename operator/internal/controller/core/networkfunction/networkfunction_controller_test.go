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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	res "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	schedulingv1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
)

const (
	TargetArchitectureV1Model = "v1model"
)

var _ = Describe("NetworkFunction Controller", func() {
	Context("When reconciling a resource", func() {
		const nfName = "test-nf"
		const nfNamespace = "default"
		const targetName = "test-target"
		ctx := context.Background()
		nf := &corev1alpha1.NetworkFunction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nfName,
				Namespace: nfNamespace,
			},
		}
		nfKey := ctrlclient.ObjectKeyFromObject(nf)
		target := &corev1alpha1.P4Target{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: nfNamespace,
			},
		}

		BeforeEach(func() {
			_ = k8sClient.Delete(ctx, target)
			_ = k8sClient.Delete(ctx, nf)
		})

		AfterEach(func() {})

		It("should successfully schedule a network function to a p4target", func() {
			By("Creating a new NetworkFunction resource")
			resource := &schedulingv1alpha1.NetworkFunction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nf.Name,
					Namespace: nf.Namespace,
				},
				Spec: schedulingv1alpha1.NetworkFunctionSpec{
					TargetSelector: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureV1Model,
					},
					P4File: "https://example.com/p4file.p4",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Creating a target resource that matches the NetworkFunction's TargetSelector")
			targetResource := &corev1alpha1.P4Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      target.Name,
					Namespace: target.Namespace,
					Labels: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureV1Model,
					},
				},
				Spec: corev1alpha1.P4TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, targetResource)).To(Succeed())

			By("Setting the target's status to Ready")
			targetResource.Status.Conditions = []corev1alpha1.P4TargetCondition{{
				Type:   corev1alpha1.P4TargetConditionReady,
				Status: metav1.ConditionTrue,
			}}
			Expect(k8sClient.Status().Update(ctx, targetResource)).To(Succeed())

			By("Setting the target's capacity of nf-slots to 1")
			targetResource.Status.Capacity = corev1.ResourceList{
				"loom.io/nf-slots": *res.NewQuantity(1, res.DecimalSI),
			}
			Expect(k8sClient.Status().Update(ctx, targetResource)).To(Succeed())

			By("Verifying the NetworkFunction is scheduled to the matching target")
			Eventually(func(g Gomega) {
				updatedResource := &schedulingv1alpha1.NetworkFunction{}
				err := k8sClient.Get(ctx, nfKey, updatedResource)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updatedResource.Spec.TargetName).To(Equal(target.Name))
			}).Should(Succeed())
		})

		It("should handle resources with missing required fields gracefully", func() {
			By("Creating a NetworkFunction resource with missing P4File")
			resource := &schedulingv1alpha1.NetworkFunction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nf.Name,
					Namespace: nf.Namespace,
				},
				Spec: schedulingv1alpha1.NetworkFunctionSpec{
					TargetSelector: map[string]string{},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Not(Succeed()))
		})

		It("should not schedule if no matching targets exist", func() {
			By("Creating a NetworkFunction resource with a specific TargetSelector")
			resource := &schedulingv1alpha1.NetworkFunction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nf.Name,
					Namespace: nf.Namespace,
				},
				Spec: schedulingv1alpha1.NetworkFunctionSpec{
					TargetSelector: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: "some-unknown-architecture",
					},
					P4File: "https://example.com/p4file.p4",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Verifying the NetworkFunction is not scheduled")
			Consistently(func(g Gomega) {
				updatedResource := &schedulingv1alpha1.NetworkFunction{}
				err := k8sClient.Get(ctx, nfKey, updatedResource)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updatedResource.Spec.TargetName).To(BeEmpty())
			}).Should(Succeed())
		})

		It("should not schedule if no matching READY targets exist", func() {
			By("Creating a NetworkFunction resource with a specific TargetSelector")
			resource := &schedulingv1alpha1.NetworkFunction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nf.Name,
					Namespace: nf.Namespace,
				},
				Spec: schedulingv1alpha1.NetworkFunctionSpec{
					TargetSelector: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: "some-unknown-architecture",
					},
					P4File: "https://example.com/p4file.p4",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Creating a target resource that matches the NetworkFunction's TargetSelector")
			targetResource := &corev1alpha1.P4Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      target.Name,
					Namespace: target.Namespace,
					Labels: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureV1Model,
					},
				},
				Spec: corev1alpha1.P4TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, targetResource)).To(Succeed())

			By("Verifying the NetworkFunction is not scheduled")
			Consistently(func(g Gomega) {
				updatedResource := &schedulingv1alpha1.NetworkFunction{}
				err := k8sClient.Get(ctx, nfKey, updatedResource)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updatedResource.Spec.TargetName).To(BeEmpty())
			}).Should(Succeed())
		})
	})
})
