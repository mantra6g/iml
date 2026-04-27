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

package nfgc

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type failingListClient struct {
	ctrlclient.Client
	failListFor any
	err         error
}

func (c *failingListClient) List(ctx context.Context, list ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
	switch c.failListFor.(type) {
	case *corev1alpha1.P4TargetList:
		if _, ok := list.(*corev1alpha1.P4TargetList); ok {
			return c.err
		}
	case *corev1alpha1.NetworkFunctionList:
		if _, ok := list.(*corev1alpha1.NetworkFunctionList); ok {
			return c.err
		}
	}
	return c.Client.List(ctx, list, opts...)
}

func newNetworkFunction(name string, phase corev1alpha1.NetworkFunctionPhase, targetName string) *corev1alpha1.NetworkFunction {
	return &corev1alpha1.NetworkFunction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: corev1alpha1.NetworkFunctionSpec{
			TargetName: targetName,
			P4File:     "dummy.p4",
		},
		Status: corev1alpha1.NetworkFunctionStatus{
			Phase: phase,
		},
	}
}

func createNFWithStatus(nf *corev1alpha1.NetworkFunction) {
	nfStatus := nf.Status
	Expect(k8sClient.Create(ctx, nf)).To(Succeed()) // This will delete the original status
	nf.Status = nfStatus                            // restore it here
	Eventually(func() error {
		created := &corev1alpha1.NetworkFunction{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: nf.Name, Namespace: nf.Namespace}, created); err != nil {
			return err
		}
		created.Status = nfStatus
		return k8sClient.Status().Update(ctx, created)
	}).Should(Succeed())
}

func createTarget(name string, ready bool, outOfService bool) *corev1alpha1.P4Target {
	conditionStatus := metav1.ConditionFalse
	if ready {
		conditionStatus = metav1.ConditionTrue
	}
	target := &corev1alpha1.P4Target{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1alpha1.P4TargetSpec{},
	}
	targetStatus := corev1alpha1.P4TargetStatus{
		Conditions: []corev1alpha1.P4TargetCondition{{
			Type:   corev1alpha1.P4TargetConditionReady,
			Status: conditionStatus,
		}},
	}
	if outOfService {
		target.Spec.Taints = []corev1alpha1.Taint{{
			Key:    corev1alpha1.TaintP4TargetOutOfService,
			Effect: corev1alpha1.TaintEffectNoExecute,
		}}
	}
	Expect(k8sClient.Create(ctx, target)).To(Succeed())
	created := &corev1alpha1.P4Target{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: target.Name}, created)).To(Succeed())
	created.Status = targetStatus
	Expect(k8sClient.Status().Update(ctx, created)).To(Succeed())
	return created
}

func forceCleanupNF(name string) {
	nf := &corev1alpha1.NetworkFunction{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nf)
	if err != nil {
		return
	}
	if len(nf.Finalizers) > 0 {
		nf.Finalizers = nil
		_ = k8sClient.Update(ctx, nf)
	}
	_ = k8sClient.Delete(ctx, nf)
}

func cleanupTarget(name string) {
	target := &corev1alpha1.P4Target{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, target)
	if err != nil {
		return
	}
	_ = k8sClient.Delete(ctx, target)
}

var _ = Describe("NetworkFunctionGC Controller", func() {
	var (
		reconciler  *ClusterWideReconciler
		cleanupNFs  []string
		cleanupP4Ts []string
	)

	newName := func(prefix string) string {
		time.Sleep(5 * time.Millisecond)
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}

	BeforeEach(func() {
		reconciler = &ClusterWideReconciler{
			Client:                k8sClient,
			Scheme:                scheme.Scheme,
			TerminatedNFThreshold: 1,
			QuarantineTime:        0,
			targetQueue: workqueue.NewTypedDelayingQueueWithConfig(
				workqueue.TypedDelayingQueueConfig[string]{Name: "nfgc-test"},
			),
		}
		cleanupNFs = nil
		cleanupP4Ts = nil
	})

	AfterEach(func() {
		if reconciler != nil && reconciler.targetQueue != nil {
			reconciler.targetQueue.ShutDown()
		}
		for _, name := range cleanupNFs {
			forceCleanupNF(name)
		}
		for _, name := range cleanupP4Ts {
			cleanupTarget(name)
		}
	})

	It("deletes terminated nfs above threshold", func() {
		nf1 := newNetworkFunction(newName("terminated"), corev1alpha1.NetworkFunctionFailed, "")
		nf2 := newNetworkFunction(newName("terminated"), corev1alpha1.NetworkFunctionFailed, "")
		nf3 := newNetworkFunction(newName("terminated"), corev1alpha1.NetworkFunctionFailed, "")
		cleanupNFs = append(cleanupNFs, nf1.Name, nf2.Name, nf3.Name)

		createNFWithStatus(nf1)
		time.Sleep(5 * time.Millisecond)
		createNFWithStatus(nf2)
		time.Sleep(5 * time.Millisecond)
		createNFWithStatus(nf3)

		allNFs, err := reconciler.listAllNetworkFunctions(ctx)
		Expect(err).NotTo(HaveOccurred())

		reconciler.gcTerminated(ctx, allNFs)

		Eventually(func() int {
			nfList := &corev1alpha1.NetworkFunctionList{}
			if err := k8sClient.List(ctx, nfList); err != nil {
				return -1
			}
			count := 0
			for _, item := range nfList.Items {
				if item.Namespace == "default" && (item.Name == nf1.Name || item.Name == nf2.Name || item.Name == nf3.Name) {
					count++
				}
			}
			return count
		}).Should(Equal(1))
	})

	It("marks and deletes terminating nfs on out-of-service targets", func() {
		target := createTarget(newName("target"), false, true)
		cleanupP4Ts = append(cleanupP4Ts, target.Name)

		nf := newNetworkFunction(newName("terminating"), corev1alpha1.NetworkFunctionRunning, target.Name)
		nf.Finalizers = []string{corev1alpha1.NetworkFunctionFinalizer}
		cleanupNFs = append(cleanupNFs, nf.Name)
		createNFWithStatus(nf)

		Expect(k8sClient.Delete(ctx, nf)).To(Succeed())
		Eventually(func() bool {
			current := &corev1alpha1.NetworkFunction{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: nf.Name, Namespace: nf.Namespace}, current)
			if err != nil {
				return false
			}
			return current.DeletionTimestamp != nil
		}).Should(BeTrue())

		allNFs, err := reconciler.listAllNetworkFunctions(ctx)
		Expect(err).NotTo(HaveOccurred())
		reconciler.gcTerminating(ctx, allNFs)

		Eventually(func() corev1alpha1.NetworkFunctionPhase {
			current := &corev1alpha1.NetworkFunction{}
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: nf.Name, Namespace: nf.Namespace}, current)
			return current.Status.Phase
		}).Should(Equal(corev1alpha1.NetworkFunctionFailed))
	})

	It("does not delete terminating nfs when target is still ready", func() {
		target := createTarget(newName("target-ready"), true, true)
		cleanupP4Ts = append(cleanupP4Ts, target.Name)
		nf := newNetworkFunction(newName("skip-terminating"), corev1alpha1.NetworkFunctionRunning, target.Name)
		nf.Finalizers = []string{corev1alpha1.NetworkFunctionFinalizer}
		cleanupNFs = append(cleanupNFs, nf.Name)
		createNFWithStatus(nf)
		Expect(k8sClient.Delete(ctx, nf)).To(Succeed())

		Eventually(func() bool {
			current := &corev1alpha1.NetworkFunction{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nf.Name, Namespace: nf.Namespace}, current); err != nil {
				return false
			}
			return current.DeletionTimestamp != nil
		}).Should(BeTrue())

		allNFs, err := reconciler.listAllNetworkFunctions(ctx)
		Expect(err).NotTo(HaveOccurred())
		reconciler.gcTerminating(ctx, allNFs)

		Consistently(func() corev1alpha1.NetworkFunctionPhase {
			current := &corev1alpha1.NetworkFunction{}
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: nf.Name, Namespace: nf.Namespace}, current)
			return current.Status.Phase
		}, 300*time.Millisecond, 50*time.Millisecond).Should(Equal(corev1alpha1.NetworkFunctionRunning))
	})

	It("marks orphaned nfs with disruption condition before deleting", func() {
		nf := newNetworkFunction(newName("orphaned"), corev1alpha1.NetworkFunctionRunning, "missing-target")
		nf.Finalizers = []string{corev1alpha1.NetworkFunctionFinalizer}
		cleanupNFs = append(cleanupNFs, nf.Name)
		createNFWithStatus(nf)

		allNFs, err := reconciler.listAllNetworkFunctions(ctx)
		Expect(err).NotTo(HaveOccurred())
		reconciler.gcOrphaned(ctx, allNFs, nil)

		Eventually(func() corev1alpha1.NetworkFunctionPhase {
			current := &corev1alpha1.NetworkFunction{}
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: nf.Name, Namespace: nf.Namespace}, current)
			return current.Status.Phase
		}).Should(Equal(corev1alpha1.NetworkFunctionFailed))

		Eventually(func() string {
			current := &corev1alpha1.NetworkFunction{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nf.Name, Namespace: nf.Namespace}, current); err != nil {
				return ""
			}
			for i := range current.Status.Conditions {
				if current.Status.Conditions[i].Type == corev1alpha1.DisruptionTarget {
					return current.Status.Conditions[i].Reason + "|" + current.Status.Conditions[i].Message
				}
			}
			return ""
		}).Should(Equal("DeletionByNetworkFunctionGC|NFGC: target no longer exists"))
	})

	It("marks unscheduled terminating nfs as failed", func() {
		nf := newNetworkFunction(newName("unscheduled-terminating"), corev1alpha1.NetworkFunctionRunning, "")
		nf.Finalizers = []string{corev1alpha1.NetworkFunctionFinalizer}
		cleanupNFs = append(cleanupNFs, nf.Name)
		createNFWithStatus(nf)
		Expect(k8sClient.Delete(ctx, nf)).To(Succeed())

		Eventually(func() bool {
			current := &corev1alpha1.NetworkFunction{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nf.Name, Namespace: nf.Namespace}, current); err != nil {
				return false
			}
			return current.DeletionTimestamp != nil
		}).Should(BeTrue())

		allNFs, err := reconciler.listAllNetworkFunctions(ctx)
		Expect(err).NotTo(HaveOccurred())
		reconciler.gcUnscheduledTerminating(ctx, allNFs)

		Eventually(func() corev1alpha1.NetworkFunctionPhase {
			current := &corev1alpha1.NetworkFunction{}
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: nf.Name, Namespace: nf.Namespace}, current)
			return current.Status.Phase
		}).Should(Equal(corev1alpha1.NetworkFunctionFailed))
	})

	It("returns early when listing targets fails during reconcile", func() {
		nf1 := newNetworkFunction(newName("reconcile-terminated"), corev1alpha1.NetworkFunctionFailed, "")
		nf2 := newNetworkFunction(newName("reconcile-terminated"), corev1alpha1.NetworkFunctionFailed, "")
		cleanupNFs = append(cleanupNFs, nf1.Name, nf2.Name)
		createNFWithStatus(nf1)
		createNFWithStatus(nf2)

		reconciler.Client = &failingListClient{
			Client:      k8sClient,
			failListFor: &corev1alpha1.P4TargetList{},
			err:         errors.New("p4target list failed"),
		}

		reconciler.Reconcile(ctx)

		Consistently(func() int {
			nfList := &corev1alpha1.NetworkFunctionList{}
			if err := k8sClient.List(ctx, nfList); err != nil {
				return -1
			}
			count := 0
			for _, item := range nfList.Items {
				if item.Namespace == "default" && (item.Name == nf1.Name || item.Name == nf2.Name) {
					count++
				}
			}
			return count
		}, 300*time.Millisecond, 50*time.Millisecond).Should(Equal(2))
	})
})
