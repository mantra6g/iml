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

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ManagedByLabel      = "app.kubernetes.io/managed-by"
	ManagedByLabelValue = "firewall-controller"
)

type Config struct {
	NetworkFunctionConfigName      string
	NetworkFunctionConfigNamespace string
	SourceAppName                  string
	DestinationAppName             string
}

func (r *FirewallConfigReconciler) isWatchedApp(object client.Object) bool {
	if object == nil {
		return false
	}
	name := object.GetName()
	return name == r.Config.SourceAppName || name == r.Config.DestinationAppName
}

func (r *FirewallConfigReconciler) isOwnedConfig(object client.Object) bool {
	return object != nil && object.GetName() == r.Config.NetworkFunctionConfigName
}

// FirewallConfigReconciler keeps the firewall NetworkFunctionConfig in sync with the subnets
// assigned to the source and destination Applications
type FirewallConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *Config
}

// SetupWithManager sets up the controller with the Manager.
func (r *FirewallConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Config == nil {
		return fmt.Errorf("expected non-nil Config")
	}
	configRequest := reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      r.Config.NetworkFunctionConfigName,
		Namespace: r.Config.NetworkFunctionConfigNamespace,
	}}
	return ctrl.NewControllerManagedBy(mgr).
		// Watching the owned config restores it whenever someone else modifies or deletes it
		For(&corev1alpha1.NetworkFunctionConfig{},
			builder.WithPredicates(predicate.NewPredicateFuncs(r.isOwnedConfig))).
		Watches(&corev1alpha1.Application{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
				if _, ok := object.(*corev1alpha1.Application); !ok {
					return nil
				}
				return []reconcile.Request{configRequest}
			}),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return r.isWatchedApp(e.Object)
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					if e.ObjectOld == nil || e.ObjectNew == nil {
						return false
					}
					if !r.isWatchedApp(e.ObjectNew) {
						return false
					}
					oldApp, okOld := e.ObjectOld.(*corev1alpha1.Application)
					newApp, okNew := e.ObjectNew.(*corev1alpha1.Application)
					if !okOld || !okNew {
						return false
					}
					return !equality.Semantic.DeepEqual(oldApp.Status.Subnets, newApp.Status.Subnets)
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return r.isWatchedApp(e.Object)
				},
				GenericFunc: func(e event.GenericEvent) bool {
					return false
				},
			})).
		Named("firewall-nf").
		Complete(r)
}

// Reconcile rebuilds the firewall tables from the current subnets of the source and destination
// Applications and writes them to the NetworkFunctionConfig, creating it if needed.
func (r *FirewallConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling firewall NetworkFunctionConfig", "request", req)

	srcApp, err := r.getApplication(ctx, r.Config.SourceAppName)
	if err != nil {
		logger.Error(err, "Failed to get source Application", "name", r.Config.SourceAppName)
		return ctrl.Result{}, err
	}
	dstApp, err := r.getApplication(ctx, r.Config.DestinationAppName)
	if err != nil {
		logger.Error(err, "Failed to get destination Application", "name", r.Config.DestinationAppName)
		return ctrl.Result{}, err
	}

	tables, err := BuildFirewallTables(srcApp, dstApp)
	if err != nil {
		logger.Error(err, "Failed to build firewall tables")
		return ctrl.Result{}, err
	}

	nfConfig := &corev1alpha1.NetworkFunctionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Config.NetworkFunctionConfigName,
			Namespace: r.Config.NetworkFunctionConfigNamespace,
		},
	}
	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, nfConfig, func() error {
		labels := nfConfig.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[ManagedByLabel] = ManagedByLabelValue
		nfConfig.SetLabels(labels)
		nfConfig.Spec.Tables = tables
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to create or update firewall NetworkFunctionConfig")
		return ctrl.Result{}, err
	}

	logger.Info("Firewall NetworkFunctionConfig reconciled",
		"operation", result,
		"srcIPv4Subnets", len(tables[FirewallSrcIPv4Table].Entries),
		"dstIPv4Subnets", len(tables[FirewallDstIPv4Table].Entries),
		"srcIPv6Subnets", len(tables[FirewallSrcIPv6Table].Entries),
		"dstIPv6Subnets", len(tables[FirewallDstIPv6Table].Entries))
	return ctrl.Result{}, nil
}

// getApplication returns the named Application in the config namespace, or nil if it does not exist.
// A missing Application contributes no subnets, so all traffic involving it is denied.
func (r *FirewallConfigReconciler) getApplication(ctx context.Context, name string) (*corev1alpha1.Application, error) {
	app := &corev1alpha1.Application{}
	key := types.NamespacedName{Name: name, Namespace: r.Config.NetworkFunctionConfigNamespace}
	if err := r.Get(ctx, key, app); err != nil {
		if apierrors.IsNotFound(err) {
			logf.FromContext(ctx).Info("Application not found, denying its traffic", "name", name)
			return nil, nil
		}
		return nil, err
	}
	return app, nil
}
