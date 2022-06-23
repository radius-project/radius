// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"testing"

	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/stretchr/testify/require"
)

type ResourceProps struct {
}

func Test_ResourceList_Sort(t *testing.T) {
	n1 := "a"
	n2 := "b"
	n3 := "c"
	n4 := "d"
	n5 := "e"

	t1 := "aType"
	t2 := "bType"
	t3 := "cType"
	t4 := "aType"
	t5 := "bType"

	input1 := radclient.RadiusResourceList{

		Value: []*radclient.RadiusResource{
			{
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n1, Type: &t1}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n4, Type: &t4}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n3, Type: &t3}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n5, Type: &t5}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n2, Type: &t2}},
			},
		},
	}

	input2 := radclient.RadiusResourceList{

		Value: []*radclient.RadiusResource{
			{
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n1, Type: &t1}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n4, Type: &t4}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n3, Type: &t3}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n5, Type: &t5}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n2, Type: &t2}},
			},
		},
	}

	input3 := radclient.RadiusResourceList{

		Value: []*radclient.RadiusResource{
			{
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n4, Type: &t4}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n2, Type: &t2}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n1, Type: &t1}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n5, Type: &t5}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n3, Type: &t3}},
			},
		},
	}

	expectedOutput := radclient.RadiusResourceList{

		Value: []*radclient.RadiusResource{
			{
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n1, Type: &t1}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n4, Type: &t4}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n2, Type: &t2}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n5, Type: &t5}},
			}, {
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n3, Type: &t3}},
			},
		},
	}

	output, err := SortResourceList(input1)
	require.NoError(t, err)
	require.Equal(t, output, expectedOutput)

	output, err = SortResourceList(input2)
	require.NoError(t, err)
	require.Equal(t, output, expectedOutput)

	output, err = SortResourceList(input3)
	require.NoError(t, err)
	require.Equal(t, output, expectedOutput)
}
