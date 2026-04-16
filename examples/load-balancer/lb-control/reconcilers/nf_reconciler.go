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
	"fmt"

	netdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1alpha1 "github.com/mantra6g/iml/operator/api/core/v1alpha1"
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

func (r *NetworkFunctionConfigReconciler) getMultusNetworkStatus(object client.Object) string {
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
					oldNetStatus := r.getMultusNetworkStatus(e.ObjectOld)
					newNetStatus := r.getMultusNetworkStatus(e.ObjectNew)
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

	return ctrl.Result{}, nil
}
