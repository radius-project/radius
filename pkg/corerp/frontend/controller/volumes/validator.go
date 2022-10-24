// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumes

import (
	"context"
	"fmt"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	secretProviderClassesCRD = "secretproviderclasses.secrets-store.csi.x-k8s.io"
)

// ValidateRequest validates the resource in the incoming request.
func ValidateRequest(ctx context.Context, newResource *datamodel.VolumeResource, oldResource *datamodel.VolumeResource, options *controller.Options) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	// Bypass the validation unless HTTP Method is PUT or PATCH
	if serviceCtx.HTTPMethod != http.MethodPut && serviceCtx.HTTPMethod != http.MethodPatch {
		return nil, nil
	}

	csiCRDValidationRequired := false

	switch newResource.Properties.Kind {
	case datamodel.AzureKeyVaultVolume:
		identity := newResource.Properties.AzureKeyVault.Identity
		if identity.Kind == rp.AzureIdentityWorkload {
			if identity.OIDCIssuer == "" {
				return rest.NewBadRequestResponse("oidcIssuer is required for workload identity."), nil
			}
			_, err := resources.ParseResource(identity.Resource)
			if err != nil {
				return rest.NewBadRequestResponse(fmt.Sprintf("'%s' is invalid resource for workload identity", identity.Resource)), nil
			}
		}
		csiCRDValidationRequired = true
	default:
		return rest.NewBadRequestResponse(fmt.Sprintf("invalid resource kind: %s", newResource.Properties.Kind)), nil
	}

	// TODO: Validate if Secret CSI driver required for ".Properties.Kind" is installed.

	if csiCRDValidationRequired {
		crd := &apiextv1.CustomResourceDefinition{}
		err := options.KubeClient.Get(ctx, client.ObjectKey{Name: secretProviderClassesCRD}, crd)
		if err != nil {
			return rest.NewBadRequestResponse("Your volume requires secret store CSI driver. Please install it by following https://secrets-store-csi-driver.sigs.k8s.io/."), nil
		}
	}

	return nil, nil
}
