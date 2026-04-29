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

package bmv2target

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"
)

var _ = Describe("BMv2Target Controller", func() {
	Context("When creating a BMv2Target", func() {
		const resourceName = "test-resource"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		AfterEach(func() {
			// Cleanup the specific resource instance BMv2Target
			resource := &infrav1alpha1.BMv2Target{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if errors.IsNotFound(err) {
				return
			}

			By("Cleaning up the specific resource instance BMv2Target")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully create a resource with all required fields", func() {
			By("Creating the custom resource for the Kind BMv2Target")
			resource := &infrav1alpha1.BMv2Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: infrav1alpha1.BMv2TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		It("should succeed to create a resource when replicas are non-nil", func() {
			By("Creating the custom resource for the Kind BMv2Target with unknown class")
			resource := &infrav1alpha1.BMv2Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: infrav1alpha1.BMv2TargetSpec{},
			}
			err := k8sClient.Create(ctx, resource)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		infraNamespace := types.NamespacedName{
			Name: infrav1alpha1.BMV2_POD_NAMESPACE,
		}
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: infrav1alpha1.BMV2_POD_NAMESPACE,
		}

		BeforeEach(func() {
			By("making sure the namespace for the infrastructure resources exists")
			namespace := &corev1.Namespace{}
			err := k8sClient.Get(ctx, infraNamespace, namespace)
			if err != nil {
				if !errors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())
				}
				namespace = &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: infrav1alpha1.BMV2_POD_NAMESPACE,
					},
				}
				Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &infrav1alpha1.BMv2Target{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				return
			}
			Expect(err).ToNot(HaveOccurred())

			By("Cleanup the specific resource instance BMv2Target")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Cleaning up the replicas Deployments and P4Targets")
			err = k8sClient.DeleteAllOf(ctx, &appsv1.Deployment{},
				client.InNamespace(infraNamespace.Name),
				client.MatchingLabels{infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: resourceName},
			)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.DeleteAllOf(ctx, &corev1alpha1.P4Target{},
				client.InNamespace(infraNamespace.Name),
				client.MatchingLabels{infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: resourceName},
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should successfully reconcile the resource", func() {
			By("creating the custom resource for the Kind BMv2Target")
			resource := &infrav1alpha1.BMv2Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: infrav1alpha1.BMv2TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &BMv2TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle reconciliation when the resource is not found", func() {
			By("Reconciling a non-existing resource")
			controllerReconciler := &BMv2TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existing-resource",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should default to 1 replica when replicas is nil", func() {
			By("creating the custom resource for the Kind BMv2Target with nil replicas")
			resource := &infrav1alpha1.BMv2Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: infrav1alpha1.BMv2TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &BMv2TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that 1 Deployment is created by default")
			deploymentList := &appsv1.DeploymentList{}
			err = k8sClient.List(ctx, deploymentList,
				client.InNamespace(infraNamespace.Name),
				client.MatchingLabels{infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: resourceName},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentList.Items).To(HaveLen(1))

			By("Verifying that 1 P4Target is created by default")
			p4TargetList := &corev1alpha1.P4TargetList{}
			err = k8sClient.List(ctx, p4TargetList,
				client.InNamespace(infraNamespace.Name),
				client.MatchingLabels{infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: resourceName},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(p4TargetList.Items).To(HaveLen(1))
		})

		It("should successfully set ownership of both P4Target and appsv1.Deployment to itself", func() {
			By("creating the custom resource for the Kind BMv2Target")
			resource := &infrav1alpha1.BMv2Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: infrav1alpha1.BMv2TargetSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &BMv2TargetReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that the BMv2Target resource exists")
			retrievedResource := &infrav1alpha1.BMv2Target{}
			err = k8sClient.Get(ctx, typeNamespacedName, retrievedResource)
			Expect(err).ToNot(HaveOccurred())

			By("Retrieving the created P4Target and appsv1.Deployment")
			p4TargetList := &corev1alpha1.P4TargetList{}
			err = k8sClient.List(ctx, p4TargetList,
				client.InNamespace(infraNamespace.Name),
				client.MatchingLabels{infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: resourceName},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(p4TargetList.Items).To(HaveLen(1))
			p4Target := &p4TargetList.Items[0]

			deploymentList := &appsv1.DeploymentList{}
			err = k8sClient.List(ctx, deploymentList,
				client.InNamespace(infraNamespace.Name),
				client.MatchingLabels{infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: resourceName},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentList.Items).To(HaveLen(1))
			deployment := &deploymentList.Items[0]

			By("Verifying that the P4Target's owner reference is set to itself")
			controllerBool := true
			blockOwnerDeletionBool := true
			bmv2TargetOwnerReference := metav1.OwnerReference{
				Kind:               "BMv2Target",
				APIVersion:         infrav1alpha1.GroupVersion.String(),
				UID:                retrievedResource.UID,
				Name:               retrievedResource.Name,
				Controller:         &controllerBool,
				BlockOwnerDeletion: &blockOwnerDeletionBool,
			}
			Expect(p4Target.OwnerReferences).To(HaveLen(1))
			Expect(p4Target.ObjectMeta.OwnerReferences).To(ContainElement(bmv2TargetOwnerReference))

			By("Verifying that the appsv1.Deployment's owner reference is set to the BMv2Target")
			Expect(deployment.OwnerReferences).To(HaveLen(1))
			Expect(deployment.ObjectMeta.OwnerReferences).To(ContainElement(bmv2TargetOwnerReference))
		})

		It("should update deployments and targets whenever template spec changes", func() {
			// So far the spec is just TargetClass for which only BMV2 is supported.
			// This test is a placeholder for future spec changes.
		})
	})
})
