package datamodel

import (
	"reflect"
	"testing"

	"github.com/mitchellh/mapstructure"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/stretchr/testify/require"
)

type RadiusResource struct {
	Resource conv.DataModelInterface
}

func Test_TestReflectionIsCool(t *testing.T) {
	properties := DaprInvokeHttpRouteProperties{
		AppId: "A",
	}

	res := RadiusResource{
		Resource: &DaprInvokeHttpRoute{
			Properties: properties,
		},
	}

	// res.Resource might contain any of our data-model types...
	//
	// We want to get the value of res.Resource.Properties without coupling to a specific type.

	resourceValue := reflect.ValueOf(res.Resource).Elem()
	require.NotNil(t, resourceValue)

	propertyValue := resourceValue.FieldByName("Properties")
	require.NotEqual(t, reflect.Value{}, propertyValue)

	actual := propertyValue.Addr().Interface()
	expected := &properties
	require.Equal(t, expected, actual)

	err := mapstructure.Decode(map[string]any{"AppId": "B"}, actual)
	require.NoError(t, err)

	require.Equal(t, res.Resource.(*DaprInvokeHttpRoute).Properties.AppId, "B")
}
