/*
Copyright 2026 The Radius Authors.

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

package terraform

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/radius-project/radius/pkg/components/secret/inmemory"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/credentials"
	"github.com/stretchr/testify/require"
)

func TestBackendUsesRegisteredUCPCredentials(t *testing.T) {
	for _, kind := range []string{
		credentials.AWSAccessKeyCredentialKind, credentials.AWSIRSACredentialKind,
		credentials.AzureServicePrincipalCredentialKind, credentials.AzureWorkloadIdentityCredentialKind,
	} {
		t.Run(kind, func(t *testing.T) {
			backend := &datamodel.TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2"}
			path := "/planes/aws/aws/providers/System.AWS/credentials/default"
			var registered any
			var rotated any
			var envKey, initialValue, rotatedValue string
			switch kind {
			case credentials.AWSAccessKeyCredentialKind:
				registered = backendTestAWSCredential(false)
				changed := backendTestAWSCredential(false)
				changed.AccessKeyCredential.AccessKeyID = "rotated-access"
				rotated = changed
				envKey, initialValue, rotatedValue = "AWS_ACCESS_KEY_ID", "registered-access", "rotated-access"
			case credentials.AWSIRSACredentialKind:
				registered = backendTestAWSCredential(true)
				changed := backendTestAWSCredential(true)
				initialValue = changed.IRSACredential.RoleARN
				changed.IRSACredential.RoleARN += "-rotated"
				rotated = changed
				envKey, rotatedValue = "AWS_ROLE_ARN", changed.IRSACredential.RoleARN
			default:
				backend = &datamodel.TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius"}
				path = "/planes/azure/azurecloud/providers/System.Azure/credentials/default"
				wi := kind == credentials.AzureWorkloadIdentityCredentialKind
				registered = backendTestAzureCredential(wi)
				changed := backendTestAzureCredential(wi)
				if wi {
					changed.WorkloadIdentity.ClientID = "rotated-client"
				} else {
					changed.ServicePrincipal.ClientID = "rotated-client"
				}
				rotated = changed
				envKey, initialValue, rotatedValue = "ARM_CLIENT_ID", "registered-client", "rotated-client"
			}
			requests := make(chan string, 10)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"properties":{"kind":%q,"storage":{"kind":"Internal","secretName":"backend-credentials"}}}`, kind)
			}))
			t.Cleanup(server.Close)
			conn, err := sdk.NewDirectConnection(server.URL)
			require.NoError(t, err)
			store := &inmemory.Client{}
			secretProvider := &secretprovider.SecretProvider{}
			secretProvider.SetClient(store)
			e := executor{ucpConn: conn, secretProvider: secretProvider}
			env := map[string]string{}
			for i, value := range []any{registered, rotated} {
				data, err := json.Marshal(value)
				require.NoError(t, err)
				require.NoError(t, store.Save(t.Context(), "backend-credentials", data))
				require.NoError(t, e.setBackendEnvironment(t.Context(), backend, env))
				require.Equal(t, path, <-requests)
				require.Equal(t, []string{initialValue, rotatedValue}[i], env[envKey])
			}
			for _, malformed := range []string{`{`, `{}`} {
				require.NoError(t, store.Save(t.Context(), "backend-credentials", []byte(malformed)))
				require.Error(t, e.setBackendEnvironment(t.Context(), backend, env))
				require.Equal(t, path, <-requests)
			}
			require.NoError(t, store.Delete(t.Context(), "backend-credentials"))
			require.Error(t, e.setBackendEnvironment(t.Context(), backend, env))
			require.Equal(t, path, <-requests)
		})
	}
}
