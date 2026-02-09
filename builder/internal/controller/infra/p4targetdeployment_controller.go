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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "builder/api/core/v1alpha1"
	infrav1alpha1 "builder/api/infra/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const BATCH_SIZE = 5

type TargetReplica struct {
	TargetName    string
	PodDeployment *appsv1.Deployment
	P4Target      *corev1alpha1.P4Target
}

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
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete

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

	// Create the namespace where the bmv2 target pods will be deployed, if it doesn't exist already
	infraNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: infrav1alpha1.BMV2_POD_NAMESPACE,
		},
	}

	foundNs := &corev1.Namespace{}
	err = r.Get(ctx, types.NamespacedName{Name: infrav1alpha1.BMV2_POD_NAMESPACE}, foundNs)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating the infra namespace")
		return ctrl.Result{}, r.Create(ctx, infraNs)
	}

	replicas := make(map[int]TargetReplica)
	err = r.buildTargetReplicaMap(ctx, deployment, replicas)
	if err != nil {
		logger.Error(err, "failed to list existing target replicas")
		return ctrl.Result{}, err
	}

	desiredReplicas := int(*deployment.Spec.Replicas)
	changedReplicas := 0
	for i := range desiredReplicas {
		existingReplica, _ := replicas[i] // if not found, zero value is fine
		changed, err := r.ensureReplica(ctx, deployment, i, existingReplica)
		if err != nil {
			logger.Error(err, "failed to ensure replica", "index", i)
			return ctrl.Result{}, err
		}
		if changed {
			changedReplicas++
		}
		if changedReplicas >= BATCH_SIZE {
			logger.Info("processed batch of replicas, requeuing", "batchSize", BATCH_SIZE)
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
	}
	if changedReplicas > 0 {
		logger.Info("modified replicas, requeuing to verify results", "changedReplicas", changedReplicas)
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	deletedReplicas := 0
	for i, extraReplica := range replicas {
		if i < desiredReplicas {
			continue
		}
		err := r.deleteReplica(ctx, extraReplica)
		if err != nil {
			logger.Error(err, "failed to delete extra replica", "index", i)
			return ctrl.Result{}, err
		}
		deletedReplicas++
		if deletedReplicas >= BATCH_SIZE {
			logger.Info("processed batch of replica deletions, requeuing", "batchSize", BATCH_SIZE)
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
	}
	if deletedReplicas > 0 {
		logger.Info("deleted replicas, requeuing to verify results", "deletedReplicas", deletedReplicas)
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Reconciliation finished, now obtain status
	observed := int32(len(replicas))
	ready := int32(0)
	for _, replica := range replicas {
		if replica.P4Target.Status.Ready {
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
		Owns(&appsv1.Deployment{}).
		Named("infra-p4targetdeployment").
		Complete(r)
}

func (r *P4TargetDeploymentReconciler) desiredDeploymentSpec(targetDeployment *infrav1alpha1.P4TargetDeployment, targetName string) appsv1.DeploymentSpec {
	// In here, we set up a Deployment spec for the P4 target.
	// This deployment spec needs to have all its fields set as they will
	// be compared in the CreateOrUpdate call in ensureReplica().
	// If a field is left nil/zero, it will cause unnecessary updates and the
	// Reconcile loop to run indefinitely.
	replicas := int32(1)

	return appsv1.DeploymentSpec{
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
		TargetClass: targetDeployment.Spec.Template.TargetClass,
	}
}

func (r *P4TargetDeploymentReconciler) buildTargetReplicaMap(ctx context.Context, deployment *infrav1alpha1.P4TargetDeployment, replicas map[int]TargetReplica) error {
	logger := logf.FromContext(ctx)

	var targetList corev1alpha1.P4TargetList
	if err := r.List(ctx, &targetList,
		client.MatchingLabels{infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: deployment.Name},
	); err != nil {
		logger.Error(err, "failed to list p4targets for deployment", "deployment", deployment.Name)
		return err
	}

	var deploymentList appsv1.DeploymentList
	if err := r.List(ctx, &deploymentList,
		client.MatchingLabels{infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: deployment.Name},
		client.InNamespace(infrav1alpha1.BMV2_POD_NAMESPACE),
	); err != nil {
		logger.Error(err, "failed to list deployments for deployment", "deployment", deployment.Name)
		return err
	}

	for _, t := range targetList.Items {
		index, exists := t.Labels[infrav1alpha1.BMV2_TARGET_REPLICA_INDEX_LABEL]
		if !exists {
			logger.V(1).Info("skipping target without replica index label", "target", t.Name)
			continue
		}
		idx, err := strconv.Atoi(index)
		if err != nil {
			logger.V(1).Info("skipping target with invalid replica index label", "target", t.Name, "index", index)
			continue
		}
		replicas[idx] = TargetReplica{
			P4Target: &t,
		}
	}

	for _, rs := range deploymentList.Items {
		index, exists := rs.Labels[infrav1alpha1.BMV2_TARGET_REPLICA_INDEX_LABEL]
		if !exists {
			logger.V(1).Info("skipping deployment without replica index label", "deployment", rs.Name)
			continue
		}
		idx, err := strconv.Atoi(index)
		if err != nil {
			logger.V(1).Info("skipping deployment with invalid replica index label", "deployment", rs.Name, "index", index)
			continue
		}
		replica, exists := replicas[idx]
		if !exists {
			replica = TargetReplica{}
		}
		replica.PodDeployment = &rs
		replicas[idx] = replica
	}

	return nil
}

func (r *P4TargetDeploymentReconciler) ensureReplica(ctx context.Context, deployment *infrav1alpha1.P4TargetDeployment, index int, replica TargetReplica) (bool, error) {
	logger := logf.FromContext(ctx)
	var replicaChanged bool
	var targetName string
	if replica.P4Target == nil {
		replica.P4Target = &corev1alpha1.P4Target{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-%d", deployment.Name, index),
			},
		}
	}
	targetResult, err := controllerutil.CreateOrPatch(ctx, r.Client, replica.P4Target, func() error {
		replica.P4Target.Spec = r.desiredP4TargetSpec(deployment)
		// set labels again in mutate so they are persisted
		if replica.P4Target.Labels == nil {
			replica.P4Target.Labels = map[string]string{}
		}
		replica.P4Target.Labels[infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL] = deployment.Name
		replica.P4Target.Labels[infrav1alpha1.BMV2_TARGET_REPLICA_INDEX_LABEL] = fmt.Sprintf("%d", index)
		return controllerutil.SetControllerReference(deployment, replica.P4Target, r.Scheme) // P4TargetDeployment owns P4Target
	})
	if err != nil {
		return replicaChanged, fmt.Errorf("failed to create or update P4Target: %w", err)
	}
	if targetResult != controllerutil.OperationResultNone {
		logger.V(1).Info("P4Target reconciled", "target", replica.P4Target.Name, "operation", targetResult)
		replicaChanged = true
	}

	targetName = replica.P4Target.Name
	if replica.PodDeployment == nil {
		replica.PodDeployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: infrav1alpha1.BMV2_POD_NAMESPACE,
			},
		}
	}
	deploymentResult, err := controllerutil.CreateOrPatch(ctx, r.Client, replica.PodDeployment, func() error {
		logger.V(1).Info("creating or updating deployment", "deployment", replica.PodDeployment)
		ensureDeployment(replica.PodDeployment, deployment, targetName)
		if replica.PodDeployment.Labels == nil {
			replica.PodDeployment.Labels = map[string]string{}
		}
		replica.PodDeployment.Labels[infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL] = deployment.Name
		replica.PodDeployment.Labels[infrav1alpha1.BMV2_TARGET_REPLICA_INDEX_LABEL] = fmt.Sprintf("%d", index)
		return controllerutil.SetControllerReference(replica.P4Target, replica.PodDeployment, r.Scheme) // P4Target owns Deployment
	})
	if err != nil {
		return replicaChanged, fmt.Errorf("failed to create or update Deployment: %w", err)
	}
	if deploymentResult != controllerutil.OperationResultNone {
		logger.V(1).Info("Deployment reconciled", "deployment", replica.P4Target.Name, "operation", deploymentResult)
		replicaChanged = true
	}

	return replicaChanged, nil
}

func (r *P4TargetDeploymentReconciler) deleteReplica(ctx context.Context, replica TargetReplica) error {
	// Only delete the P4Target, the Deployment will be garbage collected
	// as we've set the P4Target as owner of the Deployment.
	if replica.P4Target != nil {
		err := r.Delete(ctx, replica.P4Target)
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete P4Target: %w", err)
		}
	}
	return nil
}
