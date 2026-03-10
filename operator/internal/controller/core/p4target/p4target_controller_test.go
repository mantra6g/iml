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

package p4target

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "loom/api/core/v1alpha1"
	infrav1alpha1 "loom/api/infra/v1alpha1"
	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
	p4targetutil "loom/internal/controller/core/p4target/util"
)

const (
	TargetArchitectureBMv2 = "bmv2"
)

var _ = Describe("P4Target Controller", func() {
	Context("When creating a P4Target", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		p4target := &corev1alpha1.P4Target{}

		BeforeEach(func() {})

		AfterEach(func() {})

		It("should successfully create a target of class bmv2", func() {
			By("creating the custom resource for the Kind P4Target")
			err := k8sClient.Get(ctx, typeNamespacedName, p4target)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.P4Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Labels: map[string]string{
							corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureBMv2,
						},
					},
					Spec: corev1alpha1.P4TargetSpec{},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("Verifying the created resource")
			createdResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, createdResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdResource.Labels).To(
				HaveKeyWithValue(corev1alpha1.P4TargetArchitectureLabel, TargetArchitectureBMv2))

			By("Cleanup the specific resource instance P4Target")
			Expect(k8sClient.Delete(ctx, createdResource)).To(Succeed())
		})

		It("should not fail to create targets of unknown class", func() {
			By("creating a P4Target resource with an unknown target class")
			err := k8sClient.Get(ctx, typeNamespacedName, p4target)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.P4Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Labels: map[string]string{
							corev1alpha1.P4TargetArchitectureLabel: "unknown-class",
						},
					},
					Spec: corev1alpha1.P4TargetSpec{},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		It("should use a cluster-wide resource instead of a namespaced resource", func() {
			By("attempting to create a P4Target resource in a non-default namespace")
			err := k8sClient.Get(ctx, typeNamespacedName, p4target)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.P4Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "some-other-namespace",
						Labels: map[string]string{
							corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureBMv2,
						},
					},
					Spec: corev1alpha1.P4TargetSpec{},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("Verifying that the namespace of the created resource is empty")
			createdResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, createdResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdResource.ObjectMeta.Namespace).To(Equal(""))

			By("Cleanup the specific resource instance P4Target")
			Expect(k8sClient.Delete(ctx, createdResource)).To(Succeed())
		})
	})

	Context("When reconciling edge cases", func() {
		const resourceName = "edge-resource"

		ctx := context.Background()

		It("should ignore missing P4Targets without returning an error", func() {
			reconciler := &P4TargetReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a BMv2 P4Target", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		p4target := &corev1alpha1.P4Target{}

		BeforeEach(func() {
			By("creating the P4Target resource")
			err := k8sClient.Get(ctx, typeNamespacedName, p4target)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.P4Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Labels: map[string]string{
							corev1alpha1.P4TargetArchitectureLabel: "unknown-class",
						},
					},
					Spec: corev1alpha1.P4TargetSpec{},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &corev1alpha1.P4Target{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance P4Target")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			pod := &corev1.Pod{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-xyz",
				Namespace: infrav1alpha1.BMV2_POD_NAMESPACE,
			}, pod)
			if err == nil {
				By("Cleaning up the associated pod")
				Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
			}
		})

		It("should be not ready when the associated pod wasn't created yet", func() {
			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the status of the reconciled resource")
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, reconciledResource)
			Expect(err).NotTo(HaveOccurred())
			readyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(readyCondition).To(Not(BeNil()))
			Expect(readyCondition.Status).To(Equal(corev1.ConditionFalse))
		})

		It("should be not ready when the checker says the infra isn't ready", func() {
			By("Creating the namespace for the infrastructure resources")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: infrav1alpha1.BMV2_POD_NAMESPACE,
				},
			}
			_ = k8sClient.Create(ctx, namespace)

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the status of the reconciled resource")
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, reconciledResource)
			Expect(err).NotTo(HaveOccurred())
			readyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(readyCondition).To(Not(BeNil()))
			Expect(readyCondition.Status).To(Equal(corev1.ConditionFalse))
		})

		It("should be ready when the checker says the infra is ready and no nfs exist for that target", func() {
			By("Creating the namespace for the infrastructure resources")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: infrav1alpha1.BMV2_POD_NAMESPACE,
				},
			}
			_ = k8sClient.Create(ctx, namespace)

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the status of the reconciled resource")
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, reconciledResource)
			Expect(err).NotTo(HaveOccurred())
			readyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(readyCondition).To(Not(BeNil()))
			Expect(readyCondition.Status).To(Equal(corev1.ConditionTrue))
		})

		It("should be not ready when a nf already exists for that target", func() {
			By("Creating the namespace for the infrastructure resources")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: infrav1alpha1.BMV2_POD_NAMESPACE,
				},
			}
			_ = k8sClient.Create(ctx, namespace)

			By("Creating a nf for that target")
			nf := &schedulingv1alpha1.NetworkFunction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nf-for-" + resourceName,
					Namespace: "default",
					Labels: map[string]string{
						schedulingv1alpha1.TARGET_ASSIGNMENT_LABEL: resourceName,
					},
				},
				Spec: schedulingv1alpha1.NetworkFunctionSpec{
					TargetSelector: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureBMv2,
					},
					P4File: "https://example.com/p4example.p4",
				},
			}
			Expect(k8sClient.Create(ctx, nf)).To(Succeed())
			defer func() {
				By("Cleaning up the created nf")
				Expect(k8sClient.Delete(ctx, nf)).To(Succeed())
			}()

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the status of the reconciled resource")
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, reconciledResource)
			Expect(err).NotTo(HaveOccurred())
			readyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(readyCondition).To(Not(BeNil()))
			Expect(readyCondition.Status).To(Equal(corev1.ConditionFalse))
		})
	})
})
