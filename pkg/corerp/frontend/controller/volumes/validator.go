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

	"sigs.k8s.io/controller-runtime/pkg/client"
	csiv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

// ValidateRequest validates the resource in the incoming request.
func ValidateRequest(ctx context.Context, newResource *datamodel.VolumeResource, oldResource *datamodel.VolumeResource, options *controller.Options) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	// PUT and PATCH are the only operations supported for this resource.
	if serviceCtx.OperationType != http.MethodPut && serviceCtx.OperationType != http.MethodPatch {
		return rest.NewMethodNotAllowedResponse(serviceCtx.ResourceID.String(), "only PUT and PATCH are supported for the validation of this resource."), nil
	}

	switch newResource.Properties.Kind {
	case datamodel.AzureKeyVaultVolume:
		if newResource.Properties.AzureKeyVault.Identity.Kind == datamodel.AzureIdentitySystemAssigned &&
			newResource.Properties.AzureKeyVault.Identity.ClientID != "" {
			return rest.NewBadRequestResponse("clientID is not allowed when using system assigned identity"), nil
		}

		if newResource.Properties.AzureKeyVault.Identity.Kind == datamodel.AzureIdentityWorkload &&
			newResource.Properties.AzureKeyVault.Identity.ClientID == "" {
			return rest.NewBadRequestResponse("clientID can not be empty when using workload identity"), nil
		}
	default:
		return rest.NewBadRequestResponse(fmt.Sprintf("invalid resource kind: %s", newResource.Properties.Kind)), nil
	}

	// TODO: Based on the Kind (Azure, AWS, GPC, etc.), we will get the specific csi-driver.
	csiDrivers := csiv1.SecretProviderClassList{}
	err := options.KubeClient.List(ctx, &csiDrivers, &client.ListOptions{})
	if err != nil {
		return rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInternal,
				Message: err.Error(),
			},
		}), err
	}
	if len(csiDrivers.Items) == 0 {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), "csi driver is not installed"), nil
	}

	return nil, nil
}
