package secretstores

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/to"
)

// ValidateRequest validates the resource in the incoming request.
func ValidateRequest(ctx context.Context, newResource *datamodel.SecretStore, oldResource *datamodel.SecretStore, options *controller.Options) (rest.Response, error) {
	refResourceID := newResource.Properties.Resource
	if refResourceID == "" {
		// Radius manages platform secret resource.

		// Ensure that only Value is set for all secrets.
		for k, secret := range newResource.Properties.Data {
			if secret.ValueFrom != nil {
				return rest.NewBadRequestResponse(fmt.Sprintf("%s cannot set valueFrom.", k)), nil
			}

			if secret.Value == nil {
				return rest.NewBadRequestResponse(fmt.Sprintf("%s must have secret data.", k)), nil
			}
		}
	} else {
		// Radius references the external secret resource.

		// Currently, we support only kubernetes secret resource so validate if the resource name is valid for kubernetes secret.
		if !kubernetes.IsValidObjectName(refResourceID) {
			return rest.NewBadRequestResponse(fmt.Sprintf("invalid secret resource id: %s", refResourceID)), nil
		}

		// Ensure that only valueFrom is set for all secrets.
		for _, secret := range newResource.Properties.Data {
			if secret.ValueFrom == nil {
				return rest.NewBadRequestResponse(fmt.Sprintf("%s must set valueFrom.", newResource.Properties.Resource)), nil
			}
			if to.String(secret.Value) != "" {
				return rest.NewBadRequestResponse(fmt.Sprintf("%s cannot have value data.", newResource.Properties.Resource)), nil
			}
		}
	}

	return nil, nil
}
