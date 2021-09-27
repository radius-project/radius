package webhook

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Mutator specifies the interface for a generic mutating webhook.
type Mutator interface {
	// Mutate yields a response to a mutating AdmissionRequest.
	Mutate(ctx context.Context, req admission.Request, obj runtime.Object) admission.Response
}

// ensure MutatingWebhook implements Mutator
var _ Mutator = &MutatingWebhook{}

// MutatingWebhook is a generic mutating admission webhook.
type MutatingWebhook struct {
	InjectedClient
	InjectedDecoder
}

// Mutate implements the Mutator interface.
func (m *MutatingWebhook) Mutate(_ context.Context, _ admission.Request, _ runtime.Object) admission.Response {
	return admission.Allowed("")
}

// MutateFunc is a functional interface for a generic mutating admission webhook.
type MutateFunc struct {
	MutatingWebhook

	Func func(context.Context, admission.Request, runtime.Object) admission.Response
}

// Mutate implements the Mutator interface by calling the Func.
func (m *MutateFunc) Mutate(ctx context.Context, req admission.Request, obj runtime.Object) admission.Response {
	if m.Func != nil {
		return m.Func(ctx, req, obj)
	}

	return m.MutatingWebhook.Mutate(ctx, req, obj)
}
