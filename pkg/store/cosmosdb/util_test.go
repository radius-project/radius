// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"testing"

	"github.com/project-radius/radius/pkg/store"
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
		{"/subscriptions/sub/resourceGroup/rgname", ":2Fsubscriptions:2Fsub:2FresourceGroup:2Frgname"},
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

// TestGenerateCosmosDBKey creates compliant cosmosdb id using arm id. The length of the generated id must be less than 256.
func TestGenerateCosmosDBKey(t *testing.T) {
	trimTests := []struct {
		subID  string
		rgName string
		fqType string
		fqName string
		out    string
		err    error
	}{
		{
			"00000000-0000-0000-1000-000000000001",
			"testGroup",
			"applications.core/environments",
			"env0",
			"00000000000000001000000000000001-TESTGROUP-APPLICATIONS:2ECORE:2FENVIRONMENTSENV0",
			nil,
		}, {
			"eaf9116d-84e7-4720-a841-67ca2b67f888",
			"testGroup",
			"applications.core/environments",
			"appenv",
			"EAF9116D84E74720A84167CA2B67F888-TESTGROUP-APPLICATIONS:2ECORE:2FENVIRONMENTSAPPENV",
			nil,
		}, {
			"7826d962-510f-407a-92a2-5aeb37aa7b6e",
			"radius-westus",
			"applications.core/applications",
			"todoapp",
			"7826D962510F407A92A25AEB37AA7B6E-RADIUS:2DWESTUS-APPLICATIONS:2ECORE:2FAPPLICATIONSTODOAPP",
			nil,
		}, {
			"7826d962-510f-407a-92a2-5aeb37aa7b6e",
			"radius-westus",
			"applications.core/applications",
			"longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1",
			"7826D962510F407A92A25AEB37AA7B6E-RADIUS:2DWESTUS-APPLICATIONS:2ECORE:2FAPPLICATIONSLONGAPPLICATIONNAME1LONGAPPLICATIONNAME1LONGAPPLICATIONNAME1LONGAPPLICATIONNAME1LONGAPPLICATIONNAME1LONGAP|F1FC9C622C380B24",
			nil,
		}, {
			"7826d962-510f-407a-92a2-5aeb37aa7b6e",
			"radius-westus",
			"applications.core/longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0",
			"longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1",
			"7826D962510F407A92A25AEB37AA7B6E-RADIUS:2DWESTUS-APPLICATIONS:2ECORE:2FLONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME|8C5656AB3F61108E",
			nil,
		}, {
			"7826d962-510f-407a-92a2-5aeb37aa7b6e",
			"longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0longresourcegroup0",
			"applications.core/longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0longresourcename0",
			"longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1longapplicationname1",
			"7826D962510F407A92A25AEB37AA7B6E-LONGRESOURCEGROUP0LONGRESOURCEGROUP0LONGRESOURC|EF662FD5E8286859-APPLICATIONS:2ECORE:2FLONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME0LONGRESOURCENAME|8C5656AB3F61108E",
			nil,
		},
	}

	for _, tc := range trimTests {
		t.Run(tc.subID+tc.rgName+tc.fqType+tc.fqName, func(t *testing.T) {
			key, err := GenerateCosmosDBKey(tc.subID, tc.rgName, tc.fqType, tc.fqName)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.out, key)
			require.LessOrEqual(t, len(key), 255) // Max cosmosdb id length
		})
	}
}
