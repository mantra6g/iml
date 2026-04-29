package p4target

import (
	"context"
	"fmt"
	"iml-daemon/pkg/dataplane"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconciler reconciles a Node object
type Reconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Dataplane dataplane.Dataplane
}

// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	target := &corev1alpha1.P4Target{}
	err := r.Get(ctx, req.NamespacedName, target)
	if apierrors.IsNotFound(err) {
		logger.Info("P4Target resource not found. Deleting target routes", "target", req.NamespacedName)
		err = r.Dataplane.RemoveP4TargetRoutes(req.NamespacedName)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove P4Target routes: %w", err)
		}
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get p4target %s: %w", req.Name, err)
	}

	err = r.Dataplane.UpdateP4TargetRoutes(target)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update P4Target routes: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.P4Target{}).
		Named("p4target-daemon").
		Complete(r)
}
