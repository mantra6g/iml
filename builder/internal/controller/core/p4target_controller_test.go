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

package core

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "builder/api/core/v1alpha1"
	infrav1alpha1 "builder/api/infra/v1alpha1"
	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"
)

var _ = Describe("P4Target Controller", func() {
	Context("When creating a P4Target", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		p4target := &corev1alpha1.P4Target{}

		BeforeEach(func() {})

		AfterEach(func() {
			// // TODO(user): Cleanup logic after each test, like removing the resource instance.
			// resource := &corev1alpha1.P4Target{}
			// err := k8sClient.Get(ctx, typeNamespacedName, resource)
			// Expect(err).NotTo(HaveOccurred())

			// By("Cleanup the specific resource instance P4Target")
			// Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully create a target of class bmv2", func() {
			By("creating the custom resource for the Kind P4Target")
			err := k8sClient.Get(ctx, typeNamespacedName, p4target)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.P4Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: corev1alpha1.P4TargetSpec{
						TargetClass: corev1alpha1.TARGET_BMV2,
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("Verifying the created resource")
			createdResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, createdResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdResource.Spec.TargetClass).To(Equal(corev1alpha1.TARGET_BMV2))

			By("Cleanup the specific resource instance P4Target")
			Expect(k8sClient.Delete(ctx, createdResource)).To(Succeed())
		})

		It("should fail to create targets of unknown class", func() {
			By("creating a P4Target resource with an unknown target class")
			err := k8sClient.Get(ctx, typeNamespacedName, p4target)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.P4Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: corev1alpha1.P4TargetSpec{
						TargetClass: "an_unknown_class",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(MatchError(ContainSubstring("Unsupported value")))
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
					},
					Spec: corev1alpha1.P4TargetSpec{
						TargetClass: corev1alpha1.TARGET_BMV2,
					},
					// TODO(user): Specify other spec details if needed.
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

	Context("When reconciling a BMv2 P4Target", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
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
					},
					Spec: corev1alpha1.P4TargetSpec{
						TargetClass: corev1alpha1.TARGET_BMV2,
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
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
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.

			By("Verifying the status of the reconciled resource")
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, reconciledResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciledResource.Status.Ready).To(BeFalse())
		})

		It("should be not ready when the associated pod exists but isn't ready", func() {
			By("Creating the namespace for the infrastructure resources")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: infrav1alpha1.BMV2_POD_NAMESPACE,
				},
			}
			k8sClient.Create(ctx, namespace)

			By("Creating the associated pod")
			pod := createBMv2Pod(resourceName, true)
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("Updating the pod status to NotReady")
			setPodReadyCondition(pod, false)
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.

			By("Verifying the status of the reconciled resource")
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, reconciledResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciledResource.Status.Ready).To(BeFalse())
		})

		It("should be ready when the associated pod is ready and no bindings exist for that target", func() {
			By("Creating the namespace for the infrastructure resources")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: infrav1alpha1.BMV2_POD_NAMESPACE,
				},
			}
			k8sClient.Create(ctx, namespace)

			By("Creating the associated pod in ready state")
			pod := createBMv2Pod(resourceName, true)
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("Updating the pod status to Running")
			setPodReadyCondition(pod, true)
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.

			By("Verifying the status of the reconciled resource")
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, reconciledResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciledResource.Status.Ready).To(BeTrue())
		})

		It("should be not ready when a binding already exists for that target", func() {
			By("Creating the namespace for the infrastructure resources")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: infrav1alpha1.BMV2_POD_NAMESPACE,
				},
			}
			k8sClient.Create(ctx, namespace)

			By("Creating the associated pod in ready state")
			pod := createBMv2Pod(resourceName, true)
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("Updating the pod status to Running")
			setPodReadyCondition(pod, true)
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			By("Creating a binding for that target")
			binding := &schedulingv1alpha1.NetworkFunctionBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "binding-for-" + resourceName,
					Namespace: "default",
					Labels: map[string]string{
						schedulingv1alpha1.TARGET_ASSIGNMENT_LABEL: resourceName,
					},
				},
				Spec: schedulingv1alpha1.NetworkFunctionBindingSpec{
					SupportedTargets: []string{corev1alpha1.TARGET_BMV2},
					P4File:           "https://example.com/p4example.p4",
				},
			}
			Expect(k8sClient.Create(ctx, binding)).To(Succeed())
			defer func() {
				By("Cleaning up the created binding")
				Expect(k8sClient.Delete(ctx, binding)).To(Succeed())
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
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.

			By("Verifying the status of the reconciled resource")
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, reconciledResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciledResource.Status.Ready).To(BeFalse())
			Expect(reconciledResource.Status.Phase).To(Equal(corev1alpha1.P4_TARGET_PHASE_OCCUPIED))
		})
	})
})

func createBMv2Pod(targetName string, ready bool) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetName + "-xyz",
			Namespace: infrav1alpha1.BMV2_POD_NAMESPACE,
			Labels: map[string]string{
				corev1alpha1.TARGET_LABEL: targetName,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "dummy",
					Image: "dummy-image",
				},
			},
		},
	}
}

func setPodReadyCondition(pod *corev1.Pod, ready bool) {
	if !ready {
		return
	}
	pod.Status.Phase = corev1.PodRunning
	pod.Status.Conditions = []corev1.PodCondition{
		{
			Type:   corev1.PodReady,
			Status: corev1.ConditionTrue,
		},
	}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			Name:    "dummy",
			Image:   "dummy-image",
			ImageID: "dummy-image:latest",
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{
					StartedAt: metav1.Now(),
				},
			},
			Ready: true,
		},
	}
}
