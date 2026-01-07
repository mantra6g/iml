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

package cache

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cachev1alpha1 "builder/api/cache/v1alpha1"
	"builder/pkg/events"
)

// ServiceChainReconciler reconciles a ServiceChain object
type ServiceChainReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Bus    events.EventBus
}

// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=servicechains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=servicechains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=servicechains/finalizers,verbs=update
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=applications,verbs=get;list;watch
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=applications/status,verbs=get
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=networkfunctions,verbs=get;list;watch
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=networkfunctions/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServiceChain object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *ServiceChainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	logger.Info("Reconciling ServiceChain", "request", req)
	var serviceChain cachev1alpha1.ServiceChain
	if err := r.Get(ctx, req.NamespacedName, &serviceChain); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ServiceChain resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ServiceChain")
		return ctrl.Result{}, err
	}

	// Check if being deleted
	if !serviceChain.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion
		if containsString(serviceChain.GetFinalizers(), cachev1alpha1.SERVICE_CHAIN_FINALIZER_LABEL) {
			r.Bus.Publish(events.Event{
				Name:    events.EventChainPreDeleted,
				Payload: &serviceChain,
			})

			// Remove finalizer
			serviceChain.SetFinalizers(removeString(serviceChain.GetFinalizers(), cachev1alpha1.SERVICE_CHAIN_FINALIZER_LABEL))
			if err := r.Update(ctx, &serviceChain); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing
	if !containsString(serviceChain.GetFinalizers(), cachev1alpha1.SERVICE_CHAIN_FINALIZER_LABEL) {
		serviceChain.SetFinalizers(append(serviceChain.GetFinalizers(), cachev1alpha1.SERVICE_CHAIN_FINALIZER_LABEL))
		if err := r.Update(ctx, &serviceChain); err != nil {
			return ctrl.Result{}, err
		}
	}

	var srcApp, dstApp cachev1alpha1.Application
	if err := r.Get(ctx, serviceChain.Spec.From.GetObjectKey(), &srcApp); err != nil {
		if apierrors.IsNotFound(err) {
			// Source app does not exist yet. Wait and retry.
			logger.Error(err, "Source Application not found", "name", serviceChain.Spec.From.Name)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("source application not found")
		}
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, serviceChain.Spec.To.GetObjectKey(), &dstApp); err != nil {
		if apierrors.IsNotFound(err) {
			// Destination app does not exist yet. Wait and retry.
			logger.Error(err, "Destination Application not found", "name", serviceChain.Spec.To.Name)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("destination application not found")
		}
		return ctrl.Result{}, err
	}

	var nfs []cachev1alpha1.NetworkFunction
	for _, fn := range serviceChain.Spec.Functions {
		var nf cachev1alpha1.NetworkFunction
		if err := r.Get(ctx, fn.GetObjectKey(), &nf); err != nil {
			if apierrors.IsNotFound(err) {
				// Network function does not exist yet. Wait and retry.
				logger.Error(err, "Required NetworkFunction not found", "name", fn.Name)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("network function not found")
			}
			return ctrl.Result{}, err
		}
		nfs = append(nfs, nf)
	}

	// Update the status fields
	srcAppID := srcApp.UID
	if srcApp.Spec.OverrideID != "" {
		srcAppID = types.UID(srcApp.Spec.OverrideID)
	}
	serviceChain.Status.SourceAppUID = srcAppID

	dstAppID := dstApp.UID
	if dstApp.Spec.OverrideID != "" {
		dstAppID = types.UID(dstApp.Spec.OverrideID)
	}
	serviceChain.Status.DestinationAppUID = dstAppID

	serviceChain.Status.Functions = []string{}
	for _, nf := range nfs {
		serviceChain.Status.Functions = append(serviceChain.Status.Functions,
			string(nf.UID),
		)
	}
	if err := r.Status().Update(ctx, &serviceChain); err != nil {
		logger.Error(err, "Failed to update ServiceChain status")
		return ctrl.Result{}, err
	}

	// All is well
	r.Bus.Publish(events.Event{
		Name:    events.EventChainPreUpdated,
		Payload: &serviceChain,
	})

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceChainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Index the ServiceChain by its source application name
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &cachev1alpha1.ServiceChain{},
		"spec.from",
		func(obj client.Object) []string {
			sc := obj.(*cachev1alpha1.ServiceChain)
			if sc.Spec.From.Name == "" {
				return nil
			}
			ns := sc.Spec.From.Namespace
			if ns == "" {
				ns = sc.Namespace
			}
			return []string{ns + "/" + sc.Spec.From.Name}
		},
	); err != nil {
		return err
	}

	// Index the ServiceChain by its destination application name
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &cachev1alpha1.ServiceChain{},
		"spec.to",
		func(obj client.Object) []string {
			sc := obj.(*cachev1alpha1.ServiceChain)
			if sc.Spec.From.Name == "" {
				return nil
			}
			ns := sc.Spec.From.Namespace
			if ns == "" {
				ns = sc.Namespace
			}
			return []string{ns + "/" + sc.Spec.From.Name}
		},
	); err != nil {
		return err
	}

	// Index based on the functions
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &cachev1alpha1.ServiceChain{},
		"spec.functions",
		func(obj client.Object) []string {
			sc := obj.(*cachev1alpha1.ServiceChain)
			if sc.Spec.Functions == nil {
				return nil
			}
			var nfs []string
			for _, fn := range sc.Spec.Functions {
				if fn.Name == "" {
					continue
				}
				ns := fn.Namespace
				if ns == "" {
					ns = sc.Namespace
				}
				nfs = append(nfs, ns+"/"+fn.Name)
			}
			return nfs
		},
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.ServiceChain{}).
		Watches(&cachev1alpha1.Application{},
			handler.EnqueueRequestsFromMapFunc(r.mapApplicationsToRequests)).
		Watches(&cachev1alpha1.NetworkFunction{},
			handler.EnqueueRequestsFromMapFunc(r.mapNetworkFunctionsToRequests)).
		Named("cache-servicechain").
		Complete(r)
}

func (r *ServiceChainReconciler) mapApplicationsToRequests(ctx context.Context, object client.Object) []reconcile.Request {
	logger := logf.FromContext(ctx)
	app := object.(*cachev1alpha1.Application)

	// Find all ServiceChains that use this Application as source...
	var fromList cachev1alpha1.ServiceChainList
	if err := r.List(ctx, &fromList,
		client.MatchingFields{
			"spec.from": app.Namespace + "/" + app.Name,
		},
	); err != nil {
		logger.Error(err, "could not list ServiceChains. "+
			"change to Application %s.%s will not be reconciled.",
			app.Name, app.Namespace)
		return nil
	}

	// ... now find all that use this App as destination...
	var toList cachev1alpha1.ServiceChainList
	if err := r.List(ctx, &toList,
		client.MatchingFields{
			"spec.to": app.Namespace + "/" + app.Name,
		},
	); err != nil {
		logger.Error(err, "could not list ServiceChains. "+
			"change to Application %s.%s will not be reconciled.",
			app.Name, app.Namespace)
		return nil
	}

	// ... merge the service chain lists and transform them into requests.
	requests := make([]reconcile.Request, len(fromList.Items)+len(toList.Items))
	for i, sc := range fromList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&sc),
		}
	}
	for i, sc := range toList.Items {
		requests[i+len(fromList.Items)] = reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&sc),
		}
	}
	return requests
}

func (r *ServiceChainReconciler) mapNetworkFunctionsToRequests(ctx context.Context, object client.Object) []reconcile.Request {
	logger := logf.FromContext(ctx)
	nf := object.(*cachev1alpha1.NetworkFunction)

	// Find all ServiceChains that contain this NetworkFunction...
	var scList cachev1alpha1.ServiceChainList
	if err := r.List(ctx, &scList,
		client.MatchingFields{
			"spec.functions": nf.Namespace + "/" + nf.Name,
		},
	); err != nil {
		logger.Error(err, "could not list ServiceChains. "+
			"change to NetworkFunction %s.%s will not be reconciled.",
			nf.Name, nf.Namespace)
		return nil
	}

	// ... transform them into requests.
	requests := make([]reconcile.Request, len(scList.Items))
	for i, sc := range scList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&sc),
		}
	}
	return requests
}
