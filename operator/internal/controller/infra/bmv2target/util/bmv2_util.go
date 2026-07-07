package util

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"
	"github.com/mantra6g/iml/operator/pkg/util/ptr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultBMv2ControlPlaneContainerName = "bmv2-driver"
	DefaultBMv2DataPlaneContainerName    = "bmv2-switch"
)

type BMv2Config struct {
	ControlPlaneContainerName string
	ControlPlaneImage         string
	DataPlaneContainerName    string
	DataPlaneImage            string
	PodNamespace              string
	DriverServiceAccount      string
}

func ParseBMv2ConfigFromPath(path string) (*BMv2Config, error) {
	get := func(key string) (string, error) {
		data, err := os.ReadFile(filepath.Join(path, key))
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	controlPlaneContainerName, err := get("bmv2-control-plane-container-name")
	if controlPlaneContainerName == "" {
		controlPlaneContainerName = DefaultBMv2ControlPlaneContainerName
	}
	controlPlaneImage, err := get("bmv2-control-plane-image")
	if err != nil {
		return nil, fmt.Errorf("required configuration \"bmv2-control-plane-image\" is missing or empty")
	}
	dataPlaneContainerName, err := get("bmv2-data-plane-container-name")
	if dataPlaneContainerName == "" {
		dataPlaneContainerName = DefaultBMv2DataPlaneContainerName
	}
	dataPlaneImage, err := get("bmv2-data-plane-image")
	if err != nil {
		return nil, fmt.Errorf("required configuration \"bmv2-data-plane-image\" is missing or empty")
	}
	podNamespace, err := get("bmv2-pod-namespace")
	if err != nil {
		return nil, fmt.Errorf("required configuration \"bmv2-pod-namespace\" is missing or empty")
	}
	driverServiceAccount, err := get("bmv2-driver-service-account-name")
	if err != nil {
		return nil, fmt.Errorf("required configuration \"bmv2-driver-service-account-name\" is missing or empty")
	}
	return &BMv2Config{
		ControlPlaneContainerName: controlPlaneContainerName,
		ControlPlaneImage:         controlPlaneImage,
		DataPlaneContainerName:    dataPlaneContainerName,
		DataPlaneImage:            dataPlaneImage,
		PodNamespace:              podNamespace,
		DriverServiceAccount:      driverServiceAccount,
	}, nil
}

func EnsureBMv2DataPlaneContainer(bmv2Target *infrav1alpha1.BMv2Target,
	containers []corev1.Container, cfg *BMv2Config) []corev1.Container {
	if containers == nil {
		containers = []corev1.Container{}
	}
	containerIndex := -1
	for i, container := range containers {
		if container.Name == cfg.DataPlaneContainerName {
			containerIndex = i
			break
		}
	}
	if containerIndex == -1 {
		containers = append(containers, corev1.Container{})
		containerIndex = len(containers) - 1
	}
	container := &containers[containerIndex]
	container.Name = cfg.DataPlaneContainerName
	container.Image = cfg.DataPlaneImage
	container.ImagePullPolicy = corev1.PullIfNotPresent
	if container.SecurityContext == nil {
		container.SecurityContext = &corev1.SecurityContext{}
	}
	if container.SecurityContext.Capabilities == nil {
		container.SecurityContext.Capabilities = &corev1.Capabilities{}
	}
	container.SecurityContext.Capabilities.Add = []corev1.Capability{"NET_RAW"}
	container.Command = []string{"simple_switch_grpc"}
	container.Args = []string{
		"--no-p4",
		"--log-console",
		"-i",
		"0@nf0",
		"--",
		"--grpc-server-addr=0.0.0.0:9559",
		"--cpu-port=255",
	}
	return containers
}

func EnsureBMv2DriverContainer(bmv2Target *infrav1alpha1.BMv2Target,
	containers []corev1.Container, cfg *BMv2Config) []corev1.Container {
	if containers == nil {
		containers = []corev1.Container{}
	}
	containerIndex := -1
	for i, container := range containers {
		if container.Name == cfg.ControlPlaneContainerName {
			containerIndex = i
			break
		}
	}
	if containerIndex == -1 {
		containers = append(containers, corev1.Container{})
		containerIndex = len(containers) - 1
	}
	container := &containers[containerIndex]
	container.Name = cfg.ControlPlaneContainerName
	container.Image = cfg.ControlPlaneImage
	container.ImagePullPolicy = corev1.PullIfNotPresent
	if container.SecurityContext == nil {
		container.SecurityContext = &corev1.SecurityContext{}
	}
	if container.SecurityContext.Capabilities == nil {
		container.SecurityContext.Capabilities = &corev1.Capabilities{}
	}
	container.SecurityContext.Capabilities.Add = []corev1.Capability{"NET_ADMIN"}
	container.Args = []string{
		"--p4target-name", bmv2Target.Name,
		"--max-nf-slots", "1",
	}
	container.Env = EnsureDriverEnvVars(bmv2Target, container.Env)
	return containers
}

func EnsureDriverEnvVars(bmv2Target *infrav1alpha1.BMv2Target, existing []corev1.EnvVar) []corev1.EnvVar {
	if existing == nil {
		existing = []corev1.EnvVar{}
	}
	newEnvVars := make([]corev1.EnvVar, len(existing))
	copy(newEnvVars, existing)
	newEnvVars = EnsurePodNameEnvVar(newEnvVars)
	newEnvVars = EnsurePodNamespaceEnvVar(newEnvVars)
	return newEnvVars
}

func EnsurePodNameEnvVar(existing []corev1.EnvVar) []corev1.EnvVar {
	var foundEnvVar *corev1.EnvVar
	for i := range existing {
		if existing[i].Name == "POD_NAME" {
			foundEnvVar = &existing[i]
		}
	}
	if foundEnvVar == nil {
		return append(existing, corev1.EnvVar{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		})
	}
	foundEnvVar.Value = ""
	if foundEnvVar.ValueFrom == nil {
		foundEnvVar.ValueFrom = &corev1.EnvVarSource{}
	}
	if foundEnvVar.ValueFrom.FieldRef == nil {
		foundEnvVar.ValueFrom.FieldRef = &corev1.ObjectFieldSelector{}
	}
	foundEnvVar.ValueFrom.FieldRef.FieldPath = "metadata.name"
	return existing
}

func EnsurePodNamespaceEnvVar(existing []corev1.EnvVar) []corev1.EnvVar {
	var foundEnvVar *corev1.EnvVar
	for i := range existing {
		if existing[i].Name == "POD_NAMESPACE" {
			foundEnvVar = &existing[i]
		}
	}
	if foundEnvVar == nil {
		return append(existing, corev1.EnvVar{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		})
	}
	foundEnvVar.Value = ""
	if foundEnvVar.ValueFrom == nil {
		foundEnvVar.ValueFrom = &corev1.EnvVarSource{}
	}
	if foundEnvVar.ValueFrom.FieldRef == nil {
		foundEnvVar.ValueFrom.FieldRef = &corev1.ObjectFieldSelector{}
	}
	foundEnvVar.ValueFrom.FieldRef.FieldPath = "metadata.namespace"
	return existing
}

func EnsureBMv2DeploymentSpec(bmv2Target *infrav1alpha1.BMv2Target,
	spec *appsv1.DeploymentSpec, cfg *BMv2Config) *appsv1.DeploymentSpec {
	if spec == nil {
		spec = &appsv1.DeploymentSpec{}
	}
	spec.Replicas = ptr.To[int32](1)
	spec.Selector = &metav1.LabelSelector{
		MatchLabels: GetBMv2PodTemplateLabels(bmv2Target),
	}
	spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: *EnsureBMv2PodMeta(bmv2Target, &spec.Template.ObjectMeta),
		Spec:       *EnsureBMv2PodSpec(bmv2Target, &spec.Template.Spec, cfg),
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
	spec *corev1.PodSpec, cfg *BMv2Config) *corev1.PodSpec {
	if spec == nil {
		spec = &corev1.PodSpec{}
	}
	spec.ServiceAccountName = cfg.DriverServiceAccount
	if spec.Containers == nil {
		spec.Containers = []corev1.Container{}
	}
	spec.Containers = EnsureBMv2DriverContainer(bmv2Target, spec.Containers, cfg)
	spec.Containers = EnsureBMv2DataPlaneContainer(bmv2Target, spec.Containers, cfg)
	return spec
}

func EnsureBMv2DeploymentLabels(bmv2Target *infrav1alpha1.BMv2Target,
	labels map[string]string) map[string]string {
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[infrav1alpha1.BMv2TargetLabel] = bmv2Target.Name
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
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	CNIArgs   CNIArgs `json:"cni-args"`
}

type CNIArgs struct {
	AppType      string `json:"app_type"`
	TargetName   string `json:"target_name"`
	NFInterfaces uint8  `json:"nf_interfaces"`
}

func (c CNIConfig) String() string {
	data, _ := json.Marshal(c)
	return string(data)
}

func NewCNIConfigForTarget(bmv2Target *infrav1alpha1.BMv2Target) CNIConfig {
	return CNIConfig{
		Name:      "loom-cni",
		Namespace: "loom-system",
		CNIArgs: CNIArgs{
			AppType:      "p4target",
			TargetName:   bmv2Target.Name,
			NFInterfaces: 1,
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
		infrav1alpha1.BMv2TargetLabel: bmv2Target.Name,
	}
}

func EnsureP4TargetLabels(bmv2Target *infrav1alpha1.BMv2Target,
	labels map[string]string) map[string]string {
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[infrav1alpha1.BMv2TargetLabel] = bmv2Target.Name
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
	conditions := bmv2Target.Status.Conditions

	newConditions := make([]infrav1alpha1.BMv2TargetCondition, len(conditions))
	copy(newConditions, conditions)

	return newConditions
}

func RemoveBMv2TargetCondition(bmv2Target *infrav1alpha1.BMv2Target,
	conditionType infrav1alpha1.BMv2TargetConditionType) []infrav1alpha1.BMv2TargetCondition {
	newConditions := make([]infrav1alpha1.BMv2TargetCondition, 0)
	for _, cond := range bmv2Target.Status.Conditions {
		if cond.Type != conditionType {
			newConditions = append(newConditions, cond)
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
