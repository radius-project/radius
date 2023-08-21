package v20220315privatepreview

import (
	"fmt"
	"reflect"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/daprrp/datamodel"
	"github.com/radius-project/radius/pkg/linkrp"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned DaprStateStore resource to version-agnostic datamodel and returns an error
// if the resourceProvisioning is set to manual and the required fields are not specified.
func (src *DaprStateStoreResource) ConvertTo() (v1.DataModelInterface, error) {
	daprStateStoreProperties := datamodel.DaprStateStoreProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Environment: to.String(src.Properties.Environment),
			Application: to.String(src.Properties.Application),
		},
	}

	trackedResource := v1.TrackedResource{
		ID:       to.String(src.ID),
		Name:     to.String(src.Name),
		Type:     to.String(src.Type),
		Location: to.String(src.Location),
		Tags:     to.StringMap(src.Tags),
	}
	internalMetadata := v1.InternalMetadata{
		UpdatedAPIVersion:      Version,
		AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
	}
	converted := &datamodel.DaprStateStore{}
	converted.TrackedResource = trackedResource
	converted.InternalMetadata = internalMetadata
	converted.Properties = daprStateStoreProperties

	var err error
	converted.Properties.ResourceProvisioning, err = toResourceProvisiongDataModel(src.Properties.ResourceProvisioning)
	if err != nil {
		return nil, err
	}

	converted.Properties.Resources = toResourcesDataModel(src.Properties.Resources)

	// Note: The metadata, type, and version fields cannot be specified when using recipes since
	// the recipe is expected to create the Dapr Component manifest. However, they are required
	// when resourceProvisioning is set to manual.
	msgs := []string{}
	if converted.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		if src.Properties.Recipe != nil && (!reflect.ValueOf(*src.Properties.Recipe).IsZero()) {
			msgs = append(msgs, "recipe details cannot be specified when resourceProvisioning is set to manual")
		}
		if src.Properties.Metadata == nil || len(src.Properties.Metadata) == 0 {
			msgs = append(msgs, "metadata must be specified when resourceProvisioning is set to manual")
		}
		if src.Properties.Type == nil || *src.Properties.Type == "" {
			msgs = append(msgs, "type must be specified when resourceProvisioning is set to manual")
		}
		if src.Properties.Version == nil || *src.Properties.Version == "" {
			msgs = append(msgs, "version must be specified when resourceProvisioning is set to manual")
		}

		converted.Properties.Metadata = src.Properties.Metadata
		converted.Properties.Type = to.String(src.Properties.Type)
		converted.Properties.Version = to.String(src.Properties.Version)
	} else {
		if src.Properties.Metadata != nil && (!reflect.ValueOf(src.Properties.Metadata).IsZero()) {
			msgs = append(msgs, "metadata cannot be specified when resourceProvisioning is set to recipe (default)")
		}
		if src.Properties.Type != nil && (!reflect.ValueOf(*src.Properties.Type).IsZero()) {
			msgs = append(msgs, "type cannot be specified when resourceProvisioning is set to recipe (default)")
		}
		if src.Properties.Version != nil && (!reflect.ValueOf(*src.Properties.Version).IsZero()) {
			msgs = append(msgs, "version cannot be specified when resourceProvisioning is set to recipe (default)")
		}

		converted.Properties.Recipe = toRecipeDataModel(src.Properties.Recipe)
	}
	if len(msgs) > 0 {
		return nil, &v1.ErrClientRP{
			Code:    v1.CodeInvalid,
			Message: fmt.Sprintf("error(s) found:\n\t%v", strings.Join(msgs, "\n\t")),
		}
	}

	return converted, nil
}

// ConvertFrom converts a version-agnostic DataModelInterface to a versioned DaprStateStoreResource and returns an
// error if the conversion fails or the mode of the DaprStateStore is not specified.
func (dst *DaprStateStoreResource) ConvertFrom(src v1.DataModelInterface) error {
	daprStateStore, ok := src.(*datamodel.DaprStateStore)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(daprStateStore.ID)
	dst.Name = to.Ptr(daprStateStore.Name)
	dst.Type = to.Ptr(daprStateStore.Type)
	dst.SystemData = fromSystemDataModel(daprStateStore.SystemData)
	dst.Location = to.Ptr(daprStateStore.Location)
	dst.Tags = *to.StringMapPtr(daprStateStore.Tags)
	dst.Properties = &DaprStateStoreProperties{
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
		},
		ProvisioningState:    fromProvisioningStateDataModel(daprStateStore.InternalMetadata.AsyncProvisioningState),
		Environment:          to.Ptr(daprStateStore.Properties.Environment),
		Application:          to.Ptr(daprStateStore.Properties.Application),
		ComponentName:        to.Ptr(daprStateStore.Properties.ComponentName),
		ResourceProvisioning: fromResourceProvisioningDataModel(daprStateStore.Properties.ResourceProvisioning),
		Resources:            fromResourcesDataModel(daprStateStore.Properties.Resources),
	}

	if daprStateStore.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		dst.Properties.Type = to.Ptr(daprStateStore.Properties.Type)
		dst.Properties.Version = to.Ptr(daprStateStore.Properties.Version)
		dst.Properties.Metadata = daprStateStore.Properties.Metadata
	} else {
		dst.Properties.Recipe = fromRecipeDataModel(daprStateStore.Properties.Recipe)
	}

	return nil
}
