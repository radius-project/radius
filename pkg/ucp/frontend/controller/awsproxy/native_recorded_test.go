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

package awsproxy

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// recordReader is nativeType's fake handler as a native.RecordReader, like DAX: while busy, AWS doesn't return Tags,
// so a read takes them from recorded. It logs what each read got.
type recordReader struct {
	*nativefake.Handler
	got  *[]map[string]any
	busy *bool
}

func (r recordReader) ReadRecorded(ctx context.Context, region string, identifier string, recorded map[string]any) (map[string]any, error) {
	*r.got = append(*r.got, recorded)
	properties, err := r.Read(ctx, region, identifier)
	if err != nil || !*r.busy {
		return properties, err
	}
	delete(properties, "Tags")
	if tags, ok := recorded["Tags"]; ok {
		properties["Tags"] = tags
	}
	return properties, nil
}

// withRecordReader registers nativeType's handler as a recordReader and returns its log and busy flag.
func (h *nativeHarness) withRecordReader() (*[]map[string]any, *bool) {
	h.t.Helper()
	got, busy := &[]map[string]any{}, new(bool)
	catalog := schemas.Catalog{Native: map[string]string{nativeType: nativeSchemas[nativeType]}}
	registry, err := native.NewRegistry(catalog, recordReader{Handler: h.fake, got: got, busy: busy})
	require.NoError(h.t, err)
	h.native = registry
	return got, busy
}

// Test_Native_RecordReader checks the properties UCP gives a handler that can't read some properties in some states:
// the record's last-applied ones when they are confirmed, and nil otherwise, so the handler never takes an unconfirmed
// value as AWS's.
func Test_Native_RecordReader(t *testing.T) {
	tagged := nativeProperties(1)
	tagged["Tags"] = map[string]any{"team": "a"}

	t.Run("an unchanged redeploy while AWS can't read the tags only reads", func(t *testing.T) {
		h := newNativeHarness(t)
		got, busy := h.withRecordReader()
		h.deploy(nativePath, tagged)
		*busy = true
		h.fake.Calls()

		resp := h.put(nativePath, tagged)
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, []string{"Read"}, methods(h.fake.Calls()))
		require.Equal(t, h.record(nativePath).Properties.LastApplied, (*got)[len(*got)-1])
		require.Equal(t, map[string]any{"team": "a"}, (*got)[len(*got)-1]["Tags"])

		resp = h.get(nativePath)
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, map[string]any{"team": "a"}, (*got)[len(*got)-1]["Tags"], "a GET by alias gets the record's values too")
	})

	unconfirmed := map[string]func(rec *datamodel.AWSResource){
		"unresolved paths": func(rec *datamodel.AWSResource) { rec.Properties.UnresolvedPaths = []string{"Size"} },
		"pending values": func(rec *datamodel.AWSResource) {
			rec.Properties.PendingLastApplied = map[string]any{"Name": "app-native"}
		},
	}
	for name, change := range unconfirmed {
		t.Run("a record with "+name+" gives nil", func(t *testing.T) {
			h := newNativeHarness(t)
			got, busy := h.withRecordReader()
			h.deploy(nativePath, tagged)
			*busy = true
			h.saveRecord(nativePath, change)
			h.fake.Calls()

			resp := h.get(nativePath)
			require.Equal(t, http.StatusOK, resp.status, resp.raw)
			require.Nil(t, (*got)[len(*got)-1])
		})
	}

	t.Run("existing gives nil", func(t *testing.T) {
		h := newNativeHarness(t)
		got, _ := h.withRecordReader()
		h.deploy(nativePath, tagged)

		resp := h.getWithPost(map[string]any{"name": "app-native"})
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Nil(t, (*got)[len(*got)-1])
	})
}
