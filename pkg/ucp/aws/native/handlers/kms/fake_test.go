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

package kms

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// region is where the fake's keys are, and the region of a call that sets none.
	region = "us-west-2"

	// account is the account of the fake's caller and keys.
	account = "123456789012"

	// validationCode is the error the docs give for input that doesn't meet a documented constraint, such as a
	// length, a pattern, or an operation the key doesn't support.
	validationCode = "ValidationError"

	// maxGrants is how many grants a KMS key can have.
	maxGrants = 50000

	// pageSize is the most grants a ListGrants page has. AWS says a page may have fewer than Limit.
	pageSize = 2

	// The fake's keys, by key ID.
	keyID         = "1234abcd-12ab-34cd-56ef-1234567890ab"
	otherKeyID    = "0987dcba-09ba-87dc-65fe-0987654321ba"
	signingKeyID  = "5678efab-56ef-78ab-90cd-5678901234ef"
	disabledKeyID = "2468aced-24ac-68ed-02bd-2468024680ac"
	deletingKeyID = "1357bdfa-13bd-57fa-91ce-1357913579bd"
)

// keyARN returns the ARN of the fake's key with the ID.
func keyARN(id string) string {
	return "arn:aws:kms:" + region + ":" + account + ":key/" + id
}

// key is a KMS key the fake has.
type key struct {
	state types.KeyState
	usage types.KeyUsageType
}

// keys are the fake's keys, by key ID. alias/app names keyID.
var keys = map[string]key{
	keyID:         {state: types.KeyStateEnabled, usage: types.KeyUsageTypeEncryptDecrypt},
	otherKeyID:    {state: types.KeyStateEnabled, usage: types.KeyUsageTypeEncryptDecrypt},
	signingKeyID:  {state: types.KeyStateEnabled, usage: types.KeyUsageTypeSignVerify},
	disabledKeyID: {state: types.KeyStateDisabled, usage: types.KeyUsageTypeEncryptDecrypt},
	deletingKeyID: {state: types.KeyStatePendingDeletion, usage: types.KeyUsageTypeEncryptDecrypt},
}

// operationsOf are the grant operations a key of each usage supports. Every key supports the grant's own operations.
var operationsOf = map[types.KeyUsageType][]types.GrantOperation{
	types.KeyUsageTypeEncryptDecrypt: {
		types.GrantOperationDecrypt, types.GrantOperationEncrypt, types.GrantOperationGenerateDataKey,
		types.GrantOperationGenerateDataKeyWithoutPlaintext, types.GrantOperationGenerateDataKeyPair,
		types.GrantOperationGenerateDataKeyPairWithoutPlaintext, types.GrantOperationReEncryptFrom, types.GrantOperationReEncryptTo,
	},
	types.KeyUsageTypeSignVerify: {types.GrantOperationSign, types.GrantOperationVerify, types.GrantOperationGetPublicKey},
}

// grantOperations are the operations every key supports in a grant.
var grantOperations = []types.GrantOperation{types.GrantOperationCreateGrant, types.GrantOperationRetireGrant, types.GrantOperationDescribeKey}

var (
	principalPattern = regexp.MustCompile(`^[\w+=,.@:/-]+$`)
	servicePattern   = regexp.MustCompile(`^([A-Za-z0-9\-]+)\.([A-Za-z0-9\-]+)(\.[A-Za-z0-9\-]+)+$`)
	namePattern      = regexp.MustCompile(`^[a-zA-Z0-9:/_-]+$`)
	sourceArnPattern = regexp.MustCompile(`^arn:aws[a-z0-9-]*:[a-z0-9-]+:[a-z0-9-]*:[0-9]{12}:.+$`)
	keyARNPattern    = regexp.MustCompile(`^arn:aws[a-z0-9-]*:kms:([a-z0-9-]+):([0-9]{12}):(key|alias)/(.+)$`)
)

// fakeAPI is an in-memory AWS KMS API for grants. It keeps them in an awsfake store, in the type's property format and
// by identifier, so the suite's Cloud Control sees the same resources. A retired grant stays in ListGrants for one more
// read, as AWS's eventual consistency allows, and with lagNew a new grant is missing from the first read that would
// list it. It follows the rules in grantAPI.
type fakeAPI struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// lagNew hides a new grant from the first ListGrants that would list it.
	lagNew bool
	hidden map[string]bool

	// retired holds grants RetireGrant removed, which ListGrants still lists once.
	retired map[string]map[string]any

	// next numbers the grants and grant tokens the fake makes, and tokens maps each grant token to its grant.
	next   int
	tokens map[string]string

	// max is how many grants a key can have.
	max int

	// calls are the calls the fake got, and errs maps a call to an error it returns instead of acting.
	calls []string
	errs  map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{
		store: store, hidden: map[string]bool{}, retired: map[string]map[string]any{}, tokens: map[string]string{},
		max: maxGrants, errs: map[string]error{},
	}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// call records a call in the region its options set, or the fake's, and returns the error errs sets for it.
func (f *fakeAPI) call(name string, opts []func(*kms.Options)) (string, error) {
	f.calls = append(f.calls, name)
	options := kms.Options{Region: region}
	for _, fn := range opts {
		fn(&options)
	}
	return options.Region, f.errs[name]
}

// length reports whether s has from min to max characters.
func length(s string, low int, high int) bool {
	return len(s) >= low && len(s) <= high
}

// resolve returns the ID of the key that name names in the call's region: a key ID or key ARN, and with aliases, also
// an alias name or alias ARN. It returns InvalidArnException for an ARN that isn't one, and NotFoundException for a
// key the fake doesn't have in the region, or an alias where aliases aren't taken.
func resolve(callRegion string, name *string, aliases bool) (string, error) {
	id := aws.ToString(name)
	if !length(id, 1, 2048) {
		return "", fault(validationCode)
	}
	if callRegion != region {
		return "", fault("NotFoundException")
	}
	if strings.HasPrefix(id, "arn:") {
		m := keyARNPattern.FindStringSubmatch(id)
		if m == nil {
			return "", fault("InvalidArnException")
		}
		if m[1] != region || m[2] != account {
			return "", fault("NotFoundException")
		}
		if m[3] == "alias" {
			id = "alias/" + m[4]
		} else {
			id = m[4]
		}
	}
	if strings.HasPrefix(id, "alias/") {
		if !aliases || id != "alias/app" {
			return "", fault("NotFoundException")
		}
		id = keyID
	}
	if _, ok := keys[id]; !ok {
		return "", fault("NotFoundException")
	}
	return id, nil
}

func (f *fakeAPI) DescribeKey(ctx context.Context, in *kms.DescribeKeyInput, opts ...func(*kms.Options)) (*kms.DescribeKeyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	region, err := f.call("DescribeKey", opts)
	if err != nil {
		return nil, err
	}
	id, err := resolve(region, in.KeyId, true)
	if err != nil {
		return nil, err
	}
	k := keys[id]
	return &kms.DescribeKeyOutput{KeyMetadata: &types.KeyMetadata{
		KeyId:    aws.String(id),
		Arn:      aws.String(keyARN(id)),
		KeyState: k.state,
		KeyUsage: k.usage,
	}}, nil
}

// validPrincipals returns ValidationError unless the grant has one grantee, at most one retiring principal, and each
// principal fits its documented length and pattern. A grantee service principal needs a SourceArn constraint and a
// retiring principal.
func validPrincipals(in *kms.CreateGrantInput) error {
	grantee, service := aws.ToString(in.GranteePrincipal), aws.ToString(in.GranteeServicePrincipal)
	retiring, retiringService := aws.ToString(in.RetiringPrincipal), aws.ToString(in.RetiringServicePrincipal)
	switch {
	case (in.GranteePrincipal == nil) == (in.GranteeServicePrincipal == nil),
		in.RetiringPrincipal != nil && in.RetiringServicePrincipal != nil,
		in.GranteePrincipal != nil && !(length(grantee, 1, 256) && principalPattern.MatchString(grantee)),
		in.RetiringPrincipal != nil && !(length(retiring, 1, 256) && principalPattern.MatchString(retiring)),
		in.GranteeServicePrincipal != nil && !(length(service, 1, 128) && servicePattern.MatchString(service)),
		in.RetiringServicePrincipal != nil && !(length(retiringService, 1, 128) && servicePattern.MatchString(retiringService)),
		in.GranteeServicePrincipal != nil && (in.Constraints == nil || in.Constraints.SourceArn == nil || (in.RetiringPrincipal == nil && in.RetiringServicePrincipal == nil)):
		return fault(validationCode)
	}
	return nil
}

// validContext reports whether an encryption context constraint has at most 8 pairs, with values of at most 384
// characters.
func validContext(pairs map[string]string) bool {
	if len(pairs) > 8 {
		return false
	}
	for _, v := range pairs {
		if len(v) > 384 {
			return false
		}
	}
	return true
}

// validConstraints returns ValidationError unless the constraints fit their documented limits. A grant has
// EncryptionContextEquals or EncryptionContextSubset, not both, and only for a symmetric encryption key, whose
// operations take an encryption context.
func validConstraints(c *types.GrantConstraints, usage types.KeyUsageType) error {
	if c == nil {
		return nil
	}
	hasContext := len(c.EncryptionContextEquals) > 0 || len(c.EncryptionContextSubset) > 0
	arn := aws.ToString(c.SourceArn)
	if !validContext(c.EncryptionContextEquals) || !validContext(c.EncryptionContextSubset) ||
		(len(c.EncryptionContextEquals) > 0 && len(c.EncryptionContextSubset) > 0) ||
		(hasContext && usage != types.KeyUsageTypeEncryptDecrypt) ||
		(c.SourceArn != nil && !(length(arn, 20, 512) && sourceArnPattern.MatchString(arn))) {
		return fault(validationCode)
	}
	return nil
}

// validOperations returns ValidationError unless there is at least one operation, and the key supports each.
func validOperations(ops []types.GrantOperation, usage types.KeyUsageType) error {
	supported := slices.Concat(operationsOf[usage], grantOperations)
	if len(ops) == 0 || slices.ContainsFunc(ops, func(op types.GrantOperation) bool { return !slices.Contains(supported, op) }) {
		return fault(validationCode)
	}
	return nil
}

// props returns the grant's properties in the type's format.
func props(key string, grantID string, in *kms.CreateGrantInput) map[string]any {
	p := map[string]any{"KeyId": keyARN(key), "GrantId": grantID, "IssuingAccount": "arn:aws:iam::" + account + ":root"}
	for name, v := range map[string]*string{
		"Name": in.Name, "GranteePrincipal": in.GranteePrincipal, "GranteeServicePrincipal": in.GranteeServicePrincipal,
		"RetiringPrincipal": in.RetiringPrincipal, "RetiringServicePrincipal": in.RetiringServicePrincipal,
	} {
		if v != nil {
			p[name] = *v
		}
	}
	ops := []any{}
	for _, op := range in.Operations {
		ops = append(ops, string(op))
	}
	p["Operations"] = ops
	if c, _ := awsapi.EncodeObject(in.Constraints); len(c) > 0 {
		p["Constraints"] = c
	}
	return p
}

// entry returns the grant with the properties as ListGrants returns it.
func entry(p map[string]any) types.GrantListEntry {
	e := types.GrantListEntry{
		KeyId:          aws.String(awsapi.String(p, "KeyId")),
		GrantId:        aws.String(awsapi.String(p, "GrantId")),
		Name:           aws.String(awsapi.String(p, "Name")),
		IssuingAccount: aws.String(awsapi.String(p, "IssuingAccount")),
	}
	for name, field := range map[string]**string{
		"GranteePrincipal": &e.GranteePrincipal, "GranteeServicePrincipal": &e.GranteeServicePrincipal,
		"RetiringPrincipal": &e.RetiringPrincipal, "RetiringServicePrincipal": &e.RetiringServicePrincipal,
	} {
		if v := awsapi.String(p, name); v != "" {
			*field = aws.String(v)
		}
	}
	for _, op := range awsapi.Strings(p, "Operations") {
		e.Operations = append(e.Operations, types.GrantOperation(op))
	}
	if c, ok := p["Constraints"].(map[string]any); ok {
		b, _ := json.Marshal(c)
		e.Constraints = &types.GrantConstraints{}
		_ = json.Unmarshal(b, e.Constraints)
	}
	return e
}

// sameRequest reports whether the existing grant has the values of the create, with operations in any order.
func sameRequest(existing types.GrantListEntry, in *kms.CreateGrantInput) bool {
	ops := func(list []types.GrantOperation) []types.GrantOperation {
		out := slices.Clone(list)
		slices.Sort(out)
		return out
	}
	var a, b types.GrantConstraints
	if existing.Constraints != nil {
		a = *existing.Constraints
	}
	if in.Constraints != nil {
		b = *in.Constraints
	}
	return aws.ToString(existing.Name) == aws.ToString(in.Name) &&
		aws.ToString(existing.GranteePrincipal) == aws.ToString(in.GranteePrincipal) &&
		aws.ToString(existing.GranteeServicePrincipal) == aws.ToString(in.GranteeServicePrincipal) &&
		aws.ToString(existing.RetiringPrincipal) == aws.ToString(in.RetiringPrincipal) &&
		aws.ToString(existing.RetiringServicePrincipal) == aws.ToString(in.RetiringServicePrincipal) &&
		slices.Equal(ops(existing.Operations), ops(in.Operations)) &&
		maps.Equal(a.EncryptionContextEquals, b.EncryptionContextEquals) &&
		maps.Equal(a.EncryptionContextSubset, b.EncryptionContextSubset) &&
		aws.ToString(a.SourceArn) == aws.ToString(b.SourceArn)
}

// grantsOf returns the identifiers of the key's grants, in the order the store lists them.
func (f *fakeAPI) grantsOf(key string) []string {
	out := []string{}
	for _, id := range f.store.List(grantType) {
		if awsapi.String(f.store.Resource(grantType, id), "KeyId") == keyARN(key) {
			out = append(out, id)
		}
	}
	return out
}

func (f *fakeAPI) CreateGrant(ctx context.Context, in *kms.CreateGrantInput, opts ...func(*kms.Options)) (*kms.CreateGrantOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	region, err := f.call("CreateGrant", opts)
	if err != nil {
		return nil, err
	}
	if in.Name != nil && !(length(*in.Name, 1, 256) && namePattern.MatchString(*in.Name)) {
		return nil, fault(validationCode)
	}
	if err := validPrincipals(in); err != nil {
		return nil, err
	}
	id, err := resolve(region, in.KeyId, false)
	if err != nil {
		return nil, err
	}
	k := keys[id]
	switch k.state {
	case types.KeyStateDisabled:
		return nil, fault("DisabledException")
	case types.KeyStatePendingDeletion:
		return nil, fault("KMSInvalidStateException")
	}
	if err := validOperations(in.Operations, k.usage); err != nil {
		return nil, err
	}
	if err := validConstraints(in.Constraints, k.usage); err != nil {
		return nil, err
	}
	existing := f.grantsOf(id)
	for _, grant := range existing {
		if e := entry(f.store.Resource(grantType, grant)); in.Name != nil && sameRequest(e, in) {
			return &kms.CreateGrantOutput{GrantId: e.GrantId, GrantToken: aws.String(f.token(grant))}, nil
		}
	}
	if len(existing) >= f.max {
		return nil, fault("LimitExceededException")
	}
	f.next++
	grantID := fmt.Sprintf("%064x", f.next)
	identifier := keyARN(id) + "|" + grantID
	f.store.Put(grantType, identifier, props(id, grantID, in))
	if f.lagNew {
		f.hidden[identifier] = true
	}
	return &kms.CreateGrantOutput{GrantId: aws.String(grantID), GrantToken: aws.String(f.token(identifier))}, nil
}

// token returns a new grant token for the grant with the identifier. Every CreateGrant call gets a new one.
func (f *fakeAPI) token(identifier string) string {
	f.next++
	token := "token-" + strconv.Itoa(f.next)
	f.tokens[token] = identifier
	return token
}

func (f *fakeAPI) ListGrants(ctx context.Context, in *kms.ListGrantsInput, opts ...func(*kms.Options)) (*kms.ListGrantsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	region, err := f.call("ListGrants", opts)
	if err != nil {
		return nil, err
	}
	if in.Limit != nil && (*in.Limit < 1 || *in.Limit > 100) {
		return nil, fault(validationCode)
	}
	if in.GranteePrincipal != nil && in.GranteeServicePrincipal != nil {
		return nil, fault(validationCode)
	}
	if in.GrantId != nil && !length(*in.GrantId, 1, 128) {
		return nil, fault(validationCode)
	}
	id, err := resolve(region, in.KeyId, false)
	if err != nil {
		return nil, err
	}
	start := 0
	if in.Marker != nil {
		if start, err = strconv.Atoi(*in.Marker); err != nil || start < 1 {
			return nil, fault("InvalidMarkerException")
		}
	}

	ids := f.grantsOf(id)
	for ghost, p := range f.retired {
		if awsapi.String(p, "KeyId") == keyARN(id) {
			ids = append(ids, ghost)
		}
	}
	slices.Sort(ids)
	listed := []types.GrantListEntry{}
	for _, identifier := range ids {
		p := f.store.Resource(grantType, identifier)
		if p == nil {
			p = f.retired[identifier]
		}
		e := entry(p)
		if (in.GrantId != nil && aws.ToString(e.GrantId) != *in.GrantId) ||
			(in.GranteePrincipal != nil && aws.ToString(e.GranteePrincipal) != *in.GranteePrincipal) {
			continue
		}
		if f.hidden[identifier] {
			delete(f.hidden, identifier)
			continue
		}
		delete(f.retired, identifier)
		listed = append(listed, e)
	}
	limit := 50
	if in.Limit != nil {
		limit = int(*in.Limit)
	}
	end := min(start+min(limit, pageSize), len(listed))
	out := &kms.ListGrantsOutput{Grants: listed[min(start, end):end]}
	if end < len(listed) {
		out.Truncated, out.NextMarker = true, aws.String(strconv.Itoa(end))
	}
	return out, nil
}

func (f *fakeAPI) RetireGrant(ctx context.Context, in *kms.RetireGrantInput, opts ...func(*kms.Options)) (*kms.RetireGrantOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	region, err := f.call("RetireGrant", opts)
	if err != nil {
		return nil, err
	}
	var identifier string
	switch {
	case in.GrantToken != nil:
		if identifier = f.tokens[*in.GrantToken]; identifier == "" {
			return nil, fault("InvalidGrantTokenException")
		}
	case in.KeyId != nil && in.GrantId != nil:
		if !length(*in.GrantId, 1, 128) {
			return nil, fault(validationCode)
		}
		id, err := resolve(region, in.KeyId, false)
		if err != nil {
			return nil, err
		}
		identifier = keyARN(id) + "|" + *in.GrantId
	default:
		return nil, fault(validationCode)
	}
	p := f.store.Resource(grantType, identifier)
	if p == nil {
		return nil, fault("NotFoundException")
	}
	f.store.Remove(grantType, identifier)
	delete(f.hidden, identifier)
	f.retired[identifier] = p
	return &kms.RetireGrantOutput{}, nil
}
