package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "builder/api/v1alpha1"
	"builder/pkg/mqtt"
)

// NetworkFunctionReconciler reconciles a NetworkFunction object
type NetworkFunctionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	MQTT   *mqtt.MQTTService
}

// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=networkfunctions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=networkfunctions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=networkfunctions/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NetworkFunction object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *NetworkFunctionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	const finalizerName = "cache.desire6g.eu/finalizer"

	logger.Info("Reconciling NetworkFunction", "request", req)
	var nf cachev1alpha1.NetworkFunction
	if err := r.Get(ctx, req.NamespacedName, &nf); err != nil {
		logger.Error(err, "unable to fetch NetworkFunction")
		// Service chain was deleted.
		logger.Error(err, "unable to fetch NetworkFunction")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if being deleted
	if !nf.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion
		if containsString(nf.GetFinalizers(), finalizerName) {
			if err := r.MQTT.DeleteNetworkFunctionDefinition(&nf); err != nil {
				logger.Error(err, "Failed to publish deletion event")
				return ctrl.Result{}, err // retry
			}

			// Remove finalizer
			nf.SetFinalizers(removeString(nf.GetFinalizers(), finalizerName))
			if err := r.Update(ctx, &nf); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing
	if !containsString(nf.GetFinalizers(), finalizerName) {
		nf.SetFinalizers(append(nf.GetFinalizers(), finalizerName))
		if err := r.Update(ctx, &nf); err != nil {
			return ctrl.Result{}, err
		}
	}

	// All is well
	// TODO: Announce creation to MQTT
	r.MQTT.UpdateNetworkFunctionDefinition(&nf)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.NetworkFunction{}).
		Named("networkfunction").
		Complete(r)
}
