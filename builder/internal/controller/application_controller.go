package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "builder/api/v1alpha1"
	"builder/services/mqtt"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	MQTT   *mqtt.MQTTService
}

// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=applications/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Application object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	const finalizerName = "cache.desire6g.eu/finalizer"

	logger.Info("Reconciling Application", "request", req)
	var app cachev1alpha1.Application
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		logger.Error(err, "unable to fetch Application")
		// Application was deleted.
		logger.Error(err, "unable to fetch Application")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if being deleted
	if !app.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion
		if containsString(app.GetFinalizers(), finalizerName) {
			if err := r.MQTT.UpdateAppDefinition(&app); err != nil {
				logger.Error(err, "Failed to publish deletion event")
				return ctrl.Result{}, err // retry
			}

			// Remove finalizer
			app.SetFinalizers(removeString(app.GetFinalizers(), finalizerName))
			if err := r.Update(ctx, &app); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing
	if !containsString(app.GetFinalizers(), finalizerName) {
		app.SetFinalizers(append(app.GetFinalizers(), finalizerName))
		if err := r.Update(ctx, &app); err != nil {
			return ctrl.Result{}, err
		}
	}

	// All is well
	// TODO: Announce creation to MQTT
	r.MQTT.UpdateAppDefinition(&app)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.Application{}).
		Named("application").
		Complete(r)
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
