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
	"bytes"
	"encoding/json"
	"maps"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	db2Path     = providers + "AWS.Test/Database/other-db"
	newPassword = "n3w-p@ssw0rd-never-stored"
)

// keyOf returns a key source for a key made of b.
func keyOf(b byte) FingerprintKeyFunc {
	return func() (*ucp_aws.FingerprintKey, error) {
		return ucp_aws.NewFingerprintKey(bytes.Repeat([]byte{b}, ucp_aws.MinFingerprintKeyLength))
	}
}

// keyFile returns a key source that loads the key from path, like aws.Module does when UCP starts.
func keyFile(path string) FingerprintKeyFunc {
	return func() (*ucp_aws.FingerprintKey, error) {
		return ucp_aws.LoadFingerprintKey(path)
	}
}

func keyID(t *testing.T, key FingerprintKeyFunc) string {
	k, err := key()
	require.NoError(t, err)
	return k.ID()
}

// patchOps returns the operations of a JSON patch as "op path".
func patchOps(t *testing.T, document string) []string {
	t.Helper()
	patch := jsondiff.Patch{}
	require.NoError(t, json.Unmarshal([]byte(document), &patch))
	out := []string{}
	for _, op := range patch {
		out = append(out, op.Type+" "+op.Path)
	}
	slices.Sort(out)
	return out
}

// rawRecord returns the record of path as it is stored.
func (h *harness) rawRecord(path string) string {
	h.t.Helper()
	obj, err := h.db.Get(h.t.Context(), path)
	require.NoError(h.t, err)
	raw, err := json.Marshal(obj.Data)
	require.NoError(h.t, err)
	return string(raw)
}

// restart returns a harness for a UCP that restarted with key: it shares the data store, queue, and AWS.
func (h *harness) restart(key FingerprintKeyFunc) *harness {
	restarted := *h
	restarted.key = key
	return &restarted
}

// restore returns a harness for a new UCP with key, whose data store has the record of path, changed by change. AWS
// is the same.
func (h *harness) restore(path string, key FingerprintKeyFunc, change func(rec *datamodel.AWSResource)) *harness {
	h.t.Helper()
	restored := newHarness(h.t)
	restored.cc = h.cc
	restored.key = key
	rec := h.record(path)
	change(rec)
	_, err := records.NewStore(restored.db).Save(h.t.Context(), rec, "")
	require.NoError(h.t, err)
	return restored
}

func dbProperties(pw string) map[string]any {
	return map[string]any{
		"Name":     "db",
		"Password": pw,
		"Auth":     map[string]any{"Token": pw, "Mode": "token"},
		"Users":    []any{map[string]any{"Name": "u", "Secret": pw}},
	}
}

// Test_WriteOnly_UnchangedValueIsNotSent redeploys write-only values at the top level, inside an object, and inside
// array items, and checks the AWS calls: an unchanged value sends no update, and a changed one sends only itself.
func Test_WriteOnly_UnchangedValueIsNotSent(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusCreated, h.put(dbPath, dbProperties(password)).status)
	h.finish()

	rec := h.record(dbPath)
	require.Equal(t, keyID(t, testFingerprintKey), rec.Properties.Fingerprints.KeyID)
	require.ElementsMatch(t, []string{"Password", "Auth/Token", "Users"}, slices.Collect(maps.Keys(rec.Properties.Fingerprints.Digests)))
	require.Nil(t, rec.Properties.PendingFingerprints)
	h.requireNotStored()
	h.cc.Calls()

	resp := h.put(dbPath, dbProperties(password))
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))

	changes := []struct {
		name       string
		properties map[string]any
		want       []string
	}{
		{"top-level", map[string]any{"Password": newPassword}, []string{"add /Password"}},
		{"inside an object", map[string]any{"Auth": map[string]any{"Token": newPassword, "Mode": "token"}}, []string{"add /Auth/Token"}},
		{"inside array items", map[string]any{"Users": []any{map[string]any{"Name": "u", "Secret": newPassword}}}, []string{"add /Users/0/Secret"}},
	}
	properties := dbProperties(password)
	for _, change := range changes {
		for k, v := range change.properties {
			properties[k] = v
		}
		resp := h.put(dbPath, properties)
		require.Equal(t, http.StatusCreated, resp.status, change.name+": "+resp.raw)
		calls := h.cc.Calls()
		require.Equal(t, []string{"GetResource", "UpdateResource"}, operations(calls), change.name)
		require.Equal(t, change.want, patchOps(t, calls[1].Document), change.name)
		require.Contains(t, calls[1].Document, newPassword, change.name)
		h.finish()
		require.NotContains(t, h.rawRecord(dbPath), newPassword)

		// The fingerprint of the new value is saved, so deploying it again sends nothing.
		resp = h.put(dbPath, properties)
		require.Equal(t, http.StatusOK, resp.status, change.name+": "+resp.raw)
		require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()), change.name)
	}
	require.Equal(t, newPassword, h.cc.Resource(dbType, "db")["Password"])
}

// Test_WriteOnly_EncodingIsStable sends the same write-only value with its keys in another order and its numbers in
// another form. It is unchanged.
func Test_WriteOnly_EncodingIsStable(t *testing.T) {
	h := newHarness(t)
	resp := h.putBody(dbPath, testOwner, map[string]any{"properties": json.RawMessage(`{"Name": "db", "Auth": {"Token": {"b": 1, "a": [2.0, "x"]}}}`)})
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	h.finish()
	h.cc.Calls()

	resp = h.putBody(dbPath, testOwner, map[string]any{"properties": json.RawMessage(`{"Auth": {"Token": {"a": [2, "x"], "b": 1e0}}, "Name": "db"}`)})
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
}

// Test_WriteOnly_RecordedFingerprints checks what the record stores: the key ID apart from the digests, and digests
// bound to the resource and the property path.
func Test_WriteOnly_RecordedFingerprints(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusCreated, h.put(dbPath, dbProperties(password)).status)
	require.Equal(t, http.StatusCreated, h.put(db2Path, map[string]any{"Name": "db2", "Password": password}).status)
	h.finish()

	stored := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(h.rawRecord(dbPath)), &stored))
	fingerprints := stored["properties"].(map[string]any)["fingerprints"].(map[string]any)
	id := keyID(t, testFingerprintKey)
	require.Equal(t, id, fingerprints["keyId"])
	digests := fingerprints["digests"].(map[string]any)
	for path, digest := range digests {
		require.Regexp(t, `^[A-Za-z0-9+/]{43}$`, digest, path)
		require.NotContains(t, digest, id, path)
	}

	// The same password under another property or another resource has a different fingerprint.
	require.NotEqual(t, digests["Password"], digests["Auth/Token"])
	require.NotEqual(t, digests["Password"], h.record(db2Path).Properties.Fingerprints.Digests["Password"])
}

// Test_WriteOnly_CreateOnlyValue checks a write-only value AWS cannot change after create on a type that can be
// updated. Before fingerprints, every redeploy sent it, and AWS rejected the update.
func Test_WriteOnly_CreateOnlyValue(t *testing.T) {
	h := newHarness(t)
	properties := map[string]any{"Name": "c1", "MasterUserPassword": password, "SnapshotIdentifier": "snap-1"}
	require.Equal(t, http.StatusCreated, h.put(clusterPath, properties).status)
	h.finish()
	h.cc.Calls()

	resp := h.put(clusterPath, properties)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))

	// A changed updatable value is sent without the unchanged create-only one.
	properties["MasterUserPassword"] = newPassword
	resp = h.put(clusterPath, properties)
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	calls := h.cc.Calls()
	require.Equal(t, []string{"add /MasterUserPassword"}, patchOps(t, calls[1].Document))
	h.finish()

	// A changed create-only value is sent like any other changed create-only property, and AWS decides.
	properties["SnapshotIdentifier"] = "snap-2"
	resp = h.put(clusterPath, properties)
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.Equal(t, []string{"add /SnapshotIdentifier"}, patchOps(t, h.cc.Calls()[1].Document))
}

// Test_WriteOnly_ImmutableType checks a create-and-delete-only type: an unchanged write-only value succeeds without
// an update, and a changed one fails with 400 without calling AWS.
func Test_WriteOnly_ImmutableType(t *testing.T) {
	h := newHarness(t)
	properties := map[string]any{"Name": "i1", "Size": 1, "Password": password}
	require.Equal(t, http.StatusCreated, h.put(immutablePath, properties).status)
	h.finish()
	h.cc.Calls()

	resp := h.put(immutablePath, properties)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
	before := h.record(immutablePath).Properties.Fingerprints

	properties["Password"] = newPassword
	resp = h.put(immutablePath, properties)
	require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	require.Equal(t, "The AWS resource type AWS::Test::Immutable can only be created and deleted, so these properties of the existing resource cannot change: Password. To change them, delete the resource and deploy again.", resp.message())
	require.NotContains(t, resp.raw, newPassword)
	require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
	rec := h.record(immutablePath)
	require.Equal(t, before, rec.Properties.Fingerprints)
	require.Nil(t, rec.Properties.PendingFingerprints)
}

// Test_WriteOnly_NoUsableFingerprint covers each way a record can lack a usable fingerprint. An updatable value is
// sent once, a create-only value is not sent and does not fail the deploy, and the fingerprints are saved with the
// current key, so the next deploy sends nothing.
func Test_WriteOnly_NoUsableFingerprint(t *testing.T) {
	cluster := map[string]any{"Name": "c1", "MasterUserPassword": password, "SnapshotIdentifier": "snap-1"}
	immutable := map[string]any{"Name": "i1", "Size": 1, "Password": password}

	// Each case deploys path and returns the harness to deploy it again with.
	tests := []struct {
		name string
		// sendsAgain is false when the record's fingerprints are still usable.
		sendsAgain bool
		setup      func(t *testing.T, path string, properties map[string]any) *harness
	}{
		{
			name:       "taken over",
			sendsAgain: true,
			setup: func(t *testing.T, path string, properties map[string]any) *harness {
				h := newHarness(t)
				typeName := map[string]string{clusterPath: clusterType, immutablePath: immutableType}[path]
				h.cc.Put(typeName, properties["Name"].(string), properties)
				return h
			},
		},
		{
			name:       "key replaced",
			sendsAgain: true,
			setup: func(t *testing.T, path string, properties map[string]any) *harness {
				h := deployed(t, path, properties)
				return h.restart(keyOf(7))
			},
		},
		{
			name:       "restored with a different key",
			sendsAgain: true,
			setup: func(t *testing.T, path string, properties map[string]any) *harness {
				return deployed(t, path, properties).restore(path, keyOf(7), func(*datamodel.AWSResource) {})
			},
		},
		{
			name:       "restored from before fingerprints",
			sendsAgain: true,
			setup: func(t *testing.T, path string, properties map[string]any) *harness {
				return deployed(t, path, properties).restore(path, testFingerprintKey, func(rec *datamodel.AWSResource) {
					rec.Properties.Fingerprints = nil
				})
			},
		},
		{
			name: "restored with the same key",
			setup: func(t *testing.T, path string, properties map[string]any) *harness {
				return deployed(t, path, properties).restore(path, testFingerprintKey, func(*datamodel.AWSResource) {})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+" updatable", func(t *testing.T) {
			h := tt.setup(t, clusterPath, cluster)
			h.cc.Calls()
			resp := h.put(clusterPath, cluster)
			calls := h.cc.Calls()
			if !tt.sendsAgain {
				require.Equal(t, http.StatusOK, resp.status, resp.raw)
				require.Equal(t, []string{"GetResource"}, operations(calls))
				return
			}
			require.Equal(t, http.StatusCreated, resp.status, resp.raw)
			require.Equal(t, []string{"add /MasterUserPassword"}, patchOps(t, calls[len(calls)-1].Document))
			h.finish()
			rec := h.record(clusterPath)
			require.Equal(t, keyID(t, h.key), rec.Properties.Fingerprints.KeyID)
			require.ElementsMatch(t, []string{"MasterUserPassword", "SnapshotIdentifier"}, slices.Collect(maps.Keys(rec.Properties.Fingerprints.Digests)))

			resp = h.put(clusterPath, cluster)
			require.Equal(t, http.StatusOK, resp.status, resp.raw)
			require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
		})

		t.Run(tt.name+" create-and-delete-only", func(t *testing.T) {
			h := tt.setup(t, immutablePath, immutable)
			h.cc.Calls()
			resp := h.put(immutablePath, immutable)
			require.Equal(t, http.StatusOK, resp.status, resp.raw)
			require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
			rec := h.record(immutablePath)
			require.Equal(t, keyID(t, h.key), rec.Properties.Fingerprints.KeyID)
			require.Contains(t, rec.Properties.Fingerprints.Digests, "Password")

			// The saved fingerprint now catches a change.
			changed := map[string]any{"Name": "i1", "Size": 1, "Password": newPassword}
			require.Equal(t, http.StatusBadRequest, h.put(immutablePath, changed).status)
			require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
		})
	}
}

// deployed returns a harness with path deployed with properties.
func deployed(t *testing.T, path string, properties map[string]any) *harness {
	h := newHarness(t)
	resp := h.put(path, properties)
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	h.finish()
	return h
}

// Test_WriteOnly_UCPRestarts checks that fingerprints still work when UCP restarts and loads the same key again,
// including while an update is in progress.
func Test_WriteOnly_UCPRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte{5}, ucp_aws.MinFingerprintKeyLength), 0o600))

	h := newHarness(t)
	h.key = keyFile(path)
	require.Equal(t, http.StatusCreated, h.put(dbPath, dbProperties(password)).status)
	h.finish()
	h.cc.Calls()

	h = h.restart(keyFile(path))
	resp := h.put(dbPath, dbProperties(password))
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))

	// UCP restarts while an update is in progress. The worker of the new UCP finishes it and saves the fingerprints.
	properties := dbProperties(password)
	properties["Password"] = newPassword
	require.Equal(t, http.StatusCreated, h.put(dbPath, properties).status)
	h = h.restart(keyFile(path))
	h.finish()
	h.cc.Calls()
	resp = h.put(dbPath, properties)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
}

// Test_WriteOnly_PendingFingerprints checks that new fingerprints are saved as pending values before AWS is called,
// promoted when AWS succeeds, and dropped when AWS fails, so the previous ones stay.
func Test_WriteOnly_PendingFingerprints(t *testing.T) {
	setup := func(t *testing.T) (*harness, *datamodel.AWSFingerprints) {
		h := deployed(t, dbPath, dbProperties(password))
		h.cc.Calls()
		return h, h.record(dbPath).Properties.Fingerprints
	}
	changed := dbProperties(password)
	changed["Password"] = newPassword

	t.Run("saved before AWS is called and promoted on success", func(t *testing.T) {
		h, previous := setup(t)
		require.Equal(t, http.StatusCreated, h.put(dbPath, changed).status)
		rec := h.record(dbPath)
		require.Equal(t, previous, rec.Properties.Fingerprints)
		require.NotEqual(t, previous.Digests["Password"], rec.Properties.PendingFingerprints.Digests["Password"])
		pending := rec.Properties.PendingFingerprints

		h.finish()
		rec = h.record(dbPath)
		require.Equal(t, pending, rec.Properties.Fingerprints)
		require.Nil(t, rec.Properties.PendingFingerprints)
	})

	t.Run("AWS reports the update failed", func(t *testing.T) {
		h, previous := setup(t)
		h.cc.FailNext = cctypes.HandlerErrorCodeInvalidRequest
		require.Equal(t, http.StatusCreated, h.put(dbPath, changed).status)
		results := h.finish()
		require.NotNil(t, results[0].Error)
		rec := h.record(dbPath)
		require.Equal(t, previous, rec.Properties.Fingerprints)
		require.Nil(t, rec.Properties.PendingFingerprints)
		h.cc.Calls()

		// AWS did not apply the new password, so the next deploy sends it again.
		resp := h.put(dbPath, changed)
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		require.Equal(t, []string{"add /Password"}, patchOps(t, h.cc.Calls()[1].Document))
	})

	t.Run("AWS rejects the update call", func(t *testing.T) {
		h, previous := setup(t)
		h.cc.Errors["UpdateResource"] = awsError(&smithy.GenericAPIError{Code: "ValidationException", Message: "bad", Fault: smithy.FaultClient})
		require.Equal(t, http.StatusBadRequest, h.put(dbPath, changed).status)
		rec := h.record(dbPath)
		require.Equal(t, previous, rec.Properties.Fingerprints)
		require.Nil(t, rec.Properties.PendingFingerprints)
	})
}

// Test_WriteOnly_FingerprintKeyUnavailable checks a missing, unreadable, or invalid key. A PUT that sets a write-only
// value fails with 500 naming the Secret before UCP saves anything or calls Cloud Control. Other requests still work.
func Test_WriteOnly_FingerprintKeyUnavailable(t *testing.T) {
	dir := t.TempDir()
	short := filepath.Join(dir, "short")
	require.NoError(t, os.WriteFile(short, []byte("too-short"), 0o600))
	unreadable := filepath.Join(dir, "unreadable")
	require.NoError(t, os.Mkdir(unreadable, 0o700))

	keys := []struct {
		name string
		key  FingerprintKeyFunc
		want string
	}{
		{"missing", keyFile(filepath.Join(dir, "missing")), "failed to read the AWS fingerprint key"},
		{"unreadable", keyFile(unreadable), "failed to read the AWS fingerprint key"},
		{"invalid", keyFile(short), "invalid AWS fingerprint key"},
		{"not loaded", func() (*ucp_aws.FingerprintKey, error) { return nil, nil }, "the AWS fingerprint key is not loaded"},
	}
	for _, tt := range keys {
		t.Run(tt.name, func(t *testing.T) {
			requireKeyError := func(t *testing.T, resp response) {
				t.Helper()
				require.Equal(t, http.StatusInternalServerError, resp.status, resp.raw)
				require.Contains(t, resp.message(), `UCP cannot deploy the AWS resource "app-db", because it needs the key in the Kubernetes Secret radius-aws-fingerprint-key to fingerprint these write-only properties: Auth/Token, Password, Users.`)
				require.Contains(t, resp.message(), tt.want)
				require.NotContains(t, resp.raw, password)
			}

			// A first deploy saves no record and calls nothing.
			h := newHarness(t)
			h.key = tt.key
			requireKeyError(t, h.put(dbPath, dbProperties(password)))
			require.Empty(t, h.cc.Calls())
			require.Nil(t, h.record(dbPath))

			// A redeploy changes nothing in the record and calls nothing.
			h = deployed(t, dbPath, dbProperties(password))
			h.cc.Calls()
			before := h.rawRecord(dbPath)
			h.key = tt.key
			requireKeyError(t, h.put(dbPath, dbProperties(password)))
			require.Empty(t, h.cc.Calls())
			require.Equal(t, before, h.rawRecord(dbPath))

			// An operation AWS accepted that UCP never queued is not resumed, because resuming it saves the record.
			h.saveRecord(dbPath, func(rec *datamodel.AWSResource) {
				rec.Properties.RequestToken = testAWSRequestToken
				rec.Properties.PendingOperation = records.OperationUpdate
			})
			before = h.rawRecord(dbPath)
			requireKeyError(t, h.put(dbPath, dbProperties(password)))
			require.Equal(t, before, h.rawRecord(dbPath))
			require.Empty(t, h.finish())
			h.saveRecord(dbPath, func(rec *datamodel.AWSResource) {
				rec.Properties.RequestToken = ""
				records.ClearPending(rec)
			})

			// A PUT that sets no write-only value works, and so do GET and DELETE.
			resp := h.put(dbPath, map[string]any{"Name": "db", "Engine": "x"})
			require.Equal(t, http.StatusCreated, resp.status, resp.raw)
			h.finish()
			require.Nil(t, h.record(dbPath).Properties.Fingerprints)
			require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
			require.Equal(t, http.StatusOK, h.get(dbPath).status)
			require.Equal(t, http.StatusAccepted, h.delete(dbPath, testOwner).status)
		})
	}
}

func Test_NewCreateOrUpdateAWSResource_NeedsAKeySource(t *testing.T) {
	_, err := NewCreateOrUpdateAWSResource(newHarness(t).options(), newHarness(t).clients(), nil)
	require.ErrorContains(t, err, "needs a source for the fingerprint key")
}

func Test_Fingerprints_ValueThatIsNotJSON(t *testing.T) {
	h := newHarness(t)
	id, err := resources.ParseResource(dbPath)
	require.NoError(t, err)
	e := newEngine(h.options(), h.clients(), testFingerprintKey)
	schema := &typeSchema{WriteOnlyProperties: []string{"/properties/Port"}}
	_, resp, err := e.fingerprints(&request{id: id}, schema, map[string]any{"Port": math.NaN()})
	require.Nil(t, resp)
	require.ErrorContains(t, err, "failed to encode the value of Port")
}

func Test_FingerprintPaths(t *testing.T) {
	require.Empty(t, fingerprintPaths(nil))
	require.Equal(t,
		[]string{"Auth", "Auth-Key", "Password", "Users"},
		fingerprintPaths([]string{"Users/*/Secret", "Password", "Auth/Token", "Users/*/Keys/*/Value", "Auth", "Auth-Key"}))
}

func Test_ValueAt(t *testing.T) {
	properties := map[string]any{"A": map[string]any{"B": "x", "N": nil}, "L": []any{"y"}}
	tests := []struct {
		path  string
		want  any
		found bool
	}{
		{"A/B", "x", true},
		{"A/N", nil, true},
		{"A", properties["A"], true},
		{"L", properties["L"], true},
		{"A/C", nil, false},
		{"L/0", nil, false},
		{"Missing/B", nil, false},
	}
	for _, tt := range tests {
		got, ok := valueAt(properties, tt.path)
		require.Equal(t, tt.found, ok, tt.path)
		require.Equal(t, tt.want, got, tt.path)
	}
}

func Test_CompareFingerprints(t *testing.T) {
	current := &datamodel.AWSFingerprints{KeyID: "k1", Digests: map[string]string{"Same": "a", "Changed": "b", "New": "c"}}
	require.Equal(t,
		map[string]fingerprintStatus{"Same": fingerprintUnchanged, "Changed": fingerprintChanged, "New": fingerprintChanged},
		compareFingerprints(&datamodel.AWSFingerprints{KeyID: "k1", Digests: map[string]string{"Same": "a", "Changed": "x", "Removed": "r"}}, current))

	unknown := map[string]fingerprintStatus{"Same": fingerprintUnknown, "Changed": fingerprintUnknown, "New": fingerprintUnknown}
	require.Equal(t, unknown, compareFingerprints(&datamodel.AWSFingerprints{KeyID: "k0", Digests: map[string]string{"Same": "a"}}, current))
	require.Equal(t, unknown, compareFingerprints(nil, current))
	require.Empty(t, compareFingerprints(&datamodel.AWSFingerprints{KeyID: "k1"}, nil))
}

func Test_IsUnsentWriteOnly(t *testing.T) {
	schema := &typeSchema{
		CreateOnlyProperties: []string{"/properties/Snapshot", "/properties/Settings"},
		WriteOnlyProperties:  []string{"/properties/Password", "/properties/Snapshot", "/properties/Auth/Token", "/properties/Settings/Key", "/properties/Users/*/Secret"},
	}
	status := map[string]fingerprintStatus{
		"Password":     fingerprintChanged,
		"Snapshot":     fingerprintChanged,
		"Auth/Token":   fingerprintUnchanged,
		"Settings/Key": fingerprintUnknown,
	}
	add := func(path string, value any) jsondiff.Operation {
		return jsondiff.Operation{Type: jsondiff.OperationAdd, Path: path, Value: value}
	}
	tests := []struct {
		name      string
		op        jsondiff.Operation
		immutable bool
		want      bool
	}{
		{"unchanged", add("/Auth/Token", "x"), false, true},
		{"unchanged, in an object AWS omits", add("/Auth", map[string]any{"Token": "x"}), false, true},
		{"unchanged, next to a readable property", add("/Auth", map[string]any{"Token": "x", "Mode": "m"}), false, false},
		{"changed", add("/Password", "x"), false, false},
		{"changed create-only", add("/Snapshot", "x"), false, false},
		{"unknown create-only", add("/Settings/Key", "x"), false, true},
		{"unknown updatable", add("/Users/0/Secret", "x"), false, false},
		{"unknown on a create-and-delete-only type", add("/Users/0/Secret", "x"), true, true},
		{"readable", jsondiff.Operation{Type: jsondiff.OperationReplace, Path: "/Size", Value: 2}, false, false},
		{"remove", jsondiff.Operation{Type: jsondiff.OperationRemove, Path: "/Auth/Token"}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, schema.isUnsentWriteOnly(tt.op, tt.immutable, status))
		})
	}
}

func Test_IsCreateOnly(t *testing.T) {
	schema := &typeSchema{CreateOnlyProperties: []string{"/properties/Snapshot", "/properties/Settings", "/properties/Users/*/Name", "/properties/Deep/A/B"}}
	require.True(t, schema.isCreateOnly("Snapshot"))
	require.True(t, schema.isCreateOnly("Settings/Key"))
	require.True(t, schema.isCreateOnly("Users/*/Name/Secret"))
	require.True(t, schema.isCreateOnly("Deep/A"))
	require.False(t, schema.isCreateOnly("Password"))
	require.False(t, schema.isCreateOnly("SnapshotKey"))
	require.False(t, schema.isCreateOnly("Users/*/Secret"))
}
