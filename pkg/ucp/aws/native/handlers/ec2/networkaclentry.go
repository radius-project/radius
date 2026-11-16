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
	"context"
	"fmt"
	"maps"
	"math"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	networkAclEntryType = "AWS::EC2::NetworkAclEntry"

	// maxRuleNumber is the highest rule number a template can use. AWS adds rules above it to every network ACL, such
	// as the deny-all rule 32767, which can't be changed or deleted.
	maxRuleNumber = 32766
)

var (
	_ native.Handler           = (*NetworkAclEntry)(nil)
	_ native.IdentifierEncoder = (*NetworkAclEntry)(nil)
)

// NetworkAclEntry serves AWS::EC2::NetworkAclEntry, a rule in a network ACL. Its identifier, Id, is NetworkAclId,
// RuleNumber, and Egress joined with "|", such as acl-0123456789abcdef0|100|false, which the template gives, so UCP
// finds a lost create by it. AWS rejects a second rule with the same number and direction. Every call takes effect at
// once.
type NetworkAclEntry struct {
	api API
}

// NewNetworkAclEntry returns the handler for network ACL entries.
func NewNetworkAclEntry(api API) *NetworkAclEntry {
	return &NetworkAclEntry{api: api}
}

// TypeName returns AWS::EC2::NetworkAclEntry.
func (h *NetworkAclEntry) TypeName() string {
	return networkAclEntryType
}

// Options returns how UCP uses the handler.
func (h *NetworkAclEntry) Options() native.Options {
	return native.Options{
		List:       native.ListRegion,
		Recovery:   native.RecoverByIdentifier,
		Equivalent: entryEquivalent,
	}
}

// entryKey is what identifies an entry in AWS.
type entryKey struct {
	acl    string
	rule   int32
	egress bool
}

func (k entryKey) String() string {
	return fmt.Sprintf("%s|%d|%t", k.acl, k.rule, k.egress)
}

// Identifier returns NetworkAclId, RuleNumber, and Egress joined with "|". Egress defaults to false, as in AWS. A
// RuleNumber outside 1 to 32766 is rejected, before it is narrowed to the API's int32.
func (h *NetworkAclEntry) Identifier(properties map[string]any) (string, error) {
	acl := awsapi.String(properties, "NetworkAclId")
	rule, ok := integer(properties["RuleNumber"])
	if acl == "" || !ok {
		return "", nil
	}
	if rule < 1 || rule > maxRuleNumber {
		return "", &native.Failure{
			Message:     fmt.Sprintf("RuleNumber %v of the network ACL entry is not one of 1 to %d. AWS keeps the rule numbers above %d for the network ACL's own rules.", properties["RuleNumber"], maxRuleNumber, maxRuleNumber),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	egress, _ := properties["Egress"].(bool)
	return entryKey{acl: acl, rule: int32(rule), egress: egress}.String(), nil
}

func parseEntryKey(identifier string) (entryKey, error) {
	parts := strings.Split(identifier, "|")
	if len(parts) == 3 && parts[0] != "" {
		rule, ruleErr := strconv.ParseInt(parts[1], 10, 32)
		egress, egressErr := strconv.ParseBool(parts[2])
		if ruleErr == nil && egressErr == nil && rule >= 1 && rule <= maxRuleNumber {
			return entryKey{acl: parts[0], rule: int32(rule), egress: egress}, nil
		}
	}
	return entryKey{}, invalidIdentifier(networkAclEntryType, identifier, "NetworkAclId|RuleNumber|Egress, with RuleNumber one of 1 to 32766,")
}

// integer returns v, a JSON number, as an integer. A whole number too large for an int64 is returned as the nearest
// int64, which no rule number check accepts.
func integer(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if n != math.Trunc(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return int64(max(min(n, math.MaxInt64/2), math.MinInt64/2)), true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	}
	return 0, false
}

// protocol returns the Protocol property as AWS takes it, a protocol number as a string, such as "6" or "-1".
func protocol(v any) string {
	if n, ok := integer(v); ok {
		return strconv.FormatInt(n, 10)
	}
	s, _ := v.(string)
	return s
}

// entryEquivalent treats CIDR blocks that name the same network, such as 2001:DB8::/32 and 2001:db8::/32, a protocol
// number as a string or a number, and rule actions in any case as the template's values.
func entryEquivalent(path string, template any, aws any) bool {
	switch path {
	case "CidrBlock", "Ipv6CidrBlock":
		t, okT := template.(string)
		a, okA := aws.(string)
		if !okT || !okA {
			return false
		}
		tp, errT := netip.ParsePrefix(t)
		ap, errA := netip.ParsePrefix(a)
		return errT == nil && errA == nil && tp.Masked() == ap.Masked()
	case "Protocol":
		return protocol(template) != "" && protocol(template) == protocol(aws)
	case "RuleAction":
		t, okT := template.(string)
		a, okA := aws.(string)
		return okT && okA && strings.EqualFold(t, a)
	}
	return false
}

// describe returns the entry with the key, or ErrNotFound.
func (h *NetworkAclEntry) describe(ctx context.Context, region string, key entryKey) (*types.NetworkAclEntry, error) {
	out, err := h.api.DescribeNetworkAcls(ctx, &awsec2.DescribeNetworkAclsInput{NetworkAclIds: []string{key.acl}}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "InvalidNetworkAclID.NotFound")
	}
	for _, acl := range out.NetworkAcls {
		for _, e := range acl.Entries {
			if aws.ToInt32(e.RuleNumber) == key.rule && aws.ToBool(e.Egress) == key.egress {
				return &e, nil
			}
		}
	}
	return nil, native.ErrNotFound
}

// Read returns the entry's properties.
func (h *NetworkAclEntry) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	key, err := parseEntryKey(identifier)
	if err != nil {
		return nil, err
	}
	entry, err := h.describe(ctx, region, key)
	if err != nil {
		return nil, err
	}
	return entryProperties(key.acl, entry)
}

// entryProperties returns the properties of the entry of the network ACL acl. AWS returns Protocol as a string, and
// the schema has it as an integer.
func entryProperties(acl string, e *types.NetworkAclEntry) (map[string]any, error) {
	key := entryKey{acl: acl, rule: aws.ToInt32(e.RuleNumber), egress: aws.ToBool(e.Egress)}
	var proto any = aws.ToString(e.Protocol)
	if n, err := strconv.Atoi(aws.ToString(e.Protocol)); err == nil {
		proto = n
	}
	p := map[string]any{
		"Id":            key.String(),
		"NetworkAclId":  acl,
		"RuleNumber":    key.rule,
		"Egress":        key.egress,
		"RuleAction":    string(e.RuleAction),
		"Protocol":      proto,
		"CidrBlock":     e.CidrBlock,
		"Ipv6CidrBlock": e.Ipv6CidrBlock,
		"PortRange":     e.PortRange,
	}
	if e.IcmpTypeCode != nil {
		p["Icmp"] = map[string]any{"Type": e.IcmpTypeCode.Type, "Code": e.IcmpTypeCode.Code}
	}
	return awsapi.EncodeObject(p)
}

// entryFields are the properties CreateNetworkAclEntry and ReplaceNetworkAclEntry take, other than the ones in the
// identifier.
var entryFields = []string{"RuleAction", "Protocol", "CidrBlock", "Ipv6CidrBlock", "Icmp", "PortRange"}

// entryOf returns the entry the properties describe.
func entryOf(properties map[string]any) (types.NetworkAclEntry, error) {
	values := map[string]any{}
	for _, name := range entryFields {
		if v, ok := properties[name]; ok {
			values[name] = v
		}
	}
	if v, ok := values["Icmp"]; ok {
		values["IcmpTypeCode"] = v
		delete(values, "Icmp")
	}
	if v, ok := values["Protocol"]; ok {
		values["Protocol"] = protocol(v)
	}
	entry := types.NetworkAclEntry{}
	return entry, awsapi.Decode(values, &entry)
}

// Create adds the entry to the network ACL.
func (h *NetworkAclEntry) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, _ := h.Identifier(request.Properties)
	key, err := parseEntryKey(identifier)
	if err != nil {
		return "", err
	}
	entry, err := entryOf(request.Properties)
	if err != nil {
		return "", err
	}
	_, err = h.api.CreateNetworkAclEntry(ctx, &awsec2.CreateNetworkAclEntryInput{
		NetworkAclId:  aws.String(key.acl),
		RuleNumber:    aws.Int32(key.rule),
		Egress:        aws.Bool(key.egress),
		RuleAction:    entry.RuleAction,
		Protocol:      entry.Protocol,
		CidrBlock:     entry.CidrBlock,
		Ipv6CidrBlock: entry.Ipv6CidrBlock,
		IcmpTypeCode:  entry.IcmpTypeCode,
		PortRange:     entry.PortRange,
	}, inRegion(request.Region))
	if err != nil {
		return "", err
	}
	return identifier, nil
}

// Update replaces the entry with ReplaceNetworkAclEntry, which takes the whole entry: the current values, with the
// properties the patch changes set to the template's values, or left out when the template removed them.
func (h *NetworkAclEntry) Update(ctx context.Context, request native.UpdateRequest) error {
	key, err := parseEntryKey(request.Identifier)
	if err != nil {
		return err
	}
	values := maps.Clone(request.Current)
	changed := false
	for _, name := range awsapi.Changed(request) {
		if !slices.Contains(entryFields, name) {
			continue
		}
		changed = true
		if v, ok := request.Desired[name]; ok {
			values[name] = v
		} else {
			delete(values, name)
		}
	}
	if !changed {
		return nil
	}
	entry, err := entryOf(values)
	if err != nil {
		return err
	}
	_, err = h.api.ReplaceNetworkAclEntry(ctx, &awsec2.ReplaceNetworkAclEntryInput{
		NetworkAclId:  aws.String(key.acl),
		RuleNumber:    aws.Int32(key.rule),
		Egress:        aws.Bool(key.egress),
		RuleAction:    entry.RuleAction,
		Protocol:      entry.Protocol,
		CidrBlock:     entry.CidrBlock,
		Ipv6CidrBlock: entry.Ipv6CidrBlock,
		IcmpTypeCode:  entry.IcmpTypeCode,
		PortRange:     entry.PortRange,
	}, inRegion(request.Region))
	return awsapi.NotFound(err, "InvalidNetworkAclEntry.NotFound", "InvalidNetworkAclID.NotFound")
}

// Delete removes the entry from the network ACL.
func (h *NetworkAclEntry) Delete(ctx context.Context, region string, identifier string) error {
	key, err := parseEntryKey(identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DeleteNetworkAclEntry(ctx, &awsec2.DeleteNetworkAclEntryInput{
		NetworkAclId: aws.String(key.acl),
		RuleNumber:   aws.Int32(key.rule),
		Egress:       aws.Bool(key.egress),
	}, inRegion(region))
	return awsapi.NotFound(err, "InvalidNetworkAclEntry.NotFound", "InvalidNetworkAclID.NotFound")
}

// List returns every entry of every network ACL in the region, except the rules AWS adds.
func (h *NetworkAclEntry) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	pages := awsec2.NewDescribeNetworkAclsPaginator(h.api, &awsec2.DescribeNetworkAclsInput{})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, acl := range page.NetworkAcls {
			for _, e := range acl.Entries {
				if aws.ToInt32(e.RuleNumber) > maxRuleNumber {
					continue
				}
				properties, err := entryProperties(aws.ToString(acl.NetworkAclId), &e)
				if err != nil {
					return nil, err
				}
				out = append(out, native.Resource{Identifier: properties["Id"].(string), Properties: properties})
			}
		}
	}
	return out, nil
}

// Progress reports a create or update as finished, because they take effect at once, and a delete as finished once
// the entry is gone.
func (h *NetworkAclEntry) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	key, err := parseEntryKey(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	if _, err := h.describe(ctx, region, key); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationDelete {
		return native.Progress{State: "present", Waiting: "deleted"}, nil
	}
	return native.Progress{Done: true}, nil
}
