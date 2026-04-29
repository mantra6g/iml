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
	"net/netip"
	"time"

	"github.com/mantra6g/iml/operator/pkg/util/ptr"
	"github.com/mantra6g/iml/operator/test/mocks"
	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	p4targetutil "github.com/mantra6g/iml/operator/internal/controller/core/p4target/util"
)

const (
	TargetArchitectureV1Model = "bmv2"
)

var _ = Describe("P4Target Controller", func() {
	Context("When creating a P4Target", func() {
		const targetName = "test-resource"
		const targetNamespace = "default"
		ctx := context.Background()
		p4target := &corev1alpha1.P4Target{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: targetNamespace,
			},
		}
		targetKey := ctrlclient.ObjectKeyFromObject(p4target)

		BeforeEach(func() {
			_ = k8sClient.Delete(ctx, p4target)
		})

		AfterEach(func() {})

		It("should successfully create a target of class bmv2", func() {
			By("creating the custom resource for the Kind P4Target")
			resource := &corev1alpha1.P4Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetKey.Name,
					Namespace: targetKey.Namespace,
					Labels: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureV1Model,
					},
				},
				Spec: corev1alpha1.P4TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Verifying the created resource")
			createdResource := &corev1alpha1.P4Target{}
			err := k8sClient.Get(ctx, targetKey, createdResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdResource.Labels).To(
				HaveKeyWithValue(corev1alpha1.P4TargetArchitectureLabel, TargetArchitectureV1Model))

			By("Cleanup the specific resource instance P4Target")
			Expect(k8sClient.Delete(ctx, createdResource)).To(Succeed())
		})

		It("should use a cluster-wide resource instead of a namespaced resource", func() {
			By("attempting to create a P4Target resource in a non-default namespace")
			resource := &corev1alpha1.P4Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetKey.Name,
					Namespace: targetKey.Namespace,
					Labels: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureV1Model,
					},
				},
				Spec: corev1alpha1.P4TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Verifying that the namespace of the created resource is empty")
			createdResource := &corev1alpha1.P4Target{}
			err := k8sClient.Get(ctx, targetKey, createdResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdResource.ObjectMeta.Namespace).To(Equal(""))

			By("Cleanup the specific resource instance P4Target")
			Expect(k8sClient.Delete(ctx, createdResource)).To(Succeed())
		})
	})

	Context("When reconciling a BMv2 P4Target", func() {
		const targetName = "test-resource"
		const targetNamespace = "default"
		const leaseNamespace = corev1alpha1.P4TargetLeaseNamespace
		ctx := context.Background()
		p4target := &corev1alpha1.P4Target{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: targetNamespace,
			},
		}
		targetKey := ctrlclient.ObjectKeyFromObject(p4target)
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: leaseNamespace,
			},
		}
		leaseKey := ctrlclient.ObjectKeyFromObject(lease)
		leaseNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: lease.Namespace}}
		fakeAllocator := mocks.NewFakePrefixAllocator(netip.MustParsePrefix("10.123.0.0/24"))

		BeforeEach(func() {
			_ = k8sClient.Delete(ctx, p4target)
			_ = k8sClient.Delete(ctx, lease)
			_ = k8sClient.Create(ctx, leaseNs)
		})

		AfterEach(func() {})

		It("should have unknown readiness when the associated lease is missing", func() {
			By("creating the custom resource for the Kind P4Target")
			resource := &corev1alpha1.P4Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetKey.Name,
					Namespace: targetKey.Namespace,
					Labels: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureV1Model,
					},
				},
				Spec: corev1alpha1.P4TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				CIDRAllocator: fakeAllocator,
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: targetKey,
			})
			Expect(err).NotTo(HaveOccurred())
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, targetKey, reconciledResource)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the status of the reconciled resource")
			readyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(readyCondition).To(Not(BeNil()))
			Expect(readyCondition.Status).To(Equal(metav1.ConditionUnknown))

			By("Verifying the taints of the reconciled resource")
			unreachableTaint := p4targetutil.GetTaint(reconciledResource.Spec.Taints, corev1alpha1.TaintP4TargetUnreachable)
			Expect(unreachableTaint).To(Not(BeNil()))
			Expect(unreachableTaint.Effect).To(Equal(corev1alpha1.TaintEffectNoSchedule))
		})

		It("should have unknown readiness when the associated lease is expired", func() {
			By("creating the custom resource for the Kind P4Target")
			resource := &corev1alpha1.P4Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetKey.Name,
					Namespace: targetKey.Namespace,
					Labels: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureV1Model,
					},
				},
				Spec: corev1alpha1.P4TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Creating an expired lease for the target")
			lease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      leaseKey.Name,
					Namespace: leaseKey.Namespace,
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

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				CIDRAllocator: fakeAllocator,
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: targetKey,
			})
			Expect(err).NotTo(HaveOccurred())
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, targetKey, reconciledResource)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the status of the reconciled resource")
			readyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(readyCondition).To(Not(BeNil()))
			Expect(readyCondition.Status).To(Equal(metav1.ConditionUnknown))

			By("Verifying the taints of the reconciled resource")
			unreachableTaint := p4targetutil.GetTaint(reconciledResource.Spec.Taints, corev1alpha1.TaintP4TargetUnreachable)
			Expect(unreachableTaint).To(Not(BeNil()))
			Expect(unreachableTaint.Effect).To(Equal(corev1alpha1.TaintEffectNoSchedule))
		})

		It("should have unknown readiness when the lease is okay but no preexisting readiness exists", func() {
			By("creating the custom resource for the Kind P4Target")
			resource := &corev1alpha1.P4Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetKey.Name,
					Namespace: targetKey.Namespace,
					Labels: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureV1Model,
					},
				},
				Spec: corev1alpha1.P4TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Creating an active lease for the target")
			lease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      leaseKey.Name,
					Namespace: leaseKey.Namespace,
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

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				CIDRAllocator: fakeAllocator,
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: targetKey,
			})
			Expect(err).NotTo(HaveOccurred())
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, targetKey, reconciledResource)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the status of the reconciled resource")
			readyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(readyCondition).To(Not(BeNil()))
			Expect(readyCondition.Status).To(Equal(metav1.ConditionUnknown))

			By("Verifying the taints of the reconciled resource")
			unreachableTaint := p4targetutil.GetTaint(reconciledResource.Spec.Taints, corev1alpha1.TaintP4TargetUnreachable)
			Expect(unreachableTaint).To(Not(BeNil()))
			Expect(unreachableTaint.Effect).To(Equal(corev1alpha1.TaintEffectNoSchedule))
		})

		It("should be ready when the lease is okay and the last readiness was okay", func() {
			By("creating the custom resource for the Kind P4Target")
			resource := &corev1alpha1.P4Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetKey.Name,
					Namespace: targetKey.Namespace,
					Labels: map[string]string{
						corev1alpha1.P4TargetArchitectureLabel: TargetArchitectureV1Model,
					},
				},
				Spec: corev1alpha1.P4TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Creating an active lease for the target")
			lease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      leaseKey.Name,
					Namespace: leaseKey.Namespace,
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
			p4target := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, targetKey, p4target)
			Expect(err).NotTo(HaveOccurred())

			By("Setting the last readiness condition to true")
			original := p4target.DeepCopy()
			newReadyCondition := p4targetutil.NewReadyCondition(
				metav1.ConditionTrue, "InitialReady", "The P4Target is initially ready.")
			p4target.Status.Conditions, _ = p4targetutil.AddConditions(p4target.Status.Conditions, newReadyCondition)
			Expect(k8sClient.Status().Patch(ctx, p4target, ctrlclient.MergeFrom(original))).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &P4TargetReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				CIDRAllocator: fakeAllocator,
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: targetKey,
			})
			Expect(err).NotTo(HaveOccurred())
			reconciledResource := &corev1alpha1.P4Target{}
			err = k8sClient.Get(ctx, targetKey, reconciledResource)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the status of the reconciled resource")
			reconciledReadyCondition := p4targetutil.GetReadyCondition(reconciledResource)
			Expect(reconciledReadyCondition).To(Not(BeNil()))
			Expect(reconciledReadyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(reconciledResource.Spec.Taints).To(BeEmpty())
		})

	})
})
