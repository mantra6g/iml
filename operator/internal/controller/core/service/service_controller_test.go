package service

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"github.com/mantra6g/iml/operator/pkg/util/loomservice"
)

const (
	timeout  = 10 * time.Second
	interval = 100 * time.Millisecond
)

func newService(name, namespace string) *corev1alpha1.Service {
	return &corev1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1alpha1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports:    []corev1alpha1.ServicePort{{Name: "http", Port: 8080}},
		},
	}
}

func kubeServiceKey(service *corev1alpha1.Service) ctrlclient.ObjectKey {
	return ctrlclient.ObjectKey{
		Namespace: corev1alpha1.ServiceTargetNamespace,
		Name:      loomservice.KubeServiceName(service.Namespace, service.Name),
	}
}

// deleteService deletes a Service and waits until its finalizer has been handled.
func deleteService(ctx context.Context, service *corev1alpha1.Service) {
	err := k8sClient.Delete(ctx, service)
	if apierrors.IsNotFound(err) {
		return
	}
	Expect(err).NotTo(HaveOccurred())
	Eventually(func() bool {
		return apierrors.IsNotFound(k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(service),
			&corev1alpha1.Service{}))
	}, timeout, interval).Should(BeTrue())
}

func readyCondition(ctx context.Context, service *corev1alpha1.Service) func() *metav1.Condition {
	return func() *metav1.Condition {
		current := &corev1alpha1.Service{}
		if err := k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(service), current); err != nil {
			return nil
		}
		return meta.FindStatusCondition(current.Status.Conditions, corev1alpha1.ServiceConditionReady)
	}
}

var _ = Describe("Service Controller", func() {
	const namespace = "default"
	ctx := context.Background()

	Context("When creating a Service", func() {
		var service *corev1alpha1.Service

		BeforeEach(func() {
			service = newService("web", namespace)
		})

		AfterEach(func() {
			deleteService(ctx, service)
		})

		It("should create a selectorless ClusterIP Service in loom-system", func() {
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
			Expect(service.Spec.Type).To(Equal(corev1alpha1.ServiceTypeClusterIP), "type should be defaulted")
			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP), "protocol should be defaulted")

			By("verifying the generated Kubernetes Service")
			kubeService := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, kubeServiceKey(service), kubeService)
			}, timeout, interval).Should(Succeed())
			Expect(kubeService.Name).To(Equal("web--default"))
			Expect(kubeService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(kubeService.Spec.ClusterIP).NotTo(BeEmpty())
			Expect(kubeService.Spec.ClusterIP).NotTo(Equal(corev1.ClusterIPNone))
			Expect(kubeService.Spec.Selector).To(BeEmpty())
			Expect(kubeService.Spec.Ports).To(HaveLen(1))
			Expect(kubeService.Spec.Ports[0].Name).To(Equal("http"))
			Expect(kubeService.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(kubeService.Spec.Ports[0].Port).To(Equal(int32(8080)))
			Expect(kubeService.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt32(8080)))
			Expect(loomservice.IsOwnedBy(kubeService.Labels, service)).To(BeTrue())

			By("verifying the Service status and finalizer")
			Eventually(func(g Gomega) {
				current := &corev1alpha1.Service{}
				g.Expect(k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(service), current)).To(Succeed())
				g.Expect(current.Finalizers).To(ContainElement(corev1alpha1.ServiceFinalizer))
				g.Expect(current.Status.ServiceName).To(Equal("web--default"))
				g.Expect(current.Status.Hostname).To(Equal("web--default.loom-system.svc.cluster.local"))
				g.Expect(current.Status.ClusterIPs).To(Equal(kubeService.Spec.ClusterIPs))
				cond := meta.FindStatusCondition(current.Status.Conditions, corev1alpha1.ServiceConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())
		})

		It("should keep the Kubernetes Service in sync", func() {
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, kubeServiceKey(service), &corev1.Service{})
			}, timeout, interval).Should(Succeed())

			By("updating the Service ports")
			Eventually(func() error {
				current := &corev1alpha1.Service{}
				if err := k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(service), current); err != nil {
					return err
				}
				current.Spec.Ports = append(current.Spec.Ports,
					corev1alpha1.ServicePort{Name: "dns", Protocol: corev1.ProtocolUDP, Port: 53})
				return k8sClient.Update(ctx, current)
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				kubeService := &corev1.Service{}
				g.Expect(k8sClient.Get(ctx, kubeServiceKey(service), kubeService)).To(Succeed())
				g.Expect(kubeService.Spec.Ports).To(HaveLen(2))
			}, timeout, interval).Should(Succeed())

			By("deleting the Kubernetes Service behind the controller's back")
			kubeService := &corev1.Service{}
			Expect(k8sClient.Get(ctx, kubeServiceKey(service), kubeService)).To(Succeed())
			oldUID := kubeService.UID
			Expect(k8sClient.Delete(ctx, kubeService)).To(Succeed())
			Eventually(func(g Gomega) {
				recreated := &corev1.Service{}
				g.Expect(k8sClient.Get(ctx, kubeServiceKey(service), recreated)).To(Succeed())
				g.Expect(recreated.UID).NotTo(Equal(oldUID))
			}, timeout, interval).Should(Succeed())
		})

		It("should delete the Kubernetes Service when the Service is deleted", func() {
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, kubeServiceKey(service), &corev1.Service{})
			}, timeout, interval).Should(Succeed())

			deleteService(ctx, service)
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, kubeServiceKey(service), &corev1.Service{}))
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When validating a Service", func() {
		It("should reject unsupported types", func() {
			for _, serviceType := range []corev1alpha1.ServiceType{"NodePort", "LoadBalancer"} {
				service := newService("unsupported", namespace)
				service.Spec.Type = serviceType
				err := k8sClient.Create(ctx, service)
				Expect(apierrors.IsInvalid(err)).To(BeTrue(), "type %s should be rejected, got %v", serviceType, err)
			}
		})

		It("should reject names that are not DNS-1035 labels", func() {
			for _, name := range []string{"my.service", "1service"} {
				err := k8sClient.Create(ctx, newService(name, namespace))
				Expect(apierrors.IsInvalid(err)).To(BeTrue(), "name %s should be rejected, got %v", name, err)
			}
		})

		It("should reject Services without ports", func() {
			service := newService("noports", namespace)
			service.Spec.Ports = nil
			Expect(apierrors.IsInvalid(k8sClient.Create(ctx, service))).To(BeTrue())
		})
	})

	Context("When the generated name is too long", func() {
		longNamespace := strings.Repeat("n", 40)
		service := newService(strings.Repeat("s", 40), longNamespace)

		BeforeEach(func() {
			err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: longNamespace}})
			if !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		})

		AfterEach(func() {
			deleteService(ctx, service)
		})

		It("should use a hashed name that fits in a DNS label", func() {
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
			Eventually(func(g Gomega) {
				current := &corev1alpha1.Service{}
				g.Expect(k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(service), current)).To(Succeed())
				g.Expect(current.Status.ServiceName).To(Equal(kubeServiceKey(service).Name))
				g.Expect(validation.IsDNS1035Label(current.Status.ServiceName)).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
			Expect(k8sClient.Get(ctx, kubeServiceKey(service), &corev1.Service{})).To(Succeed())
		})
	})

	Context("When the generated name is already taken", func() {
		service := newService("taken", namespace)
		foreign := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "taken--default",
				Namespace: corev1alpha1.ServiceTargetNamespace,
				Labels: map[string]string{
					corev1alpha1.ServiceNameLabel:      "someone-else",
					corev1alpha1.ServiceNamespaceLabel: namespace,
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Name: "other", Port: 9090}},
			},
		}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, foreign)).To(Succeed())
		})

		AfterEach(func() {
			deleteService(ctx, service)
			Expect(ctrlclient.IgnoreNotFound(k8sClient.Delete(ctx, foreign))).To(Succeed())
		})

		It("should report a conflict and leave the existing Service alone", func() {
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
			Eventually(readyCondition(ctx, service), timeout, interval).Should(And(
				Not(BeNil()),
				HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "NameConflict"),
			))

			existing := &corev1.Service{}
			Expect(k8sClient.Get(ctx, kubeServiceKey(service), existing)).To(Succeed())
			Expect(existing.Labels[corev1alpha1.ServiceNameLabel]).To(Equal("someone-else"))
			Expect(existing.Spec.Ports[0].Name).To(Equal("other"))

			By("deleting the Service without touching the foreign one")
			deleteService(ctx, service)
			Consistently(func() error {
				return k8sClient.Get(ctx, kubeServiceKey(service), &corev1.Service{})
			}, time.Second, interval).Should(Succeed())
		})
	})
})
