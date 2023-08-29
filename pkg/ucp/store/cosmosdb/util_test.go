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

package cosmosdb

import (
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestNormalizeLetterOrDigitToUpper(t *testing.T) {
	testStrings := []struct {
		in  string
		out string
	}{
		{"00000000-0000-0000-1000-000000000001", "00000000000000001000000000000001"},
		{"test-GROUp", "TESTGROUP"},
		{"WEST US", "WESTUS"},
	}

	for _, tc := range testStrings {
		t.Run(tc.in, func(t *testing.T) {
			result := NormalizeLetterOrDigitToUpper(tc.in)
			require.Equal(t, tc.out, result)
		})
	}
}

func TestSubscriptionID(t *testing.T) {
	testStrings := []struct {
		in  string
		out string
	}{
		{"00000000-0000-0000-1000-000000000001", "00000000000000001000000000000001"},
		{"eaf9116d-84e7-4720-a841-67ca2b67f888", "EAF9116D84E74720A84167CA2B67F888"},
		{"b2c7913e-e1fe-4c1d-a843-212159d07e46", "B2C7913EE1FE4C1DA843212159D07E46"},
	}

	for _, tc := range testStrings {
		t.Run(tc.in, func(t *testing.T) {
			result := NormalizeSubscriptionID(tc.in)
			require.Equal(t, tc.out, result)
		})
	}
}

func TestEscapedStorageKey(t *testing.T) {
	escapedTests := []struct {
		in  string
		out string
	}{
		{"testgroup", "testgroup"},
		{"test-group", "test:2Dgroup"},
		{"/subscriptions/sub/resourceGroups/rgname", ":2Fsubscriptions:2Fsub:2FresourceGroups:2Frgname"},
	}

	for _, tc := range escapedTests {
		t.Run(tc.in, func(t *testing.T) {
			escaped := EscapedStorageKey(tc.in)
			require.Equal(t, tc.out, escaped)
		})
	}
}

func TestTrimStorageKey(t *testing.T) {
	trimTests := []struct {
		in  string
		len int
		out string
		err error
	}{
		{"subscripti", 10, "", &store.ErrInvalid{Message: "storage key is too short"}},
		{"subscriptions|0000000000000000|testGroup", StorageKeyTrimPaddingLen, "", &store.ErrInvalid{Message: "storage key is not properly encoded"}},
		{"subscriptions/0000000000000000/testGroup", StorageKeyTrimPaddingLen, "|DCE4A54F0A69CD0F", nil},
		{"subscriptions/00000000000000001000000000000001/resourceGroups/testGroup", 20, "sub|DB99FE979E7C972C", nil},
		{"subscriptions/00000000000000001000000000000001/resourceGroups/testGroup", 80, "subscriptions/00000000000000001000000000000001/resourceGroups/testGroup", nil},
	}

	for _, tc := range trimTests {
		t.Run(tc.in, func(t *testing.T) {
			trimed, err := TrimStorageKey(tc.in, tc.len)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.out, trimed)
		})
	}
}

func TestNormalizeStorageKey(t *testing.T) {
	trimTests := []struct {
		in  string
		len int
		out string
		err error
	}{
		{"subscripti", 10, "", &store.ErrInvalid{Message: "storage key is too short"}},
		{"subscriptions/0000000000000000/testGroup", StorageKeyTrimPaddingLen, "|7A4B44E13072BE17", nil},
		{"subscriptions/00000000000000001000000000000001/resourceGroups/testGroup", 20, "SUB|10844510550A50BD", nil},
		{"subscriptions/00000000000000001000000000000001/resourceGroups/testGroup", 80, "SUBSCRIPTIONS:2F00000000000000001000000000000001:2FRESOURCEGROUPS:2FTESTGROUP", nil},
	}

	for _, tc := range trimTests {
		t.Run(tc.in, func(t *testing.T) {
			trimed, err := NormalizeStorageKey(tc.in, tc.len)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.out, trimed)
		})
	}
}

func TestGenerateCosmosDBKey(t *testing.T) {
	cases := []struct {
		desc   string
		fullID string
		out    string
		err    error
	}{
		{
			"env-success-1",
			"/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testGroup/providers/applications.core/environments/env0",
			"00000000000000001000000000000001-TESTGROUP-APPLICATIONS:2ECORE:2FENVIRONMENTS:2FENV0",
			nil,
		},
		{
			"env-success-2",
			"/subscriptions/eaf9116d-84e7-4720-a841-67ca2b67f888/resourcegroups/testGroup/providers/Applications.Core/environments/appenv",
			"EAF9116D84E74720A84167CA2B67F888-TESTGROUP-APPLICATIONS:2ECORE:2FENVIRONMENTS:2FAPPENV",
			nil,
		},
		{
			"env-no-rg-success",
			"/subscriptions/00000000-0000-0000-1000-000000000001/providers/Applications.Core/environments/env0",
			"00000000000000001000000000000001-APPLICATIONS:2ECORE:2FENVIRONMENTS:2FENV0",
			nil,
		},
		{
			"os-success",
			"/subscriptions/00000000-0000-0000-1000-000000000001/providers/Applications.Core/locations/westus/operationStatuses/os1",
			"00000000000000001000000000000001-APPLICATIONS:2ECORE:2FLOCATIONS:2FWESTUS:2FOPERATIONSTATUSES:2FOS1",
			nil,
		},
		{
			"app-success",
			"/subscriptions/7826d962-510f-407a-92a2-5aeb37aa7b6e/resourcegroups/radius-westus/providers/Applications.Core/applications/todoapp",
			"7826D962510F407A92A25AEB37AA7B6E-RADIUS:2DWESTUS-APPLICATIONS:2ECORE:2FAPPLICATIONS:2FTODOAPP",
			nil,
		},
		{
			"app-long-name-success",
			"/subscriptions/7826d962-510f-407a-92a2-5aeb37aa7b6e/resourcegroups/radius-westus/providers/Applications.Core/applications/longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1",
			"7826D962510F407A92A25AEB37AA7B6E-RADIUS:2DWESTUS-APPLICATIONS:2ECORE:2FAPPLICATIONS:2FLONGAPPLICATIONNAME1LONGAPPLICATIONNAME1LONGAPPLICATIONNAME1LONGAPPLICATIONNAME1LONGAPPLICATIONNAME1LON|651E511DBBDDC783",
			nil,
		},
		{
			"app-long-resource-name-success",
			"/subscriptions/7826d962-510f-407a-92a2-5aeb37aa7b6e/resourcegroups/radius-westus/providers/Applications.Core/longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0/longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1",
			"7826D962510F407A92A25AEB37AA7B6E-RADIUS:2DWESTUS-APPLICATIONS:2ECORE:2FLONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME|279366913EF52FC7",
			nil,
		},
		{
			"app-long-rg-app-names-success",
			"/subscriptions/7826d962-510f-407a-92a2-5aeb37aa7b6e/resourcegroups/longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0/providers/Applications.Core/longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0/longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1",
			"7826D962510F407A92A25AEB37AA7B6E-LONGRESOURCEGROUP0LONGRESOURCEGROUP0LONGRESOURC|EF662FD5E8286859-APPLICATIONS:2ECORE:2FLONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME|279366913EF52FC7",
			nil,
		},
		{
			"ucp-success",
			"/planes/radius/local/resourcegroups/radius-westus/providers/Applications.Core/applications/todoapp",
			"-RADIUS:2DWESTUS-APPLICATIONS:2ECORE:2FAPPLICATIONS:2FTODOAPP",
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			testID, err := resources.Parse(tc.fullID)
			require.NoError(t, err)
			key, err := GenerateCosmosDBKey(testID)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.out, key)
			require.LessOrEqual(t, len(key), 255)
		})
	}
}
