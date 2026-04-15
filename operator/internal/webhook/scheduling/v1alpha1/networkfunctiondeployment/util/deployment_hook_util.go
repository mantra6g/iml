package util

import (
	schedulingv1alpha1 "github.com/mantra6g/iml/operator/api/scheduling/v1alpha1"

	"k8s.io/apimachinery/pkg/util/intstr"
)

// EnsureNonNilStrategy sets a non-nil RollingUpdate DeploymentStrategy if the provided strategy is nil
func EnsureNonNilStrategy(strategy *schedulingv1alpha1.DeploymentStrategy) *schedulingv1alpha1.DeploymentStrategy {
	if strategy == nil {
		return &schedulingv1alpha1.DeploymentStrategy{
			Type: schedulingv1alpha1.DefaultDeploymentStrategyType,
		}
	}
	return strategy
}

// SetRollingUpdateDefaults returns a RollingUpdateDeployment with default values if they are not already set
func SetRollingUpdateDefaults(
	rollingUpdate *schedulingv1alpha1.RollingUpdateDeployment,
) *schedulingv1alpha1.RollingUpdateDeployment {
	defaultMaxUnavailable := intstr.FromString(schedulingv1alpha1.DefaultRollingUpdateMaxUnavailable)
	defaultMaxSurge := intstr.FromString(schedulingv1alpha1.DefaultRollingUpdateMaxSurge)
	if rollingUpdate == nil {
		return &schedulingv1alpha1.RollingUpdateDeployment{
			MaxUnavailable: &defaultMaxUnavailable,
			MaxSurge:       &defaultMaxSurge,
		}
	}
	if rollingUpdate.MaxUnavailable == nil {
		rollingUpdate.MaxUnavailable = &defaultMaxUnavailable
	}
	if rollingUpdate.MaxSurge == nil {
		rollingUpdate.MaxSurge = &defaultMaxSurge
	}
	return rollingUpdate
}
