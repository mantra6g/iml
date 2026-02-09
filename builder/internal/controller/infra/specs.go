package infra

import (
	corev1alpha1 "builder/api/core/v1alpha1"
	infrav1alpha1 "builder/api/infra/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func desiredDeploymentSpec(targetDeployment *infrav1alpha1.P4TargetDeployment, targetName string) appsv1.DeploymentSpec {
	// In here, we set up a Deployment spec for the P4 target.
	// This deployment spec needs to have all its fields set as they will
	// be compared in the CreateOrPatch call in ensureReplica().
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
			MatchExpressions: []metav1.LabelSelectorRequirement{},
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

func ensureDeployment(current *appsv1.Deployment, targetDeployment *infrav1alpha1.P4TargetDeployment, targetName string) {
	// Spec.Replicas
	current.Spec.Replicas = ptr.To(int32(1))

	// Spec.Selector
	if current.Spec.Selector == nil {
		current.Spec.Selector = &metav1.LabelSelector{
			MatchLabels:      map[string]string{},
			MatchExpressions: []metav1.LabelSelectorRequirement{},
		}
	}
	if current.Spec.Selector.MatchLabels == nil {
		current.Spec.Selector.MatchLabels = map[string]string{}
	}
	current.Spec.Selector.MatchLabels[corev1alpha1.TARGET_LABEL] = targetName
	current.Spec.Selector.MatchLabels[infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL] = targetDeployment.Name

	// Spec.Template.ObjectMeta
	if current.Spec.Template.ObjectMeta.Labels == nil {
		current.Spec.Template.ObjectMeta.Labels = map[string]string{}
	}
	current.Spec.Template.ObjectMeta.Labels[corev1alpha1.TARGET_LABEL] = targetName
	current.Spec.Template.ObjectMeta.Labels[infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL] = targetDeployment.Name
	if current.Spec.Template.ObjectMeta.Annotations == nil {
		current.Spec.Template.ObjectMeta.Annotations = map[string]string{}
	}
	current.Spec.Template.ObjectMeta.Annotations["k8s.v1.cni.cncf.io/networks"] = "[" + CNIConfig{
		Name: "iml-cni",
		CNIArgs: CNIArgs{
			AppType:  "p4-target",
			TargetID: string(targetDeployment.UID),
		},
	}.String() + "]"

	// Spec.Template.Spec.Containers
	if current.Spec.Template.Spec.Containers == nil || len(current.Spec.Template.Spec.Containers) != 2 {
		current.Spec.Template.Spec.Containers = make([]corev1.Container, 2)
	}
	current.Spec.Template.Spec.Containers[0].Name = infrav1alpha1.BMV2_CONTROLPLANE_CONTAINER_NAME
	current.Spec.Template.Spec.Containers[0].Image = infrav1alpha1.BMV2_CONTROLPLANE_CONTAINER_IMAGE
	current.Spec.Template.Spec.Containers[0].Image = infrav1alpha1.BMV2_CONTROLPLANE_CONTAINER_IMAGE
	if current.Spec.Template.Spec.Containers[0].ReadinessProbe == nil {
		current.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{}
	}
	if current.Spec.Template.Spec.Containers[0].ReadinessProbe.ProbeHandler.HTTPGet == nil {
		current.Spec.Template.Spec.Containers[0].ReadinessProbe.ProbeHandler.HTTPGet = &corev1.HTTPGetAction{}
	}
	current.Spec.Template.Spec.Containers[0].ReadinessProbe.ProbeHandler.HTTPGet.Path =
		infrav1alpha1.BMV2_CONTROLPLANE_READY_PROBE_PATH
	current.Spec.Template.Spec.Containers[0].ReadinessProbe.ProbeHandler.HTTPGet.Port =
		intstr.FromInt32(infrav1alpha1.BMV2_CONTROLPLANE_READY_PROBE_PORT)
	current.Spec.Template.Spec.Containers[0].ReadinessProbe.InitialDelaySeconds = 5
	current.Spec.Template.Spec.Containers[0].ReadinessProbe.PeriodSeconds = 5

	current.Spec.Template.Spec.Containers[1].Name = infrav1alpha1.BMV2_DATAPLANE_CONTAINER_NAME
	current.Spec.Template.Spec.Containers[1].Image = infrav1alpha1.BMV2_DATAPLANE_CONTAINER_IMAGE
}
