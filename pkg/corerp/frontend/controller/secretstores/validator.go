package secretstores

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/to"
)

// ValidateRequest validates the resource in the incoming request.
func ValidateRequest(ctx context.Context, newResource *datamodel.SecretStore, oldResource *datamodel.SecretStore, options *controller.Options) (rest.Response, error) {
	if newResource.Properties.Type != datamodel.SecretTypeCert {
		return rest.NewBadRequestResponse(fmt.Sprintf("secret store type %s is not supported", newResource.Properties.Type)), nil
	}

	refResourceID := newResource.Properties.Resource
	if refResourceID == "" {
		// Radius manages platform secret resource.
		// TODO: Creating secret support later.
		return rest.NewBadRequestResponse("$.properties.resource must be given"), nil
	} else {
		// Radius references the external secret resource.

		// Currently, we support only kubernetes secret resource so validate if the resource name is valid for kubernetes secret.
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
