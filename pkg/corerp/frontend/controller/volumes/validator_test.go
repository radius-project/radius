// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumes

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	rptesting "github.com/project-radius/radius/pkg/corerp/testing"
)

var (
	resourceID = "/subscriptions/test-subscription-id/resourceGroups/test-resource-group/providers/applications.core/volumes/test-volume"
	keyvaultID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.KeyVault/vaults/vault0"
)

func mustParseResourceID(id string) resources.ID {
	resourceID, err := resources.ParseResource(id)
	if err != nil {
		panic(err)
	}

	return resourceID
}

func TestValidateRequest(t *testing.T) {
	// Create SecretProviderClass CRD object fake client.
	crdScheme := runtime.NewScheme()
	err := apiextv1.AddToScheme(crdScheme)
	require.NoError(t, err)

	crdFakeClient := rptesting.NewFakeKubeClient(crdScheme, &apiextv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: secretProviderClassesCRD,
		},
	})

	// Create default Kubernetes fake client.
	defaultFakeClient := rptesting.NewFakeKubeClient(nil)

	type args struct {
		ctx         context.Context
		newResource *datamodel.VolumeResource
		oldResource *datamodel.VolumeResource
		options     *controller.Options
	}

	tests := []struct {
		name    string
		args    args
		want    rest.Response
		wantErr error
	}{
		{
			name: "invalid-kind",
			args: args{
				ctx: v1.WithARMRequestContext(
					context.Background(), &v1.ARMRequestContext{
						ResourceID: mustParseResourceID(resourceID),
						HTTPMethod: http.MethodPut,
					}),
				newResource: &datamodel.VolumeResource{
					Properties: datamodel.VolumeResourceProperties{
						Kind:          "unsupported-kind",
						AzureKeyVault: &datamodel.AzureKeyVaultVolumeProperties{},
					},
				},
				oldResource: &datamodel.VolumeResource{},
				options: &controller.Options{
					KubeClient: defaultFakeClient,
				},
			},
			want:    rest.NewBadRequestResponse(fmt.Sprintf("invalid resource kind: %s", "unsupported-kind")),
			wantErr: nil,
		},
		{
			name: "csi-driver-not-installed",
			args: args{
				ctx: v1.WithARMRequestContext(
					context.Background(), &v1.ARMRequestContext{
						ResourceID: mustParseResourceID(resourceID),
						HTTPMethod: http.MethodPut,
					}),
				newResource: &datamodel.VolumeResource{
					Properties: datamodel.VolumeResourceProperties{
						Kind: datamodel.AzureKeyVaultVolume,
						AzureKeyVault: &datamodel.AzureKeyVaultVolumeProperties{
							Resource: keyvaultID,
						},
					},
				},
				oldResource: &datamodel.VolumeResource{},
				options: &controller.Options{
					KubeClient: defaultFakeClient,
				},
			},
			want:    rest.NewBadRequestResponse("Your volume requires secret store CSI driver. Please install it by following https://secrets-store-csi-driver.sigs.k8s.io/."),
			wantErr: nil,
		},
		{
			name: "csi-driver-installed",
			args: args{
				ctx: v1.WithARMRequestContext(
					context.Background(), &v1.ARMRequestContext{
						ResourceID: mustParseResourceID(resourceID),
						HTTPMethod: http.MethodPut,
					}),
				newResource: &datamodel.VolumeResource{
					Properties: datamodel.VolumeResourceProperties{
						Kind: datamodel.AzureKeyVaultVolume,
						AzureKeyVault: &datamodel.AzureKeyVaultVolumeProperties{
							Resource: keyvaultID,
						},
					},
				},
				oldResource: &datamodel.VolumeResource{},
				options: &controller.Options{
					KubeClient: crdFakeClient,
				},
			},
			want:    nil,
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ValidateRequest(tt.args.ctx, tt.args.newResource, tt.args.oldResource, tt.args.options)
			require.ErrorIs(t, tt.wantErr, err)
			require.EqualValues(t, tt.want, resp)
		})
	}
}
