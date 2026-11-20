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

package redshiftserverless

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

const (
	domainFile = "aws-redshiftserverless-customdomainassociation.json"
	domainID   = "analytics|db.example.com"
)

// createdDomain creates the association properties give and waits until the workgroup is free again.
func createdDomain(t *testing.T, h *CustomDomainAssociation, properties map[string]any) {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	require.Equal(t, domainID, id)
	waitDone(t, h, id, native.OperationCreate)
}

func Test_CustomDomain_Identifier(t *testing.T) {
	h := NewCustomDomainAssociation(newAPI())
	id, err := h.Identifier(domain)
	require.NoError(t, err)
	require.Equal(t, domainID, id)
	id, err = h.Identifier(with(domain, map[string]any{"CustomDomainName": nil}))
	require.NoError(t, err)
	require.Empty(t, id, "a template without the domain doesn't give the identifier")

	for _, bad := range []string{"", "analytics", "|db.example.com", "analytics|"} {
		_, err := h.Read(t.Context(), region, bad)
		_ = requireFailure(t, err, native.OutcomeRejected, false)
		_ = requireFailure(t, h.Delete(t.Context(), region, bad), native.OutcomeRejected, false)
	}
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(domain, map[string]any{"CustomDomainName": nil})})
	_ = requireFailure(t, err, native.OutcomeRejected, false)
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "bad", Desired: domain, Patch: patchOf(t, "/CustomDomainCertificateArn", certificates[1])})
	_ = requireFailure(t, err, native.OutcomeRejected, false)
}

func Test_CustomDomain_Create(t *testing.T) {
	api := newAPI()
	h := NewCustomDomainAssociation(api)
	createdDomain(t, h, domain)
	properties, err := h.Read(t.Context(), region, domainID)
	require.NoError(t, err)
	require.Equal(t, "2027-11-20T00:00:00Z", properties["CustomDomainCertificateExpiryTime"])

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: domain})
	require.Contains(t, requireFailure(t, err, native.OutcomeUnknown, false), "an earlier attempt of this deploy may have done", "the association may be the one an earlier attempt made")

	other := with(domain, map[string]any{"WorkgroupName": "reports"})
	api.held["reports"] = true
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: other})
	require.Contains(t, requireFailure(t, err, native.OutcomeRejected, true), "The workgroup reports is busy")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(domain, map[string]any{"CustomDomainName": "DB.example.com"})})
	_ = requireFailure(t, err, native.OutcomeRejected, false)
	require.Equal(t, 3, api.calls["CreateCustomDomainAssociation"], "a domain AWS wouldn't take as an identifier is never sent")
}

func Test_CustomDomain_Update(t *testing.T) {
	api := newAPI()
	h := NewCustomDomainAssociation(api)
	createdDomain(t, h, domain)

	require.NoError(t, update(t, h, domainFile, domainID, domain, domain))
	require.Zero(t, api.calls["UpdateCustomDomainAssociation"], "an unchanged template sends nothing")

	changed := with(domain, map[string]any{"CustomDomainCertificateArn": certificates[1]})
	require.NoError(t, update(t, h, domainFile, domainID, domain, changed))
	require.Equal(t, certificates[1], api.store.Resource(customDomainType, domainID)["CustomDomainCertificateArn"])
	waitDone(t, h, domainID, native.OperationUpdate)

	api.held["analytics"] = true
	err := update(t, h, domainFile, domainID, changed, domain)
	require.Contains(t, requireFailure(t, err, native.OutcomeRejected, true), "The workgroup analytics is busy")
	delete(api.held, "analytics")

	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "analytics|gone.example.com", Desired: domain, Patch: patchOf(t, "/CustomDomainCertificateArn", certificates[1])})
	require.ErrorIs(t, err, native.ErrNotFound)

	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: domainID, Desired: with(domain, map[string]any{"CustomDomainCertificateArn": 5})})
	_ = requireFailure(t, err, native.OutcomeRejected, false)
}

func Test_CustomDomain_Delete(t *testing.T) {
	api := newAPI()
	h := NewCustomDomainAssociation(api)
	require.ErrorIs(t, h.Delete(t.Context(), region, domainID), native.ErrNotFound)

	createdDomain(t, h, domain)
	require.NoError(t, h.Delete(t.Context(), region, domainID))
	for i := range busyReads {
		progress, err := h.Progress(t.Context(), region, domainID, native.OperationDelete)
		require.NoError(t, err, "read %d", i)
		require.False(t, progress.Done, "the workgroup is still changing")
	}
	_, err := h.Progress(t.Context(), region, domainID, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound, "the delete is done once the association is gone and the workgroup is free")

	createdDomain(t, h, domain)
	api.held["analytics"] = true
	require.Contains(t, requireFailure(t, h.Delete(t.Context(), region, domainID), native.OutcomeRejected, true), "The workgroup analytics is busy")
	progress, err := h.Progress(t.Context(), region, domainID, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "associated", Waiting: "deleted"}, progress)
}

func Test_CustomDomain_Progress(t *testing.T) {
	api := newAPI()
	h := NewCustomDomainAssociation(api)
	createdDomain(t, h, domain)

	_, err := h.Progress(t.Context(), region, "analytics|gone.example.com", native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound, "a create whose association is gone fails")

	api.errs["GetWorkgroup"] = fault("ThrottlingException", "slow down")
	_, err = h.Progress(t.Context(), region, domainID, native.OperationCreate)
	require.ErrorContains(t, err, "ThrottlingException")
	delete(api.errs, "GetWorkgroup")

	api.store.Remove(workgroupType, "analytics")
	progress, err := h.Progress(t.Context(), region, domainID, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, progress.Done, "a workgroup AWS no longer has is not busy")
}

func Test_CustomDomain_List(t *testing.T) {
	api := newAPI()
	for i := range 120 {
		d := "db" + strconv.Itoa(i) + ".example.com"
		api.store.Put(customDomainType, "analytics|"+d, map[string]any{"WorkgroupName": "analytics", "CustomDomainName": d, "CustomDomainCertificateArn": testCertificate})
	}
	h := NewCustomDomainAssociation(api)
	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, resources, 120)
	require.Equal(t, "analytics|db0.example.com", resources[0].Identifier)

	api.errs["ListCustomDomainAssociations"] = fault("ThrottlingException", "slow down")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "ThrottlingException")
}

// Test_CustomDomain_UpdateRetried checks that an update whose retry AWS rejects because the first attempt left the
// workgroup MODIFYING is not reported as rejected, and that a conflict for another certificate still is.
func Test_CustomDomain_UpdateRetried(t *testing.T) {
	api := newAPI()
	createdDomain(t, NewCustomDomainAssociation(api), domain)
	h := NewCustomDomainAssociation(lostUpdate{api})
	changed := with(domain, map[string]any{"CustomDomainCertificateArn": certificates[1]})
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: domainID, Desired: changed, Patch: patchOf(t, "/CustomDomainCertificateArn", certificates[1])}))
	require.Equal(t, certificates[1], api.store.Resource(customDomainType, domainID)["CustomDomainCertificateArn"])

	api.held["analytics"] = true
	err := h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: domainID, Desired: domain, Patch: patchOf(t, "/CustomDomainCertificateArn", testCertificate)})
	_ = requireFailure(t, err, native.OutcomeRejected, true)
}
