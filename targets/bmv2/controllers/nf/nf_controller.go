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

package nf

import (
	"github.com/mantra6g/iml/targets/bmv2/managers/nf"
	"github.com/mantra6g/iml/targets/bmv2/managers/nfcfg"
	"github.com/mantra6g/iml/targets/bmv2/managers/p4target"
	"github.com/mantra6g/iml/targets/bmv2/pkg/strutils"
	"context"
	"fmt"
	"net/netip"
	"time"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	NetworkFunctionFinalizer = "networkfunctions.loom.io/finalizer"
)

// Reconciler reconciles a NetworkFunction object
type Reconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	NFManager       nf.Manager
	NFConfigManager nfcfg.Manager
	P4TargetManager p4target.Manager
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.NetworkFunction{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.TypedCreateEvent[client.Object]) bool {
				netfunc, ok := e.Object.(*corev1alpha1.NetworkFunction)
				if !ok {
					return false
				}
				return netfunc.Spec.TargetName == r.P4TargetManager.GetName()
			},
			UpdateFunc: func(e event.TypedUpdateEvent[client.Object]) bool {
				oldNf, ok1 := e.ObjectOld.(*corev1alpha1.NetworkFunction)
				newNf, ok2 := e.ObjectNew.(*corev1alpha1.NetworkFunction)
				if !ok1 || !ok2 {
					return false
				}
				if oldNf.Generation == newNf.Generation {
					return false
				}
				return oldNf.Spec.TargetName == r.P4TargetManager.GetName() ||
					newNf.Spec.TargetName == r.P4TargetManager.GetName()
			},
			DeleteFunc: func(e event.TypedDeleteEvent[client.Object]) bool {
				netfunc, ok := e.Object.(*corev1alpha1.NetworkFunction)
				if !ok {
					return false
				}
				return netfunc.Spec.TargetName == r.P4TargetManager.GetName()
			},
		}).
		Watches(&corev1alpha1.NetworkFunctionConfig{},
			handler.EnqueueRequestsFromMapFunc(r.mapNetworkFunctionConfigToRequests)).
		Named("nf-controller").
		Complete(r)
}

func (r *Reconciler) mapNetworkFunctionConfigToRequests(_ context.Context, obj client.Object) []reconcile.Request {
	referencedNFList, err := r.NFConfigManager.GetAllNetworkFunctionsUsingConfig(client.ObjectKeyFromObject(obj))
	if err != nil {
		return nil
	}
	requests := make([]reconcile.Request, len(referencedNFList))
	for i := range referencedNFList {
		requests[i] = reconcile.Request{}
	}
	return requests
}

func isDeleted(status nf.DeploymentStatus) bool {
	return status.Phase == nf.PhaseDeleted
}

func isDeployed(status nf.DeploymentStatus) bool {
	return status.Phase == nf.PhaseReady
}

func hasFinalizer(networkFunction *corev1alpha1.NetworkFunction) bool {
	return strutils.ContainsString(networkFunction.Finalizers, NetworkFunctionFinalizer)
}

func removeFinalizer(networkFunction *corev1alpha1.NetworkFunction) {
	if !hasFinalizer(networkFunction) {
		return
	}
	networkFunction.Finalizers = strutils.RemoveString(networkFunction.Finalizers, NetworkFunctionFinalizer)
}

func ensureFinalizer(networkFunction *corev1alpha1.NetworkFunction) {
	if hasFinalizer(networkFunction) {
		return
	}
	networkFunction.Finalizers = append(networkFunction.Finalizers, NetworkFunctionFinalizer)
}

func (r *Reconciler) updateStatus(networkFunction *corev1alpha1.NetworkFunction,
	depStatus nf.DeploymentStatus, funcIP netip.Prefix) error {
	original := networkFunction.DeepCopy()
	networkFunction.Status = calculateStatus(networkFunction, depStatus, funcIP)
	return r.Status().Patch(context.Background(), networkFunction, client.MergeFrom(original))
}

func calculateStatus(netFunc *corev1alpha1.NetworkFunction,
	depStatus nf.DeploymentStatus, funcIP netip.Prefix) corev1alpha1.NetworkFunctionStatus {
	// Calculate Phase
	var calculatedPhase corev1alpha1.NetworkFunctionPhase
	var readyCondition corev1alpha1.NetworkFunctionCondition
	switch depStatus.Phase {
	case nf.PhaseReady:
		calculatedPhase = corev1alpha1.NetworkFunctionRunning
		readyCondition = corev1alpha1.NetworkFunctionCondition{
			Type:    corev1alpha1.NetworkFunctionReady,
			Status:  metav1.ConditionTrue,
			Reason:  "NetworkFunctionReady",
			Message: "The NetworkFunction is ready",
		}
	case nf.PhaseFailed:
		calculatedPhase = corev1alpha1.NetworkFunctionFailed
		readyCondition = corev1alpha1.NetworkFunctionCondition{
			Type:    corev1alpha1.NetworkFunctionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "NetworkFunctionNotReady",
			Message: "The NetworkFunction is not ready",
		}
	default:
		calculatedPhase = corev1alpha1.NetworkFunctionPending
		readyCondition = corev1alpha1.NetworkFunctionCondition{
			Type:    corev1alpha1.NetworkFunctionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "NetworkFunctionNotReady",
			Message: "The NetworkFunction is not ready",
		}
	}

	status := corev1alpha1.NetworkFunctionStatus{
		AssignedIP:         funcIP.Addr().String(),
		ObservedGeneration: netFunc.Generation,
		Phase:              calculatedPhase,
	}
	// Copy conditions from old status
	status.Conditions = CopyConditions(&netFunc.Status)
	// Update or add the ready condition
	status.Conditions = UpdateNFCondition(&status, readyCondition)
	return status
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling NetworkFunction", "name", req.Name, "namespace", req.Namespace)

	netfunc := &corev1alpha1.NetworkFunction{}
	if err := r.Get(ctx, req.NamespacedName, netfunc); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunction resource not found. Deleting associated resources if any.")
			_ = r.NFManager.EnsureAbsent(ctx, req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch NetworkFunction")
		return ctrl.Result{}, err
	}

	if r.P4TargetManager.GetNetworkConfiguredCondition().Status != metav1.ConditionTrue {
		logger.Info("Network not yet configured, requeuing", "reason", r.P4TargetManager.GetNetworkConfiguredCondition().Reason)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	var funcIP netip.Addr
	var err error
	if netfunc.Status.AssignedIP == "" {
		var err error
		funcIP, err = r.P4TargetManager.AllocateNetworkFunctionIP()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to allocate Network Function IP: %w", err)
		}
	} else {
		funcIP, err = netip.ParseAddr(netfunc.Status.AssignedIP)
		if err != nil {
			logger.Error(nil, "Invalid IP address in NetworkFunction status", "assignedIP", netfunc.Status.AssignedIP)
			return ctrl.Result{}, fmt.Errorf("invalid IP address in NetworkFunction status: %s", netfunc.Status.AssignedIP)
		}
	}
	fullNfAddress := netip.PrefixFrom(funcIP, r.P4TargetManager.GetNfCIDR().Bits())
	nfHandle := r.NFManager.EnsurePresent(ctx, netfunc, fullNfAddress)
	status := nfHandle.Status()
	err = r.updateStatus(netfunc, status, fullNfAddress)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !isDeployed(status) {
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	var nfConfig *corev1alpha1.NetworkFunctionConfig
	if netfunc.Spec.ConfigRef != nil {
		nfConfig = &corev1alpha1.NetworkFunctionConfig{}
		err := r.Get(ctx, client.ObjectKey{Namespace: netfunc.Namespace, Name: netfunc.Spec.ConfigRef.Name}, nfConfig)
		if err != nil {
			logger.Error(err, "Failed to get NetworkFunctionConfig referenced by NetworkFunction", "configName", netfunc.Spec.ConfigRef.Name)
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, r.NFConfigManager.EnsurePresentConfigForNF(ctx, nfConfig, netfunc)
}
