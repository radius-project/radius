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

package quicksight

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/quicksight"
	"github.com/aws/aws-sdk-go-v2/service/quicksight/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	folderMembershipType = "AWS::QuickSight::FolderMembership"

	// maxIDLength is the documented maximum length of a folder ID and a member ID.
	maxIDLength = 2048

	// maxListResults is the most members a page of ListFolderMembers can have.
	maxListResults = 100
)

var (
	_ native.Handler           = (*FolderMembership)(nil)
	_ native.IdentifierEncoder = (*FolderMembership)(nil)
)

// FolderMembership serves AWS::QuickSight::FolderMembership: an asset, such as a dashboard or a dataset, in a folder.
// Its identifier is AwsAccountId, FolderId, MemberType, and MemberId, joined with "|". AWS has no call that changes a
// membership, so every property is create-only, and UCP rejects a change with 400 before any AWS call. The identifier
// is known before the create, so a lost create is found by it.
type FolderMembership struct {
	api API
}

// NewFolderMembership returns the handler for Quick Sight folder memberships.
func NewFolderMembership(api API) *FolderMembership {
	return &FolderMembership{api: api}
}

// TypeName returns AWS::QuickSight::FolderMembership.
func (h *FolderMembership) TypeName() string {
	return folderMembershipType
}

// Options returns how UCP uses the handler. AWS lists members only by folder, so the type can't be listed.
func (h *FolderMembership) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier, CreateTimeout: timeout, UpdateTimeout: timeout, DeleteTimeout: timeout}
}

// membershipID names a membership: an asset of a type in a folder of an account.
type membershipID struct {
	account, folder string
	kind            types.MemberType
	member          string
}

func (id membershipID) String() string {
	return strings.Join([]string{id.account, id.folder, string(id.kind), id.member}, "|")
}

// memberKind returns the member type s names, or false when AWS has no such type.
func memberKind(s string) (types.MemberType, bool) {
	kind := types.MemberType(s)
	return kind, slices.Contains(kind.Values(), kind)
}

// parseMembership returns the membership the properties name. ok is false when they don't name all of it.
func parseMembership(properties map[string]any) (id membershipID, ok bool, err error) {
	if id.account, err = accountID(folderMembershipType, properties); err != nil {
		return membershipID{}, false, err
	}
	if id.folder, err = resourceID(folderMembershipType, properties, "FolderId", maxIDLength); err != nil {
		return membershipID{}, false, err
	}
	if id.member, err = resourceID(folderMembershipType, properties, "MemberId", maxIDLength); err != nil {
		return membershipID{}, false, err
	}
	kind, err := stringProperty(folderMembershipType, properties, "MemberType", func(s string) bool {
		_, known := memberKind(s)
		return known
	}, fmt.Sprintf("one of %v", types.MemberType("").Values()))
	if err != nil {
		return membershipID{}, false, err
	}
	id.kind = types.MemberType(kind)
	return id, id.account != "" && id.folder != "" && id.kind != "" && id.member != "", nil
}

// parseMembershipID returns the membership an identifier names.
func parseMembershipID(identifier string) (membershipID, error) {
	parts := strings.Split(identifier, "|")
	if len(parts) == 4 {
		kind, known := memberKind(parts[2])
		id := membershipID{account: parts[0], folder: parts[1], kind: kind, member: parts[3]}
		if known && accountPattern.MatchString(id.account) && validID(id.folder, maxIDLength) && validID(id.member, maxIDLength) {
			return id, nil
		}
	}
	return membershipID{}, invalid(folderMembershipType, "%q is not the identifier of a Quick Sight folder membership: it must be AwsAccountId|FolderId|MemberType|MemberId", identifier)
}

// Identifier returns AwsAccountId|FolderId|MemberType|MemberId, or "" when the properties don't give all of them. UCP
// calls it before any AWS call, so a template with a value AWS would reject is rejected here.
func (h *FolderMembership) Identifier(properties map[string]any) (string, error) {
	id, ok, err := parseMembership(properties)
	if err != nil || !ok {
		return "", err
	}
	return id.String(), nil
}

// isMember reports whether AWS's folder member m is the asset id names. AWS lists a member's ID and ARN but not its
// type, and assets of different types can have the same ID, so the type comes from the ARN's resource, such as
// dashboard/<id>. AWS marks both fields optional: a member without an ID is matched by its ARN. It fails when m may be
// the asset but has no ARN it can read, or its ID and ARN disagree, so a member is never taken for another.
func (id membershipID) isMember(m types.MemberIdArnPair) (bool, error) {
	memberID := aws.ToString(m.MemberId)
	parsed, err := arn.Parse(aws.ToString(m.MemberArn))
	kind, arnID, found := strings.Cut(parsed.Resource, "/")
	readable := err == nil && parsed.Service == "quicksight" && found && arnID != ""
	if memberID != id.member && memberID != "" && (!readable || arnID != id.member) {
		return false, nil
	}
	if readable && memberID == "" && arnID != id.member {
		return false, nil
	}
	if !readable || (memberID != "" && memberID != arnID) {
		return false, fmt.Errorf("%s: AWS listed a member of the folder %q with the ID %q and the ARN %q, which isn't the ARN of a Quick Sight asset with that ID",
			folderMembershipType, id.folder, memberID, aws.ToString(m.MemberArn))
	}
	return strings.EqualFold(kind, string(id.kind)), nil
}

// find returns the folder's member id names, or ErrNotFound when the folder doesn't exist or doesn't list it.
func (h *FolderMembership) find(ctx context.Context, region string, id membershipID) (*types.MemberIdArnPair, error) {
	input := &quicksight.ListFolderMembersInput{AwsAccountId: aws.String(id.account), FolderId: aws.String(id.folder), MaxResults: aws.Int32(maxListResults)}
	for {
		page, err := h.api.ListFolderMembers(ctx, input, inRegion(region))
		if err != nil {
			return nil, awsapi.NotFound(err, "ResourceNotFoundException")
		}
		for i := range page.FolderMemberList {
			match, err := id.isMember(page.FolderMemberList[i])
			if err != nil {
				return nil, err
			}
			if match {
				return &page.FolderMemberList[i], nil
			}
		}
		if input.NextToken = page.NextToken; aws.ToString(input.NextToken) == "" {
			return nil, native.ErrNotFound
		}
	}
}

// Read returns the membership's properties.
func (h *FolderMembership) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	id, err := parseMembershipID(identifier)
	if err != nil {
		return nil, err
	}
	m, err := h.find(ctx, region, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"AwsAccountId": id.account,
		"FolderId":     id.folder,
		"MemberType":   string(id.kind),
		"MemberId":     id.member,
		"MemberArn":    aws.ToString(m.MemberArn),
	}, nil
}

// Create adds the asset to the folder. An asset the folder already has, such as after a create whose response was
// lost, is the template's membership, because every property is in the identifier.
func (h *FolderMembership) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	id, ok, err := parseMembership(request.Properties)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", invalid(folderMembershipType, "set AwsAccountId, FolderId, MemberType, and MemberId")
	}
	_, err = h.api.CreateFolderMembership(ctx, &quicksight.CreateFolderMembershipInput{
		AwsAccountId: aws.String(id.account),
		FolderId:     aws.String(id.folder),
		MemberType:   id.kind,
		MemberId:     aws.String(id.member),
	}, inRegion(request.Region))
	if err == nil || awsapi.HasCode(err, "ResourceExistsException") {
		return id.String(), nil
	}
	// A create AWS didn't reject, such as one whose answer was lost, may have added the asset, so the identifier is
	// kept with the error.
	if err = startError(folderMembershipType, "folder", err); rejected(err) {
		return "", err
	}
	return id.String(), err
}

// Update changes nothing, because every property is create-only.
func (h *FolderMembership) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete removes the asset from the folder. It returns ErrNotFound when the folder or the asset doesn't exist.
func (h *FolderMembership) Delete(ctx context.Context, region string, identifier string) error {
	id, err := parseMembershipID(identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DeleteFolderMembership(ctx, &quicksight.DeleteFolderMembershipInput{
		AwsAccountId: aws.String(id.account),
		FolderId:     aws.String(id.folder),
		MemberType:   id.kind,
		MemberId:     aws.String(id.member),
	}, inRegion(region))
	return startError(folderMembershipType, "folder", awsapi.NotFound(err, "ResourceNotFoundException"))
}

// List returns nothing, because AWS lists members only by folder.
func (h *FolderMembership) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads the folder's members. A create is done once the folder lists the asset, and a delete once it doesn't,
// when Progress returns ErrNotFound. An update changes nothing.
func (h *FolderMembership) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	id, err := parseMembershipID(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	if _, err := h.find(ctx, region, id); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationCreate {
		return native.Progress{Done: true, State: "listed"}, nil
	}
	return native.Progress{State: "listed", Waiting: "not listed"}, nil
}
