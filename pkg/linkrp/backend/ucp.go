// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/go-autorest/autorest"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	azclients "github.com/project-radius/radius/pkg/sdk/clients"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
)

var _ autorest.Sender = (*sender)(nil)

type sender struct {
	RoundTripper http.RoundTripper
}

func (s *sender) Do(request *http.Request) (*http.Response, error) {
	return s.RoundTripper.RoundTrip(request)
}

func GetUCPDeploymentClient(options hostoptions.HostOptions) (*azclients.ResourceDeploymentsClient, error) {
	override := ""
	if options.Config.UCP.Connection == hostoptions.UCPConnectionDirect {
		if options.Config.UCP.Direct == nil || options.Config.UCP.Direct.BaseURI == "" {
			return nil, errors.New("The field '.ucp.direct.baseURI' is required for a direct connection.")
		}

		override = options.Config.UCP.Direct.BaseURI
	}

	var baseURI string
	var roundTripper http.RoundTripper
	var err error
	if override == "" {
		// Create a copy of the configuration so we can mutate it.
		config := *options.K8sConfig

		gv := schema.GroupVersion{Group: "api.ucp.dev", Version: "v1alpha3"}
		config.GroupVersion = &gv
		config.APIPath = "/apis"
		config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

		roundTripper, err = rest.TransportFor(&config)
		if err != nil {
			return nil, err
		}

		baseURI = strings.TrimSuffix(config.Host+config.APIPath, "/") + "/api.ucp.dev/v1alpha3"
		roundTripper = kubernetes.NewLocationRewriteRoundTripper(baseURI, roundTripper)
	} else {
		baseURI = strings.TrimSuffix(override, "/")
		roundTripper = kubernetes.NewLocationRewriteRoundTripper(override, http.DefaultTransport)
	}

	dc := azclients.NewResourceDeploymentClientWithBaseURI(baseURI)

	// Poll faster than the default, many deployments are quick
	dc.PollingDelay = 5 * time.Second

	dc.Sender = &sender{RoundTripper: roundTripper}
	return &dc, nil
}

func GetUCPClientOptions(options hostoptions.HostOptions) (*arm.ClientOptions, error) {
	override := ""
	if options.Config.UCP.Connection == hostoptions.UCPConnectionDirect {
		if options.Config.UCP.Direct == nil || options.Config.UCP.Direct.BaseURI == "" {
			return nil, errors.New("The field '.ucp.direct.baseURI' is required for a direct connection.")
		}

		override = options.Config.UCP.Direct.BaseURI
	}

	var baseURI string
	var roundTripper http.RoundTripper
	var err error
	if override == "" {
		// Create a copy of the configuration so we can mutate it.
		config := *options.K8sConfig

		gv := schema.GroupVersion{Group: "api.ucp.dev", Version: "v1alpha3"}
		config.GroupVersion = &gv
		config.APIPath = "/apis"
		config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

		roundTripper, err = rest.TransportFor(&config)
		if err != nil {
			return nil, err
		}

		baseURI = strings.TrimSuffix(config.Host+config.APIPath, "/") + "/api.ucp.dev/v1alpha3"
		roundTripper = kubernetes.NewLocationRewriteRoundTripper(baseURI, roundTripper)
	} else {
		baseURI = strings.TrimSuffix(override, "/")
		roundTripper = kubernetes.NewLocationRewriteRoundTripper(baseURI, http.DefaultTransport)
	}

	transporter := kubernetes.KubernetesTransporter{Client: roundTripper}
	return connections.GetClientOptions(baseURI, &transporter), nil
}
