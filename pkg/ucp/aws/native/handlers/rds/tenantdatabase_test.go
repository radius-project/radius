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
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

const tenantID = "app-cdb|pdb1"

// newTenant returns a handler whose fake has the instance app-cdb with the available tenant database pdb1, created
// from properties.
func newTenant(t *testing.T, properties map[string]any) (*TenantDatabase, *fakeAPI) {
	store := awsfake.NewCloudControl()
	addInstance(store, "app-cdb")
	api := newFakeAPI(store)
	h := NewTenantDatabase(api)
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	require.Equal(t, tenantID, id)
	waitTenant(t, h, native.OperationCreate)
	return h, api
}

func waitTenant(t *testing.T, h *TenantDatabase, op native.Operation) {
	for range 5 {
		progress, err := h.Progress(t.Context(), region, tenantID, op)
		require.NoError(t, err)
		if progress.Done {
			return
		}
	}
	t.Fatalf("the %s didn't finish", op)
}

func Test_TenantDatabase_Identifier(t *testing.T) {
	h := NewTenantDatabase(newFakeAPI(awsfake.NewCloudControl()))
	id, err := h.Identifier(map[string]any{"DBInstanceIdentifier": "App-CDB", "TenantDBName": "PDB1"})
	require.NoError(t, err)
	require.Equal(t, tenantID, id, "neither name is case-sensitive")
	id, err = h.Identifier(map[string]any{"DBInstanceIdentifier": "app-cdb"})
	require.NoError(t, err)
	require.Empty(t, id)

	for _, bad := range []string{"app-cdb", "|pdb1", "app-cdb|"} {
		_, err := h.Read(t.Context(), region, bad)
		var failure *native.Failure
		require.ErrorAs(t, err, &failure, bad)
		require.True(t, failure.ClientError)
	}
}

func Test_TenantDatabase_Read(t *testing.T) {
	h, api := newTenant(t, with(tenant, map[string]any{"MasterUserPassword": nil, "ManageMasterUserPassword": true, "MasterUserSecretKmsKeyId": keyID}))
	properties, err := h.Read(t.Context(), region, tenantID)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"DBInstanceIdentifier":     "app-cdb",
		"TenantDBName":             "pdb1",
		"MasterUsername":           "admin",
		"CharacterSetName":         "AL32UTF8",
		"NcharCharacterSetName":    "AL16UTF16",
		"ManageMasterUserPassword": true,
		"MasterUserSecret": map[string]any{
			"SecretArn":    "arn:aws:secretsmanager:us-west-2:123456789012:secret:rds!tdb-" + resourceID(tenantID),
			"SecretStatus": "active",
			"KmsKeyId":     keyID,
		},
		"TenantDatabaseARN":        arnPrefix + "db:app-cdb:pdb1",
		"TenantDatabaseResourceId": "tdb-" + resourceID(tenantID),
		"DbiResourceId":            "db-" + resourceID("app-cdb"),
		"Tags":                     []any{map[string]any{"Key": "owner", "Value": ""}, map[string]any{"Key": "team", "Value": "a"}},
	}, properties)

	_, err = h.Read(t.Context(), region, "app-cdb|missing")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Read(t.Context(), region, "missing|pdb1")
	require.ErrorIs(t, err, native.ErrNotFound, "a missing instance has no tenant databases")
	api.errs["DescribeTenantDatabases"] = fault("TenantDatabaseNotFound")
	_, err = h.Read(t.Context(), region, tenantID)
	require.ErrorIs(t, err, native.ErrNotFound)
	api.errs["DescribeTenantDatabases"] = fault("ThrottlingException")
	_, err = h.Read(t.Context(), region, tenantID)
	require.ErrorContains(t, err, "ThrottlingException")
}

// recordingAPI records the ModifyTenantDatabase inputs it passes on.
type recordingAPI struct {
	API
	modifies []*rds.ModifyTenantDatabaseInput
}

func (r *recordingAPI) ModifyTenantDatabase(ctx context.Context, in *rds.ModifyTenantDatabaseInput, opts ...func(*rds.Options)) (*rds.ModifyTenantDatabaseOutput, error) {
	r.modifies = append(r.modifies, in)
	return r.API.ModifyTenantDatabase(ctx, in, opts...)
}

func Test_TenantDatabase_Update(t *testing.T) {
	update := func(t *testing.T, h *TenantDatabase, desired map[string]any, ops ...jsondiff.Operation) error {
		current, err := h.Read(t.Context(), region, tenantID)
		require.NoError(t, err)
		return h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: tenantID, Current: current, Desired: desired, Patch: ops})
	}
	replace := func(path string, value any) jsondiff.Operation {
		return jsondiff.Operation{Type: jsondiff.OperationReplace, Path: "/" + path, Value: value}
	}
	remove := func(path string) jsondiff.Operation {
		return jsondiff.Operation{Type: jsondiff.OperationRemove, Path: "/" + path}
	}

	t.Run("turning on RDS's management sends the key and no password", func(t *testing.T) {
		h, api := newTenant(t, tenant)
		rec := &recordingAPI{API: api}
		h.api = rec
		desired := with(tenant, map[string]any{"MasterUserPassword": nil, "ManageMasterUserPassword": true, "MasterUserSecretKmsKeyId": keyID})
		require.NoError(t, update(t, h, desired, replace("ManageMasterUserPassword", true), remove("MasterUserPassword"), replace("MasterUserSecretKmsKeyId", keyID)))
		require.Len(t, rec.modifies, 1)
		require.True(t, aws.ToBool(rec.modifies[0].ManageMasterUserPassword))
		require.Nil(t, rec.modifies[0].MasterUserPassword)
		require.Equal(t, keyID, aws.ToString(rec.modifies[0].MasterUserSecretKmsKeyId))
		waitTenant(t, h, native.OperationUpdate)

		t.Run("removing ManageMasterUserPassword turns it off with the template's password", func(t *testing.T) {
			rec.modifies = nil
			require.NoError(t, update(t, h, tenant, remove("ManageMasterUserPassword"), replace("MasterUserPassword", "pass-one-1234")))
			require.Len(t, rec.modifies, 1)
			require.False(t, aws.ToBool(rec.modifies[0].ManageMasterUserPassword))
			require.NotNil(t, rec.modifies[0].ManageMasterUserPassword)
			require.Equal(t, "pass-one-1234", aws.ToString(rec.modifies[0].MasterUserPassword))
			require.Nil(t, rec.modifies[0].MasterUserSecretKmsKeyId)
		})
	})

	t.Run("turning management off sends an unchanged password too", func(t *testing.T) {
		h, api := newTenant(t, with(tenant, map[string]any{"MasterUserPassword": nil, "ManageMasterUserPassword": true}))
		rec := &recordingAPI{API: api}
		h.api = rec
		require.NoError(t, update(t, h, with(tenant, map[string]any{"ManageMasterUserPassword": false}), replace("ManageMasterUserPassword", false)))
		require.Equal(t, "pass-one-1234", aws.ToString(rec.modifies[0].MasterUserPassword))
	})

	t.Run("turning management off without a password fails with AWS's error", func(t *testing.T) {
		h, _ := newTenant(t, with(tenant, map[string]any{"MasterUserPassword": nil, "ManageMasterUserPassword": true}))
		err := update(t, h, with(tenant, map[string]any{"MasterUserPassword": nil}), remove("ManageMasterUserPassword"))
		require.ErrorContains(t, err, "MissingParameter")
	})

	t.Run("a removed password or key changes nothing", func(t *testing.T) {
		h, api := newTenant(t, tenant)
		rec := &recordingAPI{API: api}
		h.api = rec
		require.NoError(t, update(t, h, with(tenant, map[string]any{"MasterUserPassword": nil}), remove("MasterUserPassword"), remove("MasterUserSecretKmsKeyId")))
		require.Empty(t, rec.modifies)
	})

	t.Run("a key sent again after RDS started managing the password needs no call", func(t *testing.T) {
		managed := with(tenant, map[string]any{"MasterUserPassword": nil, "ManageMasterUserPassword": true, "MasterUserSecretKmsKeyId": keyID})
		h, api := newTenant(t, managed)
		rec := &recordingAPI{API: api}
		h.api = rec
		for _, key := range []string{keyID, keyArn, "alias/app-secret"} {
			require.NoError(t, update(t, h, with(managed, map[string]any{"MasterUserSecretKmsKeyId": key}), replace("MasterUserSecretKmsKeyId", key)), key)
		}
		require.Empty(t, rec.modifies, "AWS already uses the key, and can't take it again")

		err := update(t, h, with(managed, map[string]any{"MasterUserSecretKmsKeyId": "other-key"}), replace("MasterUserSecretKmsKeyId", "other-key"))
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ClientError)
		require.Contains(t, failure.Message, keyID, "the message names the key AWS uses")
		require.Empty(t, rec.modifies)
	})

	t.Run("a key without RDS's management is sent, so AWS rejects it", func(t *testing.T) {
		h, _ := newTenant(t, tenant)
		err := update(t, h, with(tenant, map[string]any{"MasterUserSecretKmsKeyId": keyID}), replace("MasterUserSecretKmsKeyId", keyID))
		require.ErrorContains(t, err, "InvalidParameterCombination")
	})

	t.Run("turning management off doesn't send the key", func(t *testing.T) {
		managed := with(tenant, map[string]any{"MasterUserPassword": nil, "ManageMasterUserPassword": true, "MasterUserSecretKmsKeyId": keyID})
		h, api := newTenant(t, managed)
		rec := &recordingAPI{API: api}
		h.api = rec
		desired := with(tenant, map[string]any{"ManageMasterUserPassword": false, "MasterUserSecretKmsKeyId": "other-key"})
		require.NoError(t, update(t, h, desired, replace("ManageMasterUserPassword", false), replace("MasterUserSecretKmsKeyId", "other-key")))
		require.Nil(t, rec.modifies[0].MasterUserSecretKmsKeyId)
	})

	t.Run("replacing a tag of 50 removes the old key first", func(t *testing.T) {
		many := []any{}
		for i := range maxTags {
			many = append(many, map[string]any{"Key": fmt.Sprintf("key-%02d", i), "Value": "v"})
		}
		h, api := newTenant(t, with(tenant, map[string]any{"Tags": many}))
		replaced := append([]any{map[string]any{"Key": "new-key", "Value": "v"}}, many[1:]...)
		require.NoError(t, update(t, h, with(tenant, map[string]any{"Tags": replaced}), replace("Tags", replaced)))
		stored := api.store.Resource(tenantDatabaseType, tenantID)
		require.Len(t, stored["Tags"], maxTags)
	})

	t.Run("a tenant database that is gone is not found", func(t *testing.T) {
		h, api := newTenant(t, tenant)
		current, err := h.Read(t.Context(), region, tenantID)
		require.NoError(t, err)
		api.store.Remove(tenantDatabaseType, tenantID)
		err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: tenantID, Current: current, Desired: tenant})
		require.ErrorIs(t, err, native.ErrNotFound)
	})
}

func Test_TenantDatabase_DeleteToken(t *testing.T) {
	h := NewTenantDatabase(newFakeAPI(awsfake.NewCloudControl()))
	h.now = func() time.Time { return time.Date(2026, 11, 4, 21, 27, 51, 0, time.FixedZone("PST", -8*3600)) }
	hash := func(identifier string) string {
		sum := sha256.Sum256([]byte(identifier))
		return hex.EncodeToString(sum[:4])
	}
	for identifier, want := range map[string]string{
		tenantID:           "app-cdb-pdb1-final-20261105-052751-" + hash(tenantID),
		"app-cdb|PDB1":     "app-cdb-pdb1-final-20261105-052751-" + hash(tenantID),
		"app-cdb|pdb_1":    "app-cdb-pdb-1-final-20261105-052751-" + hash("app-cdb|pdb_1"),
		"app-cdb|_p$#d_":   "app-cdb-p-d-final-20261105-052751-" + hash("app-cdb|_p$#d_"),
		"app-cdb|$_":       "app-cdb-final-20261105-052751-" + hash("app-cdb|$_"),
		"app-cdb|pdb--two": "app-cdb-pdb-two-final-20261105-052751-" + hash("app-cdb|pdb--two"),
	} {
		token, err := h.DeleteToken(t.Context(), region, identifier)
		require.NoError(t, err)
		require.Equal(t, want, token, identifier)
		require.True(t, validSnapshotID(token), token)
	}

	// Names that read the same once sanitized and joined still get different snapshots.
	tokens := map[string]bool{}
	for _, identifier := range []string{"app-cdb|pdb", "app|cdb-pdb", "app-cdb|pdb_1", "app-cdb|pdb-1"} {
		token, err := h.DeleteToken(t.Context(), region, identifier)
		require.NoError(t, err)
		require.False(t, tokens[token], identifier)
		tokens[token] = true
	}

	_, err := h.DeleteToken(t.Context(), region, "app-cdb")
	require.Error(t, err)
}

func Test_TenantDatabase_Delete(t *testing.T) {
	t.Run("deletes with a final snapshot", func(t *testing.T) {
		h, api := newTenant(t, tenant)
		require.NoError(t, h.Delete(t.Context(), region, tenantID))
		require.Len(t, api.snapshots, 1)
		progress, err := h.Progress(t.Context(), region, tenantID, native.OperationDelete)
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "deleting", Waiting: "deleted"}, progress)
		_, err = h.Progress(t.Context(), region, tenantID, native.OperationDelete)
		require.ErrorIs(t, err, native.ErrNotFound)
		require.ErrorIs(t, h.Delete(t.Context(), region, tenantID), native.ErrNotFound)
	})

	t.Run("a tenant database AWS is already deleting needs no call", func(t *testing.T) {
		h, api := newTenant(t, tenant)
		require.NoError(t, h.DeleteWithToken(t.Context(), region, tenantID, "snap-one"))
		api.errs["DeleteTenantDatabase"] = fmt.Errorf("unexpected call")
		require.NoError(t, h.DeleteWithToken(t.Context(), region, tenantID, "snap-one"))
		require.Equal(t, []string{"snap-one"}, api.snapshots, "a retried delete takes no second snapshot")
	})

	t.Run("a delete AWS rejects for a delete that started since the read succeeds", func(t *testing.T) {
		h, api := newTenant(t, tenant)
		api.errs["DeleteTenantDatabase"] = fault("InvalidDBInstanceState")
		h.api = deleteRace{API: api, reads: new(int)}
		require.NoError(t, h.DeleteWithToken(t.Context(), region, tenantID, "snap-one"))
	})

	t.Run("the instance's only tenant database can't be deleted", func(t *testing.T) {
		h, api := newTenant(t, tenant)
		instance := api.store.Resource(instanceType, "app-cdb")
		instance["InitialTenants"] = 0
		api.store.Put(instanceType, "app-cdb", instance)
		require.ErrorContains(t, h.Delete(t.Context(), region, tenantID), "InvalidDBInstanceState")
		require.Empty(t, api.snapshots)
	})

	t.Run("a final snapshot needs the tenant database available", func(t *testing.T) {
		h, api := newTenant(t, tenant)
		api.status[tenantID] = "backing-up"
		require.ErrorContains(t, h.Delete(t.Context(), region, tenantID), "InvalidDBInstanceState")
		require.Empty(t, api.snapshots)
	})
}

// deleteRace reports the tenant database as available to the handler's first read and deleting after it, as when a
// delete starts between the read and the handler's call.
type deleteRace struct {
	API
	reads *int
}

func (d deleteRace) DescribeTenantDatabases(ctx context.Context, in *rds.DescribeTenantDatabasesInput, opts ...func(*rds.Options)) (*rds.DescribeTenantDatabasesOutput, error) {
	out, err := d.API.DescribeTenantDatabases(ctx, in, opts...)
	if err == nil {
		*d.reads++
		out.TenantDatabases[0].Status = aws.String(map[bool]string{true: "available", false: "deleting"}[*d.reads == 1])
	}
	return out, err
}

func Test_TenantDatabase_List(t *testing.T) {
	h, api := newTenant(t, tenant)
	addInstance(api.store, "other-cdb")
	for _, name := range []string{"pdb2", "pdb3"} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(tenant, map[string]any{"DBInstanceIdentifier": "other-cdb", "TenantDBName": name})})
		require.NoError(t, err)
	}
	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	ids := []string{}
	for _, r := range resources {
		ids = append(ids, r.Identifier)
		require.NotContains(t, r.Properties, "MasterUserPassword")
	}
	require.Equal(t, []string{tenantID, "other-cdb|pdb2", "other-cdb|pdb3"}, ids, "List reads every page")

	api.errs["DescribeTenantDatabases"] = fault("ThrottlingException")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "ThrottlingException")
}

func Test_TenantDatabase_Progress(t *testing.T) {
	h, _ := newTenant(t, tenant)
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{
		Region: region, Identifier: tenantID, Desired: with(tenant, map[string]any{"MasterUserPassword": "pass-two-5678"}),
		Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/MasterUserPassword", Value: "pass-two-5678"}},
	}))
	progress, err := h.Progress(t.Context(), region, tenantID, native.OperationUpdate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "modifying", Waiting: "available"}, progress, "a pending password isn't applied yet")
	progress, err = h.Progress(t.Context(), region, tenantID, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, progress.Done)

	require.False(t, pending(nil))
	require.True(t, pending(&types.TenantDatabasePendingModifiedValues{TenantDBName: aws.String("pdb2")}))
}

func Test_TenantDatabase_Errors(t *testing.T) {
	h, api := newTenant(t, tenant)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(tenant, map[string]any{"MasterUsername": 5})})
	require.ErrorContains(t, err, "MasterUsername", "a value of the wrong type is rejected before any call")

	api.errs["CreateTenantDatabase"] = fault("TenantDatabaseQuotaExceeded")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(tenant, map[string]any{"TenantDBName": "pdb2"})})
	require.ErrorContains(t, err, "TenantDatabaseQuotaExceeded")

	require.Error(t, h.Delete(t.Context(), region, "app-cdb"), "an identifier without a tenant database name is rejected")

	api.errs["DeleteTenantDatabase"] = fault("InvalidDBInstanceState")
	h.api = deleteGone{API: api, store: api}
	require.ErrorIs(t, h.DeleteWithToken(t.Context(), region, tenantID, "snap-one"), native.ErrNotFound, "a tenant database deleted since the read is not found")
}

// deleteGone removes the tenant database before DeleteTenantDatabase, as when another delete finishes between the
// handler's read and its call.
type deleteGone struct {
	API
	store *fakeAPI
}

func (d deleteGone) DeleteTenantDatabase(ctx context.Context, in *rds.DeleteTenantDatabaseInput, opts ...func(*rds.Options)) (*rds.DeleteTenantDatabaseOutput, error) {
	d.store.store.Remove(tenantDatabaseType, tenantID)
	return d.API.DeleteTenantDatabase(ctx, in, opts...)
}
