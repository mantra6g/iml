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

package reconcilers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"strconv"

	netdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	DummyPodIPv4Table        = "dummy_pod_ipv4_table"
	DummyPodIPv6Table        = "dummy_pod_ipv6_table"
	LoadBalancedPodIPv4Table = "lb_pod_ipv4_table"
	LoadBalancedPodIPv6Table = "lb_pod_ipv6_table"
	ECMPGroupIPv4Table       = "ecmp_group_ipv4"
	ECMPGroupIPv6Table       = "ecmp_group_ipv6"
	ECMPNextHopIPv6Table     = "ecmp_nhop_ipv6"
	ECMPNextHopIPv4Table     = "ecmp_nhop_ipv4"

	MarkAsInboundTrafficAction = "mark_to_load_balance"
	MarkAsReturnTrafficAction  = "mark_to_return"
	SetECMPSelectIPv4Action    = "set_ecmp_select_ipv4"
	SetECMPSelectIPv6Action    = "set_ecmp_select_ipv6"
	SetIPv4NextHopAction       = "set_nhop_ipv4"
	SetIPv6NextHopAction       = "set_nhop_ipv6"
)

type Config struct {
	NetworkFunctionConfigName      string
	NetworkFunctionConfigNamespace string
	AppLabel                       string
	DummyAppLabelValue             string
	LoadBalancedAppLabelValue      string
}

func (r *NetworkFunctionConfigReconciler) matchesLoadBalancedOrDummyPods(object client.Object) bool {
	if object == nil {
		return false
	}
	labels := object.GetLabels()
	if labels == nil {
		return false
	}
	value, ok := labels[r.Config.AppLabel]
	return ok && (value == r.Config.DummyAppLabelValue || value == r.Config.LoadBalancedAppLabelValue)
}

func getMultusNetworkStatusString(object client.Object) string {
	if object == nil {
		return ""
	}
	annotations := object.GetAnnotations()
	if annotations == nil {
		return ""
	}
	netStatus, ok := annotations[netdefv1.NetworkStatusAnnot]
	if !ok {
		return ""
	}
	return netStatus
}

func getMultusNetworkStatus(object client.Object) *netdefv1.NetworkStatus {
	statusString := getMultusNetworkStatusString(object)
	if statusString == "" {
		return nil
	}
	status := &netdefv1.NetworkStatus{}
	if err := json.Unmarshal([]byte(statusString), status); err != nil {
		return nil
	}
	return status
}

// NetworkFunctionConfigReconciler reconciles the configuration for a NetworkFunction resource
type NetworkFunctionConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *Config
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Config == nil {
		return fmt.Errorf("expected non-nil Config")
	}
	return ctrl.NewControllerManagedBy(mgr).
		Watches(&v1.Pod{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
				if _, ok := object.(*v1.Pod); !ok {
					return nil
				}
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: r.Config.NetworkFunctionConfigName, Namespace: r.Config.NetworkFunctionConfigNamespace}}}
			}),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return r.matchesLoadBalancedOrDummyPods(e.Object)
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					if e.ObjectOld == nil || e.ObjectNew == nil {
						return false
					}
					if !r.matchesLoadBalancedOrDummyPods(e.ObjectNew) {
						return false
					}
					oldNetStatus := getMultusNetworkStatusString(e.ObjectOld)
					newNetStatus := getMultusNetworkStatusString(e.ObjectNew)
					return oldNetStatus != newNetStatus
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return r.matchesLoadBalancedOrDummyPods(e.Object)
				},
			})).
		Named("load-balancer-nf").
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *NetworkFunctionConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling NetworkFunctionConfig", "request", req)

	var nfConfig = &corev1alpha1.NetworkFunctionConfig{}
	if err := r.Get(ctx, req.NamespacedName, nfConfig); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunctionConfig resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, err
		}
		logger.Error(err, "Failed to get NetworkFunctionConfig")
		return ctrl.Result{}, err
	}

	var dummyPodLabels = map[string]string{
		r.Config.AppLabel: r.Config.DummyAppLabelValue,
	}
	var dummyPodList = &v1.PodList{}
	if err := r.List(ctx, dummyPodList, client.MatchingLabels(dummyPodLabels)); err != nil {
		logger.Error(err, "Failed to list dummy web server pods")
		return ctrl.Result{}, err
	}
	if len(dummyPodList.Items) == 0 {
		logger.V(1).Info("No dummy web server pods found with labels", "labels", dummyPodLabels)
		return ctrl.Result{}, nil
	}

	var loadBalancedPodLabels = map[string]string{
		r.Config.AppLabel: r.Config.LoadBalancedAppLabelValue,
	}
	var loadBalancedPodList = &v1.PodList{}
	if err := r.List(ctx, loadBalancedPodList, client.MatchingLabels(loadBalancedPodLabels)); err != nil {
		logger.Error(err, "Failed to list pods to load balance")
		return ctrl.Result{}, err
	}
	if len(loadBalancedPodList.Items) == 0 {
		logger.V(1).Info("No load-balanced pods found with labels", "labels", loadBalancedPodLabels)
		return ctrl.Result{}, nil
	}

	if err := r.updateNetworkFunctionConfig(ctx, nfConfig, dummyPodList, loadBalancedPodList); err != nil {
		logger.Error(err, "Failed to update NetworkFunctionConfig with new pod information")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *NetworkFunctionConfigReconciler) updateNetworkFunctionConfig(ctx context.Context,
	nfConfig *corev1alpha1.NetworkFunctionConfig,
	dummyPodList *v1.PodList,
	loadBalancedPodList *v1.PodList) error {
	original := nfConfig.DeepCopy()
	err := r.updateDummyPodConfig(nfConfig, dummyPodList)
	if err != nil {
		return fmt.Errorf("failed to update Dummy Pod Config: %w", err)
	}
	err = r.updateLoadBalancedPodConfig(nfConfig, loadBalancedPodList)
	if err != nil {
		return fmt.Errorf("failed to update Load-Balanced Pod Config: %w", err)
	}
	return r.Patch(ctx, nfConfig, client.MergeFrom(original))
}

func (r *NetworkFunctionConfigReconciler) updateDummyPodConfig(
	nfConfig *corev1alpha1.NetworkFunctionConfig, dummyPodList *v1.PodList) error {
	if nfConfig.Spec.Tables == nil {
		nfConfig.Spec.Tables = make(map[string]corev1alpha1.TableConfig)
	}
	var ipv4Entries = make([]corev1alpha1.TableEntry, len(dummyPodList.Items))
	var ipv6Entries = make([]corev1alpha1.TableEntry, len(dummyPodList.Items))
	for i := range dummyPodList.Items {
		pod := &dummyPodList.Items[i]
		podIPv4 := getPodIMLIPv4Addr(pod)
		podIPv6 := getPodIMLIPv6Addr(pod)
		if podIPv4 != nil {
			ipv4Entries = append(ipv4Entries, corev1alpha1.TableEntry{
				MatchFields: []corev1alpha1.TypedValue{{
					Name:  "hdr.inner_ipv4.src_addr",
					Value: podIPv4.String(),
					Type:  "ipv4_address",
				}},
				Action: corev1alpha1.ActionConfig{
					Name: MarkAsInboundTrafficAction,
				},
			})
		}
		if podIPv6 != nil {
			ipv6Entries = append(ipv6Entries, corev1alpha1.TableEntry{
				MatchFields: []corev1alpha1.TypedValue{{
					Name:  "hdr.inner_ipv6.src_addr",
					Value: podIPv6.String(),
					Type:  "ipv6_address",
				}},
				Action: corev1alpha1.ActionConfig{
					Name: MarkAsInboundTrafficAction,
				},
			})
		}
	}
	nfConfig.Spec.Tables[DummyPodIPv4Table] = corev1alpha1.TableConfig{
		Entries: ipv4Entries,
	}
	nfConfig.Spec.Tables[DummyPodIPv6Table] = corev1alpha1.TableConfig{
		Entries: ipv6Entries,
	}
	return nil
}

func (r *NetworkFunctionConfigReconciler) updateLoadBalancedPodConfig(
	nfConfig *corev1alpha1.NetworkFunctionConfig, lbPodList *v1.PodList) error {
	if nfConfig.Spec.Tables == nil {
		nfConfig.Spec.Tables = make(map[string]corev1alpha1.TableConfig)
	}
	var ipv4LbTableEntries = make([]corev1alpha1.TableEntry, 0, len(lbPodList.Items))
	var ipv6LbTableEntries = make([]corev1alpha1.TableEntry, 0, len(lbPodList.Items))
	var ipv4EcmpNhTableEntries = make([]corev1alpha1.TableEntry, 0, len(lbPodList.Items))
	var ipv6EcmpNhTableEntries = make([]corev1alpha1.TableEntry, 0, len(lbPodList.Items))
	for i := range lbPodList.Items {
		pod := &lbPodList.Items[i]
		podIPv4 := getPodIMLIPv4Addr(pod)
		podIPv6 := getPodIMLIPv6Addr(pod)
		if podIPv4 != nil {
			ipv4LbTableEntries = append(ipv4LbTableEntries, corev1alpha1.TableEntry{
				MatchFields: []corev1alpha1.TypedValue{{
					Name:  "hdr.inner_ipv4.src_addr",
					Value: podIPv4.String(),
					Type:  "ipv4_address",
				}},
				Action: corev1alpha1.ActionConfig{
					Name: MarkAsReturnTrafficAction,
				},
			})
			ipv4EcmpNhTableEntries = append(ipv4EcmpNhTableEntries, corev1alpha1.TableEntry{
				MatchFields: []corev1alpha1.TypedValue{{
					Name:  "meta.ecmp_select",
					Value: strconv.Itoa(i),
					Type:  "int",
				}},
				Action: corev1alpha1.ActionConfig{
					Name: SetIPv4NextHopAction,
					Parameters: []corev1alpha1.TypedValue{{
						Name:  "nhop_ipv4",
						Value: podIPv4.String(),
						Type:  "ipv4_address",
					}},
				},
			})
		}
		if podIPv6 != nil {
			ipv6LbTableEntries = append(ipv6LbTableEntries, corev1alpha1.TableEntry{
				MatchFields: []corev1alpha1.TypedValue{{
					Name:  "hdr.inner_ipv6.src_addr",
					Value: podIPv6.String(),
					Type:  "ipv6_address",
				}},
				Action: corev1alpha1.ActionConfig{
					Name: MarkAsReturnTrafficAction,
				},
			})
			ipv6EcmpNhTableEntries = append(ipv6EcmpNhTableEntries, corev1alpha1.TableEntry{
				MatchFields: []corev1alpha1.TypedValue{{
					Name:  "meta.ecmp_select",
					Value: strconv.Itoa(i),
					Type:  "int",
				}},
				Action: corev1alpha1.ActionConfig{
					Name: SetIPv6NextHopAction,
					Parameters: []corev1alpha1.TypedValue{{
						Name:  "nhop_ipv6",
						Value: podIPv6.String(),
						Type:  "ipv6_address",
					}},
				},
			})
		}
	}
	nfConfig.Spec.Tables[LoadBalancedPodIPv4Table] = corev1alpha1.TableConfig{
		Entries: ipv4LbTableEntries,
	}
	nfConfig.Spec.Tables[LoadBalancedPodIPv6Table] = corev1alpha1.TableConfig{
		Entries: ipv6LbTableEntries,
	}
	nfConfig.Spec.Tables[ECMPGroupIPv4Table] = corev1alpha1.TableConfig{
		DefaultAction: corev1alpha1.ActionConfig{
			Name: SetECMPSelectIPv4Action,
			Parameters: []corev1alpha1.TypedValue{{
				Name:  "ecmp_count",
				Value: strconv.Itoa(len(ipv4LbTableEntries)),
				Type:  "int",
			}},
		},
	}
	nfConfig.Spec.Tables[ECMPGroupIPv6Table] = corev1alpha1.TableConfig{
		DefaultAction: corev1alpha1.ActionConfig{
			Name: SetECMPSelectIPv6Action,
			Parameters: []corev1alpha1.TypedValue{{
				Name:  "ecmp_count",
				Value: strconv.Itoa(len(ipv6LbTableEntries)),
				Type:  "int",
			}},
		},
	}
	nfConfig.Spec.Tables[ECMPNextHopIPv4Table] = corev1alpha1.TableConfig{
		Entries: ipv4EcmpNhTableEntries,
	}
	nfConfig.Spec.Tables[ECMPNextHopIPv6Table] = corev1alpha1.TableConfig{
		Entries: ipv6EcmpNhTableEntries,
	}
	return nil
}

func getPodIMLIPv4Addr(pod *v1.Pod) net.IP {
	status := getMultusNetworkStatus(pod)
	if status == nil {
		return nil
	}
	for _, ip := range status.IPs {
		addr, _ := netip.ParseAddr(ip)
		if !addr.IsValid() {
			continue
		}
		if addr.Is4() {
			return addr.AsSlice()
		}
	}
	return nil
}

func getPodIMLIPv6Addr(pod *v1.Pod) net.IP {
	status := getMultusNetworkStatus(pod)
	if status == nil {
		return nil
	}
	for _, ip := range status.IPs {
		addr, _ := netip.ParseAddr(ip)
		if !addr.IsValid() {
			continue
		}
		if addr.Is6() {
			return addr.AsSlice()
		}
	}
	return nil
}
