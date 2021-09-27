// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha3

import (
	"net/http"
	"net/url"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var resourcelog = logf.Log.WithName("resource")

func SetupWebhookWithManager(mgr ctrl.Manager, generic Generic) error {
	vwh := admission.ValidatingWebhookFor(&generic)

	if vwh != nil {
		path := "/validate-radius-dev-v1alpha1-resource"

		// Checking if the path is already registered.
		// If so, just skip it.
		if !isAlreadyHandled(mgr, path) {
			mgr.GetWebhookServer().Register(path, vwh)
		}
	}

	return nil
}

func isAlreadyHandled(mgr ctrl.Manager, path string) bool {
	if mgr.GetWebhookServer().WebhookMux == nil {
		return false
	}
	h, p := mgr.GetWebhookServer().WebhookMux.Handler(&http.Request{URL: &url.URL{Path: path}})
	if p == path && h != nil {
		return true
	}
	return false
}

//+kubebuilder:webhook:path=/validate-radius-dev-v1alpha1-resource,mutating=false,failurePolicy=fail,sideEffects=None,groups=radius.dev,resources=resources,verbs=create;update;delete,versions=v1alpha1,name=resource-validation.radius.dev,admissionReviewVersions={v1,v1beta1}

type Generic struct {
	client.Object
}

var _ webhook.Validator = &Generic{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Generic) ValidateCreate() error {
	resourcelog.Info("validate create", "name", r.GetName())

	return validate(&r.Object)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Generic) ValidateUpdate(old runtime.Object) error {
	resourcelog.Info("validate update", "name", r.GetName())

	return validate(&r.Object)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Generic) ValidateDelete() error {
	resourcelog.Info("validate delete", "name", r.GetName())

	return nil
}

func validate(r *client.Object) error {
	return nil
}
