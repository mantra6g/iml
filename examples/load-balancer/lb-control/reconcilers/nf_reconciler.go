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

	corev1alpha1 "github.com/mantra6g/iml/operator/api/core/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler reconciles a NetworkFunction object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	logger.Info("Reconciling NetworkFunction", "request", req)
	var nf = &corev1alpha1.NetworkFunction{}
	if err := r.Get(ctx, req.NamespacedName, nf); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunction resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, err
		}
		logger.Error(err, "Failed to get NetworkFunction")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.NetworkFunction{}).
		Watches(&corev1alpha1.Application{},
			handler.EnqueueRequestsFromMapFunc(r.mapApplicationsToRequests),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("servicechain-daemon").
		Complete(r)
}

func (r *Reconciler) mapApplicationsToRequests(ctx context.Context, object client.Object) []reconcile.Request {
	if object == nil {
		return []reconcile.Request{}
	}
	app, ok := object.(*corev1alpha1.Application)
	if !ok {
		return []reconcile.Request{}
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKeyFromObject(app),
	}}
}
