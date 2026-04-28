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

package networkfunctiondeployment

import (
	"context"
	"fmt"
	"reflect"

	"github.com/mantra6g/iml/operator/pkg/util/ptr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	schedulingv1alpha1 "github.com/mantra6g/iml/api/scheduling/v1alpha1"
	deploymenthookutil "github.com/mantra6g/iml/operator/internal/webhook/scheduling/v1alpha1/networkfunctiondeployment/util"
)

// nolint:unused
// log is for logging in this package.
var logger = logf.Log.WithName("networkfunctiondeployment-resource")

// SetupNetworkFunctionDeploymentWebhookWithManager registers the webhook for NetworkFunctionDeployment in the manager.
func SetupNetworkFunctionDeploymentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &schedulingv1alpha1.NetworkFunctionDeployment{}).
		WithValidator(&CustomValidator{
			Client: mgr.GetClient(),
		}).WithDefaulter(&CustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-scheduling-loom-io-v1alpha1-networkfunctiondeployment,mutating=true,failurePolicy=fail,sideEffects=None,groups=scheduling.loom.io,resources=networkfunctiondeployments,verbs=create;update,versions=v1alpha1,name=mnetworkfunctiondeployment-v1alpha1.kb.io,admissionReviewVersions=v1

// CustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind NetworkFunctionDeployment when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type CustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ admission.Defaulter[*schedulingv1alpha1.NetworkFunctionDeployment] = &CustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind NetworkFunctionDeployment.
func (d *CustomDefaulter) Default(_ context.Context, deployment *schedulingv1alpha1.NetworkFunctionDeployment) error {
	logger.Info("Defaulting for NetworkFunctionDeployment",
		"name", deployment.GetName())

	// TODO(user): fill in your defaulting logic.
	// Set default replicas to 1 if not specified
	if deployment.Spec.Replicas == nil {
		deployment.Spec.Replicas = ptr.To[int32](1)
	}
	// Ensure the strategy field is non-nil and set defaults for rolling update if applicable
	deployment.Spec.Strategy = deploymenthookutil.EnsureNonNilStrategy(deployment.Spec.Strategy)
	// Set default rolling update parameters if the strategy type is RollingUpdate
	if deployment.Spec.Strategy.Type == schedulingv1alpha1.DeploymentStrategyTypeRollingUpdate {
		deployment.Spec.Strategy.RollingUpdate = deploymenthookutil.SetRollingUpdateDefaults(
			deployment.Spec.Strategy.RollingUpdate)
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-scheduling-loom-io-v1alpha1-networkfunctiondeployment,mutating=false,failurePolicy=fail,sideEffects=None,groups=scheduling.loom.io,resources=networkfunctiondeployments,verbs=create;update,versions=v1alpha1,name=vnetworkfunctiondeployment-v1alpha1.kb.io,admissionReviewVersions=v1

// CustomValidator struct is responsible for validating the NetworkFunctionDeployment resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type CustomValidator struct {
	// TODO(user): Add more fields as needed for validation
	Client client.Client
}

var _ admission.Validator[*schedulingv1alpha1.NetworkFunctionDeployment] = &CustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NetworkFunctionDeployment.
func (v *CustomValidator) ValidateCreate(
	ctx context.Context, deployment *schedulingv1alpha1.NetworkFunctionDeployment,
) (admission.Warnings, error) {
	logger.Info("Validation for NetworkFunctionDeployment upon creation",
		"name", deployment.GetName())

	// TODO(user): fill in your validation logic upon object creation.
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %v", err)
	}
	if selector.Empty() {
		return nil, fmt.Errorf("label selector cannot be empty")
	}
	if !selector.Matches(labels.Set(deployment.Spec.Template.Labels)) {
		return nil, fmt.Errorf("label selector does not match template labels")
	}

	deploymentList := &appsv1.DeploymentList{}
	err = v.Client.List(ctx, deploymentList, client.InNamespace(deployment.GetNamespace()))
	if err != nil {
		return nil, err
	}

	for i := range deploymentList.Items {
		dep := &deploymentList.Items[i]
		if dep.Name != deployment.GetName() && reflect.DeepEqual(dep.Spec.Selector, deployment.Spec.Selector) {
			return nil, fmt.Errorf("deployment with the same selector already exists: %s", dep.Name)
		}
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NetworkFunctionDeployment.
func (v *CustomValidator) ValidateUpdate(
	_ context.Context, oldDep, newDep *schedulingv1alpha1.NetworkFunctionDeployment,
) (admission.Warnings, error) {
	logger.Info("Validation for NetworkFunctionDeployment upon update", "name", newDep.GetName())

	if !reflect.DeepEqual(oldDep.Spec.Selector, newDep.Spec.Selector) {
		return nil, fmt.Errorf("spec.selector is immutable and cannot be changed")
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NetworkFunctionDeployment.
func (v *CustomValidator) ValidateDelete(
	_ context.Context, deployment *schedulingv1alpha1.NetworkFunctionDeployment,
) (admission.Warnings, error) {
	logger.Info("Validation for NetworkFunctionDeployment upon deletion", "name", deployment.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
