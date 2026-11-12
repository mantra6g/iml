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
	"encoding/binary"
	"fmt"
	"sort"
	"time"

	"lb-control/utils"

	"github.com/mantra6g/iml/dpcsclient"
	v1 "k8s.io/api/core/v1"
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
	DummyPodIPv4Table        = "MyIngress.dummy_pod_ipv4_table"
	DummyPodIPv6Table        = "MyIngress.dummy_pod_ipv6_table"
	LoadBalancedPodIPv4Table = "MyIngress.lb_pod_ipv4_table"
	LoadBalancedPodIPv6Table = "MyIngress.lb_pod_ipv6_table"
	ECMPGroupIPv4Table       = "MyIngress.ecmp_group_ipv4"
	ECMPGroupIPv6Table       = "MyIngress.ecmp_group_ipv6"
	ECMPNextHopIPv6Table     = "MyIngress.ecmp_nhop_ipv6"
	ECMPNextHopIPv4Table     = "MyIngress.ecmp_nhop_ipv4"

	MarkAsInboundTrafficAction = "MyIngress.mark_to_load_balance"
	MarkAsReturnTrafficAction  = "MyIngress.mark_to_return"
	SetECMPSelectIPv4Action    = "MyIngress.set_ecmp_select_ipv4"
	SetECMPSelectIPv6Action    = "MyIngress.set_ecmp_select_ipv6"
	SetIPv4NextHopAction       = "MyIngress.set_nhop_ipv4"
	SetIPv6NextHopAction       = "MyIngress.set_nhop_ipv6"
)

type Config struct {
	NetworkFunctionConfigNamespace string
	DPCSAddress                    string
	DeviceID                       uint64
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
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: r.Config.NetworkFunctionConfigNamespace}}}
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
					oldPod, okOld := e.ObjectOld.(*v1.Pod)
					newPod, okNew := e.ObjectNew.(*v1.Pod)
					if !okOld || !okNew {
						return false
					}
					oldNetStatus := utils.GetMultusNetworkStatusString(oldPod)
					newNetStatus := utils.GetMultusNetworkStatusString(newPod)
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

	dpcsClient, err := dpcsclient.Connect(ctx, dpcsclient.Config{
		Address:  r.Config.DPCSAddress,
		DeviceID: r.Config.DeviceID,
	})
	if err != nil {
		logger.Error(err, "Failed to connect to DPCS")
		return ctrl.Result{}, err
	}
	defer dpcsClient.Close()

	var dummyPodLabels = map[string]string{
		r.Config.AppLabel: r.Config.DummyAppLabelValue,
	}
	logger.V(1).Info("Listing dummy pods", "dummyPodLabels", dummyPodLabels)
	var dummyPodList = &v1.PodList{}
	if err := r.List(ctx, dummyPodList, client.MatchingLabels(dummyPodLabels)); err != nil {
		logger.Error(err, "Failed to list dummy web server pods")
		return ctrl.Result{}, err
	}
	if len(dummyPodList.Items) == 0 {
		logger.V(1).Info("No dummy web server pods found with labels", "labels", dummyPodLabels)
		return ctrl.Result{}, nil
	}
	logger.V(1).Info("Found dummy pods", "dummyPodLabels", dummyPodLabels, "count", len(dummyPodList.Items))

	var loadBalancedPodLabels = map[string]string{
		r.Config.AppLabel: r.Config.LoadBalancedAppLabelValue,
	}
	logger.V(1).Info("Listing load-balanced pods", "loadBalancedPodLabels", loadBalancedPodLabels)
	var loadBalancedPodList = &v1.PodList{}
	if err := r.List(ctx, loadBalancedPodList, client.MatchingLabels(loadBalancedPodLabels)); err != nil {
		logger.Error(err, "Failed to list pods to load balance")
		return ctrl.Result{}, err
	}
	if len(loadBalancedPodList.Items) == 0 {
		logger.V(1).Info("No load-balanced pods found with labels", "labels", loadBalancedPodLabels)
		return ctrl.Result{}, nil
	}
	logger.V(1).Info("Found load-balanced pods", "loadBalancedPodLabels", loadBalancedPodLabels, "count", len(loadBalancedPodList.Items))

	if err := r.updateNetworkFunctionConfig(ctx, dpcsClient, dummyPodList, loadBalancedPodList); err != nil {
		logger.Error(err, "Failed to update NetworkFunctionConfig with new pod information")
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *NetworkFunctionConfigReconciler) updateNetworkFunctionConfig(ctx context.Context,
	dpcsClient *dpcsclient.Client,
	dummyPodList *v1.PodList,
	loadBalancedPodList *v1.PodList) error {
	err := r.updateDummyPodConfig(ctx, dpcsClient, dummyPodList)
	if err != nil {
		return fmt.Errorf("failed to update Dummy Pod Config: %w", err)
	}
	err = r.updateLoadBalancedPodConfig(ctx, dpcsClient, loadBalancedPodList)
	if err != nil {
		return fmt.Errorf("failed to update Load-Balanced Pod Config: %w", err)
	}
	return nil
}

func (r *NetworkFunctionConfigReconciler) updateDummyPodConfig(ctx context.Context,
	dpcsClient *dpcsclient.Client, dummyPodList *v1.PodList) error {
	var ipv4Entries = make([]dpcsclient.Entry, 0, len(dummyPodList.Items))
	var ipv6Entries = make([]dpcsclient.Entry, 0, len(dummyPodList.Items))
	for i := range dummyPodList.Items {
		pod := &dummyPodList.Items[i]
		podIPv4 := utils.GetPodIMLIPv4Addr(pod)
		podIPv6 := utils.GetPodIMLIPv6Addr(pod)
		if podIPv4 != nil {
			ipv4Entries = append(ipv4Entries, dpcsclient.Entry{
				Match:  map[string]dpcsclient.FieldMatch{"hdr.inner_ipv4.dst_addr": dpcsclient.ExactIP(podIPv4)},
				Action: MarkAsInboundTrafficAction,
			})
		}
		if podIPv6 != nil {
			ipv6Entries = append(ipv6Entries, dpcsclient.Entry{
				Match:  map[string]dpcsclient.FieldMatch{"hdr.inner_ipv6.dst_addr": dpcsclient.ExactIP(podIPv6)},
				Action: MarkAsInboundTrafficAction,
			})
		}
	}
	if _, err := dpcsClient.SyncTable(ctx, DummyPodIPv4Table, ipv4Entries); err != nil {
		return err
	}
	if _, err := dpcsClient.SyncTable(ctx, DummyPodIPv6Table, ipv6Entries); err != nil {
		return err
	}
	return nil
}

func (r *NetworkFunctionConfigReconciler) updateLoadBalancedPodConfig(ctx context.Context,
	dpcsClient *dpcsclient.Client, lbPodList *v1.PodList) error {
	var ipv4LbTableEntries = make([]dpcsclient.Entry, 0, len(lbPodList.Items))
	var ipv6LbTableEntries = make([]dpcsclient.Entry, 0, len(lbPodList.Items))
	var ipv4EcmpNhTableEntries = make([]dpcsclient.Entry, 0, len(lbPodList.Items))
	var ipv6EcmpNhTableEntries = make([]dpcsclient.Entry, 0, len(lbPodList.Items))
	sort.Slice(lbPodList.Items, func(i, j int) bool { return lbPodList.Items[i].Name < lbPodList.Items[j].Name })
	for i := range lbPodList.Items {
		pod := &lbPodList.Items[i]
		podIPv4 := utils.GetPodIMLIPv4Addr(pod)
		podIPv6 := utils.GetPodIMLIPv6Addr(pod)
		ecmpSelect := dpcsclient.Exact(binary.BigEndian.AppendUint32(nil, uint32(i)))
		if podIPv4 != nil {
			ipv4LbTableEntries = append(ipv4LbTableEntries, dpcsclient.Entry{
				Match:  map[string]dpcsclient.FieldMatch{"hdr.inner_ipv4.src_addr": dpcsclient.ExactIP(podIPv4)},
				Action: MarkAsReturnTrafficAction,
			})
			ipv4EcmpNhTableEntries = append(ipv4EcmpNhTableEntries, dpcsclient.Entry{
				Match:  map[string]dpcsclient.FieldMatch{"meta.ecmp_select": ecmpSelect},
				Action: SetIPv4NextHopAction,
				Params: [][]byte{podIPv4},
			})
		}
		if podIPv6 != nil {
			ipv6LbTableEntries = append(ipv6LbTableEntries, dpcsclient.Entry{
				Match:  map[string]dpcsclient.FieldMatch{"hdr.inner_ipv6.src_addr": dpcsclient.ExactIP(podIPv6)},
				Action: MarkAsReturnTrafficAction,
			})
			ipv6EcmpNhTableEntries = append(ipv6EcmpNhTableEntries, dpcsclient.Entry{
				Match:  map[string]dpcsclient.FieldMatch{"meta.ecmp_select": ecmpSelect},
				Action: SetIPv6NextHopAction,
				Params: [][]byte{podIPv6},
			})
		}
	}
	if _, err := dpcsClient.SyncTable(ctx, LoadBalancedPodIPv4Table, ipv4LbTableEntries); err != nil {
		return err
	}
	if _, err := dpcsClient.SyncTable(ctx, LoadBalancedPodIPv6Table, ipv6LbTableEntries); err != nil {
		return err
	}
	ipv4EcmpCount := binary.BigEndian.AppendUint32(nil, uint32(len(ipv4LbTableEntries)))
	if err := dpcsClient.SetDefaultAction(ctx, ECMPGroupIPv4Table, SetECMPSelectIPv4Action, [][]byte{ipv4EcmpCount}); err != nil {
		return err
	}
	ipv6EcmpCount := binary.BigEndian.AppendUint32(nil, uint32(len(ipv6LbTableEntries)))
	if err := dpcsClient.SetDefaultAction(ctx, ECMPGroupIPv6Table, SetECMPSelectIPv6Action, [][]byte{ipv6EcmpCount}); err != nil {
		return err
	}
	if _, err := dpcsClient.SyncTable(ctx, ECMPNextHopIPv4Table, ipv4EcmpNhTableEntries); err != nil {
		return err
	}
	if _, err := dpcsClient.SyncTable(ctx, ECMPNextHopIPv6Table, ipv6EcmpNhTableEntries); err != nil {
		return err
	}
	return nil
}
