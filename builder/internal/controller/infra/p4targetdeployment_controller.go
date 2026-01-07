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

package infra

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "builder/api/core/v1alpha1"
	infrav1alpha1 "builder/api/infra/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type CNIArgs struct {
	AppType  string
	TargetID string
}

type CNIConfig struct {
	Name    string
	CNIArgs CNIArgs
}

func (c CNIConfig) String() string {
	return `{"name":"` + c.Name + `","cni-args":{"app_type":"` + c.CNIArgs.AppType + `","target_id":"` + c.CNIArgs.TargetID + `"}}`
}

// P4TargetDeploymentReconciler reconciles a P4TargetDeployment object
type P4TargetDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infra.desire6g.eu,resources=p4targetdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.desire6g.eu,resources=p4targetdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.desire6g.eu,resources=p4targetdeployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.desire6g.eu,resources=p4targets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the P4TargetDeployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *P4TargetDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	deployment := &infrav1alpha1.P4TargetDeployment{}
	err := r.Get(ctx, req.NamespacedName, deployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("P4TargetDeployment resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch P4TargetDeployment")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if deployment.Spec.Replicas == nil {
		// decide default behavior
		return ctrl.Result{}, fmt.Errorf("replicas must be set")
	}
	replicas := int(*deployment.Spec.Replicas)
	parentName := deployment.Name

	for i := 0; i < replicas; i++ {
		targetName := fmt.Sprintf("%s-%d", parentName, i)

		// Create the ReplicaSet for the P4 Target's pod
		replicaset := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: infrav1alpha1.BMV2_POD_NAMESPACE,
				Labels: map[string]string{
					corev1alpha1.TARGET_LABEL:                  targetName,
					infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: parentName,
				},
			},
		}
		result, err := controllerutil.CreateOrUpdate(ctx, r.Client, replicaset, func() error {
			replicaset.Spec = r.desiredReplicaSetSpec(deployment, targetName)
			if replicaset.Labels == nil {
				replicaset.Labels = map[string]string{}
			}
			replicaset.Labels[corev1alpha1.TARGET_LABEL] = targetName
			replicaset.Labels[infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL] = parentName
			return controllerutil.SetControllerReference(deployment, replicaset, r.Scheme)
		})
		if err != nil {
			logger.Error(err, "Failed to create or update P4Target's containerized deployment", "name", deployment.ObjectMeta.Name)
			return ctrl.Result{}, err
		}
		if result != controllerutil.OperationResultNone {
			logger.Info("Changed P4Target's containerized deployment", "name", replicaset.ObjectMeta.Name)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// Create the P4Target resource
		target := &corev1alpha1.P4Target{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetName,
				Labels: map[string]string{
					corev1alpha1.TARGET_LABEL:                  targetName, // helpful label for listing
					infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: parentName, // helpful label for listing
				},
			},
		}
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, target, func() error {
			target.Spec = r.desiredP4TargetSpec(deployment)
			// set labels again in mutate so they are persisted
			if target.Labels == nil {
				target.Labels = map[string]string{}
			}
			target.Labels[corev1alpha1.TARGET_LABEL] = targetName
			target.Labels[infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL] = parentName
			return controllerutil.SetControllerReference(deployment, target, r.Scheme)
		})
		if err != nil {
			logger.Error(err, "Failed to create or update P4Target", "name", targetName)
			return ctrl.Result{}, err
		}
	}

	var list corev1alpha1.P4TargetList
	if err := r.List(ctx, &list,
		client.MatchingLabels(map[string]string{infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: parentName}),
	); err != nil {
		logger.Error(err, "failed to list p4targets for cleanup")
		return ctrl.Result{}, err
	}

	for _, t := range list.Items {
		// Parse suffix from name
		suffix := strings.TrimPrefix(t.Name, parentName+"-")
		idx, err := strconv.Atoi(suffix)
		if err != nil {
			// can't determine index — skip or log and continue
			logger.Info("could not parse index, skipping", "p4target", t.Name)
			continue
		}
		if idx >= replicas {
			if err := r.Delete(ctx, &t); err != nil && !apierrors.IsNotFound(err) {
				logger.Error(err, "failed to delete extra P4Target", "name", t.Name)
				return ctrl.Result{}, err
			}
			logger.Info("deleted extra P4Target", "name", t.Name)
		}
	}

	// Reconciliation finished, now obtain status
	var targets corev1alpha1.P4TargetList
	err = r.List(ctx, &targets,
		client.MatchingLabels{infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: parentName},
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	observed := int32(len(targets.Items))

	var ready int32
	for _, t := range targets.Items {
		if t.Status.Ready {
			ready++
		}
	}

	statusChanged := false

	if deployment.Status.ObservedReplicas != observed {
		deployment.Status.ObservedReplicas = observed
		statusChanged = true
	}

	if deployment.Status.ReadyReplicas != ready {
		deployment.Status.ReadyReplicas = ready
		statusChanged = true
	}

	if statusChanged {
		err := r.Status().Update(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *P4TargetDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.P4TargetDeployment{}).
		Owns(&corev1alpha1.P4Target{}).
		Owns(&appsv1.ReplicaSet{}).
		Named("infra-p4targetdeployment").
		Complete(r)
}

func (r *P4TargetDeploymentReconciler) desiredReplicaSetSpec(targetDeployment *infrav1alpha1.P4TargetDeployment, targetName string) appsv1.ReplicaSetSpec {
	replicas := int32(1)

	return appsv1.ReplicaSetSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				corev1alpha1.TARGET_LABEL:                  targetName,
				infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: targetDeployment.Name,
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					corev1alpha1.TARGET_LABEL:                  targetName,
					infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: targetDeployment.Name,
				},
				Annotations: map[string]string{
					"k8s.v1.cni.cncf.io/networks": "[" + CNIConfig{
						Name: "iml-cni",
						CNIArgs: CNIArgs{
							AppType:  "p4-target",
							TargetID: string(targetDeployment.UID),
						},
					}.String() + "]",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  infrav1alpha1.BMV2_CONTROLPLANE_CONTAINER_NAME,
						Image: infrav1alpha1.BMV2_CONTROLPLANE_CONTAINER_IMAGE,
						Ports: []corev1.ContainerPort{
							{
								Name:          "health",
								ContainerPort: infrav1alpha1.BMV2_CONTROLPLANE_READY_PROBE_PORT,
							},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: infrav1alpha1.BMV2_CONTROLPLANE_READY_PROBE_PATH,
									Port: intstr.FromInt32(infrav1alpha1.BMV2_CONTROLPLANE_READY_PROBE_PORT),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       5,
						},
					},
					{
						Name:  infrav1alpha1.BMV2_DATAPLANE_CONTAINER_NAME,
						Image: infrav1alpha1.BMV2_DATAPLANE_CONTAINER_IMAGE,
					},
				},
			},
		},
	}
}

func (r *P4TargetDeploymentReconciler) desiredP4TargetSpec(targetDeployment *infrav1alpha1.P4TargetDeployment) corev1alpha1.P4TargetSpec {
	return corev1alpha1.P4TargetSpec{
		TargetClass: targetDeployment.Spec.Class,
	}
}
