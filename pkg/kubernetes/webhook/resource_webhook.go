package webhook

import (
	"context"
	"net/http"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/kubernetes/converters"
	"github.com/Azure/radius/pkg/renderers"
)

type ResourceWebhook struct {
	ValidatingWebhook
}

func (w *ResourceWebhook) SetupWebhookWithManager(mgr manager.Manager) error {
	return NewGenericWebhookManagedBy(mgr).
		WithValidatePath("/validate-radius-dev-v1alpha3-resource").
		Complete(w)
}

func (w *ResourceWebhook) ValidateCreate(ctx context.Context, request admission.Request, object *unstructured.Unstructured) admission.Response {
	_ = log.FromContext(ctx)

	resource := &radiusv1alpha3.Resource{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, resource)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	renderResource := &renderers.RendererResource{}
	err = converters.ConvertToRenderResource(resource, renderResource)

	return admission.Allowed("")
}
