package webhook

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Validator specifies the interface for a validating webhook.
type Validator interface {
	// ValidateCreate yields a response to a validating AdmissionRequest with operation set to Create.
	ValidateCreate(ctx context.Context, req admission.Request, obj *unstructured.Unstructured) admission.Response
	// ValidateUpdate yields a response to a validating AdmissionRequest with operation set to Update.
	ValidateUpdate(ctx context.Context, req admission.Request, obj *unstructured.Unstructured, oldObj *unstructured.Unstructured) admission.Response
	// ValidateDelete yields a response to a validating AdmissionRequest with operation set to Delete.
	ValidateDelete(ctx context.Context, req admission.Request, obj *unstructured.Unstructured) admission.Response
}

// ensure ValidatingWebhook implements Validator
var _ Validator = &ValidatingWebhook{}

// ValidatingWebhook is a generic validating admission webhook.
type ValidatingWebhook struct {
	InjectedClient
	InjectedDecoder
}

// ValidateCreate implements the Validator interface.
func (v *ValidatingWebhook) ValidateCreate(_ context.Context, _ admission.Request, _ *unstructured.Unstructured) admission.Response {
	return admission.Allowed("")
}

// ValidateUpdate implements the Validator interface.
func (v *ValidatingWebhook) ValidateUpdate(_ context.Context, _ admission.Request, _ *unstructured.Unstructured, _ *unstructured.Unstructured) admission.Response {
	return admission.Allowed("")
}

// ValidateDelete implements the Validator interface.
func (v *ValidatingWebhook) ValidateDelete(_ context.Context, _ admission.Request, _ *unstructured.Unstructured) admission.Response {
	return admission.Allowed("")
}

// ValidateFuncs is a functional interface for a generic validating admission webhook.
type ValidateFuncs struct {
	ValidatingWebhook

	CreateFunc func(context.Context, admission.Request, *unstructured.Unstructured) admission.Response
	UpdateFunc func(context.Context, admission.Request, *unstructured.Unstructured, *unstructured.Unstructured) admission.Response
	DeleteFunc func(context.Context, admission.Request, *unstructured.Unstructured) admission.Response
}

// ValidateCreate implements the Validator interface by calling the CreateFunc.
func (v *ValidateFuncs) ValidateCreate(ctx context.Context, req admission.Request, obj *unstructured.Unstructured) admission.Response {
	if v.CreateFunc != nil {
		return v.CreateFunc(ctx, req, obj)
	}

	return v.ValidatingWebhook.ValidateCreate(ctx, req, obj)
}

// ValidateUpdate implements the Validator interface by calling the UpdateFunc.
func (v *ValidateFuncs) ValidateUpdate(ctx context.Context, req admission.Request, new *unstructured.Unstructured, old *unstructured.Unstructured) admission.Response {
	if v.UpdateFunc != nil {
		return v.UpdateFunc(ctx, req, old, new)
	}

	return v.ValidatingWebhook.ValidateUpdate(ctx, req, old, new)
}

// ValidateDelete implements the Validator interface by calling the DeleteFunc.
func (v *ValidateFuncs) ValidateDelete(ctx context.Context, req admission.Request, obj *unstructured.Unstructured) admission.Response {
	if v.DeleteFunc != nil {
		return v.DeleteFunc(ctx, req, obj)
	}

	return v.ValidatingWebhook.ValidateDelete(ctx, req, obj)
}
