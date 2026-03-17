package util

import (
	"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	corev1alpha1 "loom/api/core/v1alpha1"
	infrav1alpha1 "loom/api/infra/v1alpha1"
	"loom/pkg/util/ptr"
)

func EnsureBMv2DataPlaneContainer(bmv2Target *infrav1alpha1.BMv2Target,
	containers []corev1.Container) []corev1.Container {
	if containers == nil {
		containers = []corev1.Container{}
	}
	containerIndex := -1
	for i, container := range containers {
		if container.Name == infrav1alpha1.BMV2_DATAPLANE_CONTAINER_NAME {
			containerIndex = i
			break
		}
	}
	if containerIndex == -1 {
		containers = append(containers, corev1.Container{})
		containerIndex = len(containers) - 1
	}
	container := &containers[containerIndex]
	container.Name = infrav1alpha1.BMV2_DATAPLANE_CONTAINER_NAME
	container.Image = infrav1alpha1.BMV2_DATAPLANE_CONTAINER_IMAGE
	return containers
}

func EnsureBMv2DriverContainer(bmv2Target *infrav1alpha1.BMv2Target,
	containers []corev1.Container) []corev1.Container {
	if containers == nil {
		containers = []corev1.Container{}
	}
	containerIndex := -1
	for i, container := range containers {
		if container.Name == infrav1alpha1.BMV2_CONTROLPLANE_CONTAINER_NAME {
			containerIndex = i
			break
		}
	}
	if containerIndex == -1 {
		containers = append(containers, corev1.Container{})
		containerIndex = len(containers) - 1
	}
	container := &containers[containerIndex]
	container.Name = infrav1alpha1.BMV2_CONTROLPLANE_CONTAINER_NAME
	container.Image = infrav1alpha1.BMV2_CONTROLPLANE_CONTAINER_IMAGE
	if container.Ports == nil {
		container.Ports = []corev1.ContainerPort{}
	}
	containerPortExists := false
	for i, port := range container.Ports {
		if port.Name == "health" {
			container.Ports[i].ContainerPort = infrav1alpha1.BMV2_CONTROLPLANE_READY_PROBE_PORT
			containerPortExists = true
			break
		}
	}
	if !containerPortExists {
		container.Ports = append(container.Ports, corev1.ContainerPort{
			Name:          "health",
			ContainerPort: infrav1alpha1.BMV2_CONTROLPLANE_READY_PROBE_PORT,
		})
	}
	if container.ReadinessProbe == nil {
		container.ReadinessProbe = &corev1.Probe{}
	}
	container.ReadinessProbe.ProbeHandler.HTTPGet = &corev1.HTTPGetAction{
		Path: infrav1alpha1.BMV2_CONTROLPLANE_READY_PROBE_PATH,
		Port: intstr.FromInt32(infrav1alpha1.BMV2_CONTROLPLANE_READY_PROBE_PORT),
	}
	container.ReadinessProbe.InitialDelaySeconds = 5
	container.ReadinessProbe.PeriodSeconds = 5

	return containers
}

func EnsureBMv2DeploymentSpec(bmv2Target *infrav1alpha1.BMv2Target,
	spec *appsv1.DeploymentSpec) *appsv1.DeploymentSpec {
	if spec == nil {
		spec = &appsv1.DeploymentSpec{}
	}
	spec.Replicas = ptr.To[int32](1)
	spec.Selector = &metav1.LabelSelector{
		MatchLabels: GetBMv2PodTemplateLabels(bmv2Target),
	}
	spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: *EnsureBMv2PodMeta(bmv2Target, &spec.Template.ObjectMeta),
		Spec:       *EnsureBMv2PodSpec(bmv2Target, &spec.Template.Spec),
	}
	return spec
}

func EnsureBMv2PodMeta(bmv2Target *infrav1alpha1.BMv2Target,
	meta *metav1.ObjectMeta) *metav1.ObjectMeta {
	if meta == nil {
		meta = &metav1.ObjectMeta{}
	}
	meta.Labels = GetBMv2PodTemplateLabels(bmv2Target)
	meta.Annotations = GetBMv2PodTemplateAnnotations(bmv2Target)
	return meta
}

func EnsureBMv2PodSpec(bmv2Target *infrav1alpha1.BMv2Target,
	spec *corev1.PodSpec) *corev1.PodSpec {
	if spec == nil {
		spec = &corev1.PodSpec{}
	}
	if spec.Containers == nil {
		spec.Containers = []corev1.Container{}
	}
	spec.Containers = EnsureBMv2DriverContainer(bmv2Target, spec.Containers)
	spec.Containers = EnsureBMv2DataPlaneContainer(bmv2Target, spec.Containers)
	return spec
}

func EnsureBMv2DeploymentLabels(bmv2Target *infrav1alpha1.BMv2Target,
	labels map[string]string) map[string]string {
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL] = bmv2Target.Name
	return labels
}

func EnsureBMv2DeploymentAnnotations(bmv2Target *infrav1alpha1.BMv2Target,
	annotations map[string]string) map[string]string {
	if annotations == nil {
		annotations = make(map[string]string)
	}
	return annotations
}

func EnsureBMv2DeploymentFinalizers(bmv2Target *infrav1alpha1.BMv2Target,
	finalizers []string) []string {
	return finalizers
}

type CNIConfig struct {
	Name    string  `json:"name"`
	CNIArgs CNIArgs `json:"cni-args"`
}

type CNIArgs struct {
	AppType    string `json:"app_type"`
	TargetName string `json:"target_name"`
}

func (c CNIConfig) String() string {
	data, _ := json.Marshal(c)
	return string(data)
}

func NewCNIConfigForTarget(bmv2Target *infrav1alpha1.BMv2Target) CNIConfig {
	return CNIConfig{
		Name: "iml-cni",
		CNIArgs: CNIArgs{
			AppType:    "p4target",
			TargetName: bmv2Target.Name,
		},
	}
}

func GetBMv2PodTemplateAnnotations(bmv2Target *infrav1alpha1.BMv2Target) map[string]string {
	return map[string]string{
		"k8s.v1.cni.cncf.io/networks": "[" + NewCNIConfigForTarget(bmv2Target).String() + "]",
	}
}

func GetBMv2PodTemplateLabels(bmv2Target *infrav1alpha1.BMv2Target) map[string]string {
	return map[string]string{
		infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL: bmv2Target.Name,
	}
}

func EnsureP4TargetLabels(bmv2Target *infrav1alpha1.BMv2Target,
	labels map[string]string) map[string]string {
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[infrav1alpha1.BMV2_TARGET_DEPLOYMENT_LABEL] = bmv2Target.Name
	return labels
}

func EnsureP4TargetAnnotations(bmv2Target *infrav1alpha1.BMv2Target,
	annotations map[string]string) map[string]string {
	if annotations == nil {
		annotations = make(map[string]string)
	}
	return annotations
}

func EnsureP4TargetFinalizers(bmv2Target *infrav1alpha1.BMv2Target,
	finalizers []string) []string {
	return finalizers
}

func EnsureP4TargetSpec(bmv2Target *infrav1alpha1.BMv2Target,
	spec *corev1alpha1.P4TargetSpec) *corev1alpha1.P4TargetSpec {
	if spec == nil {
		spec = &corev1alpha1.P4TargetSpec{}
	}
	return spec
}

func NewReadyCondition(status metav1.ConditionStatus, reason, message string) infrav1alpha1.BMv2TargetCondition {
	return NewBMv2TargetCondition(infrav1alpha1.BMv2TargetConditionReady, status, reason, message)
}

func RemoveReadyCondition(bmv2Target *infrav1alpha1.BMv2Target) []infrav1alpha1.BMv2TargetCondition {
	return RemoveBMv2TargetCondition(bmv2Target, infrav1alpha1.BMv2TargetConditionReady)
}

func NewBMv2TargetCondition(conditionType infrav1alpha1.BMv2TargetConditionType,
	status metav1.ConditionStatus, reason, message string) infrav1alpha1.BMv2TargetCondition {
	return infrav1alpha1.BMv2TargetCondition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

func GetBMv2TargetCondition(bmv2Target *infrav1alpha1.BMv2Target,
	conditionType infrav1alpha1.BMv2TargetConditionType) *infrav1alpha1.BMv2TargetCondition {
	for i := range bmv2Target.Status.Conditions {
		if bmv2Target.Status.Conditions[i].Type == conditionType {
			return &bmv2Target.Status.Conditions[i]
		}
	}
	return nil
}

func CopyBMv2TargetConditions(bmv2Target *infrav1alpha1.BMv2Target) []infrav1alpha1.BMv2TargetCondition {
	newConditions := make([]infrav1alpha1.BMv2TargetCondition, len(bmv2Target.Status.Conditions))
	for i := range bmv2Target.Status.Conditions {
		newConditions = append(newConditions, bmv2Target.Status.Conditions[i])
	}
	return newConditions
}

func RemoveBMv2TargetCondition(bmv2Target *infrav1alpha1.BMv2Target,
	conditionType infrav1alpha1.BMv2TargetConditionType) []infrav1alpha1.BMv2TargetCondition {
	newConditions := make([]infrav1alpha1.BMv2TargetCondition, 0)
	conditions := bmv2Target.Status.Conditions
	for i := range conditions {
		if conditions[i].Type == conditionType {
			newConditions = append(conditions[:i], conditions[i+1:]...)
		}
	}
	return newConditions
}

func UpdateBMv2TargetCondition(bmv2Target *infrav1alpha1.BMv2Target,
	newCondition infrav1alpha1.BMv2TargetCondition) []infrav1alpha1.BMv2TargetCondition {
	existingCondition := GetBMv2TargetCondition(bmv2Target, newCondition.Type)
	if existingCondition != nil && existingCondition.Status == newCondition.Status {
		return CopyBMv2TargetConditions(bmv2Target) // If the status hasn't changed, we don't need to update the LastTransitionTime
	}
	newConditions := RemoveBMv2TargetCondition(bmv2Target, newCondition.Type)
	newConditions = append(newConditions, newCondition)
	return newConditions
}
