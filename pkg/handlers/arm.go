// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/arm/resources"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/gofrs/uuid"
)

const NameGenerationRetryAttempts = 10

func NewARMHandler(arm armauth.ArmConfig) ResourceHandler {
	return &ARMHandler{arm: arm}
}

// ARMHandler is a generic ResourceHandler implementation that is suitable for most ARM resources.
type ARMHandler struct {
	arm armauth.ArmConfig
}

type ARMResource struct {
	// ResourceID is the resource ID of the existing ARM resource matching this type.
	ResourceID azresources.ResourceID

	// ResourceType is the fully-qualified ARM resource type of the resource.
	ResourceType string

	// ResourceName is the *unqualified* ARM resource name. Depending on the value of GenerateName it might be a prefix for name generation
	// or a fixed value.
	ResourceName string

	// GenerateName specifies whether to generate the final resource name (true) or use the value of ResourceName as a fixed value.
	GenerateName bool

	// API Version is the ARM api-version used to transact with the resource.
	APIVersion string

	// Body is the complete ARM payload to used when performing a PUT on the resource.
	Body interface{}
}

func (handler *ARMHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	payload, ok := options.Resource.Resource.(ARMResource)
	if !ok {
		return nil, fmt.Errorf("resource payload must be %T", ARMResource{})
	}

	if options.Resource.Managed || options.ExistingOutputResource != nil {
		id, err := handler.GetResourceID(payload, options.ExistingOutputResource)
		if err != nil {
			return nil, fmt.Errorf("failed to determine resource ID for %q: %w", payload.ResourceType, err)
		}

		resource, err := handler.GetResource(ctx, id, payload.APIVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to GET resource %q: %w", id, err)
		}

		options.Output = resource
		options.Resource.Identity = resourcemodel.NewARMIdentity(id, payload.APIVersion)
	} else {
		id, err := handler.GenerateResourceID(ctx, payload, options.Dependencies)
		if err != nil {
			return nil, fmt.Errorf("failed to generate a resource ID for %q: %w", payload.ResourceType, err)
		}

		generic, err := handler.TransformBody(ctx, *options, payload.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to transform the resource body for %q: %w", id, err)
		}

		resource, err := handler.PutResource(ctx, id, payload.APIVersion, generic)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT resource %q: %w", id, err)
		}

		options.Output = resource
		options.Resource.Identity = resourcemodel.NewARMIdentity(id, payload.APIVersion)
	}

	// ARMHandler does not use the property bag.
	return map[string]string{}, nil
}

func (handler *ARMHandler) Delete(ctx context.Context, options DeleteOptions) error {
	if !options.ExistingOutputResource.Managed {
		// There's nothing to do for a user-managed resource.
		return nil
	}

	identity := options.ExistingOutputResource.Identity
	arm, ok := identity.Data.(resourcemodel.ARMIdentity)
	if !ok {
		return fmt.Errorf("resource must be an ARM resource, was: %+v", identity)
	}

	err := handler.DeleteResource(ctx, arm.ID, arm.APIVersion)
	if err != nil {
		return fmt.Errorf("failed to DELETE resource %q: %w", arm.ID, err)
	}

	return nil
}

func (handler *ARMHandler) GetResourceID(payload ARMResource, existing *db.OutputResource) (string, error) {
	if existing == nil {
		return "", nil
	}

	identity := existing.Identity
	arm, ok := identity.Data.(resourcemodel.ARMIdentity)
	if !ok {
		return "", fmt.Errorf("resource must be an ARM resource, was: %+v", identity)
	}

	return arm.ID, nil
}

func (handler *ARMHandler) GenerateResourceID(ctx context.Context, payload ARMResource, dependencies []Dependency) (string, error) {
	parts := strings.Split(payload.ResourceType, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid resource type %q", payload.ResourceType)
	}

	// The first type is made up or the first two parts. eg: `Microsoft.DocumentDb/accounts`
	types := []azresources.ResourceType{
		{
			Type: strings.Join(parts[:2], "/"),
		},
	}
	for _, part := range parts[2:] {
		types = append(types, azresources.ResourceType{Type: part})
	}

	// Now that we have the types, we need to resolve the names.
	//
	// The last segment comes from this resource. Previous segments might need to be
	// looked up from dependencies.
	if len(types) > 1 {
		// We must have a parent dependency, so hook it up.
		var parentID *azresources.ResourceID
		for _, dependency := range dependencies {
			if dependency.Kind == DepdendencyParent && dependency.Identity.Kind == resourcemodel.IdentityKindARM {
				parsed, err := azresources.Parse(dependency.Identity.Data.(resourcemodel.ARMIdentity).ID)
				if err != nil {
					return "", fmt.Errorf("parent resource has an invalid identity: %+v", dependency.Identity)
				}

				parentID = &parsed
				break
			}
		}

		for i := 0; i < len(types)-1; i++ {
			types[i].Name = parentID.Types[i].Name
		}
	}

	id := ""
	if payload.GenerateName {
		for i := 0; i < NameGenerationRetryAttempts; i++ {
			name, err := handler.GetRandomName(payload.ResourceName + "-")
			if err != nil {
				return "", err
			}

			types[len(types)-1].Name = name
			id := azresources.MakeID(
				handler.arm.SubscriptionID,
				handler.arm.ResourceGroup,
				types[0],
				types[1:]...)

			available := handler.CheckResourceNameAvailability(ctx, id, payload.APIVersion)
			if !available {
				continue
			}
		}
	} else {
		id = azresources.MakeID(
			handler.arm.SubscriptionID,
			handler.arm.ResourceGroup,
			types[0],
			types[1:]...)
	}

	return id, nil
}

func (handler *ARMHandler) TransformBody(ctx context.Context, options PutOptions, body interface{}) (resources.GenericResource, error) {
	b, err := json.Marshal(&body)
	if err != nil {
		return resources.GenericResource{}, err
	}

	result := resources.GenericResource{}
	err = json.Unmarshal(b, &result)
	if err != nil {
		return resources.GenericResource{}, err
	}

	// TODO: should we set the location here or leave that up to rendering?

	result.Tags = keys.MakeTagsForRadiusComponent(options.Application, options.Component)
	return result, nil
}

func (handler *ARMHandler) GetRandomName(base string) (string, error) {
	// 3-24 characters - all alphanumeric and '-'
	uid, err := uuid.NewV4()
	if err != nil {
		return "", fmt.Errorf("failed to generate name: %w", err)
	}
	name := base + strings.ReplaceAll(uid.String(), "-", "")
	name = name[0:24]
	return name, nil
}

func (handler *ARMHandler) CheckResourceNameAvailability(ctx context.Context, id string, apiVersion string) bool {
	rc := clients.NewGenericResourceClient(handler.arm.K8sSubscriptionID, handler.arm.Auth)
	response, err := rc.CheckExistenceByID(ctx, id, apiVersion)
	if err != nil {
		return false
	} else if response.StatusCode == http.StatusNotFound {
		return false
	}

	return true
}

func (handler *ARMHandler) GetResource(ctx context.Context, id string, apiVersion string) (*resources.GenericResource, error) {
	rc := clients.NewGenericResourceClient(handler.arm.K8sSubscriptionID, handler.arm.Auth)
	resource, err := rc.GetByID(ctx, id, apiVersion)
	if err != nil {
		return nil, err
	}

	return &resource, nil
}

func (handler *ARMHandler) PutResource(ctx context.Context, id string, apiVersion string, body resources.GenericResource) (*resources.GenericResource, error) {
	rc := clients.NewGenericResourceClient(handler.arm.K8sSubscriptionID, handler.arm.Auth)
	future, err := rc.CreateOrUpdateByID(ctx, id, apiVersion, body)
	if err != nil {
		return nil, err
	}

	err = future.WaitForCompletionRef(ctx, rc.Client)
	if err != nil {
		return nil, err
	}

	resource, err := future.Result(rc)
	if err != nil {
		return nil, err
	}

	return &resource, nil
}

func (handler *ARMHandler) DeleteResource(ctx context.Context, id string, apiVersion string) error {
	rc := clients.NewGenericResourceClient(handler.arm.K8sSubscriptionID, handler.arm.Auth)
	future, err := rc.DeleteByID(ctx, id, apiVersion)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return err
	}

	err = future.WaitForCompletionRef(ctx, rc.Client)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return err
	}

	return nil
}
