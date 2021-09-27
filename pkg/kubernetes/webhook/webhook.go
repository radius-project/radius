package webhook

import (
	"errors"
	"net/http"
	"net/url"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// WebhookBuilder builds a Webhook.
type WebhookBuilder struct {
	mgr          manager.Manager
	config       *rest.Config
	validatePath string
	mutatePath   string
}

func NewGenericWebhookManagedBy(mgr manager.Manager) *WebhookBuilder {
	return &WebhookBuilder{
		mgr: mgr,
	}
}

// WebhookManagedBy allows inform its manager.Manager.
func WebhookManagedBy(m manager.Manager) *WebhookBuilder {
	return &WebhookBuilder{mgr: m}
}

// WithMutatePath overrides the mutate path of the webhook
func (blder *WebhookBuilder) WithMutatePath(path string) *WebhookBuilder {
	blder.mutatePath = path
	return blder
}

// WithValidatePath overrides the validate path of the webhook
func (blder *WebhookBuilder) WithValidatePath(path string) *WebhookBuilder {
	blder.validatePath = path
	return blder
}

// Complete builds the webhook.
func (blder *WebhookBuilder) Complete(i interface{}) error {
	if blder.validatePath == "" && blder.mutatePath == "" {
		return errors.New("validatePath or mutatePath must be set")
	}

	// Set the Config
	blder.loadRestConfig()

	// Set the Webhook if needed
	return blder.registerWebhooks(i)
}

func (blder *WebhookBuilder) loadRestConfig() {
	if blder.config == nil {
		blder.config = blder.mgr.GetConfig()
	}
}

func (blder *WebhookBuilder) registerWebhooks(i interface{}) error {
	// Create webhook(s) for each type
	if validator, ok := i.(Validator); ok {
		w, err := blder.createAdmissionWebhook(withValidationHandler(validator))
		if err != nil {
			return err
		}
		if err := blder.registerValidatingWebhook(w); err != nil {
			return err
		}
	}
	if mutator, ok := i.(Mutator); ok {
		w, err := blder.createAdmissionWebhook(withMutationHandler(mutator))
		if err != nil {
			return err
		}

		if err := blder.registerMutatingWebhook(w); err != nil {
			return err
		}
	}
	return nil
}

func (blder *WebhookBuilder) registerValidatingWebhook(w *admission.Webhook) error {

	path := blder.validatePath

	// Checking if the path is already registered.
	// If so, just skip it.
	if !blder.isAlreadyHandled(path) {
		blder.mgr.GetWebhookServer().Register(path, w)
	}
	return nil
}

func (blder *WebhookBuilder) registerMutatingWebhook(w *admission.Webhook) error {
	path := blder.mutatePath

	// Checking if the path is already registered.
	// If so, just skip it.
	if !blder.isAlreadyHandled(path) {
		blder.mgr.GetWebhookServer().Register(path, w)
	}
	return nil
}

func (blder *WebhookBuilder) createAdmissionWebhook(handler Handler) (*admission.Webhook, error) {
	w := &admission.Webhook{
		Handler:         handler,
		WithContextFunc: nil,
	}

	// inject scheme for decoder
	if err := w.InjectScheme(blder.mgr.GetScheme()); err != nil {
		return nil, err
	}

	// inject client
	if err := w.InjectFunc(func(i interface{}) error {
		if injector, ok := i.(inject.Client); ok {
			return injector.InjectClient(blder.mgr.GetClient())
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return w, nil
}

func (blder *WebhookBuilder) isAlreadyHandled(path string) bool {
	if blder.mgr.GetWebhookServer().WebhookMux == nil {
		return false
	}
	h, p := blder.mgr.GetWebhookServer().WebhookMux.Handler(&http.Request{URL: &url.URL{Path: path}})
	if p == path && h != nil {
		return true
	}
	return false
}
