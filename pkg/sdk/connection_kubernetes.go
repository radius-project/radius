// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sdk

import (
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
)

const (
	// ucpGroup is the Kubernetes API group of the UCP APIService extension. This is used to build
	// URLs for communicating with UCP & Radius in Kubernetes.
	ucpGroup = "api.ucp.dev"

	// ucpVersion is the Kubernetes API version of the UCP APIService extension. This is used to build
	// URLs for communicating with UCP & Radius in Kubernetes.
	ucpVersion = "v1alpha3"
)

var _ Connection = (*kubernetesConnection)(nil)

// kubernetesConnection represents a connection to Radius through the Kubernetes API server. This
// connection type is most commonly used.
type kubernetesConnection struct {
	endpoint string

	// roundTripper is the http.roundTripper used to send requests.
	roundTripper http.RoundTripper
}

// NewKubernetesConnectionFromConfig creates a KubernetesConnection from the provided Kubernetes
// configuration.
func NewKubernetesConnectionFromConfig(config *rest.Config) (Connection, error) {
	// Make a copy of the configuration because we are going to edit it.
	copied := *config

	copied.GroupVersion = &schema.GroupVersion{Group: ucpGroup, Version: ucpVersion}
	copied.APIPath = "/apis"
	copied.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	roundTripper, err := rest.TransportFor(&copied)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimSuffix(copied.Host+copied.APIPath, "/") + "/" + ucpGroup + "/" + ucpVersion
	roundTripper = newLocationRewriteRoundTripper(copied.Host, roundTripper)
	return &kubernetesConnection{endpoint: endpoint, roundTripper: roundTripper}, nil
}

// Client returns an http.Client for communicating with Radius. This satisfies both the
// autorest.Sender interface (autorest Track1 Go SDK) and policy.Transporter interface
// (autorest Track2 Go SDK).
func (c *kubernetesConnection) Client() *http.Client {
	return &http.Client{Transport: c.roundTripper}
}

// Endpoint returns the endpoint (aka. base URL) of the Radius API. This definitely includes
// the URL scheme and authority, and may include path segments.
func (c *kubernetesConnection) Endpoint() string {
	return c.endpoint
}
