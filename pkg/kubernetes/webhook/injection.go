package webhook

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// InjectedClient holds an injected client.Client
type InjectedClient struct {
	Client client.Client
}

// InjectClient implements the inject.Client interface.
func (i *InjectedClient) InjectClient(client client.Client) error {
	i.Client = client
	return nil
}

// InjectedDecoder holds an injected admission.Decoder
type InjectedDecoder struct {
	Decoder *admission.Decoder
}

// InjectDecoder implements the admission.DecoderInjector interface.
func (i *InjectedDecoder) InjectDecoder(decoder *admission.Decoder) error {
	i.Decoder = decoder
	return nil
}
