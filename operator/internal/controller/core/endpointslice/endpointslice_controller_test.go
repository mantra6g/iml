package endpointslice

import (
	"context"
	"time"

	netdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"github.com/mantra6g/iml/operator/pkg/util/loomservice"
)

const (
	timeout  = 10 * time.Second
	interval = 100 * time.Millisecond

	testNamespace = "default"
	ipv4SliceName = "web--default-ipv4"
	ipv6SliceName = "web--default-ipv6"
)

// sliceAddresses returns the addresses of the managed EndpointSlices keyed by slice name,
// with the ready condition of each address.
func sliceAddresses(ctx context.Context) func() (map[string]map[string]bool, error) {
	return func() (map[string]map[string]bool, error) {
		slices := &discoveryv1.EndpointSliceList{}
		if err := k8sClient.List(ctx, slices, ctrlclient.InNamespace(corev1alpha1.ServiceTargetNamespace),
			ctrlclient.MatchingLabels{discoveryv1.LabelManagedBy: corev1alpha1.EndpointSliceManagedBy}); err != nil {
			return nil, err
		}
		result := map[string]map[string]bool{}
		for _, slice := range slices.Items {
			result[slice.Name] = map[string]bool{}
			for _, endpoint := range slice.Endpoints {
				result[slice.Name][endpoint.Addresses[0]] = *endpoint.Conditions.Ready
			}
		}
		return result, nil
	}
}

func newPod(name string, podLabels map[string]string, networkStatus string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels:    podLabels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main", Image: "busybox"}},
		},
	}
	if networkStatus != "" {
		pod.Annotations = map[string]string{netdefv1.NetworkStatusAnnot: networkStatus}
	}
	return pod
}

func createPod(ctx context.Context, pod *corev1.Pod, ready bool) {
	Expect(k8sClient.Create(ctx, pod)).To(Succeed())
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	pod.Status.Phase = corev1.PodRunning
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: status}}
	Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
}

func relabelPod(ctx context.Context, pod *corev1.Pod, podLabels map[string]string) {
	Eventually(func() error {
		current := &corev1.Pod{}
		if err := k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(pod), current); err != nil {
			return err
		}
		current.Labels = podLabels
		return k8sClient.Update(ctx, current)
	}, timeout, interval).Should(Succeed())
}

var _ = Describe("EndpointSlice Controller", func() {
	Context("When reconciling a Service", func() {
		ctx := context.Background()
		webLabels := map[string]string{"app": "web"}
		otherLabels := map[string]string{"app": "other"}

		var service *corev1alpha1.Service
		var kubeService *corev1.Service
		var pods []*corev1.Pod

		BeforeEach(func() {
			service = &corev1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: testNamespace},
				Spec: corev1alpha1.ServiceSpec{
					Selector: webLabels,
					Ports:    []corev1alpha1.ServicePort{{Name: "http", Port: 8080}},
				},
			}
			// The Service controller isn't running in this suite, so create its output by hand.
			kubeService = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      loomservice.KubeServiceName(service.Namespace, service.Name),
					Namespace: corev1alpha1.ServiceTargetNamespace,
					Labels:    loomservice.OwnerLabels(service),
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Name: "http", Port: 8080}},
				},
			}
			pods = []*corev1.Pod{
				newPod("dual-stack", webLabels,
					`[{"name":"cbr0","ips":["10.0.0.1"]},{"name":"loom-cni","ips":["10.1.0.1","fd00::1"]}]`),
				newPod("namespaced-network", webLabels, `[{"name":"loom-system/loom-cni","ips":["fd00::2"]}]`),
				newPod("not-selected", otherLabels, `[{"name":"loom-cni","ips":["fd00::3"]}]`),
				newPod("other-network", webLabels, `[{"name":"cbr0","ips":["10.0.0.4"]}]`),
				newPod("no-annotation", webLabels, ""),
			}
		})

		AfterEach(func() {
			for _, pod := range pods {
				Expect(ctrlclient.IgnoreNotFound(k8sClient.Delete(ctx, pod,
					ctrlclient.GracePeriodSeconds(0)))).To(Succeed())
			}
			Expect(ctrlclient.IgnoreNotFound(k8sClient.Delete(ctx, service))).To(Succeed())
			Expect(ctrlclient.IgnoreNotFound(k8sClient.Delete(ctx, kubeService))).To(Succeed())
			// envtest has no garbage collector, so owned EndpointSlices must be removed by hand.
			Expect(k8sClient.DeleteAllOf(ctx, &discoveryv1.EndpointSlice{},
				ctrlclient.InNamespace(corev1alpha1.ServiceTargetNamespace))).To(Succeed())
		})

		It("should publish the loom-cni addresses of the selected pods", func() {
			Expect(k8sClient.Create(ctx, kubeService)).To(Succeed())
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
			createPod(ctx, pods[0], true)
			createPod(ctx, pods[1], false)
			for _, pod := range pods[2:] {
				createPod(ctx, pod, true)
			}

			By("verifying one EndpointSlice per address family")
			Eventually(sliceAddresses(ctx), timeout, interval).Should(Equal(map[string]map[string]bool{
				ipv4SliceName: {"10.1.0.1": true},
				ipv6SliceName: {"fd00::1": true, "fd00::2": false},
			}))

			slice := &discoveryv1.EndpointSlice{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{
				Namespace: corev1alpha1.ServiceTargetNamespace, Name: ipv6SliceName,
			}, slice)).To(Succeed())
			Expect(slice.AddressType).To(Equal(discoveryv1.AddressTypeIPv6))
			Expect(slice.Labels).To(HaveKeyWithValue(discoveryv1.LabelServiceName, kubeService.Name))
			Expect(loomservice.IsOwnedBy(slice.Labels, service)).To(BeTrue())
			Expect(metav1.IsControlledBy(slice, kubeService)).To(BeTrue())
			Expect(slice.Ports).To(HaveLen(1))
			Expect(*slice.Ports[0].Name).To(Equal("http"))
			Expect(*slice.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(*slice.Ports[0].Port).To(Equal(int32(8080)))
			Expect(slice.Endpoints[0].TargetRef).NotTo(BeNil())
			Expect(slice.Endpoints[0].TargetRef.Name).To(Equal("dual-stack"))
			Expect(*slice.Endpoints[1].Conditions.Serving).To(BeFalse())

			By("relabeling pods in and out of the selector")
			relabelPod(ctx, pods[0], otherLabels)
			relabelPod(ctx, pods[2], webLabels)
			Eventually(sliceAddresses(ctx), timeout, interval).Should(Equal(map[string]map[string]bool{
				ipv6SliceName: {"fd00::2": false, "fd00::3": true},
			}))

			By("deleting the selected pods")
			Expect(k8sClient.Delete(ctx, pods[1], ctrlclient.GracePeriodSeconds(0))).To(Succeed())
			Expect(k8sClient.Delete(ctx, pods[2], ctrlclient.GracePeriodSeconds(0))).To(Succeed())
			Eventually(sliceAddresses(ctx), timeout, interval).Should(BeEmpty())
		})

		It("should wait for the Kubernetes Service to exist", func() {
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
			createPod(ctx, pods[0], true)
			Consistently(sliceAddresses(ctx), time.Second, interval).Should(BeEmpty())

			Expect(k8sClient.Create(ctx, kubeService)).To(Succeed())
			Eventually(sliceAddresses(ctx), timeout, interval).Should(HaveLen(2))
		})

		It("should restore deleted EndpointSlices", func() {
			Expect(k8sClient.Create(ctx, kubeService)).To(Succeed())
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
			createPod(ctx, pods[0], true)
			Eventually(sliceAddresses(ctx), timeout, interval).Should(HaveLen(2))

			slice := &discoveryv1.EndpointSlice{}
			key := ctrlclient.ObjectKey{Namespace: corev1alpha1.ServiceTargetNamespace, Name: ipv4SliceName}
			Expect(k8sClient.Get(ctx, key, slice)).To(Succeed())
			oldUID := slice.UID
			Expect(k8sClient.Delete(ctx, slice)).To(Succeed())
			Eventually(func(g Gomega) {
				restored := &discoveryv1.EndpointSlice{}
				g.Expect(k8sClient.Get(ctx, key, restored)).To(Succeed())
				g.Expect(restored.UID).NotTo(Equal(oldUID))
			}, timeout, interval).Should(Succeed())
		})

		It("should ignore Kubernetes Services owned by another loom Service", func() {
			kubeService.Labels = map[string]string{
				corev1alpha1.ServiceNameLabel:      "someone-else",
				corev1alpha1.ServiceNamespaceLabel: testNamespace,
			}
			Expect(k8sClient.Create(ctx, kubeService)).To(Succeed())
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
			createPod(ctx, pods[0], true)
			Consistently(sliceAddresses(ctx), time.Second, interval).Should(BeEmpty())
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, ctrlclient.ObjectKey{
				Namespace: corev1alpha1.ServiceTargetNamespace, Name: ipv4SliceName,
			}, &discoveryv1.EndpointSlice{}))).To(BeTrue())
		})
	})
})
