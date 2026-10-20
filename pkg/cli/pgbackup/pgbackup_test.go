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

package pgbackup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeDumps(t *testing.T, dir string, dbs ...string) {
	t.Helper()
	for _, db := range dbs {
		require.NoError(t, os.WriteFile(filepath.Join(dir, db+".sql"), []byte("-- dump"), 0o644))
	}
}

func Test_HasBackup_AllDumpsPresent(t *testing.T) {
	dir := t.TempDir()
	writeDumps(t, dir, Databases...)

	require.True(t, HasBackup(dir), "HasBackup should be true when every database dump exists")
}

func Test_HasBackup_EmptyDirectory(t *testing.T) {
	require.False(t, HasBackup(t.TempDir()), "an empty directory is not a backup")
}

func Test_HasBackup_PartialDumpsAreNotABackup(t *testing.T) {
	dir := t.TempDir()
	// Only the first database has a dump; the others are missing.
	writeDumps(t, dir, Databases[0])

	require.False(t, HasBackup(dir), "a partial set of dumps must not be treated as a complete backup")
}

func Test_HasBackup_MissingDirectory(t *testing.T) {
	require.False(t, HasBackup(filepath.Join(t.TempDir(), "does-not-exist")))
}

func Test_IsControlPlaneEmpty_NoDataRows(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"),
		[]byte("COPY public.resources (id, resource_type) FROM stdin;\n\\.\n"), 0o644))

	empty, err := IsControlPlaneEmpty(dir)
	require.NoError(t, err)
	require.True(t, empty, "a COPY block with no data rows is an empty control plane")
}

func Test_IsControlPlaneEmpty_NoResourcesTableInDump(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte("-- empty dump, no tables\n"), 0o644))

	empty, err := IsControlPlaneEmpty(dir)
	require.NoError(t, err)
	require.True(t, empty, "a dump with no resources table at all is at least as degenerate as an empty one")
}

func Test_IsControlPlaneEmpty_HasDataRows(t *testing.T) {
	dir := t.TempDir()
	dump := "COPY public.resources (id, resource_type) FROM stdin;\n" +
		"/planes/radius/local/resourcegroups/default\t/system.resources/resourcegroups/\n" +
		"\\.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte(dump), 0o644))

	empty, err := IsControlPlaneEmpty(dir)
	require.NoError(t, err)
	require.False(t, empty, "a COPY block with at least one data row is not an empty control plane")
}

func Test_IsControlPlaneEmpty_SchemaQualifiedTableNames(t *testing.T) {
	headers := []string{
		"COPY myschema.resources (id, resource_type) FROM stdin;",
		`COPY "public"."resources" (id, resource_type) FROM stdin;`,
		`COPY "resources" (id, resource_type) FROM stdin;`,
	}
	for _, header := range headers {
		t.Run(header, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte(header+"\n\\.\n"), 0o644))
			empty, err := IsControlPlaneEmpty(dir)
			require.NoError(t, err)
			require.True(t, empty, "no data rows")

			dump := header + "\n/planes/radius/local/resourcegroups/default\tresourcegroups\n\\.\n"
			require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte(dump), 0o644))
			empty, err = IsControlPlaneEmpty(dir)
			require.NoError(t, err)
			require.False(t, empty, "one data row")
		})
	}
}

func Test_IsControlPlaneEmpty_IgnoresSimilarlyNamedTables(t *testing.T) {
	dir := t.TempDir()
	dump := "COPY public.resources_archive (id) FROM stdin;\n" +
		"/planes/radius/local/resourcegroups/old\n" +
		"\\.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte(dump), 0o644))

	empty, err := IsControlPlaneEmpty(dir)
	require.NoError(t, err)
	require.True(t, empty, "rows in resources_archive must not count as rows in resources")
}

func Test_IsControlPlaneEmpty_LongDataRow(t *testing.T) {
	dir := t.TempDir()
	// Longer than bufio.Scanner's 64KB default token size.
	longRow := "/planes/radius/local/resourcegroups/default\t" + strings.Repeat("x", 200*1024)
	dump := "COPY public.resources (id, properties) FROM stdin;\n" + longRow + "\n\\.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte(dump), 0o644))

	empty, err := IsControlPlaneEmpty(dir)
	require.NoError(t, err)
	require.False(t, empty)
}

// resourcesHeader is the COPY header pg_dump --format=plain writes for the resources table
// (deploy/Chart/templates/database/configmap-initdb.yaml).
const resourcesHeader = "COPY public.resources (id, original_id, resource_type, root_scope, routing_scope, etag, created_at, resource_data) FROM stdin;\n"

// row builds one COPY data row for the resources table with the given normalized resource_type.
func row(id, originalID, resourceType, routingScope string) string {
	return id + "\t" + originalID + "\t" + resourceType + "\t/planes/radius/local/\t" + routingScope + "\tetag\t2026-09-30 10:00:00.000000+00\t{}\n"
}

// seedOnlyDump is what a reset control plane dumps: UCP re-registers its resource provider metadata
// on every boot and creates its planes from configuration, so the resources table is never empty.
var seedOnlyDump = resourcesHeader +
	row("/planes/radius/local/", "/planes/radius/local", "/system.radius/planes/", "/system.radius/planes/local/") +
	row("/planes/radius/local/providers/system.resources/resourceproviders/applications.core/", "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Core", "/system.resources/resourceproviders/", "/system.resources/resourceproviders/applications.core/") +
	row("/planes/radius/local/providers/system.resources/resourceproviders/applications.core/resourcetypes/environments/", "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Core/resourceTypes/environments", "/system.resources/resourceproviders/resourcetypes/", "/system.resources/resourceproviders/applications.core/resourcetypes/environments/") +
	row("/planes/radius/local/providers/system.resources/resourceproviders/applications.core/resourcetypes/environments/apiversions/2023-10-01-preview/", "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Core/resourceTypes/environments/apiVersions/2023-10-01-preview", "/system.resources/resourceproviders/resourcetypes/apiversions/", "/system.resources/resourceproviders/applications.core/resourcetypes/environments/apiversions/2023-10-01-preview/") +
	row("/planes/radius/local/providers/system.resources/resourceproviders/applications.core/locations/global/", "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Core/locations/global", "/system.resources/resourceproviders/locations/", "/system.resources/resourceproviders/applications.core/locations/global/") +
	row("/planes/radius/local/providers/system.resources/resourceprovidersummaries/applications.core/", "/planes/radius/local/providers/System.Resources/resourceProviderSummaries/Applications.Core", "/system.resources/resourceprovidersummaries/", "/system.resources/resourceprovidersummaries/applications.core/")

var resourceGroupRow = row("/planes/radius/local/resourcegroups/default/", "/planes/radius/local/resourceGroups/default", "/system.resources/resourcegroups/", "/system.resources/resourcegroups/default/")

func Test_IsControlPlaneEmpty_OnlySeedRows(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte(seedOnlyDump+"\\.\n"), 0o644))

	empty, err := IsControlPlaneEmpty(dir)
	require.NoError(t, err)
	require.True(t, empty, "provider metadata and planes are seeded on every boot, so they are not user data")
}

func Test_IsControlPlaneEmpty_SeedRowsAndAResourceGroup(t *testing.T) {
	dir := t.TempDir()
	dump := seedOnlyDump + resourceGroupRow + "\\.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte(dump), 0o644))

	empty, err := IsControlPlaneEmpty(dir)
	require.NoError(t, err)
	require.False(t, empty, "a resource group is user-created data")
}

func Test_IsControlPlaneEmpty_TruncatedBlock(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte(seedOnlyDump), 0o644))

	empty, err := IsControlPlaneEmpty(dir)
	require.NoError(t, err)
	require.True(t, empty, "a resources block that ends before its terminator has no user data to protect")
}

func Test_IsControlPlaneEmpty_ChecksEveryResourcesTable(t *testing.T) {
	dir := t.TempDir()
	// A seed-only resources table in another schema must not decide the answer for public.resources.
	other := "COPY other.resources (id, original_id, resource_type, root_scope, routing_scope, etag, created_at, resource_data) FROM stdin;\n" +
		row("/planes/radius/local/", "/planes/radius/local", "/system.radius/planes/", "/system.radius/planes/local/") + "\\.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte(other+seedOnlyDump+resourceGroupRow+"\\.\n"), 0o644))

	empty, err := IsControlPlaneEmpty(dir)
	require.NoError(t, err)
	require.False(t, empty, "user data in any resources table means the snapshot is not empty")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte(other+seedOnlyDump+"\\.\n"), 0o644))
	empty, err = IsControlPlaneEmpty(dir)
	require.NoError(t, err)
	require.True(t, empty, "seed rows in every resources table is still empty")
}

func Test_IsControlPlaneEmpty_NoResourceTypeColumn(t *testing.T) {
	dir := t.TempDir()
	// Without a resource_type column seed rows cannot be told apart, so any row counts as user data
	// rather than refusing to persist a real snapshot.
	dump := "COPY public.resources (id, original_id) FROM stdin;\n/planes/radius/local/\t/planes/radius/local\n\\.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ucp.sql"), []byte(dump), 0o644))

	empty, err := IsControlPlaneEmpty(dir)
	require.NoError(t, err)
	require.False(t, empty)
}

func Test_IsControlPlaneEmpty_MissingFile(t *testing.T) {
	_, err := IsControlPlaneEmpty(t.TempDir())
	require.ErrorContains(t, err, "failed to read backup file")
}

func Test_StateBranchName_DefaultsWhenUnset(t *testing.T) {
	// t.Setenv unsets after the test; explicitly clear to isolate from the ambient environment.
	t.Setenv(StateBranchEnvVar, "")
	require.NoError(t, os.Unsetenv(StateBranchEnvVar))

	require.Equal(t, DefaultStateBranch, StateBranchName(), "an unset override must fall back to the default branch")
}

func Test_StateBranchName_HonorsOverride(t *testing.T) {
	t.Setenv(StateBranchEnvVar, "radius-state-pr-42")
	require.Equal(t, "radius-state-pr-42", StateBranchName(), "%s must override the default branch", StateBranchEnvVar)
}

func Test_StateArchiveName_HonorsArchiveOverride(t *testing.T) {
	t.Setenv(StateArchiveEnvVar, "radius-state-pr-42")
	require.Equal(t, "radius-state-pr-42", StateArchiveName())
}

func Test_StateArchiveName_PrefersArchiveOverride(t *testing.T) {
	t.Setenv(StateArchiveEnvVar, "radius-state-archive")
	t.Setenv(StateBranchEnvVar, "radius-state-branch")
	require.Equal(t, "radius-state-archive", StateArchiveName())
}
