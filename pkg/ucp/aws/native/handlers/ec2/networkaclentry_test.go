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

package ec2

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

func icmpEntry() map[string]any {
	return map[string]any{
		"NetworkAclId": "acl-1",
		"RuleNumber":   float64(200),
		"Egress":       true,
		"Protocol":     float64(1),
		"RuleAction":   "allow",
		"CidrBlock":    "10.0.0.0/16",
		"Icmp":         map[string]any{"Type": float64(8), "Code": float64(-1)},
	}
}

func Test_NetworkAclEntry_Identifier(t *testing.T) {
	h := NewNetworkAclEntry(nil)
	tests := []struct {
		name       string
		properties map[string]any
		want       string
	}{
		{"egress defaults to false", map[string]any{"NetworkAclId": "acl-1", "RuleNumber": float64(100)}, "acl-1|100|false"},
		{"egress", map[string]any{"NetworkAclId": "acl-1", "RuleNumber": int32(100), "Egress": true}, "acl-1|100|true"},
		{"rule number as int64", map[string]any{"NetworkAclId": "acl-1", "RuleNumber": int64(7)}, "acl-1|7|false"},
		{"no network ACL", map[string]any{"RuleNumber": 100}, ""},
		{"no rule number", map[string]any{"NetworkAclId": "acl-1"}, ""},
		{"rule number that is not a whole number", map[string]any{"NetworkAclId": "acl-1", "RuleNumber": 1.5}, ""},
		{"rule number as a string", map[string]any{"NetworkAclId": "acl-1", "RuleNumber": "100"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := h.Identifier(tt.properties)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	for _, identifier := range []string{"acl-1", "acl-1|x|false", "acl-1|100|maybe", "|100|false", "acl-1|100|false|x"} {
		_, err := parseEntryKey(identifier)
		var failure *native.Failure
		require.ErrorAs(t, err, &failure, identifier)
		require.Contains(t, failure.Message, "NetworkAclId|RuleNumber|Egress")
	}
}

func Test_NetworkAclEntry_Equivalent(t *testing.T) {
	tests := []struct {
		path     string
		template any
		aws      any
		want     bool
	}{
		{"Ipv6CidrBlock", "2001:DB8::/32", "2001:db8::/32", true},
		{"Ipv6CidrBlock", "2001:0db8:0000::/32", "2001:db8::/32", true},
		{"CidrBlock", "10.0.0.0/16", "10.0.0.0/16", true},
		{"CidrBlock", "10.0.0.0/16", "10.1.0.0/16", false},
		{"CidrBlock", "not a block", "10.0.0.0/16", false},
		{"CidrBlock", 10, "10.0.0.0/16", false},
		{"Protocol", "6", float64(6), true},
		{"Protocol", float64(6), "6", true},
		{"Protocol", float64(6), float64(17), false},
		{"Protocol", nil, "", false},
		{"RuleAction", "Allow", "allow", true},
		{"RuleAction", "allow", "deny", false},
		{"RuleAction", true, "allow", false},
		{"Egress", false, false, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, entryEquivalent(tt.path, tt.template, tt.aws), "%s %v %v", tt.path, tt.template, tt.aws)
	}
}

func Test_NetworkAclEntry_IcmpEntry(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewNetworkAclEntry(api)
	id, err := h.Create(t.Context(), createRequest(icmpEntry()))
	require.NoError(t, err)
	require.Equal(t, "acl-1|200|true", id)

	props, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"Id":           "acl-1|200|true",
		"NetworkAclId": "acl-1",
		"RuleNumber":   float64(200),
		"Egress":       true,
		"Protocol":     float64(1),
		"RuleAction":   "allow",
		"CidrBlock":    "10.0.0.0/16",
		"Icmp":         map[string]any{"Type": float64(8), "Code": float64(-1)},
	}, props)

	_, err = h.Create(t.Context(), createRequest(icmpEntry()))
	require.ErrorContains(t, err, "NetworkAclEntryAlreadyExists", "AWS rejects a second entry with the same rule number and direction")
}

func Test_NetworkAclEntry_CreateFails(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewNetworkAclEntry(api)

	_, err := h.Create(t.Context(), createRequest(with(icmpEntry(), map[string]any{"RuleNumber": nil})))
	require.Error(t, err)
	_, err = h.Create(t.Context(), createRequest(with(icmpEntry(), map[string]any{"RuleAction": 1})))
	require.Error(t, err)
	_, err = h.Create(t.Context(), createRequest(with(icmpEntry(), map[string]any{"NetworkAclId": "acl-404"})))
	require.ErrorContains(t, err, "InvalidNetworkAclID.NotFound")
	require.Equal(t, []string{"CreateNetworkAclEntry"}, api.calls)
}

func Test_NetworkAclEntry_Update(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewNetworkAclEntry(api)
	id, err := h.Create(t.Context(), createRequest(icmpEntry()))
	require.NoError(t, err)
	current, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)

	t.Run("only the identifier changes", func(t *testing.T) {
		api.calls = nil
		require.NoError(t, h.Update(t.Context(), updateRequest(t, id, current, with(current, map[string]any{"Id": "other"}))))
		require.Empty(t, api.calls, "nothing to replace")
	})
	t.Run("move to TCP", func(t *testing.T) {
		desired := with(current, map[string]any{"Protocol": float64(6), "Icmp": nil, "PortRange": map[string]any{"From": float64(22), "To": float64(22)}})
		require.NoError(t, h.Update(t.Context(), updateRequest(t, id, current, desired)))
		props, err := h.Read(t.Context(), testRegion, id)
		require.NoError(t, err)
		require.Equal(t, float64(6), props["Protocol"])
		require.NotContains(t, props, "Icmp")
		require.Equal(t, map[string]any{"From": float64(22), "To": float64(22)}, props["PortRange"])
	})
	t.Run("a property has the wrong type", func(t *testing.T) {
		api.calls = nil
		require.Error(t, h.Update(t.Context(), updateRequest(t, id, current, with(current, map[string]any{"PortRange": "22"}))))
		require.Empty(t, api.calls)
	})
	t.Run("the identifier is not an entry's", func(t *testing.T) {
		require.Error(t, h.Update(t.Context(), updateRequest(t, "acl-1", current, current)))
	})
	t.Run("the entry is gone", func(t *testing.T) {
		err := h.Update(t.Context(), updateRequest(t, "acl-1|300|false", current, with(current, map[string]any{"RuleAction": "deny"})))
		require.ErrorIs(t, err, native.ErrNotFound)
	})
}

func Test_NetworkAclEntry_ReadDeleteProgress(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewNetworkAclEntry(api)
	id, err := h.Create(t.Context(), createRequest(icmpEntry()))
	require.NoError(t, err)

	_, err = h.Read(t.Context(), testRegion, "acl-404|100|false")
	require.ErrorIs(t, err, native.ErrNotFound, "the network ACL is gone")
	_, err = h.Read(t.Context(), testRegion, "acl-1|100|false")
	require.ErrorIs(t, err, native.ErrNotFound, "the network ACL has no such entry")
	_, err = h.Read(t.Context(), testRegion, "acl-1")
	require.Error(t, err)

	progress, err := h.Progress(t.Context(), testRegion, id, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "present", Waiting: "deleted"}, progress)
	_, err = h.Progress(t.Context(), testRegion, "acl-1", native.OperationCreate)
	require.Error(t, err)

	require.Error(t, h.Delete(t.Context(), testRegion, "acl-1"))
	require.NoError(t, h.Delete(t.Context(), testRegion, id))
	require.ErrorIs(t, h.Delete(t.Context(), testRegion, id), native.ErrNotFound)
	_, err = h.Progress(t.Context(), testRegion, id, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_NetworkAclEntry_List(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewNetworkAclEntry(api)
	_, err := h.Create(t.Context(), createRequest(icmpEntry()))
	require.NoError(t, err)

	resources, err := h.List(t.Context(), testRegion)
	require.NoError(t, err)
	require.Len(t, resources, 1, "the rules AWS adds to each network ACL are left out")
	require.Equal(t, "acl-1|200|true", resources[0].Identifier)

	api.errs["DescribeNetworkAcls"] = fault("UnauthorizedOperation")
	_, err = h.List(t.Context(), testRegion)
	require.Error(t, err)
}
