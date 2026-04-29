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

package v1alpha1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var networkfunctionconfiglog = logf.Log.WithName("networkfunctionconfig-resource")

// SetupNetworkFunctionConfigWebhookWithManager registers the webhook for NetworkFunctionConfig in the manager.
func SetupNetworkFunctionConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1alpha1.NetworkFunctionConfig{}).
		WithValidator(&NetworkFunctionConfigCustomValidator{}).
		WithDefaulter(&NetworkFunctionConfigCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-core-loom-io-v1alpha1-networkfunctionconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=core.loom.io,resources=networkfunctionconfigs,verbs=create;update,versions=v1alpha1,name=mnetworkfunctionconfig-v1alpha1.kb.io,admissionReviewVersions=v1

// NetworkFunctionConfigCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind NetworkFunctionConfig when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type NetworkFunctionConfigCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind NetworkFunctionConfig.
func (d *NetworkFunctionConfigCustomDefaulter) Default(_ context.Context, obj *corev1alpha1.NetworkFunctionConfig) error {
	networkfunctionconfiglog.Info("Defaulting for NetworkFunctionConfig", "name", obj.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-core-loom-io-v1alpha1-networkfunctionconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=core.loom.io,resources=networkfunctionconfigs,verbs=create;update,versions=v1alpha1,name=vnetworkfunctionconfig-v1alpha1.kb.io,admissionReviewVersions=v1

// NetworkFunctionConfigCustomValidator struct is responsible for validating the NetworkFunctionConfig resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NetworkFunctionConfigCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NetworkFunctionConfig.
func (v *NetworkFunctionConfigCustomValidator) ValidateCreate(_ context.Context, obj *corev1alpha1.NetworkFunctionConfig) (admission.Warnings, error) {
	networkfunctionconfiglog.Info("Validation for NetworkFunctionConfig upon creation", "name", obj.GetName())

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NetworkFunctionConfig.
func (v *NetworkFunctionConfigCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *corev1alpha1.NetworkFunctionConfig) (admission.Warnings, error) {
	networkfunctionconfiglog.Info("Validation for NetworkFunctionConfig upon update", "name", newObj.GetName())

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NetworkFunctionConfig.
func (v *NetworkFunctionConfigCustomValidator) ValidateDelete(_ context.Context, obj *corev1alpha1.NetworkFunctionConfig) (admission.Warnings, error) {
	networkfunctionconfiglog.Info("Validation for NetworkFunctionConfig upon deletion", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
