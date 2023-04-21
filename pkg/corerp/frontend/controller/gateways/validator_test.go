// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateways

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestValidateAndMutateRequest_Gateway(t *testing.T) {
	requestTests := []struct {
		desc            string
		newResource     *datamodel.Gateway
		oldResource     *datamodel.Gateway
		mutatedResource *datamodel.Gateway
		resp            rest.Response
	}{
		{
			desc: "empty Gateway spec",
			newResource: &datamodel.Gateway{
				Properties: datamodel.GatewayProperties{},
			},
			oldResource: nil,
			mutatedResource: &datamodel.Gateway{
				Properties: datamodel.GatewayProperties{},
			},
			resp: nil,
		},
		{
			desc: "specify both SSL Passthrough and TLS Termination",
			newResource: &datamodel.Gateway{
				Properties: datamodel.GatewayProperties{
					TLS: &datamodel.GatewayPropertiesTLS{
						SSLPassthrough:  true,
						CertificateFrom: "secretname",
					},
				},
			},
			resp: rest.NewBadRequestResponse("Only one of tls.certificateFrom and tls.sslPassthrough can be specified at a time."),
		},
		{
			desc: "cannot set TLS protocol version without certificateFrom",
			newResource: &datamodel.Gateway{
				Properties: datamodel.GatewayProperties{
					TLS: &datamodel.GatewayPropertiesTLS{
						MinimumProtocolVersion: "1.2",
					},
				},
			},
			resp: rest.NewBadRequestResponse("Field tls.certificateFrom is required when tls.minimumProtocolVersion is set."),
		},
		{
			desc: "can set minimum TLS protocol version",
			newResource: &datamodel.Gateway{
				Properties: datamodel.GatewayProperties{
					TLS: &datamodel.GatewayPropertiesTLS{
						CertificateFrom:        "secretname",
						MinimumProtocolVersion: "1.2",
					},
				},
			},
			oldResource: nil,
			mutatedResource: &datamodel.Gateway{
				Properties: datamodel.GatewayProperties{
					TLS: &datamodel.GatewayPropertiesTLS{
						CertificateFrom:        "secretname",
						MinimumProtocolVersion: "1.2",
					},
				},
			},
			resp: nil,
		},
		{
			desc: "TLS protocol version defaults to 1.2",
			newResource: &datamodel.Gateway{
				Properties: datamodel.GatewayProperties{
					TLS: &datamodel.GatewayPropertiesTLS{
						CertificateFrom: "secretname",
					},
				},
			},
			oldResource: nil,
			mutatedResource: &datamodel.Gateway{
				Properties: datamodel.GatewayProperties{
					TLS: &datamodel.GatewayPropertiesTLS{
						CertificateFrom:        "secretname",
						MinimumProtocolVersion: "1.2",
					},
				},
			},
			resp: nil,
		},
	}

	for _, tc := range requestTests {
		t.Run(tc.desc, func(t *testing.T) {
			r, err := ValidateAndMutateRequest(context.Background(), tc.newResource, tc.oldResource, nil)

			require.NoError(t, err)
			if tc.resp != nil {
				require.Equal(t, tc.resp, r)
			} else {
				require.Nil(t, r)
				require.Equal(t, tc.mutatedResource, tc.newResource)
			}
		})
	}
}
