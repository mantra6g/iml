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
	"loom/pkg/util/ptr"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "loom/api/core/v1alpha1"
	infrav1alpha1 "loom/api/infra/v1alpha1"
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

	Context("When reconciling a BMv2 P4Target", func() {
		const resourceName = "test-resource"

		ctx := context.Background()
		logger := GinkgoLogr.WithName("P4TargetReconcilerTest")

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
		})

		It("should have unknown readiness when the associated lease is missing", func() {
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
			logger.Info("Reconciled resource", "resource", reconciledResource)
			readyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(readyCondition).To(Not(BeNil()))
			Expect(readyCondition.Status).To(Equal(metav1.ConditionUnknown))
			unreachableTaint := p4targetutil.GetTaint(reconciledResource.Spec.Taints, corev1alpha1.TaintP4TargetUnreachable)
			Expect(unreachableTaint).To(Not(BeNil()))
			Expect(unreachableTaint.Effect).To(Equal(corev1alpha1.TaintEffectNoSchedule))
		})

		It("should have unknown readiness when the associated lease is expired", func() {
			By("Creating the namespace for the infrastructure resources")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: corev1alpha1.P4TargetLeaseNamespace,
				},
			}
			_ = k8sClient.Create(ctx, namespace) // Remember to ignore error in case the namespace already exists

			By("Creating an expired lease for the target")
			lease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: corev1alpha1.P4TargetLeaseNamespace,
				},
				Spec: coordinationv1.LeaseSpec{
					LeaseDurationSeconds: ptr.To[int32](30),
					RenewTime: &metav1.MicroTime{
						Time: metav1.Now().Add(-time.Minute), // Set acquire time in the past to make it expired
					},
				},
			}
			err := k8sClient.Create(ctx, lease)
			Expect(err).NotTo(HaveOccurred())

			defer func() {
				By("Cleaning up the created lease")
				err := k8sClient.Delete(ctx, lease)
				Expect(err).NotTo(HaveOccurred())
			}()

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the status of the reconciled resource")
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, reconciledResource)
			Expect(err).NotTo(HaveOccurred())
			readyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(readyCondition).To(Not(BeNil()))
			Expect(readyCondition.Status).To(Equal(metav1.ConditionUnknown))
			unreachableTaint := p4targetutil.GetTaint(reconciledResource.Spec.Taints, corev1alpha1.TaintP4TargetUnreachable)
			Expect(unreachableTaint).To(Not(BeNil()))
			Expect(unreachableTaint.Effect).To(Equal(corev1alpha1.TaintEffectNoSchedule))
		})

		It("should have unknown readiness when the lease is okay but no preexisting readiness exists", func() {
			By("Creating the namespace for the infrastructure resources")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: infrav1alpha1.BMV2_POD_NAMESPACE,
				},
			}
			_ = k8sClient.Create(ctx, namespace)

			By("Creating an active lease for the target")
			lease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: corev1alpha1.P4TargetLeaseNamespace,
				},
				Spec: coordinationv1.LeaseSpec{
					LeaseDurationSeconds: ptr.To[int32](120),
					RenewTime: &metav1.MicroTime{
						Time: metav1.Now().Add(-time.Minute), // Set acquire time in the past to make it expired
					},
				},
			}
			err := k8sClient.Create(ctx, lease)
			Expect(err).NotTo(HaveOccurred())

			defer func() {
				By("Cleaning up the created lease")
				err := k8sClient.Delete(ctx, lease)
				Expect(err).NotTo(HaveOccurred())
			}()

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the status of the reconciled resource")
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, reconciledResource)
			Expect(err).NotTo(HaveOccurred())
			readyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(readyCondition).To(Not(BeNil()))
			Expect(readyCondition.Status).To(Equal(metav1.ConditionUnknown))
			unreachableTaint := p4targetutil.GetTaint(reconciledResource.Spec.Taints, corev1alpha1.TaintP4TargetUnreachable)
			Expect(unreachableTaint).To(Not(BeNil()))
			Expect(unreachableTaint.Effect).To(Equal(corev1alpha1.TaintEffectNoSchedule))
		})

		It("should be ready when the lease is okay and the last readiness was okay", func() {
			By("Creating the namespace for the infrastructure resources")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: infrav1alpha1.BMV2_POD_NAMESPACE,
				},
			}
			_ = k8sClient.Create(ctx, namespace)

			By("Creating an active lease for the target")
			lease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: corev1alpha1.P4TargetLeaseNamespace,
				},
				Spec: coordinationv1.LeaseSpec{
					LeaseDurationSeconds: ptr.To[int32](120),
					RenewTime: &metav1.MicroTime{
						Time: metav1.Now().Add(-time.Minute), // Set acquire time in the past to make it expired
					},
				},
			}
			err := k8sClient.Create(ctx, lease)
			Expect(err).NotTo(HaveOccurred())

			defer func() {
				By("Cleaning up the created lease")
				err := k8sClient.Delete(ctx, lease)
				Expect(err).NotTo(HaveOccurred())
			}()

			By("Setting the last readiness condition to true")
			p4target := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, p4target)
			Expect(err).NotTo(HaveOccurred())
			original := p4target.DeepCopy()
			newReadyCondition := p4targetutil.NewReadyCondition(
				metav1.ConditionTrue, "InitialReady", "The P4Target is initially ready.")
			p4target.Status.Conditions, _ = p4targetutil.AddConditions(p4target.Status.Conditions, newReadyCondition)
			Expect(k8sClient.Status().Patch(ctx, p4target, ctrlclient.MergeFrom(original))).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the status of the reconciled resource")
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, reconciledResource)
			Expect(err).NotTo(HaveOccurred())
			reconciledReadyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(reconciledReadyCondition).To(Not(BeNil()))
			Expect(reconciledReadyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(reconciledResource.Spec.Taints).To(BeEmpty())
		})

	})
})
