// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/stretchr/testify/require"
)

func TestValidateAndMutateRequest_IdentityProperty(t *testing.T) {
	requestTests := []struct {
		desc            string
		newResource     *datamodel.ContainerResource
		oldResource     *datamodel.ContainerResource
		mutatedResource *datamodel.ContainerResource
	}{
		{
			desc: "nil identity",
			newResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{},
			},
			oldResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{},
			},
			mutatedResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{},
			},
		},
		{
			desc: "valid identity",
			newResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{},
			},
			oldResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Identity: &rp.IdentitySettings{
						Kind:       rp.AzureIdentityWorkload,
						OIDCIssuer: "https://oidcurl/id",
						Resource:   "identity-resource-id",
					},
				},
			},
			mutatedResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Identity: &rp.IdentitySettings{
						Kind:       rp.AzureIdentityWorkload,
						OIDCIssuer: "https://oidcurl/id",
						Resource:   "identity-resource-id",
					},
				},
			},
		},
	}

	for _, tc := range requestTests {
		t.Run(tc.desc, func(t *testing.T) {
			r, err := ValidateAndMutateRequest(context.Background(), tc.newResource, tc.oldResource, nil)

			require.NoError(t, err)
			require.Nil(t, r)

			require.Equal(t, tc.mutatedResource, tc.newResource)
		})
	}
}
