package v20250801preview

import (
	"reflect"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

func TestConvertTo_Minimal(t *testing.T) {
	recipe := &RecipePackResource{
		ID:       toPtr("/planes/radius/local/resourceGroups/rg/providers/Radius.Core/recipePacks/foo"),
		Name:     toPtr("foo"),
		Type:     toPtr("Radius.Core/recipePacks"),
		Location: toPtr("global"),
		Tags:     map[string]*string{"env": toPtr("dev")},
		Properties: &RecipePackProperties{
			ProvisioningState: toProvisioningStatePtr(ProvisioningStateSucceeded),
		},
	}
	model, err := recipe.ConvertTo()
	if err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}
	m, ok := model.(*datamodel.RecipePack)
	if !ok {
		t.Fatalf("ConvertTo did not return RecipePack")
	}
	if m.ID != *recipe.ID || m.Name != *recipe.Name || m.Type != *recipe.Type || m.Location != *recipe.Location {
		t.Errorf("Basic fields not converted correctly")
	}
	if m.BaseResource.InternalMetadata.AsyncProvisioningState != "Succeeded" {
		t.Errorf("ProvisioningState not converted")
	}
}

func TestConvertTo_Full(t *testing.T) {
	desc := "desc"
	referenced := []*string{toPtr("/foo"), toPtr("/bar")}
	recipes := map[string]*RecipeDefinition{
		"r1": {
			RecipeKind:     toRecipeKindPtr("Container"),
			RecipeLocation: toPtr("/location"),
			Parameters:     map[string]any{"p": "v"},
			PlainHTTP:      toPtr(true),
		},
	}
	recipe := &RecipePackResource{
		ID:       toPtr("/id"),
		Name:     toPtr("name"),
		Type:     toPtr("type"),
		Location: toPtr("loc"),
		Tags:     map[string]*string{"t": toPtr("v")},
		Properties: &RecipePackProperties{
			ProvisioningState: toProvisioningStatePtr(ProvisioningStateProvisioning),
			Description:       &desc,
			ReferencedBy:      referenced,
			Recipes:           recipes,
		},
	}
	model, err := recipe.ConvertTo()
	if err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}
	m := model.(*datamodel.RecipePack)
	if m.Properties.Description != desc {
		t.Errorf("Description not converted")
	}
	if !reflect.DeepEqual(m.Properties.ReferencedBy, []string{"/foo", "/bar"}) {
		t.Errorf("ReferencedBy not converted")
	}
	if len(m.Properties.Recipes) != 1 {
		t.Errorf("Recipes not converted")
	}
	if m.Properties.Recipes["r1"].RecipeKind != "Container" {
		t.Errorf("RecipeKind not converted")
	}
	if m.Properties.Recipes["r1"].PlainHTTP != true {
		t.Errorf("PlainHTTP not converted")
	}
}

func TestConvertFrom_Minimal(t *testing.T) {
	dm := &datamodel.RecipePack{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       "/id",
				Name:     "name",
				Type:     "type",
				Location: "loc",
				Tags:     map[string]string{"t": "v"},
			},
			InternalMetadata: v1.InternalMetadata{
				AsyncProvisioningState: "Provisioning",
			},
		},
		Properties: datamodel.RecipePackProperties{},
	}
	var dst RecipePackResource
	err := dst.ConvertFrom(dm)
	if err != nil {
		t.Fatalf("ConvertFrom failed: %v", err)
	}
	if *dst.ID != dm.ID || *dst.Name != dm.Name || *dst.Type != dm.Type || *dst.Location != dm.Location {
		t.Errorf("Basic fields not converted correctly")
	}
	if dst.Properties.ProvisioningState == nil || *dst.Properties.ProvisioningState != ProvisioningStateProvisioning {
		t.Errorf("ProvisioningState not converted")
	}
}

func TestConvertFrom_Full(t *testing.T) {
	desc := "desc"
	referenced := []string{"/foo", "/bar"}
	recipes := map[string]*datamodel.RecipeDefinition{
		"r1": {
			RecipeKind:     "Container",
			RecipeLocation: "/location",
			Parameters:     map[string]any{"p": "v"},
			PlainHTTP:      true,
		},
	}
	dm := &datamodel.RecipePack{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       "/id",
				Name:     "name",
				Type:     "type",
				Location: "loc",
				Tags:     map[string]string{"t": "v"},
			},
			InternalMetadata: v1.InternalMetadata{
				AsyncProvisioningState: "Provisioning",
			},
		},
		Properties: datamodel.RecipePackProperties{
			Description:  desc,
			ReferencedBy: referenced,
			Recipes:      recipes,
		},
	}
	var dst RecipePackResource
	err := dst.ConvertFrom(dm)
	if err != nil {
		t.Fatalf("ConvertFrom failed: %v", err)
	}
	if dst.Properties.Description == nil || *dst.Properties.Description != desc {
		t.Errorf("Description not converted")
	}
	if len(dst.Properties.ReferencedBy) != 2 {
		t.Errorf("ReferencedBy not converted")
	}
	if len(dst.Properties.Recipes) != 1 {
		t.Errorf("Recipes not converted")
	}
	if *dst.Properties.Recipes["r1"].RecipeKind != RecipeKind("Container") {
		t.Errorf("RecipeKind not converted")
	}
	if dst.Properties.Recipes["r1"].PlainHTTP == nil || *dst.Properties.Recipes["r1"].PlainHTTP != true {
		t.Errorf("PlainHTTP not converted")
	}
}

// helpers
func toPtr[T any](v T) *T                                           { return &v }
func toRecipeKindPtr(s string) *RecipeKind                          { k := RecipeKind(s); return &k }
func toProvisioningStatePtr(s ProvisioningState) *ProvisioningState { return &s }
