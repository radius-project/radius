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

package opensearch

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

func newAssociationHandler() (*PackageAssociation, *pkgFake) {
	api := newPkgFake(awsfake.NewCloudControl())
	return NewPackageAssociation(api), api
}

// createAssociation associates a package with the handler and waits until it is ACTIVE.
func createAssociation(t *testing.T, h *PackageAssociation, properties map[string]any) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: properties})
	require.NoError(t, err)
	require.Nil(t, pkgWait(t, h, id, native.OperationCreate).Failure)
	return id
}

// plugin, license, and their templates are the seeded plugin for logs and its license, which needs the plugin.
const (
	plugin  = "pkg-0a1b2c3d"
	license = "pkg-1c2d3e4f"
)

var (
	pluginTemplate  = map[string]any{"DomainName": "logs", "PackageID": plugin}
	licenseTemplate = map[string]any{"DomainName": "logs", "PackageID": license, "PrerequisitePackageIDList": []any{plugin}}
)

func Test_PackageAssociation_Identifier(t *testing.T) {
	h, _ := newAssociationHandler()
	for _, tc := range []struct {
		properties map[string]any
		want       string
	}{
		{map[string]any{"DomainName": "logs", "PackageID": "F111111111"}, "logs|F111111111"},
		{map[string]any{"DomainName": "logs", "PackageID": "G333333333"}, "logs|G333333333"},
		{licenseTemplate, "logs|" + license},
		{map[string]any{"DomainName": "logs"}, ""},
		{map[string]any{"PackageID": plugin}, ""},
	} {
		id, err := h.Identifier(tc.properties)
		require.NoError(t, err)
		require.Equal(t, tc.want, id)
	}
	for _, properties := range []map[string]any{
		{"DomainName": 5, "PackageID": plugin},
		{"DomainName": "lo", "PackageID": plugin},
		{"DomainName": "logs", "PackageID": "pkg-XYZ"},
		{"DomainName": "logs", "PackageID": plugin, "PrerequisitePackageIDList": []any{5}},
		{"DomainName": "logs", "PackageID": plugin, "AssociationConfiguration": map[string]any{"KeyStoreAccessOption": "on"}},
	} {
		_, err := h.Identifier(properties)
		requirePkgRejected(t, err)
	}
	for _, identifier := range []string{"logs", "logs|", "Logs|" + plugin, "logs|plugin", "|" + plugin} {
		_, err := h.Read(t.Context(), pkgRegion, identifier)
		requirePkgRejected(t, err)
		requirePkgRejected(t, h.Delete(t.Context(), pkgRegion, identifier))
		_, err = h.Progress(t.Context(), pkgRegion, identifier, native.OperationCreate)
		requirePkgRejected(t, err)
	}
	for _, properties := range []map[string]any{
		{"DomainName": "logs"},
		{"DomainName": "Logs", "PackageID": plugin},
		{"DomainName": "logs", "PackageID": plugin, "PrerequisitePackageIDList": "F111111111"},
	} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: properties})
		requirePkgRejected(t, err)
	}
}

// Test_PackageAssociation_FirstLook checks that a new association isn't ACTIVE the first time UCP reads it, and that
// Read returns what AWS lists for it.
func Test_PackageAssociation_FirstLook(t *testing.T) {
	h, _ := newAssociationHandler()
	properties := map[string]any{"DomainName": "logs", "PackageID": "F111111111"}
	id, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: properties})
	require.NoError(t, err)
	p, err := h.Progress(t.Context(), pkgRegion, id, native.OperationCreate)
	require.NoError(t, err)
	require.False(t, p.Done)
	require.Equal(t, "ACTIVE", p.Waiting)

	pkgWait(t, h, id, native.OperationCreate)
	read, err := h.Read(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	require.Equal(t, withProps(properties, map[string]any{
		"DomainPackageStatus": "ACTIVE", "PackageName": "synonyms", "PackageType": "TXT-DICTIONARY", "PackageVersion": "v1",
		"ReferencePath": "analyzers/F111111111",
	}), read)
	p, err = h.Progress(t.Context(), pkgRegion, id, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, p.Done)
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{}))
}

// Test_PackageAssociation_CreateFindsExisting checks that a create sent again, after AWS took the first one, sends
// nothing, because associating a package again applies its latest version, and that one whose create-only values
// differ from what AWS has fails instead of taking the association as the template's.
func Test_PackageAssociation_CreateFindsExisting(t *testing.T) {
	h, api := newAssociationHandler()
	api.echoOptional = true
	createAssociation(t, h, pluginTemplate)
	id := createAssociation(t, h, licenseTemplate)
	again, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: licenseTemplate})
	require.NoError(t, err)
	require.Equal(t, id, again)
	require.Equal(t, 2, api.calls["AssociatePackage"])

	stored := api.store.Resource(packageAssociationType, id)
	api.store.Put(packageAssociationType, id, withProps(stored, map[string]any{
		"AssociationConfiguration": map[string]any{"KeyStoreAccessOption": map[string]any{"KeyStoreAccessEnabled": false}},
	}))
	for _, other := range []map[string]any{
		{"DomainName": "logs", "PackageID": license},
		withProps(licenseTemplate, map[string]any{"AssociationConfiguration": map[string]any{"KeyStoreAccessOption": map[string]any{"KeyStoreAccessEnabled": true}}}),
	} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: other})
		requirePkgRejectedBefore(t, err, "PrerequisitePackageIDList or AssociationConfiguration")
	}
	require.Equal(t, 2, api.calls["AssociatePackage"])

	// The same prerequisites in another order are the same association, and so is a configuration AWS doesn't return.
	api.store.Put(packageAssociationType, id, stored)
	reordered := withProps(licenseTemplate, map[string]any{"PrerequisitePackageIDList": []any{plugin}})
	_, err = h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: reordered})
	require.NoError(t, err)
}

// Test_PackageAssociation_DomainBusy checks that an association or dissociation while AWS is changing the domain, or
// another of its packages, is a conflict that changes nothing, and that the next deploy succeeds once AWS is done.
func Test_PackageAssociation_DomainBusy(t *testing.T) {
	h, api := newAssociationHandler()
	api.processing["logs"] = 3
	_, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: pluginTemplate})
	requirePkgConflict(t, err, "dependsOn")
	require.Empty(t, api.store.List(packageAssociationType))
	delete(api.processing, "logs")

	id, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: pluginTemplate})
	require.NoError(t, err)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: map[string]any{"DomainName": "logs", "PackageID": "F111111111"}})
	requirePkgConflict(t, err, "dependsOn")
	requirePkgConflict(t, h.Delete(t.Context(), pkgRegion, id), "ASSOCIATING")
	require.Zero(t, api.calls["DissociatePackage"])
	pkgWait(t, h, id, native.OperationCreate)

	api.processing["logs"] = 3
	requirePkgConflict(t, h.Delete(t.Context(), pkgRegion, id), "dependsOn")
	delete(api.processing, "logs")
	require.NoError(t, h.Delete(t.Context(), pkgRegion, id))
	require.NoError(t, h.Delete(t.Context(), pkgRegion, id), "a dissociation AWS already started succeeds")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: pluginTemplate})
	requirePkgConflict(t, err, "DISSOCIATING")
	require.Equal(t, 2, api.calls["DissociatePackage"], "the dissociation AWS rejected while the domain was busy, and the one it took")
	pkgWait(t, h, id, native.OperationDelete)
}

// Test_PackageAssociation_Failures checks that an association that fails ends the create with AWS's details and is
// associated again by the next create, and that a dissociation that fails ends the delete with a message that says
// what to do, and is sent again by the next delete.
func Test_PackageAssociation_Failures(t *testing.T) {
	h, api := newAssociationHandler()
	properties := map[string]any{"DomainName": "logs", "PackageID": "F111111111"}
	api.failAssociate = true
	id, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: properties})
	require.NoError(t, err)
	p := pkgWait(t, h, id, native.OperationCreate)
	require.NotNil(t, p.Failure)
	require.Equal(t, "ASSOCIATION_FAILED", p.State)
	require.Contains(t, p.Failure.Message, "an index uses the package")
	require.Equal(t, native.OutcomeRejected, p.Failure.Outcome, "the domain doesn't have the package, so the next deploy associates it again")

	createAssociation(t, h, properties)
	require.Equal(t, 2, api.calls["AssociatePackage"])

	api.failDissociate = true
	require.NoError(t, h.Delete(t.Context(), pkgRegion, id))
	p = pkgWait(t, h, id, native.OperationDelete)
	require.NotNil(t, p.Failure)
	require.Contains(t, p.Failure.Message, "Remove the package from every index")
	require.NoError(t, h.Delete(t.Context(), pkgRegion, id))
	pkgWait(t, h, id, native.OperationDelete)
	require.Empty(t, api.store.List(packageAssociationType))
	require.ErrorIs(t, h.Delete(t.Context(), pkgRegion, id), native.ErrNotFound)
}

// Test_PackageAssociation_AWSRejects checks that AWS's rejections of an association are returned as rejected, never as
// not found: a missing domain or package, a plugin for another engine version, and a license before its plugin.
func Test_PackageAssociation_AWSRejects(t *testing.T) {
	h, api := newAssociationHandler()
	for _, properties := range []map[string]any{
		{"DomainName": "gone", "PackageID": "F111111111"},
		{"DomainName": "logs", "PackageID": "F999999999"},
		{"DomainName": "metrics", "PackageID": plugin},
		licenseTemplate,
	} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: properties})
		requirePkgRejected(t, err)
		require.NotErrorIs(t, err, native.ErrNotFound)
	}
	require.Empty(t, api.store.List(packageAssociationType))

	_, err := h.Read(t.Context(), pkgRegion, "gone|F111111111")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Read(t.Context(), pkgRegion, "logs|F111111111")
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.Delete(t.Context(), pkgRegion, "gone|F111111111"), native.ErrNotFound)
	resources, err := h.List(t.Context(), pkgRegion)
	require.NoError(t, err)
	require.Empty(t, resources)
}

// Test_PackageAssociation_Pages checks that a package on a later page of the domain's list is found.
func Test_PackageAssociation_Pages(t *testing.T) {
	h, _ := newAssociationHandler()
	ids := []string{}
	for _, id := range []string{"F111111111", "F222222222", "G333333333"} {
		ids = append(ids, createAssociation(t, h, map[string]any{"DomainName": "logs", "PackageID": id}))
	}
	read, err := h.Read(t.Context(), pkgRegion, ids[2])
	require.NoError(t, err)
	require.Equal(t, "analysis-icu", read["PackageName"])
}

// Test_PackageAssociation_PluginRemovesLicense checks that a delete of a license whose plugin AWS already dissociated,
// which removes the license too, finds nothing to delete.
func Test_PackageAssociation_PluginRemovesLicense(t *testing.T) {
	h, _ := newAssociationHandler()
	pluginID := createAssociation(t, h, pluginTemplate)
	licenseID := createAssociation(t, h, licenseTemplate)
	require.NoError(t, h.Delete(t.Context(), pkgRegion, pluginID))
	pkgWait(t, h, pluginID, native.OperationDelete)
	require.ErrorIs(t, h.Delete(t.Context(), pkgRegion, licenseID), native.ErrNotFound)
}

// Test_PackageAssociation_FailsBeforeChange checks that a list that fails before a create or delete sends anything is
// reported as rejected.
func Test_PackageAssociation_FailsBeforeChange(t *testing.T) {
	h, api := newAssociationHandler()
	id := createAssociation(t, h, pluginTemplate)
	api.errs["ListPackagesForDomain"] = errors.New("list down")
	_, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: map[string]any{"DomainName": "logs", "PackageID": "F111111111"}})
	requirePkgRejectedBefore(t, err, "list down")
	requirePkgRejectedBefore(t, h.Delete(t.Context(), pkgRegion, id), "list down")
	require.Equal(t, 1, api.calls["AssociatePackage"])
	require.Zero(t, api.calls["DissociatePackage"])
}

// associateErrors is a package API whose AssociatePackage and DissociatePackage fail with err.
type associateErrors struct {
	*pkgFake
	err error
}

func (a associateErrors) AssociatePackage(context.Context, *opensearch.AssociatePackageInput, ...func(*opensearch.Options)) (*opensearch.AssociatePackageOutput, error) {
	return nil, a.err
}

func (a associateErrors) DissociatePackage(context.Context, *opensearch.DissociatePackageInput, ...func(*opensearch.Options)) (*opensearch.DissociatePackageOutput, error) {
	return nil, a.err
}

// Test_PackageAssociation_ServerErrors checks that a server error from AssociatePackage or DissociatePackage is
// returned as it is, so UCP counts the call as possibly applied, not as rejected.
func Test_PackageAssociation_ServerErrors(t *testing.T) {
	api := newPkgFake(awsfake.NewCloudControl())
	id := createAssociation(t, NewPackageAssociation(api), pluginTemplate)
	boom := fmt.Errorf("connection reset")
	h := NewPackageAssociation(associateErrors{pkgFake: api, err: boom})
	_, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: map[string]any{"DomainName": "logs", "PackageID": "F111111111"}})
	require.ErrorIs(t, err, boom)
	require.ErrorIs(t, h.Delete(t.Context(), pkgRegion, id), boom)
}
