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
	corev1alpha1 "bmv2-driver/api/core/v1alpha1"
	"bmv2-driver/managers/nf"
	"bmv2-driver/managers/nfcfg"
	"bmv2-driver/managers/p4target"
	"bmv2-driver/pkg/strutils"
	"context"
	"net"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	depStatus nf.DeploymentStatus, funcIP net.IP) error {
	original := networkFunction.DeepCopy()
	networkFunction.Status = calculateStatus(networkFunction, depStatus, funcIP)
	return r.Status().Patch(context.Background(), networkFunction, client.MergeFrom(original))
}

func calculateStatus(netFunc *corev1alpha1.NetworkFunction,
	depStatus nf.DeploymentStatus, funcIP net.IP) corev1alpha1.NetworkFunctionStatus {
	// Calculate Phase
	var calculatedPhase corev1alpha1.NetworkFunctionPhase
	switch depStatus.Phase {
	case nf.PhaseReady:
		calculatedPhase = corev1alpha1.NetworkFunctionRunning
	case nf.PhaseFailed:
		calculatedPhase = corev1alpha1.NetworkFunctionFailed
	default:
		calculatedPhase = corev1alpha1.NetworkFunctionPending
	}

	status := corev1alpha1.NetworkFunctionStatus{
		AssignedIP:         funcIP.String(),
		ObservedGeneration: netFunc.Generation,
		Phase:              calculatedPhase,
	}
	// Copy conditions from old status
	for i := range netFunc.Status.Conditions {
		status.Conditions = append(status.Conditions, netFunc.Status.Conditions[i])
	}
	return status
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling NetworkFunction", "name", req.Name, "namespace", req.Namespace)

	netfunc := &corev1alpha1.NetworkFunction{}
	if err := r.Get(ctx, req.NamespacedName, netfunc); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunction resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch NetworkFunction")
		return ctrl.Result{}, err
	}

	if netfunc.DeletionTimestamp != nil {
		if !hasFinalizer(netfunc) {
			return ctrl.Result{}, nil
		}
		handle := r.NFManager.EnsureAbsent(ctx, netfunc)
		status := handle.Status()

		if !isDeleted(status) {
			return ctrl.Result{RequeueAfter: 2 * time.Second}, r.updateStatus(netfunc, status, nil)
		}
		original := netfunc.DeepCopy()
		removeFinalizer(netfunc)
		return ctrl.Result{}, r.Patch(ctx, netfunc, client.MergeFrom(original))
	}
	if !hasFinalizer(netfunc) {
		original := netfunc.DeepCopy()
		ensureFinalizer(netfunc)
		return ctrl.Result{RequeueAfter: 2 * time.Second}, r.Patch(ctx, netfunc, client.MergeFrom(original))
	}

	var funcIP net.IP
	if netfunc.Status.AssignedIP == "" {
		var err error
		funcIP, err = r.P4TargetManager.AllocateNetworkFunctionIP()
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	//TODO: What do we do now with this IP? How should the manager retrieve this IP?

	var nfConfig *corev1alpha1.NetworkFunctionConfig
	if netfunc.Spec.ConfigRef != nil {
		nfConfig = &corev1alpha1.NetworkFunctionConfig{}
		err := r.Get(ctx, client.ObjectKey{Namespace: netfunc.Namespace, Name: netfunc.Spec.ConfigRef.Name}, nfConfig)
		if err != nil {
			logger.Error(err, "Failed to get NetworkFunctionConfig referenced by NetworkFunction", "configName", netfunc.Spec.ConfigRef.Name)
			return ctrl.Result{}, err
		}
	}
	err := r.NFConfigManager.EnsurePresentConfigForNF(nfConfig, netfunc)
	if err != nil {
		return ctrl.Result{}, err
	}

	nfHandle := r.NFManager.EnsurePresent(ctx, netfunc)
	status := nfHandle.Status()
	err = r.updateStatus(netfunc, status, funcIP)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !isDeployed(status) {
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}
