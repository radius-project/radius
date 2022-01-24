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
	n1 := "c"
	n2 := "a"
	n3 := "b"

	t1 := "aType"
	t2 := "bType"
	t3 := "aType"

	input := radclient.RadiusResourceList{

		Value: []*radclient.RadiusResource{
			{
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n1, Type: &t1}},
			},
			{
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n2, Type: &t2}},
			},
			{
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n3, Type: &t3}},
			},
		},
	}

	s := ResourceListSorter{}

	expectedOutput := radclient.RadiusResourceList{

		Value: []*radclient.RadiusResource{
			{
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n3, Type: &t3}},
			},
			{
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n1, Type: &t1}},
			},
			{
				ProxyResource: radclient.ProxyResource{Resource: radclient.Resource{Name: &n2, Type: &t2}},
			},
		},
	}

	for i := 0; i < 5; i++ {
		output, err := s.SortResourceList(input)
		require.NoError(t, err)
		require.Equal(t, output, expectedOutput)
	}
}
