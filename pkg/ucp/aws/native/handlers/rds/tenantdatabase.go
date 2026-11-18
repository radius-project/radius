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

package rds

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const tenantDatabaseType = "AWS::RDS::TenantDatabase"

var (
	_ native.Handler           = (*TenantDatabase)(nil)
	_ native.IdentifierEncoder = (*TenantDatabase)(nil)
	_ native.DeletePreparer    = (*TenantDatabase)(nil)
)

// TenantDatabase serves AWS::RDS::TenantDatabase: a tenant database (PDB) in an RDS for Oracle CDB instance that uses
// the multi-tenant configuration. Its identifier is DBInstanceIdentifier and TenantDBName joined with "|", both in
// lowercase, because neither is case-sensitive. Deleting a tenant database takes a final DB snapshot named after it
// and the time of the delete, which Radius never deletes.
type TenantDatabase struct {
	api API

	// now returns the current time, for the name of the final snapshot.
	now func() time.Time
}

// NewTenantDatabase returns the handler for RDS for Oracle tenant databases.
func NewTenantDatabase(api API) *TenantDatabase {
	return &TenantDatabase{api: api, now: time.Now}
}

// TypeName returns AWS::RDS::TenantDatabase.
func (h *TenantDatabase) TypeName() string {
	return tenantDatabaseType
}

// Options returns how UCP uses the handler.
func (h *TenantDatabase) Options() native.Options {
	return native.Options{
		List:       native.ListRegion,
		Recovery:   native.RecoverByIdentifier,
		Equivalent: awsapi.CaseInsensitive("DBInstanceIdentifier", "TenantDBName"),
	}
}

// Identifier returns DBInstanceIdentifier and TenantDBName in lowercase, joined with "|".
func (h *TenantDatabase) Identifier(properties map[string]any) (string, error) {
	instance, name := awsapi.String(properties, "DBInstanceIdentifier"), awsapi.String(properties, "TenantDBName")
	if instance == "" || name == "" {
		return "", nil
	}
	return tenantIdentifier(instance, name), nil
}

func tenantIdentifier(instance string, name string) string {
	return strings.ToLower(instance) + "|" + strings.ToLower(name)
}

// splitTenant returns the instance and the tenant database name of an identifier.
func splitTenant(identifier string) (string, string, error) {
	instance, name, ok := strings.Cut(identifier, "|")
	if !ok || instance == "" || name == "" {
		return "", "", &native.Failure{
			Message:     fmt.Sprintf("The AWS identifier %q of a tenant database must be the DB instance identifier and the tenant database name, joined with |.", identifier),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return instance, name, nil
}

// describe returns the tenant database with the identifier, or ErrNotFound.
func (h *TenantDatabase) describe(ctx context.Context, region string, identifier string) (*types.TenantDatabase, error) {
	instance, name, err := splitTenant(identifier)
	if err != nil {
		return nil, err
	}
	out, err := h.api.DescribeTenantDatabases(ctx, &rds.DescribeTenantDatabasesInput{
		DBInstanceIdentifier: aws.String(instance),
		TenantDBName:         aws.String(name),
	}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "DBInstanceNotFound", "TenantDatabaseNotFound")
	}
	for i := range out.TenantDatabases {
		if strings.EqualFold(aws.ToString(out.TenantDatabases[i].TenantDBName), name) {
			return &out.TenantDatabases[i], nil
		}
	}
	return nil, native.ErrNotFound
}

// Read returns the tenant database's properties.
func (h *TenantDatabase) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	tenant, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return tenantProperties(tenant), nil
}

func tenantProperties(t *types.TenantDatabase) map[string]any {
	p := map[string]any{"ManageMasterUserPassword": t.MasterUserSecret != nil}
	setString(p, "DBInstanceIdentifier", t.DBInstanceIdentifier)
	setString(p, "TenantDBName", t.TenantDBName)
	setString(p, "MasterUsername", t.MasterUsername)
	setString(p, "CharacterSetName", t.CharacterSetName)
	setString(p, "NcharCharacterSetName", t.NcharCharacterSetName)
	setString(p, "TenantDatabaseARN", t.TenantDatabaseARN)
	setString(p, "TenantDatabaseResourceId", t.TenantDatabaseResourceId)
	setString(p, "DbiResourceId", t.DbiResourceId)
	if t.MasterUserSecret != nil {
		secret := map[string]any{}
		setString(secret, "SecretArn", t.MasterUserSecret.SecretArn)
		setString(secret, "SecretStatus", t.MasterUserSecret.SecretStatus)
		setString(secret, "KmsKeyId", t.MasterUserSecret.KmsKeyId)
		p["MasterUserSecret"] = secret
	}
	if tags := awsapi.TagListOf(t.TagList); tags != nil {
		p["Tags"] = tags
	}
	return p
}

// setString sets p[name] to v, unless v is nil or empty.
func setString(p map[string]any, name string, v *string) {
	if aws.ToString(v) != "" {
		p[name] = *v
	}
}

// createFields are the properties CreateTenantDatabase takes. Tags are sent as the SDK's tags.
var createFields = []string{
	"DBInstanceIdentifier", "TenantDBName", "MasterUsername", "MasterUserPassword", "ManageMasterUserPassword",
	"MasterUserSecretKmsKeyId", "CharacterSetName", "NcharCharacterSetName",
}

// Create creates the tenant database.
func (h *TenantDatabase) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, _ := h.Identifier(request.Properties)
	input := &rds.CreateTenantDatabaseInput{}
	if err := awsapi.Decode(only(request.Properties, createFields), input); err != nil {
		return "", err
	}
	if tags := awsapi.Tags(request.Properties); len(tags) > 0 {
		input.Tags = awsapi.SDKTags[types.Tag](tags)
	}
	if _, err := h.api.CreateTenantDatabase(ctx, input, inRegion(request.Region)); err != nil {
		return "", err
	}
	return identifier, nil
}

// only returns the properties named in names.
func only(properties map[string]any, names []string) map[string]any {
	out := map[string]any{}
	for _, name := range names {
		if v, ok := properties[name]; ok {
			out[name] = v
		}
	}
	return out
}

// Update sets the master user password, or whether RDS manages it, with ModifyTenantDatabase, then removes and adds
// tags. Removing ManageMasterUserPassword turns RDS's management off, AWS's default, which takes MasterUserPassword in
// the same call. A removed password or key stays as AWS has it, because AWS can't clear either.
func (h *TenantDatabase) Update(ctx context.Context, request native.UpdateRequest) error {
	tenant, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	opt := inRegion(request.Region)
	changed := awsapi.Changed(request)
	calls := []func() error{}
	input, err := modifyInput(request, changed, tenant.MasterUserSecret)
	if err != nil {
		return err
	}
	if input != nil {
		input.DBInstanceIdentifier, input.TenantDBName = tenant.DBInstanceIdentifier, tenant.TenantDBName
		calls = append(calls, func() error {
			_, err := h.api.ModifyTenantDatabase(ctx, input, opt)
			return err
		})
	}
	if slices.Contains(changed, "Tags") {
		set, remove := awsapi.TagChanges(awsapi.Tags(request.Current), awsapi.Tags(request.Desired))
		arn := tenant.TenantDatabaseARN
		// Removing first keeps the tag set within RDS's 50 tags when the template replaces some.
		if len(remove) > 0 {
			calls = append(calls, func() error {
				_, err := h.api.RemoveTagsFromResource(ctx, &rds.RemoveTagsFromResourceInput{ResourceName: arn, TagKeys: remove}, opt)
				return err
			})
		}
		if len(set) > 0 {
			calls = append(calls, func() error {
				_, err := h.api.AddTagsToResource(ctx, &rds.AddTagsToResourceInput{ResourceName: arn, Tags: awsapi.SDKTags[types.Tag](set)}, opt)
				return err
			})
		}
	}
	return awsapi.Calls(calls...)
}

// modifyInput returns the ModifyTenantDatabase input for the changed properties, or nil when none needs the call.
// secret is the secret RDS manages for the password, or nil when it doesn't manage it.
func modifyInput(request native.UpdateRequest, changed []string, secret *types.MasterUserSecret) (*rds.ModifyTenantDatabaseInput, error) {
	input := &rds.ModifyTenantDatabaseInput{}
	send := false
	manage, manageSet := request.Desired["ManageMasterUserPassword"].(bool)
	if slices.Contains(changed, "ManageMasterUserPassword") {
		input.ManageMasterUserPassword = aws.Bool(manageSet && manage)
		send = true
	}
	if password, ok := request.Desired["MasterUserPassword"].(string); ok && (slices.Contains(changed, "MasterUserPassword") || (input.ManageMasterUserPassword != nil && !*input.ManageMasterUserPassword)) {
		input.MasterUserPassword = aws.String(password)
		send = true
	}
	key, ok := request.Desired["MasterUserSecretKmsKeyId"].(string)
	switch {
	case !ok, input.ManageMasterUserPassword != nil && !*input.ManageMasterUserPassword:
		// A call that turns off RDS's management of the password deletes the secret, so the key doesn't apply.
	case aws.ToBool(input.ManageMasterUserPassword) && secret == nil:
		// The key applies only to the call that turns on RDS's management of the password.
		input.MasterUserSecretKmsKeyId = aws.String(key)
		send = true
	case slices.Contains(changed, "MasterUserSecretKmsKeyId") && secret != nil:
		// AWS can't change the key once RDS manages the password. UCP also sends a key it has no fingerprint of,
		// such as after an update whose response was lost, so the key AWS reports decides.
		if err := sameSecretKey(key, aws.ToString(secret.KmsKeyId)); err != nil {
			return nil, err
		}
	case slices.Contains(changed, "MasterUserSecretKmsKeyId"):
		// RDS doesn't manage the password and the call doesn't turn it on, so AWS rejects the key with its error.
		input.MasterUserSecretKmsKeyId = aws.String(key)
		send = true
	}
	if !send {
		return nil, nil
	}
	return input, nil
}

// sameSecretKey returns nil when key, the template's MasterUserSecretKmsKeyId, names current, the key of the secret RDS
// manages, by its ID or ARN, and a rejected failure otherwise. An alias can't be told from another key without asking
// KMS, so it fails too, and the message names the key to set instead.
func sameSecretKey(key string, current string) error {
	if key == current || strings.HasSuffix(current, ":key/"+key) || strings.HasSuffix(key, ":key/"+current) {
		return nil
	}
	return &native.Failure{
		Message: fmt.Sprintf("RDS already manages the master user password with the KMS key %q, and AWS can't change it. "+
			"Set MasterUserSecretKmsKeyId to that key's ID or ARN, or turn off ManageMasterUserPassword first.", current),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// Delete deletes the tenant database with a final snapshot named by DeleteToken. UCP calls DeleteWithToken instead,
// with the name it saved on the record.
func (h *TenantDatabase) Delete(ctx context.Context, region string, identifier string) error {
	token, err := h.DeleteToken(ctx, region, identifier)
	if err != nil {
		return err
	}
	return h.DeleteWithToken(ctx, region, identifier, token)
}

// nonSnapshotCharacters are the characters a DB snapshot identifier can't have.
var nonSnapshotCharacters = regexp.MustCompile(`[^a-z0-9]+`)

// DeleteToken returns the name of the final snapshot: the instance, the tenant database name, "-final-", the UTC time,
// and a hash of the identifier, such as app-cdb-pdb1-final-20261104-212751-1a2b3c4d. A tenant database name can have
// characters a snapshot identifier can't, such as _, so each run of them becomes one hyphen, and the hash keeps the
// names of two tenant databases deleted in the same second apart, such as pdb_1 and pdb-1. The name starts with the
// instance's identifier, which starts with a letter, and fits a snapshot identifier's 255 characters, because an
// instance identifier has at most 63 and a tenant database name at most 8.
func (h *TenantDatabase) DeleteToken(ctx context.Context, region string, identifier string) (string, error) {
	instance, name, err := splitTenant(identifier)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.ToLower(identifier)))
	parts := []string{}
	for _, part := range []string{instance, name, "final", h.now().UTC().Format("20060102-150405"), hex.EncodeToString(sum[:4])} {
		if part = strings.Trim(nonSnapshotCharacters.ReplaceAllString(strings.ToLower(part), "-"), "-"); part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "-"), nil
}

// DeleteWithToken deletes the tenant database with a final snapshot named token. A tenant database AWS is already
// deleting had a delete reach AWS before, so the delete succeeds. If AWS can't take the snapshot, it rejects the
// delete, and the tenant database stays.
func (h *TenantDatabase) DeleteWithToken(ctx context.Context, region string, identifier string, token string) error {
	tenant, err := h.describe(ctx, region, identifier)
	if err != nil {
		return err
	}
	if deleting(tenant) {
		return nil
	}
	_, err = h.api.DeleteTenantDatabase(ctx, &rds.DeleteTenantDatabaseInput{
		DBInstanceIdentifier:      tenant.DBInstanceIdentifier,
		TenantDBName:              tenant.TenantDBName,
		FinalDBSnapshotIdentifier: aws.String(token),
	}, inRegion(region))
	if awsapi.HasCode(err, "InvalidDBInstanceState") {
		tenant, describeErr := h.describe(ctx, region, identifier)
		if describeErr != nil {
			return describeErr
		}
		if deleting(tenant) {
			return nil
		}
	}
	return awsapi.NotFound(err, "DBInstanceNotFound", "TenantDatabaseNotFound")
}

func deleting(t *types.TenantDatabase) bool {
	return strings.EqualFold(aws.ToString(t.Status), "deleting")
}

// List returns every tenant database in the region.
func (h *TenantDatabase) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	var marker *string
	for {
		page, err := h.api.DescribeTenantDatabases(ctx, &rds.DescribeTenantDatabasesInput{Marker: marker}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for i := range page.TenantDatabases {
			t := &page.TenantDatabases[i]
			identifier := tenantIdentifier(aws.ToString(t.DBInstanceIdentifier), aws.ToString(t.TenantDBName))
			out = append(out, native.Resource{Identifier: identifier, Properties: tenantProperties(t)})
		}
		if marker = page.Marker; marker == nil {
			return out, nil
		}
	}
}

// Progress reads the tenant database's status. A create or update waits until it is available and has no pending
// changes, such as a new master user password, and a delete until it is gone.
func (h *TenantDatabase) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	tenant, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := aws.ToString(tenant.Status)
	if op == native.OperationDelete {
		return native.Progress{State: status, Waiting: "deleted"}, nil
	}
	if strings.EqualFold(status, "available") && !pending(tenant.PendingModifiedValues) {
		return native.Progress{Done: true, State: status}, nil
	}
	return native.Progress{State: status, Waiting: "available"}, nil
}

func pending(p *types.TenantDatabasePendingModifiedValues) bool {
	return p != nil && (p.MasterUserPassword != nil || p.TenantDBName != nil)
}
