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

package transfer

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/transfer"
	"github.com/aws/aws-sdk-go-v2/service/transfer/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
)

const (
	// server is the fake's server, whose identity provider is a directory.
	server = "s-0123456789abcdef0"

	// serverType keeps the fake's servers in the store, apart from the accesses.
	serverType = "AWS::Transfer::Server"

	// validationCode is the error the Common Errors page gives for input that doesn't meet a documented constraint,
	// such as a length or a pattern.
	validationCode = "ValidationError"

	// invalidRequest is the error each call lists for a malformed request, which the fake returns for a rule the
	// docs state in words.
	invalidRequest = "InvalidRequestException"

	maxMappings      = 50000
	maxSecondaryGids = 16
)

var (
	serverPattern   = regexp.MustCompile(`^s-[0-9a-f]{17}$`)
	externalPattern = regexp.MustCompile(`^S-1-[\d-]+$`)
	rolePattern     = regexp.MustCompile(`^arn:.*role/\S+$`)
	pathPattern     = regexp.MustCompile(`^/.*$`)
)

// fakeAPI is an in-memory AWS Transfer Family API for accesses. It keeps them in an awsfake store, in the type's
// property format, so the suite's Cloud Control sees the same resources. Servers are in the store too, under
// serverType. It follows the rules in accessAPI.
type fakeAPI struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// updates counts the UpdateAccess calls that reached the fake, and lastUpdate is the last one's input.
	updates    int
	lastUpdate *transfer.UpdateAccessInput

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	store.Put(serverType, server, map[string]any{"ServerId": server, "IdentityProviderType": "AWS_DIRECTORY_SERVICE"})
	return &fakeAPI{store: store, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// rules collects the first rule a call breaks, as the client fault AWS returns for it.
type rules struct {
	err error
}

func (r *rules) check(ok bool, code string) {
	if r.err == nil && !ok {
		r.err = fault(code)
	}
}

// length reports whether s is min to max characters.
func length(s string, min int, max int) bool {
	n := utf8.RuneCountInString(s)
	return n >= min && n <= max
}

// checkID checks a call's ServerId and ExternalId.
func checkID(r *rules, serverID *string, externalID *string) {
	r.check(serverPattern.MatchString(aws.ToString(serverID)), validationCode)
	r.check(length(aws.ToString(externalID), 1, 256) && externalPattern.MatchString(aws.ToString(externalID)), validationCode)
}

// settings are the values of an access a create or an update sets. A value the call leaves out is nil.
type settings struct {
	role, homeDirectory, policy *string
	homeDirectoryType           types.HomeDirectoryType
	mappings                    []types.HomeDirectoryMapEntry
	posix                       *types.PosixProfile
}

// checkSettings checks the documented limits of the values a call sets.
func checkSettings(r *rules, s settings) {
	if s.role != nil {
		r.check(length(*s.role, 20, 2048) && rolePattern.MatchString(*s.role), validationCode)
	}
	if s.homeDirectory != nil {
		r.check(length(*s.homeDirectory, 0, 1024) && (*s.homeDirectory == "" || pathPattern.MatchString(*s.homeDirectory)), validationCode)
	}
	if s.policy != nil {
		r.check(length(*s.policy, 0, 2048), validationCode)
	}
	r.check(s.homeDirectoryType == "" || slices.Contains(types.HomeDirectoryType("").Values(), s.homeDirectoryType), validationCode)
	if s.mappings != nil {
		r.check(len(s.mappings) >= 1 && len(s.mappings) <= maxMappings, validationCode)
	}
	for _, m := range s.mappings {
		for _, p := range []*string{m.Entry, m.Target} {
			r.check(p != nil && length(*p, 0, 1024) && pathPattern.MatchString(*p), validationCode)
		}
		r.check(!strings.HasSuffix(aws.ToString(m.Target), "/"), invalidRequest)
		r.check(m.Type == "" || slices.Contains(types.MapType("").Values(), m.Type), validationCode)
	}
	if p := s.posix; p != nil {
		r.check(p.Uid != nil && p.Gid != nil, validationCode)
		r.check(len(p.SecondaryGids) <= maxSecondaryGids, validationCode)
		for _, n := range append([]int64{aws.ToInt64(p.Uid), aws.ToInt64(p.Gid)}, p.SecondaryGids...) {
			r.check(n >= 0 && n <= 4294967295, validationCode)
		}
	}
}

// checkHome checks the home directory an access ends with: HomeDirectoryMappings only, and always, with LOGICAL.
func checkHome(r *rules, homeDirectoryType string, hasMappings bool, setsMappings bool) {
	r.check(homeDirectoryType == string(types.HomeDirectoryTypeLogical) || !setsMappings, invalidRequest)
	r.check(homeDirectoryType != string(types.HomeDirectoryTypeLogical) || hasMappings, invalidRequest)
}

func mappingsList(mappings []types.HomeDirectoryMapEntry) []any {
	out := []any{}
	for _, m := range mappings {
		typ := m.Type
		if typ == "" {
			typ = types.MapTypeDirectory
		}
		out = append(out, map[string]any{"Entry": aws.ToString(m.Entry), "Target": aws.ToString(m.Target), "Type": string(typ)})
	}
	return out
}

func posixMap(p *types.PosixProfile) map[string]any {
	gids := []any{}
	for _, gid := range p.SecondaryGids {
		gids = append(gids, float64(gid))
	}
	return map[string]any{"Uid": float64(aws.ToInt64(p.Uid)), "Gid": float64(aws.ToInt64(p.Gid)), "SecondaryGids": gids}
}

// apply sets the values s sets on props. An empty HomeDirectory or Policy clears it.
func apply(props map[string]any, s settings) {
	for name, value := range map[string]*string{"Role": s.role, "HomeDirectory": s.homeDirectory, "Policy": s.policy} {
		switch {
		case value == nil:
		case *value == "":
			delete(props, name)
		default:
			props[name] = *value
		}
	}
	if s.homeDirectoryType != "" {
		props["HomeDirectoryType"] = string(s.homeDirectoryType)
	}
	if s.mappings != nil {
		props["HomeDirectoryMappings"] = mappingsList(s.mappings)
	}
	if s.posix != nil {
		props["PosixProfile"] = posixMap(s.posix)
	}
}

func (f *fakeAPI) CreateAccess(ctx context.Context, in *transfer.CreateAccessInput, _ ...func(*transfer.Options)) (*transfer.CreateAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["CreateAccess"]; err != nil {
		return nil, err
	}
	s := settings{role: in.Role, homeDirectory: in.HomeDirectory, policy: in.Policy, homeDirectoryType: in.HomeDirectoryType, mappings: in.HomeDirectoryMappings, posix: in.PosixProfile}
	r := &rules{}
	checkID(r, in.ServerId, in.ExternalId)
	r.check(in.Role != nil, validationCode)
	checkSettings(r, s)
	if s.homeDirectoryType == "" {
		s.homeDirectoryType = types.HomeDirectoryTypePath
	}
	checkHome(r, string(s.homeDirectoryType), len(s.mappings) > 0, s.mappings != nil)
	r.check(f.store.Resource(serverType, aws.ToString(in.ServerId)) != nil, "ResourceNotFoundException")
	id := aws.ToString(in.ServerId) + "|" + aws.ToString(in.ExternalId)
	r.check(f.store.Resource(accessType, id) == nil, "ResourceExistsException")
	if r.err != nil {
		return nil, r.err
	}
	props := map[string]any{"ServerId": aws.ToString(in.ServerId), "ExternalId": aws.ToString(in.ExternalId)}
	apply(props, s)
	f.store.Put(accessType, id, props)
	return &transfer.CreateAccessOutput{ServerId: in.ServerId, ExternalId: in.ExternalId}, nil
}

// access returns the access a call names, or ResourceNotFoundException.
func (f *fakeAPI) access(serverID *string, externalID *string) (string, map[string]any, error) {
	r := &rules{}
	checkID(r, serverID, externalID)
	if r.err != nil {
		return "", nil, r.err
	}
	id := aws.ToString(serverID) + "|" + aws.ToString(externalID)
	props := f.store.Resource(accessType, id)
	if props == nil {
		return "", nil, fault("ResourceNotFoundException")
	}
	return id, props, nil
}

func (f *fakeAPI) DescribeAccess(ctx context.Context, in *transfer.DescribeAccessInput, _ ...func(*transfer.Options)) (*transfer.DescribeAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DescribeAccess"]; err != nil {
		return nil, err
	}
	_, props, err := f.access(in.ServerId, in.ExternalId)
	if err != nil {
		return nil, err
	}
	access := &types.DescribedAccess{ExternalId: in.ExternalId, HomeDirectoryType: types.HomeDirectoryTypePath}
	if v, ok := props["HomeDirectoryType"].(string); ok {
		access.HomeDirectoryType = types.HomeDirectoryType(v)
	}
	for name, field := range map[string]**string{"Role": &access.Role, "HomeDirectory": &access.HomeDirectory, "Policy": &access.Policy} {
		if v, ok := props[name].(string); ok {
			*field = aws.String(v)
		}
	}
	mappings, _ := props["HomeDirectoryMappings"].([]any)
	for _, item := range mappings {
		m, _ := item.(map[string]any)
		entry, _ := m["Entry"].(string)
		target, _ := m["Target"].(string)
		typ, _ := m["Type"].(string)
		access.HomeDirectoryMappings = append(access.HomeDirectoryMappings, types.HomeDirectoryMapEntry{Entry: aws.String(entry), Target: aws.String(target), Type: types.MapType(typ)})
	}
	if p, ok := props["PosixProfile"].(map[string]any); ok {
		uid, _ := p["Uid"].(float64)
		gid, _ := p["Gid"].(float64)
		profile := &types.PosixProfile{Uid: aws.Int64(int64(uid)), Gid: aws.Int64(int64(gid)), SecondaryGids: []int64{}}
		gids, _ := p["SecondaryGids"].([]any)
		for _, g := range gids {
			n, _ := g.(float64)
			profile.SecondaryGids = append(profile.SecondaryGids, int64(n))
		}
		access.PosixProfile = profile
	}
	return &transfer.DescribeAccessOutput{ServerId: in.ServerId, Access: access}, nil
}

func (f *fakeAPI) UpdateAccess(ctx context.Context, in *transfer.UpdateAccessInput, _ ...func(*transfer.Options)) (*transfer.UpdateAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates++
	f.lastUpdate = in
	if err := f.errs["UpdateAccess"]; err != nil {
		return nil, err
	}
	id, props, err := f.access(in.ServerId, in.ExternalId)
	if err != nil {
		return nil, err
	}
	s := settings{role: in.Role, homeDirectory: in.HomeDirectory, policy: in.Policy, homeDirectoryType: in.HomeDirectoryType, mappings: in.HomeDirectoryMappings, posix: in.PosixProfile}
	r := &rules{}
	checkSettings(r, s)
	homeDirectoryType, _ := props["HomeDirectoryType"].(string)
	if s.homeDirectoryType != "" {
		homeDirectoryType = string(s.homeDirectoryType)
	}
	_, hadMappings := props["HomeDirectoryMappings"]
	checkHome(r, homeDirectoryType, len(s.mappings) > 0 || (s.mappings == nil && hadMappings), s.mappings != nil)
	if r.err != nil {
		return nil, r.err
	}
	apply(props, s)
	f.store.Put(accessType, id, props)
	return &transfer.UpdateAccessOutput{ServerId: in.ServerId, ExternalId: in.ExternalId}, nil
}

func (f *fakeAPI) DeleteAccess(ctx context.Context, in *transfer.DeleteAccessInput, _ ...func(*transfer.Options)) (*transfer.DeleteAccessOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DeleteAccess"]; err != nil {
		return nil, err
	}
	id, _, err := f.access(in.ServerId, in.ExternalId)
	if err != nil {
		return nil, err
	}
	f.store.Remove(accessType, id)
	return &transfer.DeleteAccessOutput{}, nil
}
