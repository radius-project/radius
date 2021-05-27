// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rad

import (
	"bytes"
	"testing"

	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func Test_ReadEnvironmentSection_NoContent(t *testing.T) {
	var yaml = ``

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadEnvironmentSection(v)
	require.NoError(t, err)
	require.Empty(t, es.Default)
	require.Empty(t, es.Items)
}

func Test_ReadEnvironmentSection_SomeItems(t *testing.T) {
	var yaml = `
environment:
  default: test
  items:
    test:
      kind: testing
    test2:
      kind: testing
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadEnvironmentSection(v)
	require.NoError(t, err)
	require.Equal(t, es.Default, "test")
	require.Len(t, es.Items, 2)
}

func Test_GetEnvironment_Invalid_NoKind(t *testing.T) {
	var yaml = `
environment:
  items:
    test:
      someProperty: test
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadEnvironmentSection(v)
	require.NoError(t, err)

	_, err = es.GetEnvironment("test")
	require.Error(t, err)
}

func Test_GetEnvironment_Invalid_NotFound(t *testing.T) {
	var yaml = `
environment:
  items:
    another: {}
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadEnvironmentSection(v)
	require.NoError(t, err)

	_, err = es.GetEnvironment("test")
	require.Error(t, err)
}

func Test_GetEnvironment_Invalid_KindIsNotString(t *testing.T) {
	var yaml = `
environment:
  default: test
  items:
    test:
      kind: 3
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadEnvironmentSection(v)
	require.NoError(t, err)

	_, err = es.GetEnvironment("test")
	require.Error(t, err)
}

func Test_GetEnvironment_Invalid_AzureEnvironmentMissingProperties(t *testing.T) {
	var yaml = `
environment:
  default: test
  items:
    test:
      kind: azure
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadEnvironmentSection(v)
	require.NoError(t, err)

	_, err = es.GetEnvironment("test")
	require.Error(t, err)
}

func Test_GetEnvironment_ValidAzureEnvironment(t *testing.T) {
	var yaml = `
environment:
  default: test
  items:
    test:
      kind: azure
      subscriptionid: testsub
      resourcegroup: testrg
      clustername: testcluster
      extra: testextra
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadEnvironmentSection(v)
	require.NoError(t, err)

	e, err := es.GetEnvironment("")
	require.NoError(t, err)

	aenv, ok := e.(*environments.AzureCloudEnvironment)
	require.True(t, ok)

	require.Equal(t, "test", aenv.Name)
	require.Equal(t, "azure", aenv.Kind)
	require.Equal(t, "testsub", aenv.SubscriptionID)
	require.Equal(t, "testrg", aenv.ResourceGroup)
	require.Equal(t, map[string]interface{}{"extra": "testextra"}, aenv.Properties)
}

func Test_GetEnvironment_ValidGenericEnvironment(t *testing.T) {
	var yaml = `
environment:
  default: test
  items:
    test:
      kind: other
      extra: testextra
`

	v, err := makeConfig(yaml)
	require.NoError(t, err)

	es, err := ReadEnvironmentSection(v)
	require.NoError(t, err)

	e, err := es.GetEnvironment("")
	require.NoError(t, err)

	aenv, ok := e.(*environments.GenericEnvironment)
	require.True(t, ok)

	require.Equal(t, "test", aenv.Name)
	require.Equal(t, "other", aenv.Kind)
	require.Equal(t, map[string]interface{}{"extra": "testextra"}, aenv.Properties)
}

func makeConfig(yaml string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigType("YAML")
	err := v.ReadConfig(bytes.NewBuffer([]byte(yaml)))
	if err != nil {
		return nil, err
	}

	return v, nil
}
