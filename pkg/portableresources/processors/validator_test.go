/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package processors

import (
	"testing"

	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

type testDatamodel struct {
	StringField            string
	AnotherCoolStringField string
	Int32Field             int32
	BooleanField           bool
}

func Test_NewValidator(t *testing.T) {
	t.Run("empty datastores", func(t *testing.T) {
		var outputResources []rpv1.OutputResource
		var values map[string]any
		var secrets map[string]rpv1.SecretValueReference

		v := NewValidator(&values, &secrets, &outputResources)
		require.NotNil(t, v)
		require.NotNil(t, outputResources)
		require.NotNil(t, values)
		require.NotNil(t, secrets)

		require.Same(t, &outputResources, v.OutputResources)
		require.Equal(t, values, v.ConnectionValues)
		require.Equal(t, secrets, v.ConnectionSecrets)
	})

	t.Run("provided datastores", func(t *testing.T) {
		outputResources := []rpv1.OutputResource{
			{},
		}
		values := map[string]any{"test": ""}
		secrets := map[string]rpv1.SecretValueReference{"test": {}}

		v := NewValidator(&values, &secrets, &outputResources)
		require.NotNil(t, v)
		require.NotNil(t, outputResources)
		require.NotNil(t, values)
		require.NotNil(t, secrets)

		require.Same(t, &outputResources, v.OutputResources)
		require.Equal(t, values, v.ConnectionValues)
		require.Equal(t, secrets, v.ConnectionSecrets)

		require.Empty(t, outputResources)
		require.Empty(t, values)
		require.Empty(t, secrets)
	})
}

func Test_Validator_SetAndValidate_OutputResources(t *testing.T) {
	t.Run("no resource field and no recipe", func(t *testing.T) {
		outputResources := []rpv1.OutputResource{}
		values := map[string]any{}
		secrets := map[string]rpv1.SecretValueReference{}

		v := NewValidator(&values, &secrets, &outputResources)
		err := v.SetAndValidate(nil)
		require.NoError(t, err)

		require.Empty(t, outputResources)
	})
	t.Run("resources field is nil", func(t *testing.T) {
		outputResources := []rpv1.OutputResource{}
		values := map[string]any{}
		secrets := map[string]rpv1.SecretValueReference{}

		var resources *[]*portableresources.ResourceReference

		v := NewValidator(&values, &secrets, &outputResources)
		v.AddResourcesField(resources)

		err := v.SetAndValidate(nil)
		require.NoError(t, err)

		require.Empty(t, outputResources)
	})
	t.Run("resources field is empty", func(t *testing.T) {
		outputResources := []rpv1.OutputResource{}
		values := map[string]any{}
		secrets := map[string]rpv1.SecretValueReference{}

		resources := []*portableresources.ResourceReference{}

		v := NewValidator(&values, &secrets, &outputResources)
		v.AddResourcesField(&resources)

		err := v.SetAndValidate(nil)
		require.NoError(t, err)

		require.Empty(t, outputResources)
	})
	t.Run("recipe has no resources", func(t *testing.T) {
		outputResources := []rpv1.OutputResource{}
		values := map[string]any{}
		secrets := map[string]rpv1.SecretValueReference{}

		v := NewValidator(&values, &secrets, &outputResources)

		err := v.SetAndValidate(&recipes.RecipeOutput{})
		require.NoError(t, err)

		require.Empty(t, outputResources)
	})
	t.Run("resources field invalid id", func(t *testing.T) {
		outputResources := []rpv1.OutputResource{}
		values := map[string]any{}
		secrets := map[string]rpv1.SecretValueReference{}

		resources := []*portableresources.ResourceReference{{ID: "////invalid//////"}}

		v := NewValidator(&values, &secrets, &outputResources)
		v.AddResourcesField(&resources)

		err := v.SetAndValidate(nil)
		require.Error(t, err)
		require.IsType(t, &ValidationError{}, err)
		require.Equal(t, "resource id \"////invalid//////\" is invalid", err.Error())

	})

	t.Run("resources success", func(t *testing.T) {
		outputResources := []rpv1.OutputResource{}
		values := map[string]any{}
		secrets := map[string]rpv1.SecretValueReference{}

		resourcesField := []*portableresources.ResourceReference{
			{
				ID: "/planes/aws/aws/accounts/1234/regions/us-west-1/providers/AWS.Kinesis/Stream/my-stream1",
			},
		}
		or := []rpv1.OutputResource{}
		for _, resource := range []string{"/planes/aws/aws/accounts/1234/regions/us-west-1/providers/AWS.Kinesis/Stream/my-stream2"} {
			id, err := resources.ParseResource(resource)
			require.NoError(t, err)
			result := rpv1.OutputResource{
				ID:            id,
				RadiusManaged: to.Ptr(true),
			}
			or = append(or, result)
		}
		output := recipes.RecipeOutput{
			OutputResources: or,
		}

		v := NewValidator(&values, &secrets, &outputResources)
		v.AddResourcesField(&resourcesField)

		err := v.SetAndValidate(&output)
		require.NoError(t, err)

		expected := []rpv1.OutputResource{
			{
				ID:            or[0].ID,
				RadiusManaged: to.Ptr(true),
			},
			{
				ID:            or[0].ID,
				RadiusManaged: to.Ptr(false),
			},
		}

		require.Equal(t, expected, outputResources)
	})
}

func Test_Validator_SetAndValidate_Required_Strings(t *testing.T) {
	t.Run("existing required value preserved", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "existing",
		}

		v.AddRequiredStringField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": "ignored"}})
		require.NoError(t, err)
		require.Equal(t, "existing", model.StringField)
		require.Equal(t, "existing", v.ConnectionValues["test"])
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("existing required secret preserved", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "existing",
		}

		v.AddRequiredSecretField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{Secrets: map[string]any{"test": "ignored"}})
		require.NoError(t, err)
		require.Equal(t, "existing", model.StringField)
		require.Equal(t, rpv1.SecretValueReference{Value: "existing"}, v.ConnectionSecrets["test"])
		require.Empty(t, v.ConnectionValues)
	})

	t.Run("required recipe value set", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddRequiredStringField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": "new"}})
		require.NoError(t, err)
		require.Equal(t, "new", model.StringField)
		require.Equal(t, "new", v.ConnectionValues["test"])
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("required recipe secret set", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddRequiredSecretField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{Secrets: map[string]any{"test": "new"}})
		require.NoError(t, err)
		require.Equal(t, "new", model.StringField)
		require.Equal(t, rpv1.SecretValueReference{Value: "new"}, v.ConnectionSecrets["test"])
		require.Empty(t, v.ConnectionValues)
	})

	t.Run("required value missing with recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddRequiredStringField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{})
		require.Error(t, err)
		require.IsType(t, &ValidationError{}, err)
		require.Equal(t, "the connection value \"test\" should be provided by the recipe, set '.properties.test' to provide a value manually", err.Error())
	})

	t.Run("required value missing without recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddRequiredStringField("test", &model.StringField)

		err := v.SetAndValidate(nil)
		require.Error(t, err)
		require.IsType(t, &ValidationError{}, err)
		require.Equal(t, "the connection value \"test\" must be provided when not using a recipe. Set '.properties.test' to provide a value manually", err.Error())
	})

	t.Run("required secret missing with recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddRequiredSecretField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{})
		require.Error(t, err)
		require.IsType(t, &ValidationError{}, err)
		require.Equal(t, "the connection secret \"test\" should be provided by the recipe, set '.properties.secrets.test' to provide a value manually", err.Error())
	})

	t.Run("required secret missing without recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddRequiredSecretField("test", &model.StringField)

		err := v.SetAndValidate(nil)
		require.Error(t, err)
		require.IsType(t, &ValidationError{}, err)
		require.Equal(t, "the connection secret \"test\" must be provided when not using a recipe. Set '.properties.secrets.test' to provide a value manually", err.Error())
	})
}

func Test_Validator_SetAndValidate_Optional_Strings(t *testing.T) {
	t.Run("existing optional value preserved", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "existing",
		}

		v.AddOptionalStringField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": "ignored"}})
		require.NoError(t, err)
		require.Equal(t, "existing", model.StringField)
		require.Equal(t, "existing", v.ConnectionValues["test"])
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("existing optional secret preserved", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "existing",
		}

		v.AddOptionalSecretField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{Secrets: map[string]any{"test": "ignored"}})
		require.NoError(t, err)
		require.Equal(t, "existing", model.StringField)
		require.Equal(t, rpv1.SecretValueReference{Value: "existing"}, v.ConnectionSecrets["test"])
		require.Empty(t, v.ConnectionValues)
	})

	t.Run("optional recipe value set", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddOptionalStringField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": "new"}})
		require.NoError(t, err)
		require.Equal(t, "new", model.StringField)
		require.Equal(t, "new", v.ConnectionValues["test"])
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("Optional recipe secret set", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddOptionalSecretField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{Secrets: map[string]any{"test": "new"}})
		require.NoError(t, err)
		require.Equal(t, "new", model.StringField)
		require.Equal(t, rpv1.SecretValueReference{Value: "new"}, v.ConnectionSecrets["test"])
		require.Empty(t, v.ConnectionValues)
	})

	t.Run("optional value missing with recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddOptionalStringField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{})
		require.NoError(t, err)
		require.Equal(t, "", model.StringField)
		require.Empty(t, v.ConnectionValues)
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("optional value missing without recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddOptionalStringField("test", &model.StringField)

		err := v.SetAndValidate(nil)
		require.NoError(t, err)
		require.Equal(t, "", model.StringField)
		require.Empty(t, v.ConnectionValues)
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("optional secret missing with recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddOptionalSecretField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{})
		require.NoError(t, err)
		require.Equal(t, "", model.StringField)
		require.Empty(t, v.ConnectionValues)
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("optional secret missing without recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}
		v.AddOptionalSecretField("test", &model.StringField)

		err := v.SetAndValidate(nil)
		require.NoError(t, err)
		require.Equal(t, "", model.StringField)
		require.Empty(t, v.ConnectionValues)
		require.Empty(t, v.ConnectionSecrets)
	})
}

func Test_Validator_SetAndValidate_Computed_Strings(t *testing.T) {
	// Code path for computed strings is the same as optional, except when the value is missing.\
	t.Run("computed secret is computed with recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField:            "",
			AnotherCoolStringField: "",
		}
		v.AddComputedSecretField("computed", &model.AnotherCoolStringField, func() (string, *ValidationError) {
			// Computed values have access to non-computed values
			return model.StringField + "-computed", nil
		})
		v.AddOptionalStringField("regular", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"regular": "YO"}})
		require.NoError(t, err)
		require.Equal(t, "YO", model.StringField)
		require.Equal(t, "YO-computed", model.AnotherCoolStringField)
		require.Equal(t, map[string]any{"regular": "YO"}, v.ConnectionValues)
		require.Equal(t, map[string]rpv1.SecretValueReference{"computed": {Value: "YO-computed"}}, v.ConnectionSecrets)
	})

	t.Run("computed secret is computed without recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField:            "YO",
			AnotherCoolStringField: "",
		}
		v.AddComputedSecretField("computed", &model.AnotherCoolStringField, func() (string, *ValidationError) {
			// Computed values have access to non-computed values
			return model.StringField + "-computed", nil
		})
		v.AddOptionalStringField("regular", &model.StringField)

		err := v.SetAndValidate(nil)
		require.NoError(t, err)
		require.Equal(t, "YO", model.StringField)
		require.Equal(t, "YO-computed", model.AnotherCoolStringField)
		require.Equal(t, map[string]any{"regular": "YO"}, v.ConnectionValues)
		require.Equal(t, map[string]rpv1.SecretValueReference{"computed": {Value: "YO-computed"}}, v.ConnectionSecrets)
	})

	t.Run("computed secret error", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField:            "",
			AnotherCoolStringField: "",
		}
		v.AddComputedSecretField("computed", &model.AnotherCoolStringField, func() (string, *ValidationError) {
			return "", &ValidationError{Message: "OH NO!"}
		})
		v.AddOptionalStringField("regular", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"regular": "YO"}})
		require.Error(t, err)
		require.IsType(t, &ValidationError{}, err)
		require.Equal(t, "OH NO!", err.Error())
	})

	t.Run("computed secret not run if regular fields error", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField:            "",
			AnotherCoolStringField: "",
		}
		v.AddComputedSecretField("computed", &model.AnotherCoolStringField, func() (string, *ValidationError) {
			t.FailNow() // Should not be called
			return "", &ValidationError{Message: "OH NO!"}
		})
		v.AddRequiredStringField("regular", &model.StringField)

		err := v.SetAndValidate(nil)
		require.Error(t, err)
		require.IsType(t, &ValidationError{}, err)
		require.Equal(t, "the connection value \"regular\" must be provided when not using a recipe. Set '.properties.regular' to provide a value manually", err.Error())
	})
}

func Test_Validator_SetAndValidate_Computed_Boolean(t *testing.T) {
	// Code path for computed booleans is the same as optional, except when the value is missing.\
	t.Run("computed value is computed with recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			BooleanField: false,
			Int32Field:   6380,
		}
		v.AddComputedBoolField("computed", &model.BooleanField, func() (bool, *ValidationError) {
			// Computed values have access to non-computed values
			return model.Int32Field == 6380, nil
		})
		v.AddOptionalInt32Field("regular", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"regular": 6380}})
		require.NoError(t, err)
		require.Equal(t, true, model.BooleanField)
		require.Equal(t, map[string]any{"regular": int32(6380), "computed": true}, v.ConnectionValues)
	})

	t.Run("computed value is computed without recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			BooleanField: false,
			Int32Field:   6379,
		}
		v.AddComputedBoolField("computed", &model.BooleanField, func() (bool, *ValidationError) {
			// Computed values have access to non-computed values
			return model.Int32Field == 6380, nil
		})
		v.AddOptionalInt32Field("regular", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"regular": 6380}})
		require.NoError(t, err)
		require.Equal(t, false, model.BooleanField)
		require.Equal(t, map[string]any{"regular": int32(6379), "computed": false}, v.ConnectionValues)
	})

	t.Run("computed value with recipe override", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			BooleanField: false,
			Int32Field:   6380,
		}
		v.AddComputedBoolField("computed", &model.BooleanField, func() (bool, *ValidationError) {
			// Computed values have access to non-computed values
			return model.Int32Field == 6380, nil
		})
		v.AddOptionalInt32Field("regular", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"regular": 6380}})
		require.NoError(t, err)
		require.Equal(t, true, model.BooleanField)
		require.Equal(t, map[string]any{"regular": int32(6380), "computed": true}, v.ConnectionValues)
	})
}

func Test_Validator_SetAndValidate_TypeMismatch_Strings(t *testing.T) {
	// Type mismatches are only possible with recipes
	t.Run("type mismatch with recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			StringField: "",
		}

		v.AddRequiredStringField("test", &model.StringField)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": 3}})
		require.Error(t, err)
		require.IsType(t, &ValidationError{}, err)
		require.Equal(t, "the connection value \"test\" provided by the recipe is expected to be a string, got int", err.Error())
	})
}

func Test_Validator_SetAndValidate_Required_Int32(t *testing.T) {
	t.Run("existing required value preserved", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			Int32Field: 47,
		}

		v.AddRequiredInt32Field("test", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": int32(43)}})
		require.NoError(t, err)
		require.Equal(t, int32(47), model.Int32Field)
		require.Equal(t, int32(47), v.ConnectionValues["test"])
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("required recipe value set", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			Int32Field: 0,
		}
		v.AddRequiredInt32Field("test", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": int32(43)}})
		require.NoError(t, err)
		require.Equal(t, int32(43), model.Int32Field)
		require.Equal(t, int32(43), v.ConnectionValues["test"])
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("required value missing with recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			Int32Field: 0,
		}
		v.AddRequiredInt32Field("test", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{})
		require.Error(t, err)
		require.IsType(t, &ValidationError{}, err)
		require.Equal(t, "the connection value \"test\" should be provided by the recipe, set '.properties.test' to provide a value manually", err.Error())
	})

	t.Run("required value missing without recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			Int32Field: 0,
		}
		v.AddRequiredInt32Field("test", &model.Int32Field)

		err := v.SetAndValidate(nil)
		require.Error(t, err)
		require.IsType(t, &ValidationError{}, err)
		require.Equal(t, "the connection value \"test\" must be provided when not using a recipe. Set '.properties.test' to provide a value manually", err.Error())
	})
}

func Test_Validator_SetAndValidate_Optional_Int32(t *testing.T) {
	t.Run("existing optional value preserved", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			Int32Field: 47,
		}

		v.AddOptionalInt32Field("test", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": int32(43)}})
		require.NoError(t, err)
		require.Equal(t, int32(47), model.Int32Field)
		require.Equal(t, int32(47), v.ConnectionValues["test"])
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("optional recipe value set", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			Int32Field: 0,
		}
		v.AddOptionalInt32Field("test", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": int32(43)}})
		require.NoError(t, err)
		require.Equal(t, int32(43), model.Int32Field)
		require.Equal(t, int32(43), v.ConnectionValues["test"])
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("optional value missing with recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			Int32Field: 0,
		}
		v.AddOptionalInt32Field("test", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{})
		require.NoError(t, err)
		require.Equal(t, int32(0), model.Int32Field)
		require.Empty(t, v.ConnectionValues)
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("optional value missing without recipe", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			Int32Field: 0,
		}
		v.AddOptionalInt32Field("test", &model.Int32Field)

		err := v.SetAndValidate(nil)
		require.NoError(t, err)
		require.Equal(t, int32(0), model.Int32Field)
		require.Empty(t, v.ConnectionValues)
		require.Empty(t, v.ConnectionSecrets)
	})
}

func Test_Validator_TypeConversions_Int32(t *testing.T) {
	t.Run("conversion from int", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			Int32Field: 0,
		}
		v.AddOptionalInt32Field("test", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": int(43)}})
		require.NoError(t, err)
		require.Equal(t, int32(43), model.Int32Field)
		require.Equal(t, int32(43), v.ConnectionValues["test"])
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("conversion from float64", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			Int32Field: 0,
		}
		v.AddOptionalInt32Field("test", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": float64(43.1)}})
		require.NoError(t, err)
		require.Equal(t, int32(43), model.Int32Field)
		require.Equal(t, int32(43), v.ConnectionValues["test"])
		require.Empty(t, v.ConnectionSecrets)
	})

	t.Run("failed conversion", func(t *testing.T) {
		v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

		model := testDatamodel{
			Int32Field: 0,
		}
		v.AddOptionalInt32Field("test", &model.Int32Field)

		err := v.SetAndValidate(&recipes.RecipeOutput{Values: map[string]any{"test": "heyyyyy"}})
		require.Error(t, err)
		require.IsType(t, &ValidationError{}, err)
		require.Equal(t, "the connection value \"test\" provided by the recipe is expected to be a int32, got string", err.Error())
	})
}

func Test_Validator_SetAndValidate_MultipleErrors(t *testing.T) {
	v := NewValidator(&map[string]any{}, &map[string]rpv1.SecretValueReference{}, &[]rpv1.OutputResource{})

	model := testDatamodel{
		StringField:            "",
		AnotherCoolStringField: "",
	}
	v.AddRequiredStringField("one", &model.StringField)
	v.AddRequiredStringField("two", &model.AnotherCoolStringField)

	err := v.SetAndValidate(nil)
	require.Error(t, err)
	require.IsType(t, &ValidationError{}, err)
	require.Equal(t, "validation returned multiple errors:\n\nthe connection value \"one\" must be provided when not using a recipe. Set '.properties.one' to provide a value manually\nthe connection value \"two\" must be provided when not using a recipe. Set '.properties.two' to provide a value manually", err.Error())
}
