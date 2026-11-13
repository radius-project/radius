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

package docdb

import (
	"maps"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

func schema(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/" + file)
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

// addClusters adds available DocumentDB clusters to store, for instances to be created in.
func addClusters(store *awsfake.CloudControl, ids ...string) {
	for _, id := range ids {
		store.Put(clusterType, id, map[string]any{"DBClusterIdentifier": id, "Engine": Engine})
	}
}

func Test_Suite_Cluster(t *testing.T) {
	var fs fakes
	cluster := map[string]any{
		"DBClusterIdentifier":   "app-docdb",
		"MasterUsername":        "radius",
		"MasterUserPassword":    "pass-one-1234",
		"EngineVersion":         "5.0",
		"BackupRetentionPeriod": 1,
		"DeletionProtection":    false,
		"Tags":                  tags("owner", "", "team", "a"),
	}
	nativetest.Run(t, nativetest.Suite{
		TypeName: clusterType,
		Schema:   schema(t, "aws-docdb-dbcluster.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewCluster(api), api)
		},
		IdentifierProperty: "DBClusterIdentifier",
		WriteOnly:          []string{"MasterUserPassword"},
		Properties:         cluster,
		Identifier:         "app-docdb",
		Changed:            with(cluster, map[string]any{"BackupRetentionPeriod": 7}),
		ChangedWriteOnly:   with(cluster, map[string]any{"MasterUserPassword": "pass-two-5678"}),
		Removable:          "Tags",
		CreateOnlyChange:   with(cluster, map[string]any{"MasterUsername": "other"}),
		OtherForm:          map[string]any{"EngineVersion": "5.0.0", "DBClusterIdentifier": "APP-DOCDB"},
		OtherProduct: func(store *awsfake.CloudControl, identifier string) {
			store.Put(clusterType, identifier, map[string]any{"DBClusterIdentifier": identifier, "Engine": "aurora-postgresql"})
		},
		MultiCall: with(cluster, map[string]any{"BackupRetentionPeriod": 3, "Tags": tags("env", "dev", "team", "b")}),
		Calls:     2,
		FailCall: nativetest.FailCall(2, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
		API: clusterAPI,
	})
}

func Test_Suite_Instance(t *testing.T) {
	var fs fakes
	instance := map[string]any{
		"DBInstanceIdentifier":    "app-docdb-1",
		"DBClusterIdentifier":     "app-docdb",
		"DBInstanceClass":         "db.t3.medium",
		"AutoMinorVersionUpgrade": true,
		"Tags":                    tags("owner", "", "team", "a"),
	}
	nativetest.Run(t, nativetest.Suite{
		TypeName: instanceType,
		Schema:   schema(t, "aws-docdb-dbinstance.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			addClusters(store, "app-docdb", "other-cluster")
			api := newFakeAPI(store)
			return fs.add(NewInstance(api), api)
		},
		IdentifierProperty: "DBInstanceIdentifier",
		Properties:         instance,
		Identifier:         "app-docdb-1",
		Changed:            with(instance, map[string]any{"DBInstanceClass": "db.r6g.large"}),
		Removable:          "Tags",
		CreateOnlyChange:   with(instance, map[string]any{"DBClusterIdentifier": "other-cluster"}),
		OtherForm:          map[string]any{"DBInstanceIdentifier": "App-DocDB-1"},
		OtherProduct: func(store *awsfake.CloudControl, identifier string) {
			store.Put(instanceType, identifier, map[string]any{"DBInstanceIdentifier": identifier, "Engine": "neptune"})
		},
		MultiCall: with(instance, map[string]any{"DBInstanceClass": "db.r6g.large", "Tags": tags("env", "dev", "team", "b")}),
		Calls:     2,
		FailCall: nativetest.FailCall(2, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
		API: instanceAPI,
	})
}

func Test_DeleteToken(t *testing.T) {
	h := NewCluster(newFakeAPI(awsfake.NewCloudControl()))
	h.now = func() time.Time { return time.Date(2026, 11, 4, 21, 27, 51, 0, time.FixedZone("PST", -8*3600)) }
	token, err := h.DeleteToken(t.Context(), "us-west-2", "app-docdb")
	require.NoError(t, err)
	require.Equal(t, "app-docdb-final-20261105-052751", token, "the snapshot is named in UTC")
}

func Test_Equivalent(t *testing.T) {
	tests := []struct {
		path     string
		template any
		aws      any
		want     bool
	}{
		{"EngineVersion", "5.0", "5.0.0", true},
		{"EngineVersion", "5.0", "5.0.1", false},
		{"EngineVersion", "5", "5.0.0", true},
		{"EngineVersion", "4.0", "5.0.0", false},
		{"DBSubnetGroupName", "App-Subnets", "app-subnets", true},
		{"MasterUsername", "Radius", "radius", false},
		{"Port", 27017, 27017, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, equivalent(tt.path, tt.template, tt.aws), "%s %v %v", tt.path, tt.template, tt.aws)
	}
}
