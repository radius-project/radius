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

package chimevoice

import (
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region   = "us-west-2"
	token    = "11111111-2222-3333-4444-555555555555"
	outsider = "outsider"

	// testVC and otherVC are IDs of Voice Connectors the fake has, missingVC of one it doesn't have, and
	// noTerminationVC of one without termination settings.
	testVC          = "abcdefghijklmnopqrstuv"
	otherVC         = "vutsrqponmlkjihgfedcba"
	missingVC       = missingPrefix + "abcdefghijklmno"
	noTerminationVC = noTerminationPrefix + "abcdefghi"
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// creds returns a template for the Voice Connector vc with the credentials, each a username and its password.
func creds(vc string, pairs ...string) map[string]any {
	list := []any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		list = append(list, map[string]any{usernameField: pairs[i], passwordField: pairs[i+1]})
	}
	return map[string]any{voiceConnectorIDProperty: vc, credentialsProperty: list}
}

// read returns the properties a read returns for the Voice Connector vc with the usernames.
func read(identifier string, vc string, usernames ...string) map[string]any {
	list := []any{}
	for _, name := range usernames {
		list = append(list, map[string]any{usernameField: name})
	}
	return map[string]any{"Id": identifier, voiceConnectorIDProperty: vc, credentialsProperty: list}
}

func Test_Suite_TerminationCredentials(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    credentialsType,
		Schema:      readFile(t, "aws-chime-voiceconnectorterminationcredentials.json"),
		Metadata:    readFile(t, "aws-chime-voiceconnectorterminationcredentials.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return newTracked(store)
		},
		IdentifierProperty: "Id",
		WriteOnly:          []string{"Credentials/*/Password"},
		Properties:         creds(testVC, "ana", "secret-ana", "bo", "secret-bo"),
		Changed:            creds(testVC, "bo", "secret-bo", "cy", "secret-cy"),
		ChangedWriteOnly:   creds(testVC, "ana", "secret-ana-2", "bo", "secret-bo"),
		CreateOnlyChange:   creds(otherVC, "ana", "secret-ana", "bo", "secret-bo"),
		SharedMember:       &nativetest.SharedMember{Overlapping: creds(testVC, "bo", "secret-other", "cy", "secret-cy"), Disjoint: creds(testVC, "cy", "secret-cy")},
		MultiCall:          creds(testVC, "bo", "secret-bo", "cy", "secret-cy", "di", "secret-di"),
		Calls:              2,
		RejectedCalls:      1,
		FailCall: nativetest.FailCall(2, func(h native.Handler) *nativetest.Failer {
			return &h.(*tracked).api.failer
		}),
		Invalid: []map[string]any{
			creds(testVC, "ana", "secret-ana", "ana", "secret-other"),
			creds(testVC, "ana", ""),
			creds(testVC),
			creds("abc|def", "ana", "secret-ana"),
			{"Id": testVC + "|x", voiceConnectorIDProperty: testVC, credentialsProperty: []any{map[string]any{usernameField: "ana"}}},
		},
		API: credentialsAPI,
		Outside: func(store *awsfake.CloudControl, properties map[string]any) func() bool {
			vc := properties[voiceConnectorIDProperty].(string)
			current := store.Resource(outsideType, vc)
			if current == nil {
				current = creds(vc)
			}
			current[credentialsProperty] = append(current[credentialsProperty].([]any), map[string]any{usernameField: outsider, passwordField: "secret-outsider"})
			store.Put(outsideType, vc, current)
			return func() bool {
				return slices.Contains(newFakeAPI(store).usernames(vc), outsider)
			}
		},
	})
}

func Test_ListAndProgress(t *testing.T) {
	h := newTracked(awsfake.NewCloudControl())
	require.Equal(t, credentialsType, h.TypeName())
	_, err := h.List(t.Context(), region)
	require.ErrorContains(t, err, "can't be listed")
	for _, op := range []native.Operation{native.OperationCreate, native.OperationUpdate, native.OperationDelete} {
		progress, err := h.Progress(t.Context(), region, testVC+"|x", op)
		require.NoError(t, err)
		require.True(t, progress.Done)
	}
}
