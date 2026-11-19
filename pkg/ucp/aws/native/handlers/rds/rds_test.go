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
	"maps"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region     = "us-west-2"
	clusterArn = arnPrefix + "cluster:app-aurora"
	keyID      = "1234abcd-12ab-34cd-56ef-1234567890ab"
	keyArn     = "arn:aws:kms:us-west-2:123456789012:key/" + keyID
)

func schema(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// fakes maps each handler the suite makes to its fake API.
type fakes struct {
	mu  sync.Mutex
	api map[native.Handler]*fakeAPI
}

func (f *fakes) add(h native.Handler, api *fakeAPI) native.Handler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.api == nil {
		f.api = map[native.Handler]*fakeAPI{}
	}
	f.api[h] = api
	return h
}

func (f *fakes) of(h native.Handler) *fakeAPI {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.api[h]
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

func tags(pairs ...string) []any {
	out := []any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"Key": pairs[i], "Value": pairs[i+1]})
	}
	return out
}

var stream = map[string]any{
	"ResourceArn": clusterArn,
	"Mode":        "async",
	"KmsKeyId":    keyID,
}

func Test_Suite_ActivityStream(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    activityStreamType,
		Schema:      schema(t, "aws-rds-dbclusteractivitystream.json"),
		HandWritten: true,
		Metadata:    schema(t, "aws-rds-dbclusteractivitystream.metadata.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			addCluster(store, "app-aurora")
			return NewActivityStream(newFakeAPI(store))
		},
		IdentifierProperty: "ResourceArn",
		Properties:         stream,
		Identifier:         clusterArn,
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          stream,
		CreateOnlyChange: with(stream, map[string]any{"Mode": "sync"}),
		OtherForm:        map[string]any{"KmsKeyId": keyArn},
		RecordedForm:     &nativetest.RecordedForm{Property: "KmsKeyId", Template: "alias/app-das", Other: "alias/other", Stored: keyArn},
		SameResource:     []map[string]any{with(stream, map[string]any{"ResourceArn": arnPrefix + "cluster:APP-AURORA"})},
		Invalid: []map[string]any{
			with(stream, map[string]any{"ResourceArn": arnPrefix + "db:app-oracle"}),
			with(stream, map[string]any{"ResourceArn": "app-aurora"}),
		},
		API: activityStreamAPI,
	})
}

var tenant = map[string]any{
	"DBInstanceIdentifier": "app-cdb",
	"TenantDBName":         "pdb1",
	"MasterUsername":       "admin",
	"MasterUserPassword":   "pass-one-1234",
	"Tags":                 tags("owner", "", "team", "a"),
}

func Test_Suite_TenantDatabase(t *testing.T) {
	var fs fakes
	nativetest.Run(t, nativetest.Suite{
		TypeName:    tenantDatabaseType,
		Schema:      schema(t, "aws-rds-tenantdatabase.json"),
		HandWritten: true,
		Metadata:    schema(t, "aws-rds-tenantdatabase.metadata.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			addInstance(store, "app-cdb")
			addInstance(store, "other-cdb")
			api := newFakeAPI(store)
			return fs.add(newTenantHandler(api), api)
		},
		IdentifierProperty: "TenantDBName",
		IdentifierProperties: func(identifier string) map[string]any {
			instance, name, _ := strings.Cut(identifier, "|")
			return map[string]any{"DBInstanceIdentifier": instance, "TenantDBName": name}
		},
		WriteOnly:        []string{"MasterUserPassword", "MasterUserSecretKmsKeyId"},
		Properties:       tenant,
		Identifier:       "app-cdb|pdb1",
		Changed:          with(tenant, map[string]any{"Tags": tags("team", "b")}),
		ChangedWriteOnly: with(tenant, map[string]any{"MasterUserPassword": "pass-two-5678"}),
		Removable:        "Tags",
		CreateOnlyChange: with(tenant, map[string]any{"MasterUsername": "other"}),
		OtherForm:        map[string]any{"DBInstanceIdentifier": "APP-CDB", "TenantDBName": "PDB1"},
		SameResource:     []map[string]any{with(tenant, map[string]any{"DBInstanceIdentifier": "App-CDB", "TenantDBName": "PDB1"})},
		MultiCall:        with(tenant, map[string]any{"MasterUserPassword": "pass-three-9", "Tags": tags("env", "dev", "team", "b")}),
		Calls:            3,
		FailCall: nativetest.FailCall(3, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
		API: tenantDatabaseAPI,
	})
}

// newTenantHandler returns the handler for tenant databases on api, with a clock that moves only when it waits for a
// busy instance, so a wait returns at once and still ends at the handler's limit.
func newTenantHandler(api API) *TenantDatabase {
	h := NewTenantDatabase(api)
	var mu sync.Mutex
	clock := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	h.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return clock
	}
	h.sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		clock = clock.Add(d)
		return ctx.Err()
	}
	return h
}
